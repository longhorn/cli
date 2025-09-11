package replica

import (
	"encoding/json"
	"path/filepath"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"

	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Getter provide functions for the replica getter.
type Getter struct {
	GetterCmdOptions

	kubeClient *kubeclient.Clientset

	appName string // App name of the DaemonSet.
}

// GetterCmdOptions holds the options for the command.
type GetterCmdOptions struct {
	types.GlobalCmdOptions

	LonghornDataDirectory string
	VolumeName            string
	ReplicaName           string
}

// Init initializes the Getter.
func (remote *Getter) Init() error {
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	remote.appName = consts.AppNameReplicaGetter

	return nil
}

// Run creates the DaemonSet for the replica getter. It ensures that the
// init container and the output container completes before collecting the
// replica information and returning it as a YAML string.
func (remote *Getter) Run() (string, error) {
	nodeSelector, err := kubeutils.ParseNodeSelector(remote.NodeSelector)
	if err != nil {
		return "", errors.Wrapf(err, "failed to parse %q argument", consts.CmdOptNodeSelector)
	}
	newDaemonSet := remote.newDaemonSet(nodeSelector)

	daemonSet, err := commonkube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
	if err != nil {
		return "", err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameInit, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationMedium))
	if err != nil {
		return "", err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameOutput, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationShort))
	if err != nil {
		return "", err
	}

	podCollections, err := kubeutils.GetDaemonSetPodCollections(remote.kubeClient, daemonSet, consts.ContainerNameOutput, false, false, nil)
	if err != nil {
		return "", err
	}

	replicaCollections := types.ReplicaCollection{
		Replicas: make(map[string][]*types.ReplicaInfo),
	}
	for _, collection := range podCollections.Pods {
		var resultMap types.ReplicaCollection
		if err := json.Unmarshal([]byte(collection.Log), &resultMap); err != nil {
			return "", err
		}

		for replicaName, replicaInfo := range resultMap.Replicas {
			replicaCollections.Replicas[replicaName] = append(replicaCollections.Replicas[replicaName], replicaInfo...)
		}
	}

	yamlData, err := yaml.Marshal(replicaCollections)
	if err != nil {
		return "", err
	}

	return string(yamlData), nil
}

// Cleanup deletes the DaemonSet created for the replica getter.
func (remote *Getter) Cleanup() error {
	return commonkube.DeleteDaemonSet(remote.kubeClient, remote.Namespace, remote.appName)
}

// newDaemonSet prepares the DaemonSet for the replica getter.
func (remote *Getter) newDaemonSet(nodeSelector map[string]string) *appsv1.DaemonSet {
	outputFilePath := filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNameOutputJSON)
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.appName,
			Namespace: remote.Namespace,
			Labels: map[string]string{
				"app": remote.appName,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": remote.appName,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": remote.appName,
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{
							Name:    consts.ContainerNameInit,
							Image:   utils.BuildImageName(remote.Image, remote.ImageRegistry),
							Command: []string{consts.CmdLonghornctlLocal, consts.SubCmdGet, consts.SubCmdReplica},
							Env: []corev1.EnvVar{
								{
									Name: consts.EnvCurrentNodeID,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
								},
								{
									Name:  consts.EnvLogLevel,
									Value: remote.LogLevel,
								},
								{
									Name:  consts.EnvOutputFilePath,
									Value: outputFilePath,
								},
								{
									Name:  consts.EnvLonghornVolumeName,
									Value: remote.VolumeName,
								},
								{
									Name:  consts.EnvLonghornReplicaName,
									Value: remote.ReplicaName,
								},
								{
									Name:  consts.EnvLonghornDataDirectory,
									Value: remote.LonghornDataDirectory,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      consts.VolumeMountHostName,
									MountPath: consts.VolumeMountHostDirectory,
									ReadOnly:  true,
								},
								{
									Name:      consts.VolumeMountSharedName,
									MountPath: consts.VolumeMountSharedDirectory,
								},
							},
						},
						{
							Name:    consts.ContainerNameOutput,
							Image:   utils.BuildImageName(remote.Image, remote.ImageRegistry),
							Command: []string{"cat", outputFilePath},
							Env:     []corev1.EnvVar{},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      consts.VolumeMountSharedName,
									MountPath: consts.VolumeMountSharedDirectory,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  consts.ContainerNamePause,
							Image: utils.BuildImageName(consts.ImagePause, remote.ImageRegistry),
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: consts.VolumeMountHostName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/",
								},
							},
						},
						{
							Name: consts.VolumeMountSharedName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
					NodeSelector:     nodeSelector,
					ImagePullSecrets: kubeutils.GetImagePullSecrets(remote.ImagePullSecret),
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}
}

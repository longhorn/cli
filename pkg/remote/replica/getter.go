package replica

import (
	"encoding/json"
	"path/filepath"

	"gopkg.in/yaml.v3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	lhgokube "github.com/longhorn/go-common-libs/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Getter provide functions for the replica getter.
type Getter struct {
	GetterCmdOptions

	kubeClient *kubeclient.Clientset

	appName   string // App name of the DaemonSet.
	namespace string
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
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", remote.KubeConfigPath)
	if err != nil {
		return err
	}

	kubeClient, err := kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	remote.namespace = metav1.NamespaceDefault
	remote.appName = consts.AppNameReplicaGetter

	return nil
}

// Run creates the DaemonSet for the replica getter. It ensures that the
// init container and the output container completes before collecting the
// replica information and returning it as a YAML string.
func (remote *Getter) Run() (string, error) {
	newDaemonSet := remote.newDaemonSet()

	daemonSet, err := lhgokube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
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
	return lhgokube.DeleteDaemonSet(remote.kubeClient, remote.namespace, remote.appName)
}

// newDaemonSet prepares the DaemonSet for the replica getter.
func (remote *Getter) newDaemonSet() *appsv1.DaemonSet {
	outputFilePath := filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNameOutputJSON)
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.appName,
			Namespace: remote.namespace,
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
							Image:   remote.Image,
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
							Image:   remote.Image,
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
							Image: consts.ImagePause,
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
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}
}

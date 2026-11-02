package preflight

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/pkg/errors"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
	commonutils "github.com/longhorn/go-common-libs/utils"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Checker provide functions for the preflight check.
type Checker struct {
	CheckerCmdOptions

	kubeClient *kubeclient.Clientset

	appName          string // App name of the DaemonSet.
	createdDaemonSet *appsv1.DaemonSet
	createdRBAC      *v2RBACReceipt
}

// CheckerCmdOptions holds the options for the command.
type CheckerCmdOptions struct {
	types.GlobalCmdOptions

	DataEngineMode  string
	EnableSpdk      bool
	HugePageSize    int
	UserspaceDriver string
}

// Init initializes the Checker.
func (remote *Checker) Init() error {
	if err := remote.ValidateDataEngineMode(); err != nil {
		return err
	}
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}

	remote.kubeClient = kubeClient

	remote.appName = consts.AppNamePreflightChecker
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		// A V2 run never reuses or pre-cleans another run's checker or reports.
		remote.appName += "-" + strings.ToLower(rand.Text())
	}
	return nil
}

// Run creates the DaemonSet for the preflight check, and waits for it to complete.
func (remote *Checker) Run() (string, error) {
	if err := remote.ValidateDataEngineMode(); err != nil {
		return "", err
	}
	// Create RBAC to check:
	// - the node agent existence when the cluster is running on Container-Optimized OS (COS)
	// - replica count of the DNS deployment
	// - hugepages-2Mi capacity on nodes
	rbacRules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{"apps"},
			Resources: []string{"daemonsets", "deployments"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"nodes", "nodes/status"},
			Verbs:     []string{"get"},
		},
	}
	var err error
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		remote.createdRBAC, err = createV2RBAC(ctx, remote.kubeClient, remote.Namespace, remote.appName, rbacRules)
	} else {
		err = kubeutils.CreateRbac(remote.kubeClient, remote.Namespace, remote.appName, rbacRules)
	}
	if err != nil {
		return "", err
	}

	newDaemonSet, err := kubeutils.PrepareDaemonSet(remote.newDaemonSet(), remote.kubeClient, remote.NodeSelector, remote.ImagePullSecret, remote.Tolerations)
	if err != nil {
		return "", err
	}
	var daemonSet *appsv1.DaemonSet
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		daemonSet, err = remote.kubeClient.AppsV1().DaemonSets(remote.Namespace).Create(ctx, newDaemonSet, metav1.CreateOptions{})
	} else {
		daemonSet, err = commonkube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
	}
	if err != nil {
		return "", err
	}
	remote.createdDaemonSet = daemonSet.DeepCopy()
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		if err := validateV2CreatedDaemonSet(newDaemonSet, daemonSet, consts.SubCmdCheck); err != nil {
			return "", err
		}
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameInit, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationMedium))
	if err != nil {
		return "", err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameOutput, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationShort))
	if err != nil {
		return "", err
	}
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		return collectV2Reports(context.Background(), remote.kubeClient, daemonSet, readV2PodReport(remote.kubeClient))
	}

	podCollections, err := kubeutils.GetDaemonSetPodCollections(remote.kubeClient, daemonSet, consts.ContainerNameOutput, false, false, nil)
	if err != nil {
		return "", err
	}

	nodeCollections := map[string]*types.LogCollection{}
	for _, collection := range podCollections.Pods {
		var resultMap types.NodeCollection
		if err := json.Unmarshal([]byte(collection.Log), &resultMap); err != nil {
			return "", err
		}

		if reflect.DeepEqual(resultMap, types.NodeCollection{}) {
			continue
		}

		nodeCollections[collection.Node] = resultMap.Log
	}

	if reflect.DeepEqual(nodeCollections, map[string]types.LogCollection{}) {
		return "", nil
	}

	yamlData, err := yaml.Marshal(nodeCollections)
	if err != nil {
		return "", err
	}

	return string(yamlData), nil
}

// Cleanup deletes the DaemonSet created for the preflight check.
func (remote *Checker) Cleanup() error {
	var resultErr error
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if remote.createdDaemonSet != nil {
			uid := remote.createdDaemonSet.UID
			if uid == "" {
				return errors.New("refusing V2 checker cleanup without its created UID")
			}
			if err := remote.kubeClient.AppsV1().DaemonSets(remote.createdDaemonSet.Namespace).Delete(ctx, remote.createdDaemonSet.Name, metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}}); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}
		return cleanupV2RBAC(ctx, remote.kubeClient, remote.createdRBAC)
	}

	if err := commonkube.DeleteDaemonSet(remote.kubeClient, remote.Namespace, remote.appName); err != nil {
		resultErr = errors.Wrap(err, "failed to delete DaemonSet")
	}

	if err := kubeutils.DeleteRbac(remote.kubeClient, remote.Namespace, remote.appName); err != nil {
		if resultErr != nil {
			resultErr = errors.Wrap(resultErr, err.Error())
		} else {
			resultErr = errors.Wrap(err, "failed to delete RBAC")
		}
	}

	return resultErr
}

// newDaemonSet prepares a DaemonSet for the preflight check.
func (remote *Checker) newDaemonSet() *appsv1.DaemonSet {
	outputFilePath := filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNameOutputJSON)
	command := []string{consts.CmdLonghornctlLocal, consts.SubCmdCheck, consts.SubCmdPreflight}
	if remote.DataEngineMode == string(types.DataEngineModeV2Only) {
		// Older local images must reject the explicit flag, rather than
		// silently ignore an environment variable and report V1 checks.
		command = append(command, "--"+consts.CmdOptDataEngineMode, remote.DataEngineMode)
	}
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
					ServiceAccountName: remote.appName,
					HostPID:            true,
					InitContainers: []corev1.Container{
						{
							Name:    consts.ContainerNameInit,
							Image:   utils.BuildImageName(remote.Image, remote.ImageRegistry),
							Command: command,
							Env: []corev1.EnvVar{
								{
									Name:  consts.EnvLogLevel,
									Value: remote.LogLevel,
								},
								{
									Name:  consts.EnvOutputFilePath,
									Value: outputFilePath,
								},
								{
									Name:  consts.EnvDataEngineMode,
									Value: remote.DataEngineMode,
								},
								{
									Name:  consts.EnvPreflightCollectionOnly,
									Value: commonutils.ConvertTypeToString(remote.DataEngineMode == string(types.DataEngineModeV2Only)),
								},
								{
									Name:  consts.EnvEnableSpdk,
									Value: commonutils.ConvertTypeToString(remote.EnableSpdk),
								},
								{
									Name:  consts.EnvHugePageSize,
									Value: commonutils.ConvertTypeToString(remote.HugePageSize),
								},
								{
									Name:  consts.EnvUserspaceDriver,
									Value: remote.UserspaceDriver,
								},
								{
									Name: consts.EnvCurrentNodeID,
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "spec.nodeName",
										},
									},
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
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}
}

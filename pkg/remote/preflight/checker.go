package preflight

import (
	"encoding/json"
	"path/filepath"

	"github.com/pkg/errors"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
	commonutils "github.com/longhorn/go-common-libs/utils"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"

	"reflect"
)

// Checker provide functions for the preflight check.
type Checker struct {
	CheckerCmdOptions

	kubeClient *kubeclient.Clientset

	namespace string
	appName   string // App name of the DaemonSet.
}

// CheckerCmdOptions holds the options for the command.
type CheckerCmdOptions struct {
	types.GlobalCmdOptions

	EnableSpdk      bool
	HugePageSize    int
	UserspaceDriver string
}

// Init initializes the Checker.
func (remote *Checker) Init() error {
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}

	remote.kubeClient = kubeClient

	remote.namespace = metav1.NamespaceDefault
	remote.appName = consts.AppNamePreflightChecker
	return nil
}

// Run creates the DaemonSet for the preflight check, and waits for it to complete.
func (remote *Checker) Run() (string, error) {
	err := remote.createRbacForNodeAgent()
	if err != nil {
		return "", err
	}

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

// createRbacForNodeAgent creates the RBAC for checking if node agent exists when the cluster is running on Container-Optimized OS (COS).
// It creates a new ServiceAccount, ClusterRole, and ClusterRoleBinding to provide permission to get the node agent DaemonSet.
func (remote *Checker) createRbacForNodeAgent() error {
	// Create the RBAC for checking if node agent exists when the cluster is running on Container-Optimized OS.
	newServiceAccount := remote.newServiceAccount()
	_, err := commonkube.CreateServiceAccount(remote.kubeClient, newServiceAccount)
	if err != nil {
		return err
	}

	newClusterRole := remote.newClusterRole()
	_, err = commonkube.CreateClusterRole(remote.kubeClient, newClusterRole)
	if err != nil {
		return err
	}

	newClusterRoleBinding := remote.newClusterRoleBinding()
	_, err = commonkube.CreateClusterRoleBinding(remote.kubeClient, newClusterRoleBinding)
	if err != nil {
		return err
	}

	return nil
}

// Cleanup deletes the DaemonSet created for the preflight check.
func (remote *Checker) Cleanup() error {
	if err := commonkube.DeleteDaemonSet(remote.kubeClient, remote.namespace, remote.appName); err != nil {
		return err
	}

	if err := commonkube.DeleteClusterRoleBinding(remote.kubeClient, remote.appName); err != nil {
		return err
	}

	if err := commonkube.DeleteClusterRole(remote.kubeClient, remote.appName); err != nil {
		return err
	}

	return commonkube.DeleteServiceAccount(remote.kubeClient, remote.namespace, remote.appName)
}

func (remote *Checker) newClusterRole() *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: remote.appName,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"apps"},
				Resources: []string{"daemonsets", "deployments"},
				Verbs:     []string{"get", "list"},
			},
		},
	}
}

func (remote *Checker) newClusterRoleBinding() *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: remote.appName,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     remote.appName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      remote.appName,
				Namespace: metav1.NamespaceDefault,
			},
		},
	}
}

func (remote *Checker) newServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.appName,
			Namespace: metav1.NamespaceDefault,
		},
	}
}

// NewDaemonSet prepares a DaemonSet for the preflight check.
func (remote *Checker) newDaemonSet(nodeSelector map[string]string) *appsv1.DaemonSet {
	outputFilePath := filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNameOutputJSON)
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.appName,
			Namespace: metav1.NamespaceDefault,
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
							Image:   remote.Image,
							Command: []string{consts.CmdLonghornctlLocal, consts.SubCmdCheck, consts.SubCmdPreflight},
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
					NodeSelector: nodeSelector,
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}
}

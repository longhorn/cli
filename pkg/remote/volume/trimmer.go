package volume

import (
	"github.com/pkg/errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Trimmer provide functions for the volume trimmer.
type Trimmer struct {
	TrimmerCmdOptions

	kubeClient *kubeclient.Clientset

	appName string // App name of the DaemonSet.
}

// TrimmerCmdOptions holds the options for the command.
type TrimmerCmdOptions struct {
	types.GlobalCmdOptions

	CurrentNodeID     string
	LonghornNamespace string
	VolumeName        string
}

// Validate validates the command options.
func (remote *Trimmer) Validate() error {
	if remote.LonghornNamespace == "" {
		return errors.New("Longhorn namespace  (--namespace) is required")
	}

	if remote.VolumeName == "" {
		return errors.New("Longhorn volume name (--name) is required")
	}

	return nil
}

// Init initializes the Trimmer.
func (remote *Trimmer) Init() error {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", remote.KubeConfigPath)
	if err != nil {
		return err
	}

	kubeClient, err := kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	remote.appName = consts.AppNameVolumeTrimmer
	return nil
}

// Run creates the DaemonSet for the volume trimmer, and waits for it to complete.
func (remote *Trimmer) Run() error {
	nodeSelector, err := kubeutils.ParseNodeSelector(remote.NodeSelector)
	if err != nil {
		return errors.Wrapf(err, "failed to parse %q argument", consts.CmdOptNodeSelector)
	}
	newDaemonSet := remote.newDaemonSet(nodeSelector)
	daemonSet, err := commonkube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
	if err != nil {
		return err
	}

	return kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameInit, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationMedium))
}

// Cleanup deletes the DaemonSet created for the volume trimmer.
func (remote *Trimmer) Cleanup() error {
	return commonkube.DeleteDaemonSet(remote.kubeClient, remote.LonghornNamespace, remote.appName)
}

// NewDaemonSet prepares the DaemonSet for the volume trimmer.
func (remote *Trimmer) newDaemonSet(nodeSelector map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.appName,
			Namespace: remote.LonghornNamespace,
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
					ServiceAccountName: consts.LonghornServiceAccountName,
					InitContainers: []corev1.Container{
						{
							Name:    consts.ContainerNameInit,
							Image:   remote.Image,
							Command: []string{consts.CmdLonghornctlLocal, consts.SubCmdTrim, consts.SubCmdVolume},
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
									Name:  consts.EnvLonghornVolumeName,
									Value: remote.VolumeName,
								},
								{
									Name:  consts.EnvLonghornNamespace,
									Value: remote.LonghornNamespace,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      consts.VolumeMountHostName,
									MountPath: consts.VolumeMountHostDirectory,
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

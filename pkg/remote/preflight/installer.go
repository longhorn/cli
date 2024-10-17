package preflight

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
	commonutils "github.com/longhorn/go-common-libs/utils"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Installer provide functions for the preflight install.
type Installer struct {
	InstallerCmdOptions

	kubeClient *kubeclient.Clientset

	appName string // App name of the DaemonSet.
}

// InstallerCmdOptions holds the options for the command.
type InstallerCmdOptions struct {
	types.GlobalCmdOptions

	OperatingSystem string

	UpdatePackages bool
	EnableSpdk     bool
	SpdkOptions    string
	HugePageSize   int
	AllowPci       string
	DriverOverride string
}

// Init initializes the Installer.
func (remote *Installer) Init() error {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", remote.KubeConfigPath)
	if err != nil {
		return err
	}

	kubeClient, err := kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	operatingSystem := consts.OperatingSystem(remote.OperatingSystem)
	switch operatingSystem {
	case consts.OperatingSystemContainerOptimizedOS:
		remote.appName = consts.AppNamePreflightContainerOptimizedOS
	default:
		remote.appName = consts.AppNamePreflightInstaller
	}

	return nil
}

// Run creates the DaemonSet for the preflight install.
// It checks if the operating system is specified, and installs the dependencies accordingly.
// If the operating system is not specified, it installs the dependencies with package manager.
func (remote *Installer) Run() error {
	operatingSystem := consts.OperatingSystem(remote.OperatingSystem)
	switch operatingSystem {
	case consts.OperatingSystemContainerOptimizedOS:
		logrus.Infof("Installing dependencies on Container Optimized OS (%v)", operatingSystem)

		if err := remote.InstallByContainerOptimizedOS(); err != nil {
			return errors.Wrapf(err, "failed to install dependencies on Container Optimized OS (%v)", operatingSystem)
		}

		logrus.Infof("Installed dependencies on Container Optimized OS (%v)", operatingSystem)

	default:
		logrus.Info("Installing dependencies with package manager")

		if err := remote.InstallByPackageManager(); err != nil {
			return errors.Wrapf(err, "failed to install dependencies with package manager")
		}

		logrus.Info("Installed dependencies with package manager")
	}

	return nil
}

// Cleanup deletes the DaemonSet created for the preflight install when it's installed with package manager.
func (remote *Installer) Cleanup() error {
	return commonkube.DeleteDaemonSet(remote.kubeClient, metav1.NamespaceDefault, remote.appName)
}

// InstallByContainerOptimizedOS installs the dependencies on Container Optimized OS.
// It creates a ConfigMap and a DaemonSet. Then it waits for the DaemonSet to be ready.
func (remote *Installer) InstallByContainerOptimizedOS() error {
	newConfigMap := remote.newConfigMapForContainerOptimizedOS()
	_, err := commonkube.CreateConfigMap(remote.kubeClient, newConfigMap)
	if err != nil {
		return err
	}

	newDaemonSet := remote.newDaemonSetForContainerOptimizedOS()
	daemonSet, err := commonkube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
	if err != nil {
		return err
	}

	return kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerName, kubeutils.WaitForDaemonSetContainersReady, ptr.To(consts.ContainerConditionMaxTolerationShort))
}

// InstallByPackageManager installs the dependencies with package manager.
// It creates a DaemonSet. Then it waits for the DaemonSet to complete.
func (remote *Installer) InstallByPackageManager() error {
	newDaemonSet := remote.NewDaemonSetForPackageManager()
	daemonSet, err := commonkube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
	if err != nil {
		return err
	}

	return kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameInit, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationLong))
}

// newConfigMapForContainerOptimizedOS prepares a ConfigMap for installing the dependencies on Container Optimized OS.
func (remote *Installer) newConfigMapForContainerOptimizedOS() *corev1.ConfigMap {
	entrypointScript := `#!/bin/bash

set -euo pipefail

# Define default directories
HOST_MOUNT_DIR="${HOST_MOUNT_DIR:-/host}"
KUBERNETES_ROOTFS="${KUBERNETES_ROOTFS:-/home/kubernetes/containerized_mounter/rootfs}"
KUBERNETES_MOUNT_DIR="${HOST_MOUNT_DIR}${KUBERNETES_ROOTFS}"
LONGHORN_DATA_PATHS="${LONGHORN_DATA_PATHS:-/var/lib/longhorn}"
IFS=',' read -ra LONGHORN_DATA_DIRS <<< "$LONGHORN_DATA_PATHS"  # Split comma-separated dirs

# Function to check the operating system is Container Optimized OS (cos)
function check_operating_system() {
	os=$(cat ${HOST_MOUNT_DIR}/etc/os-release | grep '^ID=' | cut -d= -f2)
	if [ "$os" != "cos" ]; then
		echo "ERROR: Operating system ($os) is not Container Optimized OS (cos)"
		exit 1
	fi
}

# Function to mount the Longhorn data directory on the host
mount_longhorn_data_dir_on_host() {
  local _longhorn_data_dir="$1"

  if is_mounted_on_host "${_longhorn_data_dir}"; then
    echo "Longhorn data directory ${_longhorn_data_dir} is already mounted"
  else
    echo "Mounting Longhorn data directory ${_longhorn_data_dir} on the host"

    chroot "${HOST_MOUNT_DIR}" mkdir -p "${_longhorn_data_dir}"
    chroot "${KUBERNETES_MOUNT_DIR}" mkdir -p "${_longhorn_data_dir}"

    nsenter --mount="${HOST_MOUNT_DIR}/proc/1/ns/mnt" mount --rbind "${_longhorn_data_dir}" "${KUBERNETES_ROOTFS}${_longhorn_data_dir}"
    nsenter --mount="${HOST_MOUNT_DIR}/proc/1/ns/mnt" mount --make-shared "${KUBERNETES_ROOTFS}${_longhorn_data_dir}"
    nsenter --mount="${HOST_MOUNT_DIR}/proc/1/ns/mnt" mount -o remount,exec "${_longhorn_data_dir}"
  fi
}

# Function to check if a directory is already mounted
is_mounted_on_host() {
  nsenter --mount="${HOST_MOUNT_DIR}/proc/1/ns/mnt" findmnt --noheadings --output TARGET "$1"
}

# Function to check if a kernel module is loaded
is_module_loaded_on_host() {
  local _module="$1"
  nsenter --mount="${HOST_MOUNT_DIR}/proc/1/ns/mnt" lsmod | grep -q "${_module}"
}

# Function to load the iscsi_tcp kernel module on the host
load_iscsi_tcp_module_on_host() {
  if is_module_loaded_on_host "iscsi_tcp"; then
    echo "iscsi_tcp kernel module is already loaded"
  else
    echo "Loading iscsi_tcp kernel module"
    nsenter --mount="${HOST_MOUNT_DIR}/proc/1/ns/mnt" modprobe iscsi_tcp
  fi
}

# Function to install and start open-iscsi
install_and_start_iscsid() {
  echo "Installing and starting open-iscsi"
  zypper install -y open-iscsi
  /sbin/iscsid
}

# Validate the operating system
check_operating_system

# Mount the Longhorn data directories
for LONGHORN_DATA_DIR in "${LONGHORN_DATA_DIRS[@]}"; do
  mount_longhorn_data_dir_on_host "${LONGHORN_DATA_DIR}"
done

install_and_start_iscsid
load_iscsi_tcp_module_on_host

echo "Complete!"
echo "Keep the container running for iSCSI daemon"
sleep infinity
`

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      remote.appName,
			Namespace: metav1.NamespaceDefault,
			Labels: map[string]string{
				"app": remote.appName,
			},
		},
		Data: map[string]string{
			"entrypoint.sh": entrypointScript,
		},
	}
}

// newDaemonSetForContainerOptimizedOS prepares a DaemonSet for installing the dependencies on Container Optimized OS.
func (remote *Installer) newDaemonSetForContainerOptimizedOS() *appsv1.DaemonSet {
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
					HostNetwork: true,
					Containers: []corev1.Container{
						{
							Name:    consts.ContainerName,
							Image:   consts.ImageBciBase,
							Command: []string{"/scripts/entrypoint.sh"},
							Env: []corev1.EnvVar{
								{
									Name:  "HOST_MOUNT_DIR",
									Value: consts.VolumeMountHostDirectory,
								},
								{
									Name:  "KUBERNETES_ROOTFS",
									Value: "/home/kubernetes/containerized_mounter/rootfs",
								},
								{
									Name:  "LONGHORN_DATA_PATHS",
									Value: "/var/lib/longhorn",
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_MODULE",
									},
								},
								Privileged: ptr.To(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      consts.VolumeMountHostName,
									MountPath: consts.VolumeMountHostDirectory,
								},
								{
									Name:      consts.VolumeMountEntrypointName,
									MountPath: consts.VolumeMountEntrypointDirectory,
								},
							},

							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"/bin/bash",
											"-c",
											"nsenter --mount=${HOST_MOUNT_DIR}/proc/1/ns/mnt pgrep -x iscsid && nsenter --mount=${HOST_MOUNT_DIR}/proc/1/ns/mnt lsmod | grep -q iscsi_tcp",
										},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"/bin/bash",
											"-c",
											"nsenter --mount=${HOST_MOUNT_DIR}/proc/1/ns/mnt pgrep -x iscsid && nsenter --mount=${HOST_MOUNT_DIR}/proc/1/ns/mnt lsmod | grep -q iscsi_tcp",
										},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
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
							Name: consts.VolumeMountEntrypointName,
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: remote.appName,
									},
									DefaultMode: ptr.To[int32](0744),
								},
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

// NewDaemonSetForPackageManager prepares a DaemonSet for installing dependencies with the package manager.
func (remote *Installer) NewDaemonSetForPackageManager() *appsv1.DaemonSet {
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
					// Required for running systemd tasks.
					HostNetwork: true,
					HostPID:     true,

					InitContainers: []corev1.Container{
						{
							Name:    consts.ContainerNameInit,
							Image:   remote.Image,
							Command: []string{consts.CmdLonghornctlLocal, consts.SubCmdInstall, consts.SubCmdPreflight},
							Env: []corev1.EnvVar{
								{
									Name:  consts.EnvLogLevel,
									Value: remote.LogLevel,
								},
								{
									Name:  consts.EnvUpdatePackageList,
									Value: commonutils.ConvertTypeToString(remote.UpdatePackages),
								},
								{
									Name:  consts.EnvEnableSpdk,
									Value: commonutils.ConvertTypeToString(remote.EnableSpdk),
								},
								{
									Name:  consts.EnvSpdkOptions,
									Value: remote.SpdkOptions,
								},
								{
									Name:  consts.EnvHugePageSize,
									Value: commonutils.ConvertTypeToString(remote.HugePageSize),
								},
								{
									Name:  consts.EnvPciAllowed,
									Value: remote.AllowPci,
								},
								{
									Name:  consts.EnvDriverOverride,
									Value: remote.DriverOverride,
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
				},
			},
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.RollingUpdateDaemonSetStrategyType,
			},
		},
	}
}

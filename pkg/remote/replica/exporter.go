package replica

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"

	lhgokube "github.com/longhorn/go-common-libs/kubernetes"
	lhgolonghorn "github.com/longhorn/go-common-libs/longhorn"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

// Exporter provide functions for the replica exporter.
type Exporter struct {
	ExporterCmdOptions

	kubeClient *kubeclient.Clientset

	appName   string // App name of the DaemonSet.
	namespace string

	volumeName string
}

// ExporterCmdOptions holds the options for the command.
type ExporterCmdOptions struct {
	types.GlobalCmdOptions

	EngineImage           string
	ReplicaName           string
	LonghornDataDirectory string
	HostTargetDirectory   string
}

// Validate validates the command options.
func (remote *Exporter) Validate() error {
	if remote.ReplicaName == "" {
		return errors.New("Replica name (--name) is required")
	}

	if remote.EngineImage == "" {
		return errors.New("Engine image (--engine-image) is required")
	}

	if remote.HostTargetDirectory == "" {
		return errors.New("Host target directory (--target-dir) is required")
	}

	return nil
}

// Init initializes the Exporter.
func (remote *Exporter) Init() error {
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
	remote.appName = consts.AppNameReplicaExporter

	// Not required for cleanup
	if remote.ReplicaName != "" {
		remote.volumeName, err = lhgolonghorn.GetVolumeNameFromReplicaDataDirectoryName(remote.ReplicaName)
		if err != nil {
			return err
		}
	}

	return nil
}

// Run creates the ConfigMap and DaemonSet for the replica exporter.
// It ensures the init container completes and the engine container is ready
// before collecting volume information and returning it as a YAML string.
func (remote *Exporter) Run() (string, error) {
	newConfigMap := remote.newConfigMapForSimpleLonghorn()
	_, err := lhgokube.CreateConfigMap(remote.kubeClient, newConfigMap)
	if err != nil {
		return "", err
	}

	newDaemonSet := remote.newDaemonSet()
	daemonSet, err := lhgokube.CreateDaemonSet(remote.kubeClient, newDaemonSet)
	if err != nil {
		return "", err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameInit, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationMedium))
	if err != nil {
		return "", err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.kubeClient, daemonSet, consts.ContainerNameEngine, kubeutils.WaitForDaemonSetContainersReady, ptr.To(consts.ContainerConditionMaxTolerationMedium))
	if err != nil {
		return "", err
	}

	podCollections, err := kubeutils.GetDaemonSetPodCollections(remote.kubeClient, daemonSet, consts.ContainerNameEngine, false, false, ptr.To(int64(1)))
	if err != nil {
		return "", err
	}

	replicaExportedDirectory := filepath.Join(remote.HostTargetDirectory, remote.volumeName)

	volumeInfo := &types.VolumeInfo{
		Replicas: []*types.ReplicaInfo{},
	}
	for podName, collection := range podCollections.Pods {
		logrus.Tracef("Collecting log from %s/%s", daemonSet.Namespace, podName)

		if strings.HasPrefix(collection.Log, consts.LogPrefixPause) {
			continue
		}

		replicaInfo := &types.ReplicaInfo{
			Node:              collection.Node,
			ExportedDirectory: replicaExportedDirectory,
		}

		if strings.HasPrefix(collection.Log, consts.LogPrefixError) {
			replicaInfo.Error = strings.ReplaceAll(strings.TrimPrefix(collection.Log, consts.LogPrefixError), "\n", "")
			replicaInfo.ExportedDirectory = ""
		}

		volumeInfo.Replicas = append(volumeInfo.Replicas, replicaInfo)
	}

	volumeCollections := types.VolumeCollection{
		Volumes: make(map[string][]*types.VolumeInfo),
	}
	volumeCollections.Volumes[remote.volumeName] = append(volumeCollections.Volumes[remote.volumeName], volumeInfo)

	yamlData, err := yaml.Marshal(volumeCollections)
	if err != nil {
		return "", err
	}

	return string(yamlData), nil
}

// Cleanup deletes the ConfigMap and DaemonSet created for the replica exporter.
func (remote *Exporter) Cleanup() error {
	err := lhgokube.DeleteConfigMap(remote.kubeClient, remote.namespace, remote.appName)
	if err != nil {
		return err
	}

	return lhgokube.DeleteDaemonSet(remote.kubeClient, remote.namespace, remote.appName)
}

// newConfigMapForSimpleLonghorn prepares a ConfigMap with entrypoint script for the replica exporter.
func (remote *Exporter) newConfigMapForSimpleLonghorn() *corev1.ConfigMap {
	entrypointScript := `#!/bin/bash
set -euo pipefail

EXPORTED_DIR="/host-exporter"
HOST_DIR="/host"
DEV_DIR="${HOST_DIR}/dev/longhorn"
REPLICA_JSON_FILE="/shared/output.json"
PAUSED=false

# Function to pause the script.
function pause() {
	PAUSED=true
	sleep infinity
}

# Function to check if dependencies are installed.
function pause_no_dependencies() {
    if ! command -v jq &>/dev/null; then
        echo "jq is not installed"
        exit 1
    fi
}

# Function to mount a volume to /${EXPORTED_DIR}/${VOLUME_NAME}/.
function mount_volume() {
	mkdir -p ${EXPORTED_DIR}/${VOLUME_NAME}/

	while true;
	do
		[[ -b ${DEV_DIR}/${VOLUME_NAME} ]] && break

		echo "Waiting for ${DEV_DIR}/${VOLUME_NAME} to be created..."
		sleep 1
	done

	echo "Mounting ${DEV_DIR}/${VOLUME_NAME} to ${EXPORTED_DIR}/${VOLUME_NAME}/"

	mount -o ro ${DEV_DIR}/${VOLUME_NAME} ${EXPORTED_DIR}/${VOLUME_NAME}/
}

PRESTOP_SCRIPT_FILE="/shared/pre-stop.sh"
touch ${PRESTOP_SCRIPT_FILE}
chmod +x ${PRESTOP_SCRIPT_FILE}

# Function to create a pre-stop script.
function create_prestop_script() {
	cat > "${PRESTOP_SCRIPT_FILE}" <<-EOF
	#!/bin/bash

	EXPORTED_DIR="${EXPORTED_DIR}"

	VOLUME_NAME="${VOLUME_NAME}"

	echo "Unmounting \${EXPORTED_DIR}/\${VOLUME_NAME}/"
	if ! umount "\${EXPORTED_DIR}/\${VOLUME_NAME}/"; then
		echo "Failed to unmount \${EXPORTED_DIR}/\${VOLUME_NAME}/"
		exit 1
	fi

	echo "Removing \${EXPORTED_DIR}/\${VOLUME_NAME}/"
	rm -rf "\${EXPORTED_DIR}/\${VOLUME_NAME}/"

	echo "Removing \${DEV_DIR}/\${VOLUME_NAME}"
	rm -f "\${DEV_DIR}/\${VOLUME_NAME}"
	EOF
}

function pause_no_replica() {
	# Check if there is a matched replica. If not, then pause the exporter.
	if ! jq -e '.replicas | length != 0' "${REPLICA_JSON_FILE}" > /dev/null 2>&1; then
	  echo "PAUSE: No matched replica found on this node"
	  pause
	fi
}

function pause_failed_replica() {
	# Extract the value of the "error" field from the JSON, and check if it is not empty
	error=$(jq -r --arg replica_name "${REPLICA_NAME}" '.replicas[$replica_name][0].error' "${REPLICA_JSON_FILE}")
	if [ -n "${error}" ] && [ "${error}" != "null" ]; then
		echo "PAUSE: Found error of replica ${REPLICA_NAME}: ${error}"
		PAUSED=true
		sleep infinity
	fi
}

function get_replica_name() {
	local _replica_name=${REPLICA_NAME}

	if [ -z "${_replica_name}" ]; then
		_replica_name=$(jq --arg volume_name "${VOLUME_NAME}" '.replicas | to_entries | .[] | select(.value[] | .volumeName == $volume_name and (.error // "") == "" and .metadata.Size > 0) | .key' ${REPLICA_JSON_FILE} | head -n 1)
		_replica_name="${_replica_name//\"/}"  # remove double quotes
	fi

	REPLICA_NAME="${_replica_name}"
}

function get_volume_name() {
	local _volume_name=${VOLUME_NAME}

	# If volume name is not provided, then use the volume name of the replica from the output.json file.
	if [ -z "${_volume_name}" ]; then
	  _volume_name=$(jq --arg replica_name "${REPLICA_NAME}" '.replicas[$replica_name][0].volumeName' "${REPLICA_JSON_FILE}")
	  _volume_name="${_volume_name//\"/}"  # remove double quotes
	fi

	VOLUME_NAME="${_volume_name}"
}

function get_volume_size() {
	local _volume_size=$(jq --arg replica_name "${REPLICA_NAME}" '.replicas[$replica_name][0].metadata.Size' "${REPLICA_JSON_FILE}")
	echo "${_volume_size}"
}

function is_volume_in_use() {
	local _is_in_use=$(jq --arg replica_name "${REPLICA_NAME}" '.replicas[$replica_name][0].isInUse' "${REPLICA_JSON_FILE}")
	echo "${_is_in_use}"
}

function prepare_mount() {
	mount --rbind "${HOST_DIR}/dev" /dev

	if [[ -b ${DEV_DIR}/${VOLUME_NAME} ]]; then
	  echo "Cleaning up ${DEV_DIR}/${VOLUME_NAME}"
	  rm "${DEV_DIR}/${VOLUME_NAME}"
	fi
}

pause_no_dependencies
pause_no_replica

get_replica_name
if [ -z "${REPLICA_NAME}" ]; then
  echo "Failed to get replica name"
  pause
fi
echo "Replica name: ${REPLICA_NAME}"

pause_failed_replica

get_volume_name
if [ -z "$VOLUME_NAME" ]; then
  echo "Failed to get volume name"
  pause
fi
echo "Volume name: ${VOLUME_NAME}"

# VOLUME_SIZE=$(jq --arg replica_name "${REPLICA_NAME}" '.replicas[$replica_name][0].metadata.Size' "${REPLICA_JSON_FILE}")
VOLUME_SIZE=$(get_volume_size)
if [ -z "${VOLUME_SIZE}" ]; then
  echo "ERROR: failed to get volume size"
  pause
fi
echo "Volume size: ${VOLUME_SIZE}"

# IS_IN_USE=$(jq --arg replica_name "${REPLICA_NAME}" '.replicas[$replica_name][0].isInUse' "${REPLICA_JSON_FILE}")
IS_IN_USE=$(is_volume_in_use)
if [ -z "${IS_IN_USE}" ]; then
  echo "Failed to get isInUse"
  exit 1
elif [ "${IS_IN_USE}" == "true" ]; then
  echo "ERROR: replica ${REPLICA_NAME} is in use"
  pause
fi

echo "Preparing mount"
prepare_mount

chmod +x /usr/local/bin/longhorn-instance-manager

echo "Launching simple-longhorn for volume ${VOLUME_NAME} in the background"
launch-simple-longhorn ${VOLUME_NAME} ${VOLUME_SIZE} &

echo "Creating ${PRESTOP_SCRIPT_FILE}"
create_prestop_script

echo "Mounting /dev/longhorn/${VOLUME_NAME}"
mount_volume

echo "Complete!"
echo "Keep the container running to export replica"
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

// newDaemonSet prepares the DaemonSet for the replica exporter.
func (remote *Exporter) newDaemonSet() *appsv1.DaemonSet {
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
								},
								{
									Name:      consts.VolumeMountSharedName,
									MountPath: consts.VolumeMountSharedDirectory,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:    consts.ContainerNameEngine,
							Image:   remote.EngineImage,
							Command: []string{"/scripts/entrypoint.sh"},
							Env: []corev1.EnvVar{
								{
									Name:  consts.EnvLonghornReplicaName,
									Value: remote.ReplicaName,
								},
								{
									Name:  consts.EnvLonghornVolumeName,
									Value: remote.volumeName,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      consts.VolumeMountEntrypointName,
									MountPath: consts.VolumeMountEntrypointDirectory,
								},
								{
									Name:      consts.VolumeMountHostName,
									MountPath: consts.VolumeMountHostDirectory,
								},
								{
									Name:      consts.VolumeMountSharedName,
									MountPath: consts.VolumeMountSharedDirectory,
								},
								{
									Name:             consts.VolumeMountHostExporterName,
									MountPath:        consts.VolumeMountHostExporterDirectory,
									MountPropagation: ptr.To(corev1.MountPropagationBidirectional),
								},
								{
									Name:      consts.VolumeMountVolumeName,
									MountPath: consts.VolumeMountVolumeDirectory,
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: ptr.To(true),
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"/bin/bash", "-c",
											"[[ -d /host-exporter/${VOLUME_NAME}/lost+found ]] || ${PAUSE}",
										},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Lifecycle: &corev1.Lifecycle{
								PreStop: &corev1.LifecycleHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"/bin/bash", "-c",
											filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNamePreStopScript),
										},
									},
								},
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
						{
							Name: consts.VolumeMountSharedName,
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: consts.VolumeMountHostExporterName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: remote.HostTargetDirectory,
								},
							},
						},
						{
							Name: consts.VolumeMountVolumeName,
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: filepath.Join(remote.LonghornDataDirectory, "replicas", remote.ReplicaName),
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

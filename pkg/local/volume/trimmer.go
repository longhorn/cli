package volume

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericiooptions"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/kubectl/pkg/cmd/exec"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclient "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	lhmgrtypes "github.com/longhorn/longhorn-manager/types"
	lhmgrutils "github.com/longhorn/longhorn-manager/util"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"

	remote "github.com/longhorn/cli/pkg/remote/volume"
)

// Trimmer provide functions for the volume trimmer.
type Trimmer struct {
	remote.TrimmerCmdOptions

	logger *logrus.Entry

	config         *rest.Config
	kubeClient     *kubeclient.Clientset
	longhornClient *lhclient.Clientset
	executor       *commonns.Executor
}

// Validate validates the command options.
func (local *Trimmer) Validate() error {
	if local.VolumeName == "" {
		return errors.New("Longhorn volume name (--name) is required")
	}

	return nil
}

// Init initializes the Trimmer.
func (local *Trimmer) Init() error {
	namespaces := []commontypes.Namespace{
		commontypes.NamespaceMnt,
		commontypes.NamespaceNet,
	}

	executor, err := commonns.NewNamespaceExecutor(commontypes.ProcessSelf, commontypes.HostProcDirectory, namespaces)
	if err != nil {
		return err
	}
	local.executor = executor

	local.config, err = commonkube.GetInClusterConfig()
	if err != nil {
		return errors.Wrap(err, "failed to get client config")
	}

	local.kubeClient, err = kubeclient.NewForConfig(local.config)
	if err != nil {
		return errors.Wrap(err, "failed to get Kubernetes clientset")
	}

	local.longhornClient, err = lhclient.NewForConfig(local.config)
	if err != nil {
		return errors.Wrap(err, "failed to get Longhorn clientset")
	}

	local.logger = logrus.WithFields(logrus.Fields{
		"volume": local.VolumeName,
		"node":   local.CurrentNodeID,
	})

	return nil
}

// Run trims the volume based on the volume's access mode.
func (local *Trimmer) Run() error {
	volume, err := local.longhornClient.LonghornV1beta2().Volumes(local.Namespace).Get(context.TODO(), local.VolumeName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	accessMode := longhorn.AccessMode(volume.Spec.AccessMode)
	local.logger = local.logger.WithField("access-mode", accessMode)

	switch accessMode {
	case longhorn.AccessModeReadWriteOnce:
		return local.trimReadWriteOnce(volume)
	case longhorn.AccessModeReadWriteMany:
		return local.trimReadWriteMany(volume)
	default:
		return errors.Errorf("Unrecognized access mode: %s", accessMode)
	}
}

// trimReadWriteOnce trims the volume with access mode ReadWriteOnce (RWO).
func (local *Trimmer) trimReadWriteOnce(volume *longhorn.Volume) error {
	local.logger.Info("Trimming volume with access mode ReadWriteOnce (RWO)")

	if volume.Status.CurrentNodeID != local.CurrentNodeID {
		local.logger.Debug("Trimmer aborting because volume is not on current node")
		return nil
	}

	return lhmgrutils.TrimFilesystem(volume.Name, volume.Spec.Encrypted)
}

// trimReadWriteMany trims the volume with access mode ReadWriteMany (RWX).
func (local *Trimmer) trimReadWriteMany(volume *longhorn.Volume) error {
	local.logger.Info("Trimming volume with access mode ReadWriteMany (RWX)")

	if volume.Status.CurrentNodeID != local.CurrentNodeID {
		logrus.Info("Trimmer aborting because volume is not on current node")
		return nil
	}

	shareManager, err := local.longhornClient.LonghornV1beta2().ShareManagers(local.Namespace).Get(context.TODO(), volume.Name, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get share manager for volume %v", volume.Name)
	}

	if shareManager.Status.State != longhorn.ShareManagerStateRunning {
		return errors.Errorf("share manager %v is not running", shareManager.Name)
	}

	shareManagerPodName := lhmgrtypes.GetShareManagerPodNameFromShareManagerName(shareManager.Name)
	shareManagerPod, err := local.kubeClient.CoreV1().Pods(local.Namespace).Get(context.TODO(), shareManagerPodName, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get share manager pod for trimming volume %v in namespace", volume.Name)

	}

	execOptions := &exec.ExecOptions{
		Config:    local.config,
		PodClient: local.kubeClient.CoreV1(),
		StreamOptions: exec.StreamOptions{
			IOStreams: genericiooptions.IOStreams{
				Out:    local.logger.Writer(),
				ErrOut: local.logger.Writer(),
			},

			Namespace:     shareManagerPod.Namespace,
			PodName:       shareManagerPod.Name,
			ContainerName: shareManagerPod.Spec.Containers[0].Name,
		},

		Command:  []string{"fstrim", "/export/" + volume.Name},
		Executor: &exec.DefaultRemoteExecutor{},
	}

	local.logger = local.logger.WithFields(logrus.Fields{
		"share-manager": shareManager.Name,
		"pod":           shareManagerPodName,
	})
	local.logger.Debugf("Executing command: %v", execOptions.Command)

	if err := execOptions.Validate(); err != nil {
		return err
	}

	return execOptions.Run()
}

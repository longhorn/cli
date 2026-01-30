package volume

import (
	"context"
	"os"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/longhorn/cli/pkg/types"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhTypes "github.com/longhorn/longhorn-manager/types"
	lhClient "github.com/longhorn/longhorn-manager/util/client"
)

type ChecksumRequester struct {
	ChecksumCmdOptions

	kubeClient     *kubeclient.Clientset
	longhornClient *lhClient.Clients
	cancel         context.CancelFunc
}

type ChecksumCmdOptions struct {
	types.GlobalCmdOptions

	VolumeName string
}

func (remote *ChecksumRequester) Validate() error {
	if remote.VolumeName == "" {
		return errors.New("Longhorn volume name (--name) is required")
	}
	return nil
}

func (remote *ChecksumRequester) Init() error {
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	err = os.Setenv(lhTypes.EnvPodNamespace, remote.Namespace)
	if err != nil {
		return errors.Wrapf(err, "failed to set env %v to %v", lhTypes.EnvPodNamespace, remote.Namespace)
	}

	lhClient, cancel, err := kubeutils.NewLonghornClient(remote.KubeConfigPath)
	if err != nil {
		return errors.Wrap(err, "failed to get clientset")
	}
	remote.longhornClient = lhClient
	remote.cancel = cancel

	return nil
}

func (remote *ChecksumRequester) Run() error {
	dataIntegrity, err := remote.longhornClient.Datastore.GetVolumeSnapshotDataIntegrity(remote.VolumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get snapshot data integrity setting for volume %v", remote.VolumeName)
	}
	if dataIntegrity == longhorn.SnapshotDataIntegrityDisabled {
		return errors.Errorf("snapshot data integrity is disabled for volume %v, cannot calculate snapshot checksums", remote.VolumeName)
	}

	volume, err := remote.longhornClient.Datastore.GetVolume(remote.VolumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get volume %v", remote.VolumeName)
	}

	// align with Longhorn Manager's logic
	if volume.Spec.NumberOfReplicas < 2 {
		return errors.Errorf("volume %v must have at least 2 replicas to calculate snapshot checksums; current number of replicas is %v", remote.VolumeName, volume.Spec.NumberOfReplicas)
	}

	snapshots, err := remote.longhornClient.Datastore.ListVolumeSnapshotsRO(volume.Name)
	if err != nil {
		return err
	}
	if len(snapshots) == 0 {
		return errors.Errorf("volume %v has no snapshots to calculate checksum", remote.VolumeName)
	}

	// Print snapshots info
	for _, snapshot := range snapshots {
		if !snapshot.Status.UserCreated {
			continue
		}

		if snapshot.Status.Checksum != "" {
			logrus.Infof("Snapshot %s with checksum %s", snapshot.Name, snapshot.Status.Checksum)
		} else {
			logrus.Infof("Snapshot %s has no checksum", snapshot.Name)
		}
	}

	volume.Spec.OnDemandChecksumRequestedAt = time.Now().UTC().Format(time.RFC3339)

	_, err = remote.longhornClient.Datastore.UpdateVolume(volume)
	if err != nil {
		return errors.Wrapf(err, "failed to update volume %v", remote.VolumeName)
	}

	logrus.Infof("Requested on-demand checksum calculation for volume %s", remote.VolumeName)
	logrus.Info("Calculating snapshot checksums may take some time. You can check the snapshot checksum by kubectl.")

	return nil
}

func (remote *ChecksumRequester) Cleanup() error {
	remote.cancel()
	return nil
}

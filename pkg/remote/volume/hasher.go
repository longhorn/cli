package volume

import (
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"k8s.io/client-go/util/retry"

	k8sErr "k8s.io/apimachinery/pkg/api/errors"

	"github.com/longhorn/cli/pkg/types"

	utilslonghorn "github.com/longhorn/cli/pkg/utils/longhorn"
)

type ChecksumRequester struct {
	ChecksumCmdOptions

	longhornClient *utilslonghorn.LonghornClient
}

type ChecksumCmdOptions struct {
	types.GlobalCmdOptions

	VolumeName string
	NodeID     string
	AllVolumes bool
}

func (remote *ChecksumRequester) Validate() error {
	hasName := remote.VolumeName != ""
	hasAll := remote.AllVolumes
	hasNode := remote.NodeID != ""

	// 1. Count how many options are provided
	count := 0
	for _, isUsed := range []bool{hasName, hasAll, hasNode} {
		if isUsed {
			count++
		}
	}

	// 2. Mutual exclusion check
	if count > 1 {
		return errors.New("only one of --name, --all, or --node-id can be specified")
	}

	// 3. Requirement check
	if count == 0 {
		return errors.New("one of --name, --all, or --node-id must be specified")
	}

	return nil
}

func (remote *ChecksumRequester) Init() error {
	clients, err := utilslonghorn.NewLonghornClient(remote.KubeConfigPath, remote.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Longhorn client")
	}

	remote.longhornClient = clients
	return nil
}

func (remote *ChecksumRequester) Run() error {
	// Use volume names instead of volume objects since updating a volume requires
	// fetching the latest version at the time of the request for safety.
	var volumeNames []string
	if remote.VolumeName != "" {
		volumeNames = []string{remote.VolumeName}
	} else {
		volumes, err := remote.longhornClient.ListVolumes()
		if err != nil {
			return errors.Wrap(err, "failed to list volumes")
		}
		for _, v := range volumes.Items {
			if remote.AllVolumes || v.Spec.NodeID == remote.NodeID {
				volumeNames = append(volumeNames, v.Name)
			}
		}
	}

	if len(volumeNames) == 0 {
		return errors.New("no volumes found")
	}

	errs := map[string]error{}
	for _, volumeName := range volumeNames {
		if err := remote.requestSnapshotChecksumCalculation(volumeName); err != nil {
			errs[volumeName] = err
		}
	}

	if len(errs) > 0 {
		for volumeName, err := range errs {
			logrus.WithFields(logrus.Fields{
				"volume": volumeName,
				"error":  err,
			}).Error("Failed to request snapshot checksum calculation")
		}

		return errors.Errorf("failed to request snapshot checksum calculation for %d volumes", len(errs))
	}

	logrus.Info("Snapshot checksum calculation may take some time. You can check the snapshot checksum via kubectl.")
	return nil
}

func (remote *ChecksumRequester) requestSnapshotChecksumCalculation(volumeName string) error {
	log := logrus.WithField("volume", volumeName)

	volume, err := remote.longhornClient.GetVolume(volumeName)
	if err != nil {
		return errors.Wrapf(err, "failed to get volume %v", volumeName)
	}

	// align with Longhorn Manager's logic
	if volume.Spec.NumberOfReplicas < 2 {
		return errors.Errorf("volume %v must have at least 2 replicas to calculate snapshot checksums; current number of replicas is %v", volumeName, volume.Spec.NumberOfReplicas)
	}

	snapshots, err := remote.longhornClient.ListVolumeSnapshots(volume.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to list snapshots for volume %v", volumeName)
	}
	if len(snapshots.Items) == 0 {
		log.Warn("volume has no snapshots for checksum calculation")
		return nil
	}

	// Do not validate SnapshotHashingRequestedAt or LastOnDemandSnapshotHashingCompleteAt here.
	// The validation logic is already handled by the webhook to avoid inconsistent behavior.
	// If the update is invalid, the webhook will reject it.
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		volume, err := remote.longhornClient.GetVolume(volumeName)
		if err != nil {
			return err
		}

		volume.Spec.SnapshotHashingRequestedAt = time.Now().UTC().Format(time.RFC3339)
		_, err = remote.longhornClient.UpdateVolume(volume)
		if k8sErr.IsConflict(err) {
			log.WithError(err).Warn("Conflict detected when updating volume, retrying")
		}
		return err
	})
	if err != nil {
		return errors.Wrapf(err, "failed to update volume %v", volumeName)
	}

	log.Infof("Requested on-demand checksum calculation for volume %s", volumeName)
	return nil
}

func (remote *ChecksumRequester) Cleanup() error {
	return nil
}

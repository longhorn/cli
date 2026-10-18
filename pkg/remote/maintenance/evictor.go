package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kubeclient "k8s.io/client-go/kubernetes"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"

	"github.com/longhorn/cli/pkg/types"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
	utilslonghorn "github.com/longhorn/cli/pkg/utils/longhorn"
)

const (
	// evictionPollInterval is how often the evictor re-checks replica counts while waiting.
	evictionPollInterval = 10 * time.Second
	// evictionWaitTimeout bounds how long --wait blocks before giving up.
	evictionWaitTimeout = 30 * time.Minute
)

// EvictorCmdOptions holds the options for the maintenance evict commands.
type EvictorCmdOptions struct {
	types.GlobalCmdOptions

	NodeID   string
	DiskUUID string
	Wait     bool
}

// Evictor provides functions for Longhorn node maintenance eviction.
type Evictor struct {
	EvictorCmdOptions

	kubeClient     *kubeclient.Clientset
	longhornClient *utilslonghorn.LonghornClient
}

// Init initializes the Evictor.
func (remote *Evictor) Init() error {
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	longhornClient, err := utilslonghorn.NewLonghornClient(remote.KubeConfigPath, remote.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Longhorn client")
	}
	remote.longhornClient = longhornClient

	return nil
}

// Validate checks that the required options are set.
func (remote *Evictor) Validate() error {
	if remote.NodeID == "" {
		return errors.New("node ID must be specified")
	}
	return nil
}

// ValidateDiskEviction checks the options required for disk eviction.
func (remote *Evictor) ValidateDiskEviction() error {
	if err := remote.Validate(); err != nil {
		return err
	}
	if remote.DiskUUID == "" {
		return errors.New("disk UUID must be specified")
	}
	return nil
}

// EvictNode prepares a node for maintenance: it cordons the Kubernetes node,
// disables Longhorn replica scheduling on it and requests eviction of all its
// replicas. With Wait it blocks until the node holds no replicas anymore.
func (remote *Evictor) EvictNode(ctx context.Context) error {
	log := logrus.WithField("node", remote.NodeID)

	if err := remote.cordonNode(ctx); err != nil {
		return err
	}

	node, err := remote.longhornClient.GetNode(ctx, remote.NodeID)
	if err != nil {
		return errors.Wrapf(err, "failed to get Longhorn node %s", remote.NodeID)
	}

	if !node.Spec.AllowScheduling || node.Spec.EvictionRequested {
		log.Info("Node eviction was already requested")
	} else {
		node.Spec.AllowScheduling = false
		node.Spec.EvictionRequested = true
		if _, err := remote.longhornClient.UpdateNode(ctx, node); err != nil {
			return errors.Wrapf(err, "failed to request eviction for node %s", remote.NodeID)
		}
		log.Info("Requested eviction for node")
	}

	if remote.Wait {
		log.Info("Waiting for replicas to leave the node")
		if err := remote.waitForNodeEviction(ctx); err != nil {
			return err
		}
		log.Info("All replicas evicted from node")
	}

	log.Infof("To finish maintenance, drain the node:\n  %s", drainCommand(remote.NodeID))
	return nil
}

// EvictDisk requests eviction of all replicas from a single disk of a node.
// With Wait it blocks until the disk holds no replicas anymore.
func (remote *Evictor) EvictDisk(ctx context.Context) error {
	log := logrus.WithFields(logrus.Fields{"node": remote.NodeID, "disk": remote.DiskUUID})

	node, err := remote.longhornClient.GetNode(ctx, remote.NodeID)
	if err != nil {
		return errors.Wrapf(err, "failed to get Longhorn node %s", remote.NodeID)
	}

	diskName, err := findDiskNameByUUID(node, remote.DiskUUID)
	if err != nil {
		return err
	}

	diskSpec, ok := node.Spec.Disks[diskName]
	if !ok {
		return errors.Errorf("disk %q (uuid %s) not found in node %s spec", diskName, remote.DiskUUID, remote.NodeID)
	}

	if diskSpec.EvictionRequested {
		log.Info("Disk eviction was already requested")
	} else {
		diskSpec.AllowScheduling = false
		diskSpec.EvictionRequested = true
		node.Spec.Disks[diskName] = diskSpec
		if _, err := remote.longhornClient.UpdateNode(ctx, node); err != nil {
			return errors.Wrapf(err, "failed to request eviction for disk %s on node %s", remote.DiskUUID, remote.NodeID)
		}
		log.Info("Requested eviction for disk")
	}

	if remote.Wait {
		log.Info("Waiting for replicas to leave the disk")
		if err := remote.waitForDiskEviction(ctx, remote.NodeID, diskName); err != nil {
			return err
		}
		log.Info("All replicas evicted from disk")
	}

	return nil
}

// cordonNode marks the Kubernetes node unschedulable so no new pods land on it
// while it is being maintained.
func (remote *Evictor) cordonNode(ctx context.Context) error {
	node, err := remote.kubeClient.CoreV1().Nodes().Get(ctx, remote.NodeID, metav1.GetOptions{})
	if err != nil {
		return errors.Wrapf(err, "failed to get Kubernetes node %s", remote.NodeID)
	}

	if node.Spec.Unschedulable {
		logrus.WithField("node", remote.NodeID).Info("Node is already cordoned")
		return nil
	}

	node.Spec.Unschedulable = true
	if _, err := remote.kubeClient.CoreV1().Nodes().Update(ctx, node, metav1.UpdateOptions{}); err != nil {
		return errors.Wrapf(err, "failed to cordon Kubernetes node %s", remote.NodeID)
	}
	logrus.WithField("node", remote.NodeID).Info("Cordoned node")

	return nil
}

// waitForNodeEviction polls until no replicas remain scheduled on the node.
func (remote *Evictor) waitForNodeEviction(ctx context.Context) error {
	return wait.PollUntilContextTimeout(ctx, evictionPollInterval, evictionWaitTimeout, true,
		func(ctx context.Context) (bool, error) {
			node, err := remote.longhornClient.GetNode(ctx, remote.NodeID)
			if err != nil {
				return false, errors.Wrapf(err, "failed to get Longhorn node %s", remote.NodeID)
			}
			return countScheduledReplicas(node, "") == 0, nil
		})
}

// waitForDiskEviction polls until no replicas remain scheduled on the disk.
func (remote *Evictor) waitForDiskEviction(ctx context.Context, nodeID, diskName string) error {
	return wait.PollUntilContextTimeout(ctx, evictionPollInterval, evictionWaitTimeout, true,
		func(ctx context.Context) (bool, error) {
			node, err := remote.longhornClient.GetNode(ctx, nodeID)
			if err != nil {
				return false, errors.Wrapf(err, "failed to get Longhorn node %s", nodeID)
			}
			return countScheduledReplicas(node, diskName) == 0, nil
		})
}

// findDiskNameByUUID resolves a disk UUID to its disk name in the node spec.
// The spec is keyed by disk name while users identify disks by UUID, so the
// lookup goes through the node status.
func findDiskNameByUUID(node *longhorn.Node, diskUUID string) (string, error) {
	for diskName, diskStatus := range node.Status.DiskStatus {
		if diskStatus != nil && diskStatus.DiskUUID == diskUUID {
			return diskName, nil
		}
	}
	return "", errors.Errorf("disk with UUID %s not found on node %s", diskUUID, node.Name)
}

// countScheduledReplicas returns the number of replicas scheduled on the node,
// or on a single disk when diskName is not empty.
func countScheduledReplicas(node *longhorn.Node, diskName string) int {
	count := 0
	for name, diskStatus := range node.Status.DiskStatus {
		if diskName != "" && name != diskName {
			continue
		}
		if diskStatus != nil {
			count += len(diskStatus.ScheduledReplica)
		}
	}
	return count
}

// drainCommand returns the kubectl command that finishes node maintenance once
// eviction is done. Draining itself stays a kubectl operation because the drain
// helper is not vendored by the CLI.
func drainCommand(nodeID string) string {
	return fmt.Sprintf("kubectl drain %s --ignore-daemonsets", nodeID)
}

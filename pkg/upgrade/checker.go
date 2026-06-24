package upgrade

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	commonio "github.com/longhorn/go-common-libs/io"
	lhtypes "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhmanager "github.com/longhorn/longhorn-manager/types"

	"github.com/longhorn/cli/pkg/types"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"

	utilslonghorn "github.com/longhorn/cli/pkg/utils/longhorn"
)

// Checker provides checks for Longhorn live-upgrade readiness.
type Checker struct {
	CheckerCmdOptions

	logger *logrus.Entry

	kubeClient     *kubeclient.Clientset
	longhornClient *utilslonghorn.LonghornClient

	collection         *types.LogCollection
	v2NodeCapabilities *v2NodeCapabilities
}

// CheckerCmdOptions holds the options for the upgrade check command.
type CheckerCmdOptions struct {
	types.GlobalCmdOptions

	OutputFilePath string
}

// v2NodeCapabilities caches node-level prerequisites shared by V2 volume checks.
type v2NodeCapabilities struct {
	runningInstanceManagerNodes map[string]struct{}
	schedulableBlockDiskNodes   map[string]struct{}
}

// Init initializes clients used by the upgrade checker.
func (c *Checker) Init() error {
	c.logger = logrus.WithField("data-engine", lhtypes.DataEngineTypeV2)

	kubeClient, err := kubeutils.NewKubeClient("", c.KubeConfigPath)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Kubernetes client")
	}

	client, err := utilslonghorn.NewLonghornClient(c.KubeConfigPath, c.Namespace)
	if err != nil {
		return errors.Wrap(err, "failed to initialize Longhorn client")
	}

	c.kubeClient = kubeClient
	c.longhornClient = client
	c.collection = &types.LogCollection{}
	c.v2NodeCapabilities = nil

	return nil
}

// Run checks V2 instance-manager live-upgrade readiness.
func (c *Checker) Run() error {
	c.logger.Info("Checking attached v2 volumes")
	if err := c.runV2Checks(); err != nil {
		return err
	}

	if len(c.collection.Error) != 0 || len(c.collection.Warn) != 0 {
		return errors.New("upgrade check failed")
	}

	return nil
}

// Output writes the check result to stdout or the configured output file.
func (c *Checker) Output() error {
	output, err := formatLogCollection(c.collection)
	if err != nil {
		return errors.Wrap(err, "failed to format upgrade check result")
	}

	if c.OutputFilePath == "" {
		fmt.Print(output)
		return nil
	}

	_, err = commonio.CreateDirectory(filepath.Dir(c.OutputFilePath), time.Now())
	if err != nil {
		return errors.Wrap(err, "failed to create directory")
	}

	c.logger.Debug("Writing result to file")
	if err := os.WriteFile(c.OutputFilePath, []byte(output), 0644); err != nil {
		return errors.Wrap(err, "failed to write output file")
	}

	return nil
}

func (c *Checker) runV2Checks() error {
	// V2 checks attached volumes for:
	// - no manager-defined unsupported topology (sharded, ublk, or unsupported DR)
	// - replicas distributed across at least two nodes
	// - no expansion or engine switchover in progress
	// - current engine existence, running state, and healthy volume state
	// - at least one eligible remote RW replica
	// - a candidate temporary node with a running V2 instance manager and schedulable block disk

	v2DataEngine, err := c.longhornClient.GetSetting(string(lhmanager.SettingNameV2DataEngine))
	if err != nil {
		return errors.Wrap(err, "failed to get v2 data engine setting")
	}
	v2DataEngineEnabled, err := strconv.ParseBool(v2DataEngine.Value)
	if err != nil {
		return errors.Wrap(err, "failed to parse v2 data engine setting")
	}
	if !v2DataEngineEnabled {
		c.addCheckPass("v2 data engine is not enabled; skipping live-upgrade checks")
		return nil
	}

	volumes, err := c.longhornClient.ListVolumesByDataEngine(lhtypes.DataEngineTypeV2)
	if err != nil {
		return errors.Wrap(err, "failed to list v2 volumes")
	}
	setting, err := c.longhornClient.GetSetting(string(lhmanager.SettingNameCurrentLonghornVersion))
	if err != nil {
		return errors.Wrap(err, "failed to get current Longhorn version setting")
	}
	if setting.Annotations[lhmanager.GetLonghornLabelKey(lhmanager.V2InstanceManagerLiveUpgradeUnsupported)] == lhtypes.TrueValue {
		c.addUnsupported("cluster", fmt.Sprintf("v2 instance manager live upgrade is unsupported before Longhorn %s", lhmanager.MinimumLonghornVersionForV2InstanceManagerLiveUpgrade))
	}

	attachedVolumeCount := 0
	for _, volume := range volumes.Items {
		if volume.Status.State != lhtypes.VolumeStateAttached {
			continue
		}

		attachedVolumeCount++
		ready, err := c.checkV2Volume(&volume)
		if err != nil {
			return err
		}
		if !ready {
			continue
		}
	}

	if attachedVolumeCount == 0 {
		c.addCheckPass(
			"no attached v2 volumes require live-upgrade checks")
	} else {
		c.addCheckPass(
			"checked attached v2 volumes for live-upgrade conditions")
	}

	return nil
}

func (c *Checker) checkV2Volume(volume *lhtypes.Volume) (bool, error) {
	if volume.Spec.DataLayout.Type == lhtypes.VolumeDataLayoutTypeSharded {
		c.addUnsupported(volume.Name, "sharded volume does not support v2 instance manager live upgrade")
		return false, nil
	}
	if volume.Spec.Frontend == lhtypes.VolumeFrontendUblk {
		c.addUnsupported(volume.Name, "ublk frontend volume does not support v2 instance manager live upgrade")
		return false, nil
	}
	if volume.Status.ExpansionRequired {
		c.addNotReady(volume.Name, "volume expansion is in progress; wait for it to complete and rerun the check")
		return false, nil
	}
	// A detached volume was skipped by runV2Checks. An attached volume without
	// a frontend path is not relocated during an IM live upgrade either.
	if !volume.Status.IsStandby && (volume.Spec.Frontend == lhtypes.VolumeFrontendEmpty || volume.Spec.DisableFrontend || volume.Status.FrontendDisabled) {
		return true, nil
	}
	return c.checkV2VolumeEngineAndReplicas(volume)
}

func (c *Checker) checkV2VolumeEngineAndReplicas(volume *lhtypes.Volume) (bool, error) {
	if volume.Status.CurrentEngineNodeID == "" {
		c.addNotReady(volume.Name, "current engine node is missing")
		return false, nil
	}

	engines, err := c.longhornClient.ListVolumeEngines(volume.Name)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list engines for volume %v", volume.Name)
	}
	var currentEngine *lhtypes.Engine
	enginesByName := map[string]*lhtypes.Engine{}
	for i := range engines.Items {
		engine := &engines.Items[i]
		enginesByName[engine.Name] = engine
		if engine.Spec.NodeID == volume.Status.CurrentEngineNodeID {
			currentEngine = engine
		}
	}
	if currentEngine == nil {
		c.addNotReady(volume.Name, fmt.Sprintf("current engine was not found on node %v", volume.Status.CurrentEngineNodeID))
		return false, nil
	}
	if volume.Spec.EngineNodeID != "" && volume.Spec.EngineNodeID != volume.Status.CurrentEngineNodeID {
		c.addNotReady(volume.Name, "engine switchover is in progress; wait for it to complete and rerun the check")
		return false, nil
	}
	if volume.Status.Robustness != lhtypes.VolumeRobustnessHealthy {
		c.addNotReady(volume.Name, "volume is not healthy; wait for replica rebuild or recovery and rerun the check")
		return false, nil
	}
	if currentEngine.Status.CurrentState != lhtypes.InstanceStateRunning {
		c.addNotReady(volume.Name, "current engine is not running")
		return false, nil
	}

	replicas, err := c.longhornClient.ListVolumeReplicas(volume.Name)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list replicas for volume %v", volume.Name)
	}
	if !replicasOnMultipleNodes(replicas.Items) {
		c.addUnsupported(volume.Name, "volume replicas are not distributed across multiple nodes")
		return false, nil
	}
	if volume.Status.IsStandby {
		replicasByName := map[string]*lhtypes.Replica{}
		for i := range replicas.Items {
			replicasByName[replicas.Items[i].Name] = &replicas.Items[i]
		}
		if len(currentEngine.Status.ReplicaModeMap) != volume.Spec.NumberOfReplicas {
			c.addNotReady(volume.Name, "DR volume does not have all replicas connected to the current engine")
			return false, nil
		}
		for replicaName, mode := range currentEngine.Status.ReplicaModeMap {
			replica := replicasByName[replicaName]
			if mode != lhtypes.ReplicaModeRW || replica == nil || replica.Status.CurrentState != lhtypes.InstanceStateRunning {
				c.addNotReady(volume.Name, "DR volume replicas are not fully recovered")
				return false, nil
			}
			if currentEngine.Status.CurrentReplicaAddressMap[replicaName] != net.JoinHostPort(replica.Status.StorageIP, strconv.Itoa(replica.Status.Port)) {
				c.addNotReady(volume.Name, "DR volume replica address has not been restored")
				return false, nil
			}
		}
		return true, nil
	}

	healthyReplicaNodes := map[string]struct{}{}
	for _, replica := range replicas.Items {
		if replica.Spec.NodeID == "" || replica.Spec.NodeID == volume.Status.CurrentEngineNodeID ||
			replica.Status.CurrentState != lhtypes.InstanceStateRunning ||
			replica.Spec.FailedAt != "" || replica.Spec.HealthyAt == "" ||
			!replica.Spec.Active || replica.Spec.EngineName == "" {
			continue
		}
		replicaEngine, ok := enginesByName[replica.Spec.EngineName]
		if !ok || replicaEngine.Status.ReplicaModeMap[replica.Name] != lhtypes.ReplicaModeRW {
			continue
		}
		healthyReplicaNodes[replica.Spec.NodeID] = struct{}{}
	}
	if len(healthyReplicaNodes) == 0 {
		c.addNotReady(volume.Name, "no active healthy RW replica is available on another node")
		return false, nil
	}

	capabilities, err := c.getV2NodeCapabilities()
	if err != nil {
		return false, err
	}
	// A regular attached volume needs a ready remote replica node for its
	// temporary frontend relocation. DR volumes return before this point.
	for nodeID := range healthyReplicaNodes {
		if _, ok := capabilities.runningInstanceManagerNodes[nodeID]; !ok {
			continue
		}
		if _, ok := capabilities.schedulableBlockDiskNodes[nodeID]; !ok {
			continue
		}
		return true, nil
	}

	c.addNotReady(volume.Name, "no eligible healthy replica node has a running v2 instance manager and schedulable block disk")
	return false, nil
}

func (c *Checker) addCheckPass(message string) {
	c.collection.Info = append(c.collection.Info, message)
}

func (c *Checker) addUnsupported(resourceName, message string) {
	c.collection.Error = append(c.collection.Error, fmt.Sprintf("%s: %s", resourceName, message))
}

func (c *Checker) addNotReady(resourceName, message string) {
	c.collection.Warn = append(c.collection.Warn, fmt.Sprintf("%s: %s", resourceName, message))
}

// getV2NodeCapabilities snapshots node prerequisites used to select temporary
// relocation nodes. It is shared by every regular attached V2 volume.
func (c *Checker) getV2NodeCapabilities() (*v2NodeCapabilities, error) {
	if c.v2NodeCapabilities != nil {
		return c.v2NodeCapabilities, nil
	}

	longhornNodes, err := c.longhornClient.ListNodes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list Longhorn nodes")
	}
	longhornNodesByName := map[string]*lhtypes.Node{}
	for i := range longhornNodes.Items {
		longhornNodesByName[longhornNodes.Items[i].Name] = &longhornNodes.Items[i]
	}

	kubernetesNodes, err := c.kubeClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list Kubernetes nodes")
	}
	kubernetesNodesByName := map[string]*corev1.Node{}
	for i := range kubernetesNodes.Items {
		kubernetesNodesByName[kubernetesNodes.Items[i].Name] = &kubernetesNodes.Items[i]
	}

	instanceManagers, err := c.longhornClient.ListInstanceManagers()
	if err != nil {
		return nil, errors.Wrap(err, "failed to list instance managers")
	}

	capabilities := &v2NodeCapabilities{
		runningInstanceManagerNodes: map[string]struct{}{},
		schedulableBlockDiskNodes:   map[string]struct{}{},
	}
	for _, im := range instanceManagers.Items {
		if im.Spec.DataEngine != lhtypes.DataEngineTypeV2 ||
			im.Spec.Type != lhtypes.InstanceManagerTypeAllInOne ||
			im.Status.CurrentState != lhtypes.InstanceManagerStateRunning ||
			im.DeletionTimestamp != nil ||
			!isV2NodeReady(im.Spec.NodeID, longhornNodesByName, kubernetesNodesByName) {
			continue
		}
		capabilities.runningInstanceManagerNodes[im.Spec.NodeID] = struct{}{}
	}
	for nodeID, node := range longhornNodesByName {
		if isV2NodeReady(nodeID, longhornNodesByName, kubernetesNodesByName) && hasSchedulableBlockDisk(node) {
			capabilities.schedulableBlockDiskNodes[nodeID] = struct{}{}
		}
	}

	c.v2NodeCapabilities = capabilities
	return capabilities, nil
}

func replicasOnMultipleNodes(replicas []lhtypes.Replica) bool {
	nodes := map[string]struct{}{}
	for _, replica := range replicas {
		if replica.DeletionTimestamp == nil && replica.Spec.NodeID != "" {
			nodes[replica.Spec.NodeID] = struct{}{}
		}
	}
	return len(nodes) >= 2
}

func formatLogCollection(collection *types.LogCollection) (string, error) {
	if collection == nil {
		return "", nil
	}

	var output strings.Builder
	writeSection := func(title string, lines []string) {
		if len(lines) == 0 {
			return
		}

		if output.Len() != 0 {
			output.WriteByte('\n')
		}
		output.WriteString(title)
		output.WriteString(":\n")
		for _, line := range lines {
			output.WriteString("- ")
			output.WriteString(line)
			output.WriteByte('\n')
		}
	}

	writeSection("Result", collection.Info)
	writeSection("NotReady", collection.Warn)
	writeSection("Unsupported", collection.Error)

	if output.Len() != 0 {
		return output.String(), nil
	}

	return "", nil
}

func isKubernetesNodeReady(node *corev1.Node) bool {
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady {
			return condition.Status == corev1.ConditionTrue
		}
	}

	return false
}

func getLonghornNodeCondition(conditions []lhtypes.Condition, conditionType string) *lhtypes.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			conditionCopy := condition
			return &conditionCopy
		}
	}

	return nil
}

func isV2NodeReady(nodeName string, longhornNodes map[string]*lhtypes.Node, kubernetesNodes map[string]*corev1.Node) bool {
	longhornNode, longhornNodeFound := longhornNodes[nodeName]
	kubernetesNode, kubernetesNodeFound := kubernetesNodes[nodeName]
	if !longhornNodeFound || !kubernetesNodeFound || !isKubernetesNodeReady(kubernetesNode) {
		return false
	}
	readyCondition := getLonghornNodeCondition(longhornNode.Status.Conditions, lhtypes.NodeConditionTypeReady)
	return readyCondition != nil && readyCondition.Status == lhtypes.ConditionStatusTrue
}

func hasSchedulableBlockDisk(node *lhtypes.Node) bool {
	if node == nil {
		return false
	}
	for diskName, diskSpec := range node.Spec.Disks {
		if diskSpec.Type != lhtypes.DiskTypeBlock {
			continue
		}
		diskStatus, ok := node.Status.DiskStatus[diskName]
		if !ok {
			continue
		}
		condition := lhmanager.GetCondition(diskStatus.Conditions, lhtypes.DiskConditionTypeSchedulable)
		if condition.Status == lhtypes.ConditionStatusTrue {
			return true
		}
	}
	return false
}

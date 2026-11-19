package spdk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/pkg/errors"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/kustomize/kyaml/yaml"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
)

const (
	OutputFormatYAML = "yaml"
	OutputFormatJSON = "json"
	OutputFormatText = "text"
)

// OutputFormats are the supported output formats for longhornctl diagnose v2-data-engine.
var OutputFormats = []string{OutputFormatYAML, OutputFormatJSON, OutputFormatText}

// Diagnoser provides cluster-wide SPDK target memory, iobuf, and deadlock diagnosis.
type Diagnoser struct {
	DiagnoserCmdOptions

	kubeClient kubeclient.Interface
	rawClient  *kubeclient.Clientset

	appName    string
	collection *types.SpdkCollection
}

// DiagnoserCmdOptions holds the CLI options for longhornctl diagnose v2-data-engine.
type DiagnoserCmdOptions struct {
	types.GlobalCmdOptions

	NodeID       string
	OutputFormat string
	IncludeAll   bool
}

// Validate validates the command options.
func (remote *Diagnoser) Validate() error {
	if remote.OutputFormat == "" {
		remote.OutputFormat = OutputFormatYAML
	}
	if !slices.Contains(OutputFormats, remote.OutputFormat) {
		return errors.Errorf("invalid --%s %q, must be one of %v", consts.CmdOptOutput, remote.OutputFormat, OutputFormats)
	}
	return nil
}

// Init initializes the remote SPDK Diagnoser.
func (remote *Diagnoser) Init() error {
	kubeClient, err := kubeutils.NewKubeClient("", remote.KubeConfigPath)
	if err != nil {
		return err
	}
	remote.rawClient = kubeClient
	remote.kubeClient = kubeClient
	remote.appName = consts.AppNameV2DataEngineDiagnoser
	return nil
}

// Run deploys the SPDK diagnoser DaemonSet, collects per-node SPDK diagnostics,
// analyzes cross-node dependencies and deadlock cycles, and marshals the output.
func (remote *Diagnoser) Run() ([]byte, error) {
	newDaemonSet, err := kubeutils.PrepareDaemonSet(remote.newDaemonSet(), remote.rawClient, remote.NodeSelector, remote.ImagePullSecret, remote.Tolerations)
	if err != nil {
		return nil, err
	}
	if remote.NodeID != "" {
		if newDaemonSet.Spec.Template.Spec.NodeSelector == nil {
			newDaemonSet.Spec.Template.Spec.NodeSelector = make(map[string]string)
		}
		newDaemonSet.Spec.Template.Spec.NodeSelector[corev1.LabelHostname] = remote.NodeID
	}

	daemonSet, err := commonkube.CreateDaemonSet(remote.rawClient, newDaemonSet)
	if err != nil {
		return nil, err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.rawClient, daemonSet, consts.ContainerNameInit, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationMedium))
	if err != nil {
		return nil, err
	}

	err = kubeutils.MonitorDaemonSetContainer(remote.rawClient, daemonSet, consts.ContainerNameOutput, kubeutils.WaitForDaemonSetContainersExit, ptr.To(consts.ContainerConditionMaxTolerationShort))
	if err != nil {
		return nil, err
	}

	podCollections, err := kubeutils.GetDaemonSetPodCollections(remote.rawClient, daemonSet, consts.ContainerNameOutput, false, false, nil)
	if err != nil {
		return nil, err
	}

	collection := &types.SpdkCollection{
		Nodes: make(map[string]*types.SpdkNodeInfo),
	}
	for _, podLog := range podCollections.Pods {
		var nodeInfo types.SpdkNodeInfo
		if err := json.Unmarshal([]byte(podLog.Log), &nodeInfo); err != nil {
			return nil, errors.Wrapf(err, "failed to parse SPDK output from node %s", podLog.Node)
		}
		nodeName := podLog.Node
		if nodeName == "" {
			nodeName = nodeInfo.Node
		}
		nodeInfo.Node = nodeName
		collection.Nodes[nodeName] = &nodeInfo
	}

	podIPToNode := remote.listInstanceManagerIPMap(context.Background())
	AnalyzeCrossNodeDependencies(collection, podIPToNode)
	remote.collection = collection

	return MarshalCollection(collection, remote.OutputFormat)
}

// UnhealthyReason returns a non-empty summary string when the diagnosed SPDK targets are stalled or deadlocked.
func (remote *Diagnoser) UnhealthyReason() string {
	return UnhealthyReason(remote.collection)
}

// UnhealthyReason returns a non-empty summary string when collection indicates Stalled or Deadlocked SPDK state.
func UnhealthyReason(collection *types.SpdkCollection) string {
	if collection == nil || collection.Summary == nil {
		return ""
	}
	status := collection.Summary.Status
	if status != types.SpdkStatusDeadlocked && status != types.SpdkStatusStalled {
		return ""
	}

	var unhealthyNodes []string
	for _, ns := range collection.Summary.Nodes {
		if ns.Status == types.SpdkStatusDeadlocked || ns.Status == types.SpdkStatusStalled {
			unhealthyNodes = append(unhealthyNodes, fmt.Sprintf("%s (%s)", ns.Node, ns.Status))
		}
	}
	if len(unhealthyNodes) > 0 {
		return fmt.Sprintf("status=%s, unhealthy nodes: %s", status, strings.Join(unhealthyNodes, ", "))
	}
	return fmt.Sprintf("status=%s", status)
}

// Cleanup deletes the DaemonSet created for the SPDK diagnoser.
func (remote *Diagnoser) Cleanup() error {
	return commonkube.DeleteDaemonSet(remote.rawClient, remote.Namespace, remote.appName)
}

func (remote *Diagnoser) listInstanceManagerIPMap(ctx context.Context) map[string]string {
	ipToNode := make(map[string]string)
	if remote.kubeClient == nil {
		return ipToNode
	}
	pods, err := remote.kubeClient.CoreV1().Pods(remote.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "longhorn.io/component=instance-manager",
	})
	if err != nil {
		return ipToNode
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.PodIP != "" && pod.Spec.NodeName != "" {
			ipToNode[pod.Status.PodIP] = pod.Spec.NodeName
		}
	}
	return ipToNode
}

func (remote *Diagnoser) newDaemonSet() *appsv1.DaemonSet {
	outputFilePath := filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNameOutputJSON)
	cmd := []string{consts.CmdLonghornctlLocal, consts.SubCmdDiagnose, consts.SubCmdV2DataEngine}
	if remote.IncludeAll {
		cmd = append(cmd, "--"+consts.CmdOptAll)
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
					HostPID: true,
					InitContainers: []corev1.Container{
						{
							Name:    consts.ContainerNameInit,
							Image:   utils.BuildImageName(remote.Image, remote.ImageRegistry),
							Command: cmd,
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

// MarshalCollection encodes a SpdkCollection in YAML, JSON, or human-readable text format.
func MarshalCollection(collection *types.SpdkCollection, format string) ([]byte, error) {
	switch format {
	case OutputFormatYAML:
		return yaml.Marshal(collection)
	case OutputFormatJSON:
		out, err := json.MarshalIndent(collection, "", "  ")
		if err != nil {
			return nil, err
		}
		return append(out, '\n'), nil
	case OutputFormatText:
		return []byte(FormatCollectionText(collection)), nil
	default:
		return nil, errors.Errorf("unsupported output format %q, must be one of %v", format, OutputFormats)
	}
}

// AnalyzeCrossNodeDependencies enriches active requests with target node names,
// populates CrossNodeDependencies, detects DeadlockCycles, and synthesizes Summary.
// All raw per-node fields under collection.Nodes are preserved intact.
func AnalyzeCrossNodeDependencies(collection *types.SpdkCollection, podIPToNode map[string]string) {
	if collection == nil {
		return
	}
	collection.CrossNodeDependencies = nil
	collection.DeadlockCycles = nil

	if len(collection.Nodes) == 0 {
		collection.Summary = &types.SpdkSummary{
			Status: types.SpdkStatusHealthy,
		}
		return
	}

	nodeNames := make([]string, 0, len(collection.Nodes))
	stallReasons := make(map[string]string, len(collection.Nodes))
	for nodeName, nodeInfo := range collection.Nodes {
		nodeNames = append(nodeNames, nodeName)
		stallReasons[nodeName] = summarizeNodeStallReason(nodeInfo)
	}
	sort.Strings(nodeNames)

	// Resolve RemoteTargetNode on every active request first.
	for _, fromNode := range nodeNames {
		nodeInfo := collection.Nodes[fromNode]
		if nodeInfo == nil {
			continue
		}
		for _, proc := range nodeInfo.Processes {
			for _, th := range proc.Threads {
				for _, bsCh := range th.BlobstoreChannels {
					for _, act := range bsCh.ActiveRequests {
						if act.RemoteTargetAddress == "" {
							continue
						}
						host := act.RemoteTargetAddress
						if h, _, err := net.SplitHostPort(act.RemoteTargetAddress); err == nil {
							host = h
						}
						if toNode := podIPToNode[host]; toNode != "" {
							act.RemoteTargetNode = toNode
						}
					}
				}
			}
		}
	}

	// Propagate transitive stalls when a channel with queued cluster allocations waits on a stalled remote node.
	changed := true
	for changed {
		changed = false
		for _, fromNode := range nodeNames {
			if stallReasons[fromNode] != "" {
				continue
			}
			nodeInfo := collection.Nodes[fromNode]
			if nodeInfo == nil {
				continue
			}
			for _, proc := range nodeInfo.Processes {
				for _, th := range proc.Threads {
					for _, bsCh := range th.BlobstoreChannels {
						if bsCh.NeedClusterAlloc == 0 && bsCh.FreeReqs > 0 {
							continue
						}
						for _, act := range bsCh.ActiveRequests {
							if act.RemoteTargetNode != "" && stallReasons[act.RemoteTargetNode] != "" {
								stallReasons[fromNode] = fmt.Sprintf(
									"%s bs_channel %s cluster_alloc blocked (need_cluster_alloc=%d, free_reqs=%d/%d) waiting on stalled node %s",
									bsCh.Device, bsCh.Address, bsCh.NeedClusterAlloc, bsCh.FreeReqs, bsCh.TotalReqs, act.RemoteTargetNode)
								changed = true
								break
							}
						}
						if changed {
							break
						}
					}
					if changed {
						break
					}
				}
				if changed {
					break
				}
			}
		}
	}

	adj := make(map[string]map[string]struct{})
	for _, fromNode := range nodeNames {
		nodeInfo := collection.Nodes[fromNode]
		if nodeInfo == nil {
			continue
		}
		for _, proc := range nodeInfo.Processes {
			for _, th := range proc.Threads {
				for _, bsCh := range th.BlobstoreChannels {
					for _, act := range bsCh.ActiveRequests {
						if act.RemoteTargetAddress == "" {
							continue
						}
						toNode := act.RemoteTargetNode
						reason := stallReasons[toNode]
						lvolName := act.LvolName
						if lvolName == "" {
							lvolName = bsCh.LvolName
						}
						remoteSnap := extractRemoteSnapshotLvol(act.BackingDevice)
						readCnt, maxRetries := findNodeNoMemReads(collection.Nodes[toNode], remoteSnap)

						dep := &types.SpdkCrossNodeDependency{
							FromNode:              fromNode,
							ToNode:                toNode,
							OpType:                act.OpType,
							TargetAddress:         act.RemoteTargetAddress,
							LocalLvol:             lvolName,
							RemoteSnapshotLvol:    remoteSnap,
							BackingDevice:         act.BackingDevice,
							IOUnit:                act.IOUnit,
							TargetStalled:         reason != "",
							TargetReason:          reason,
							RemoteNoMemReadCount:  readCnt,
							RemoteNoMemMaxRetries: maxRetries,
						}
						collection.CrossNodeDependencies = append(collection.CrossNodeDependencies, dep)

						if toNode != "" && toNode != fromNode && reason != "" && stallReasons[fromNode] != "" {
							if adj[fromNode] == nil {
								adj[fromNode] = make(map[string]struct{})
							}
							adj[fromNode][toNode] = struct{}{}
						}
					}
				}
			}
		}
	}

	collection.DeadlockCycles = findDirectedCycles(adj)
	collection.Summary = buildCollectionSummary(collection, nodeNames, stallReasons)
}

func summarizeNodeStallReason(nodeInfo *types.SpdkNodeInfo) string {
	if nodeInfo == nil {
		return ""
	}
	var reasons []string
	for _, proc := range nodeInfo.Processes {
		for _, th := range proc.Threads {
			if th.IobufChannel != nil && (th.IobufChannel.LargeWaiters > 0 || th.IobufChannel.SmallWaiters > 0) {
				reasons = append(reasons,
					fmt.Sprintf("iobuf pool exhausted (free=%d, waiters=%d)",
						th.IobufChannel.TotalFreeLargeBuffers, th.IobufChannel.LargeWaiters+th.IobufChannel.SmallWaiters))
			}
			for _, bsCh := range th.BlobstoreChannels {
				if bsCh.FreeReqs == 0 && bsCh.TotalReqs > 0 {
					if len(bsCh.NeedClusterAllocByLvol) > 0 {
						reasons = append(reasons,
							fmt.Sprintf("%s bs_channel %s free_reqs=0/%d (need_cluster_alloc=%d across %d lvols)",
								bsCh.Device, bsCh.Address, bsCh.TotalReqs, bsCh.NeedClusterAlloc, len(bsCh.NeedClusterAllocByLvol)))
					} else {
						reasons = append(reasons,
							fmt.Sprintf("%s bs_channel %s free_reqs=0/%d (need_cluster_alloc=%d)",
								bsCh.Device, bsCh.Address, bsCh.TotalReqs, bsCh.NeedClusterAlloc))
					}
				} else if bsCh.FreeReqs > 0 && bsCh.NeedClusterAlloc > 0 && len(bsCh.ActiveRequests) > 0 &&
					(bsCh.NeedClusterAlloc >= bsCh.TotalReqs/2 || len(th.BdevNoMemResources) > 0) {
					reasons = append(reasons,
						fmt.Sprintf("%s bs_channel %s cluster_alloc blocked (need_cluster_alloc=%d, free_reqs=%d/%d)",
							bsCh.Device, bsCh.Address, bsCh.NeedClusterAlloc, bsCh.FreeReqs, bsCh.TotalReqs))
				}
			}

			totalNoMemIO := 0
			noMemResCount := 0
			for _, nm := range th.BdevNoMemResources {
				if nm.NoMemIOCount > 0 {
					totalNoMemIO += nm.NoMemIOCount
					noMemResCount++
				}
			}
			if totalNoMemIO > 0 {
				reasons = append(reasons, fmt.Sprintf("bdev nomem_io=%d across %d bdevs", totalNoMemIO, noMemResCount))
			}
		}
	}
	return strings.Join(reasons, "; ")
}

func extractRemoteSnapshotLvol(backingDev string) string {
	if backingDev == "" || backingDev == "blob" {
		return ""
	}
	trimmed := strings.TrimPrefix(backingDev, "bdev_")
	if idx := strings.LastIndexByte(trimmed, 'n'); idx > 0 && idx < len(trimmed)-1 {
		allDigits := true
		for _, r := range trimmed[idx+1:] {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return trimmed[:idx]
		}
	}
	return trimmed
}

func findNodeNoMemReads(nodeInfo *types.SpdkNodeInfo, targetLvol string) (int, uint16) {
	if nodeInfo == nil || targetLvol == "" {
		return 0, 0
	}
	totalReads := 0
	var maxRetries uint16
	for _, proc := range nodeInfo.Processes {
		for _, th := range proc.Threads {
			for _, nm := range th.BdevNoMemResources {
				for _, entry := range nm.ByBdev {
					if entry.IOType != "read" {
						continue
					}
					if entry.LvolName == targetLvol || entry.BdevName == targetLvol {
						totalReads += entry.Count
						if entry.MaxRetries > maxRetries {
							maxRetries = entry.MaxRetries
						}
					}
				}
			}
		}
	}
	return totalReads, maxRetries
}

func buildCollectionSummary(
	collection *types.SpdkCollection,
	nodeNames []string,
	stallReasons map[string]string,
) *types.SpdkSummary {
	deadlockedSet := make(map[string]bool)
	for _, cycle := range collection.DeadlockCycles {
		for _, n := range cycle {
			deadlockedSet[n] = true
		}
	}

	summary := &types.SpdkSummary{
		Status: types.SpdkStatusHealthy,
		Nodes:  make([]*types.SpdkNodeSummary, 0, len(nodeNames)),
	}

	totalIobufWaiters := 0
	hasIobufTelemetry := false
	hasBsExhaustionOrBlock := false
	hasBdevNoMem := false

	for _, nodeName := range nodeNames {
		nodeInfo := collection.Nodes[nodeName]
		ns := buildNodeSummary(nodeName, nodeInfo, deadlockedSet[nodeName], stallReasons[nodeName], collection)
		summary.Nodes = append(summary.Nodes, ns)

		totalIobufWaiters += ns.IobufWaiters
		if ns.IobufLargeFree > 0 || ns.IobufWaiters > 0 {
			hasIobufTelemetry = true
		}
		if ns.BsChannelTotalReqs > 0 && (ns.BsChannelFreeReqs == 0 || ns.BsChannelNeedClusterAlloc > 0) {
			hasBsExhaustionOrBlock = true
		}
		if ns.BdevNoMemIOCount > 0 {
			hasBdevNoMem = true
		}

		switch ns.Status {
		case types.SpdkStatusDeadlocked:
			summary.Status = types.SpdkStatusDeadlocked
		case types.SpdkStatusStalled:
			if summary.Status != types.SpdkStatusDeadlocked {
				summary.Status = types.SpdkStatusStalled
			}
		case types.SpdkStatusWarning:
			if summary.Status == types.SpdkStatusHealthy {
				summary.Status = types.SpdkStatusWarning
			}
		}
	}

	for _, cycle := range collection.DeadlockCycles {
		summary.Findings = append(summary.Findings,
			fmt.Sprintf("Cross-node SPDK deadlock cycle detected: %s", strings.Join(cycle, " -> ")))
	}

	for _, dep := range collection.CrossNodeDependencies {
		if !dep.TargetStalled {
			continue
		}
		targetLabel := dep.ToNode
		if targetLabel == "" {
			targetLabel = dep.TargetAddress
		}
		snapLabel := dep.RemoteSnapshotLvol
		if snapLabel == "" {
			snapLabel = dep.BackingDevice
		}
		if !deadlockedSet[dep.FromNode] {
			summary.Findings = append(summary.Findings,
				fmt.Sprintf("Node %s is blocked on local lvol %s waiting for stalled node %s (%s, snapshot %s)",
					dep.FromNode, dep.LocalLvol, targetLabel, dep.TargetAddress, snapLabel))
		}
		if dep.RemoteNoMemReadCount > 0 {
			summary.Findings = append(summary.Findings,
				fmt.Sprintf("Cross-node snapshot read %s -> %s (%s) has %d read I/O(s) stuck in %s nomem_io (maxRetries=%d)",
					dep.FromNode, targetLabel, snapLabel, dep.RemoteNoMemReadCount, targetLabel, dep.RemoteNoMemMaxRetries))
		}
	}

	for _, ns := range summary.Nodes {
		if ns.IobufWaiters > 0 {
			summary.Findings = append(summary.Findings,
				fmt.Sprintf("Node %s has %d request(s) blocked on SPDK iobuf pool exhaustion (iobufLargeFree=%d)",
					ns.Node, ns.IobufWaiters, ns.IobufLargeFree))
		} else if len(collection.DeadlockCycles) == 0 && ns.Status == types.SpdkStatusStalled && ns.Reason != "" {
			alreadyCovered := false
			for _, dep := range collection.CrossNodeDependencies {
				if dep.FromNode == ns.Node && dep.TargetStalled {
					alreadyCovered = true
					break
				}
			}
			if !alreadyCovered {
				summary.Findings = append(summary.Findings, fmt.Sprintf("Node %s is stalled: %s", ns.Node, ns.Reason))
			}
		}
	}

	if (summary.Status == types.SpdkStatusDeadlocked || summary.Status == types.SpdkStatusStalled) &&
		totalIobufWaiters == 0 && hasIobufTelemetry && hasBsExhaustionOrBlock {
		msg := "SPDK iobuf pools are healthy on all inspected nodes (0 waiters)"
		if hasBdevNoMem {
			msg += "; bdev nomem_io queues are a downstream symptom of blobstore channel exhaustion/blocking"
		}
		summary.Findings = append(summary.Findings, msg)
	}

	return summary
}

func buildNodeSummary(
	nodeName string,
	nodeInfo *types.SpdkNodeInfo,
	inDeadlock bool,
	stallReason string,
	collection *types.SpdkCollection,
) *types.SpdkNodeSummary {
	ns := &types.SpdkNodeSummary{
		Node:   nodeName,
		Status: types.SpdkStatusHealthy,
		Reason: stallReason,
	}
	if nodeInfo == nil {
		return ns
	}

	hasErrors := len(nodeInfo.Errors) > 0
	hasWarnings := len(nodeInfo.Warnings) > 0
	affectedLvols := make(map[string]struct{})

	var bestBsCh *types.SpdkBsChannelInfo
	bestBsIsDataPlane := false

	for _, proc := range nodeInfo.Processes {
		if ns.PID == 0 {
			ns.PID = proc.PID
			ns.PerBlobChannelSupported = proc.PerBlobChannelSupported
		}
		if len(proc.Errors) > 0 {
			hasErrors = true
		}
		if len(proc.Warnings) > 0 {
			hasWarnings = true
		}
		if proc.Iobuf != nil && uint64(proc.Iobuf.LargeRingFree) > ns.IobufLargeFree {
			ns.IobufLargeFree = uint64(proc.Iobuf.LargeRingFree)
		}

		for _, th := range proc.Threads {
			isDataPlane := th.Name != "app_thread"
			if th.IobufChannel != nil {
				ns.IobufWaiters += th.IobufChannel.SmallWaiters + th.IobufChannel.LargeWaiters
				if th.IobufChannel.TotalFreeLargeBuffers > ns.IobufLargeFree {
					ns.IobufLargeFree = th.IobufChannel.TotalFreeLargeBuffers
				}
			}

			for _, bsCh := range th.BlobstoreChannels {
				if isMoreConstrainedBsChannel(bsCh, isDataPlane, bestBsCh, bestBsIsDataPlane) {
					bestBsCh = bsCh
					bestBsIsDataPlane = isDataPlane
				}
				if bsCh.NeedClusterAlloc > 0 || bsCh.FreeReqs == 0 {
					for lvol := range bsCh.NeedClusterAllocByLvol {
						affectedLvols[lvol] = struct{}{}
					}
					if bsCh.LvolName != "" {
						affectedLvols[bsCh.LvolName] = struct{}{}
					}
					for _, act := range bsCh.ActiveRequests {
						ns.BlockingRequests = append(ns.BlockingRequests, formatBlockingRequest(act, bsCh, collection))
					}
				}
			}

			for _, nm := range th.BdevNoMemResources {
				if nm.NoMemIOCount > 0 {
					ns.BdevNoMemIOCount += nm.NoMemIOCount
					ns.BdevNoMemBdevCount++
				}
			}
		}
	}

	if bestBsCh != nil {
		ns.BsChannelFreeReqs = bestBsCh.FreeReqs
		ns.BsChannelTotalReqs = bestBsCh.TotalReqs
		ns.BsChannelNeedClusterAlloc = bestBsCh.NeedClusterAlloc
	}
	ns.BsChannelAffectedLvols = len(affectedLvols)

	switch {
	case inDeadlock:
		ns.Status = types.SpdkStatusDeadlocked
	case stallReason != "" || hasErrors:
		ns.Status = types.SpdkStatusStalled
		if ns.Reason == "" {
			ns.Reason = firstNodeError(nodeInfo)
		}
	case hasWarnings:
		ns.Status = types.SpdkStatusWarning
	default:
		ns.Status = types.SpdkStatusHealthy
	}

	return ns
}

func isMoreConstrainedBsChannel(
	cand *types.SpdkBsChannelInfo, candDataPlane bool,
	cur *types.SpdkBsChannelInfo, curDataPlane bool,
) bool {
	if cur == nil {
		return true
	}
	candBusy := cand.NeedClusterAlloc > 0 || cand.FreeReqs < cand.TotalReqs || len(cand.ActiveRequests) > 0
	curBusy := cur.NeedClusterAlloc > 0 || cur.FreeReqs < cur.TotalReqs || len(cur.ActiveRequests) > 0
	if candBusy != curBusy {
		return candBusy
	}
	if candDataPlane != curDataPlane {
		return candDataPlane
	}
	if cand.FreeReqs != cur.FreeReqs {
		return cand.FreeReqs < cur.FreeReqs
	}
	return cand.NeedClusterAlloc > cur.NeedClusterAlloc
}

func formatBlockingRequest(
	act *types.SpdkBsActiveRequest,
	bsCh *types.SpdkBsChannelInfo,
	collection *types.SpdkCollection,
) string {
	lvol := act.LvolName
	if lvol == "" {
		lvol = bsCh.LvolName
	}
	if lvol == "" {
		lvol = act.BlobID
	}

	desc := fmt.Sprintf("req #%d (%s)", act.Index, act.OpType)
	if lvol != "" {
		desc += fmt.Sprintf(" on %s", lvol)
	}
	if act.IOUnit > 0 {
		desc += fmt.Sprintf(" [ioUnit=%d]", act.IOUnit)
	}
	if act.RemoteTargetAddress != "" {
		target := act.RemoteTargetAddress
		if act.RemoteTargetNode != "" {
			target = fmt.Sprintf("%s (%s)", act.RemoteTargetNode, act.RemoteTargetAddress)
		}
		snap := extractRemoteSnapshotLvol(act.BackingDevice)
		if snap != "" {
			desc += fmt.Sprintf(" -> %s snap=%s", target, snap)
			if collection != nil && act.RemoteTargetNode != "" {
				if readCnt, retries := findNodeNoMemReads(collection.Nodes[act.RemoteTargetNode], snap); readCnt > 0 {
					desc += fmt.Sprintf(" [%d read I/O(s) stuck in target nomem_io, maxRetries=%d]", readCnt, retries)
				}
			}
		} else {
			desc += fmt.Sprintf(" -> %s", target)
		}
	}
	return desc
}

func firstNodeError(nodeInfo *types.SpdkNodeInfo) string {
	if len(nodeInfo.Errors) > 0 {
		return nodeInfo.Errors[0]
	}
	for _, proc := range nodeInfo.Processes {
		if len(proc.Errors) > 0 {
			return proc.Errors[0]
		}
	}
	return ""
}

// FormatCollectionText renders a concise, human-readable diagnostic report for quick troubleshooting.
func FormatCollectionText(collection *types.SpdkCollection) string {
	if collection == nil {
		return ""
	}
	if collection.Summary == nil {
		nodeNames := make([]string, 0, len(collection.Nodes))
		stallReasons := make(map[string]string, len(collection.Nodes))
		for n, info := range collection.Nodes {
			nodeNames = append(nodeNames, n)
			stallReasons[n] = summarizeNodeStallReason(info)
		}
		sort.Strings(nodeNames)
		collection.Summary = buildCollectionSummary(collection, nodeNames, stallReasons)
	}

	var buf bytes.Buffer
	sum := collection.Summary
	fmt.Fprintf(&buf, "CLUSTER SPDK STATUS: %s\n", strings.ToUpper(sum.Status))

	if len(sum.Findings) > 0 {
		fmt.Fprintf(&buf, "\nFINDINGS:\n")
		for _, f := range sum.Findings {
			fmt.Fprintf(&buf, "  - %s\n", f)
		}
	}

	if len(collection.DeadlockCycles) > 0 {
		fmt.Fprintf(&buf, "\nDEADLOCK CYCLES:\n")
		for i, cycle := range collection.DeadlockCycles {
			fmt.Fprintf(&buf, "  [%d] %s\n", i+1, strings.Join(cycle, " -> "))
		}
	}

	if len(collection.CrossNodeDependencies) > 0 {
		fmt.Fprintf(&buf, "\nCROSS-NODE DEPENDENCIES:\n")
		for _, dep := range collection.CrossNodeDependencies {
			toLabel := dep.ToNode
			if toLabel == "" {
				toLabel = dep.TargetAddress
			} else {
				toLabel = fmt.Sprintf("%s (%s)", dep.ToNode, dep.TargetAddress)
			}
			snap := dep.RemoteSnapshotLvol
			if snap == "" {
				snap = dep.BackingDevice
			}
			state := "target=ok"
			if dep.TargetStalled {
				state = "target=STALLED"
			}
			line := fmt.Sprintf("  - %s (%s) --[%s]--> %s snap=%s [%s]",
				dep.FromNode, dep.LocalLvol, dep.OpType, toLabel, snap, state)
			if dep.RemoteNoMemReadCount > 0 {
				line += fmt.Sprintf(" (%d read I/Os stuck in target nomem_io, maxRetries=%d)",
					dep.RemoteNoMemReadCount, dep.RemoteNoMemMaxRetries)
			}
			fmt.Fprintln(&buf, line)
		}
	}

	if len(sum.Nodes) > 0 {
		fmt.Fprintf(&buf, "\nNODE SUMMARY:\n")
		tw := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(tw, "  NODE\tSTATUS\tPID\tPER-BLOB-CH\tIOBUF-LARGE-FREE\tBS-REQ-FREE\tCLUSTER-ALLOC-Q\tBDEV-NOMEM-IO")
		for _, ns := range sum.Nodes {
			iobufCol := fmt.Sprintf("%d (%d waiters)", ns.IobufLargeFree, ns.IobufWaiters)
			bsFreeCol := "-"
			allocCol := "-"
			if ns.BsChannelTotalReqs > 0 {
				bsFreeCol = fmt.Sprintf("%d/%d", ns.BsChannelFreeReqs, ns.BsChannelTotalReqs)
				if ns.BsChannelAffectedLvols > 0 {
					allocCol = fmt.Sprintf("%d (%d lvols)", ns.BsChannelNeedClusterAlloc, ns.BsChannelAffectedLvols)
				} else {
					allocCol = fmt.Sprintf("%d", ns.BsChannelNeedClusterAlloc)
				}
			}
			nomemCol := "0"
			if ns.BdevNoMemIOCount > 0 {
				nomemCol = fmt.Sprintf("%d (%d bdevs)", ns.BdevNoMemIOCount, ns.BdevNoMemBdevCount)
			}
			_, _ = fmt.Fprintf(tw, "  %s\t%s\t%d\t%t\t%s\t%s\t%s\t%s\n",
				ns.Node, ns.Status, ns.PID, ns.PerBlobChannelSupported, iobufCol, bsFreeCol, allocCol, nomemCol)
		}
		_ = tw.Flush()
	}

	hasDetails := false
	for _, ns := range sum.Nodes {
		if len(ns.BlockingRequests) > 0 || (ns.Status != types.SpdkStatusHealthy && ns.Reason != "") {
			if !hasDetails {
				fmt.Fprintf(&buf, "\nNODE DETAILS:\n")
				hasDetails = true
			}
			fmt.Fprintf(&buf, "  %s (%s):\n", ns.Node, ns.Status)
			if ns.Reason != "" {
				fmt.Fprintf(&buf, "    reason: %s\n", ns.Reason)
			}
			for _, br := range ns.BlockingRequests {
				fmt.Fprintf(&buf, "    blocker: %s\n", br)
			}
		}
	}

	fmt.Fprintf(&buf, "\nNote: Use \"-o yaml\" or \"-o json\" for full raw thread, poller, iobuf cache, and per-lvol queue forensic data.\n")
	return buf.String()
}

func findDirectedCycles(adj map[string]map[string]struct{}) [][]string {
	if len(adj) == 0 {
		return nil
	}
	nodes := make([]string, 0, len(adj))
	for n := range adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)

	seenCycles := make(map[string]struct{})
	var cycles [][]string

	for _, start := range nodes {
		var dfs func(cur string, path []string, inPath map[string]bool)
		dfs = func(cur string, path []string, inPath map[string]bool) {
			nexts := make([]string, 0, len(adj[cur]))
			for nxt := range adj[cur] {
				nexts = append(nexts, nxt)
			}
			sort.Strings(nexts)

			for _, nxt := range nexts {
				if nxt < start {
					continue
				}
				if nxt == start && len(path) >= 2 {
					cycle := make([]string, len(path)+1)
					copy(cycle, path)
					cycle[len(path)] = start
					key := strings.Join(cycle, "->")
					if _, dup := seenCycles[key]; !dup {
						seenCycles[key] = struct{}{}
						cycles = append(cycles, cycle)
					}
					continue
				}
				if inPath[nxt] {
					continue
				}
				inPath[nxt] = true
				dfs(nxt, append(path, nxt), inPath)
				delete(inPath, nxt)
			}
		}
		dfs(start, []string{start}, map[string]bool{start: true})
	}

	return cycles
}

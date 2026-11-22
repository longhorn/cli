package spdk

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/longhorn/cli/pkg/types"
)

func TestAnalyzeCrossNodeDependencies_ThreeNodeDeadlock(t *testing.T) {
	collection := &types.SpdkCollection{
		Nodes: map[string]*types.SpdkNodeInfo{
			"hp-23": {
				Node: "hp-23",
				Processes: []*types.SpdkProcessInfo{
					{
						PID: 23,
						Iobuf: &types.SpdkIobufInfo{
							LargePoolCount: 6144,
							LargeRingFree:  3008,
						},
						Threads: []*types.SpdkThreadInfo{
							{
								Name: "app_thread",
								BlobstoreChannels: []*types.SpdkBsChannelInfo{
									{
										Device:    "blobstore",
										Address:   "0x4b4cf90",
										TotalReqs: 512,
										FreeReqs:  512,
									},
								},
							},
							{
								Name: "nvmf_tgt_poll_group_000",
								IobufChannel: &types.SpdkThreadIobufInfo{
									TotalFreeLargeBuffers: 4409,
								},
								BlobstoreChannels: []*types.SpdkBsChannelInfo{
									{
										Device:           "blobstore",
										Address:          "0x7f77442cdfb0",
										TotalReqs:        512,
										FreeReqs:         0,
										NeedClusterAlloc: 511,
										NeedClusterAllocByLvol: map[string]int{
											"pvc-a-r-1": 300,
											"pvc-x-r-9": 211,
										},
										ActiveRequests: []*types.SpdkBsActiveRequest{
											{
												Index:               4,
												OpType:              "esnap_cow",
												LvolName:            "pvc-a-r-1",
												IOUnit:              4636672,
												BackingDevice:       "bdev_pvc-a-snap-on-123n1",
												RemoteTargetAddress: "10.42.1.123:20010",
											},
										},
									},
								},
								BdevNoMemResources: []*types.SpdkBdevNoMemInfo{
									{
										NoMemIOCount: 8,
										ByBdev: []*types.SpdkBdevNoMemEntry{
											{
												BdevName:   "1af7ab1b-ffe1-4687-95f6-2719672fcda0",
												LvolName:   "pvc-b-snap-on-23",
												IOType:     "read",
												Count:      8,
												MaxRetries: 38719,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"hp-123": {
				Node: "hp-123",
				Processes: []*types.SpdkProcessInfo{
					{
						PID: 23,
						Iobuf: &types.SpdkIobufInfo{
							LargePoolCount: 6144,
							LargeRingFree:  3008,
						},
						Threads: []*types.SpdkThreadInfo{
							{
								Name: "nvmf_tgt_poll_group_000",
								IobufChannel: &types.SpdkThreadIobufInfo{
									TotalFreeLargeBuffers: 4906,
								},
								BlobstoreChannels: []*types.SpdkBsChannelInfo{
									{
										Device:           "blobstore",
										Address:          "0x7fb7a43949a0",
										TotalReqs:        512,
										FreeReqs:         0,
										NeedClusterAlloc: 511,
										ActiveRequests: []*types.SpdkBsActiveRequest{
											{
												Index:               12,
												OpType:              "esnap_cow",
												LvolName:            "pvc-b-r-2",
												IOUnit:              7585792,
												BackingDevice:       "bdev_pvc-b-snap-on-23n1",
												RemoteTargetAddress: "10.42.0.23:20020",
											},
										},
									},
								},
								BdevNoMemResources: []*types.SpdkBdevNoMemInfo{
									{
										NoMemIOCount: 8,
										ByBdev: []*types.SpdkBdevNoMemEntry{
											{
												BdevName:   "cd5bc02d-cad2-4e81-bbad-8a4a27d23230",
												LvolName:   "pvc-a-snap-on-123",
												IOType:     "read",
												Count:      8,
												MaxRetries: 38746,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			"hp-161": {
				Node: "hp-161",
				Processes: []*types.SpdkProcessInfo{
					{
						PID: 23,
						Iobuf: &types.SpdkIobufInfo{
							LargePoolCount: 6144,
							LargeRingFree:  2944,
						},
						Threads: []*types.SpdkThreadInfo{
							{
								Name: "nvmf_tgt_poll_group_000",
								IobufChannel: &types.SpdkThreadIobufInfo{
									TotalFreeLargeBuffers: 4171,
								},
								BlobstoreChannels: []*types.SpdkBsChannelInfo{
									{
										Device:           "blobstore",
										Address:          "0x7f1100200000",
										TotalReqs:        512,
										FreeReqs:         47,
										NeedClusterAlloc: 464,
										ActiveRequests: []*types.SpdkBsActiveRequest{
											{
												Index:               1,
												OpType:              "esnap_cow",
												LvolName:            "pvc-c-r-3",
												BackingDevice:       "bdev_pvc-c-snap-on-23n1",
												RemoteTargetAddress: "10.42.0.23:20025",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	podIPToNode := map[string]string{
		"10.42.0.23":  "hp-23",
		"10.42.1.123": "hp-123",
		"10.42.2.161": "hp-161",
	}

	AnalyzeCrossNodeDependencies(collection, podIPToNode)

	require.Len(t, collection.CrossNodeDependencies, 3)
	for _, dep := range collection.CrossNodeDependencies {
		assert.True(t, dep.TargetStalled)
		assert.Contains(t, dep.TargetReason, "free_reqs=0/512")
	}
	// Verify cross-node snapshot read correlation against remote nomem_io
	assert.Equal(t, "pvc-b-snap-on-23", collection.CrossNodeDependencies[0].RemoteSnapshotLvol)
	assert.Equal(t, 8, collection.CrossNodeDependencies[0].RemoteNoMemReadCount)
	assert.Equal(t, uint16(38719), collection.CrossNodeDependencies[0].RemoteNoMemMaxRetries)

	require.Len(t, collection.DeadlockCycles, 1)
	assert.Equal(t, []string{"hp-123", "hp-23", "hp-123"}, collection.DeadlockCycles[0])

	require.NotNil(t, collection.Summary)
	assert.Equal(t, types.SpdkStatusDeadlocked, collection.Summary.Status)
	require.Len(t, collection.Summary.Nodes, 3)
	// Sorted node order: hp-123, hp-161, hp-23
	assert.Equal(t, types.SpdkStatusDeadlocked, collection.Summary.Nodes[0].Status)
	assert.Equal(t, types.SpdkStatusStalled, collection.Summary.Nodes[1].Status)
	assert.Equal(t, 47, collection.Summary.Nodes[1].BsChannelFreeReqs)
	assert.Equal(t, 464, collection.Summary.Nodes[1].BsChannelNeedClusterAlloc)
	assert.Equal(t, types.SpdkStatusDeadlocked, collection.Summary.Nodes[2].Status)
	// Ensure data-plane channel (0/512) was chosen over idle app_thread (512/512)
	assert.Equal(t, 0, collection.Summary.Nodes[2].BsChannelFreeReqs)
	assert.Equal(t, 2, collection.Summary.Nodes[2].BsChannelAffectedLvols)

	// Verify YAML places summary, deadlockCycles, and crossNodeDependencies BEFORE raw nodes while preserving raw data
	yamlBytes, err := MarshalCollection(collection, OutputFormatYAML)
	require.NoError(t, err)
	yamlStr := string(yamlBytes)
	summaryIdx := strings.Index(yamlStr, "summary:")
	cycleIdx := strings.Index(yamlStr, "deadlockCycles:")
	depIdx := strings.Index(yamlStr, "crossNodeDependencies:")
	nodesIdx := strings.Index(yamlStr, "\nnodes:\n")
	require.True(t, summaryIdx == 0 && summaryIdx < cycleIdx && cycleIdx < depIdx && depIdx < nodesIdx)
	// Raw data (such as app_thread and raw UUID BdevName) remains preserved under nodes
	assert.Contains(t, yamlStr[nodesIdx:], "app_thread")
	assert.Contains(t, yamlStr[nodesIdx:], "1af7ab1b-ffe1-4687-95f6-2719672fcda0")

	textBytes, err := MarshalCollection(collection, OutputFormatText)
	require.NoError(t, err)
	textStr := string(textBytes)
	assert.Contains(t, textStr, "CLUSTER SPDK STATUS: DEADLOCKED")
	assert.Contains(t, textStr, "hp-123 -> hp-23 -> hp-123")
	assert.Contains(t, textStr, "8 read I/Os stuck in target nomem_io")
	assert.Contains(t, textStr, "NODE SUMMARY:")

	reason := UnhealthyReason(collection)
	assert.Equal(t, "status=Deadlocked, unhealthy nodes: hp-123 (Deadlocked), hp-161 (Stalled), hp-23 (Deadlocked)", reason)
}

func TestDiagnoserValidateMarshalAndDaemonSet(t *testing.T) {
	d := &Diagnoser{}
	require.NoError(t, d.Validate())
	assert.Equal(t, OutputFormatYAML, d.OutputFormat)

	d.OutputFormat = OutputFormatText
	require.NoError(t, d.Validate())

	d.OutputFormat = "invalid"
	assert.EqualError(t, d.Validate(), `invalid --output "invalid", must be one of [yaml json text]`)

	d.IncludeAll = true
	ds := d.newDaemonSet()
	require.Len(t, ds.Spec.Template.Spec.InitContainers, 2)
	assert.Equal(t, []string{"longhornctl-local", "diagnose", "v2-data-engine", "--all"}, ds.Spec.Template.Spec.InitContainers[0].Command)

	col := &types.SpdkCollection{
		Nodes: map[string]*types.SpdkNodeInfo{
			"node-1": {Node: "node-1", Processes: []*types.SpdkProcessInfo{{PID: 100}}},
		},
	}
	AnalyzeCrossNodeDependencies(col, nil)
	assert.Empty(t, UnhealthyReason(col))

	yamlBytes, err := MarshalCollection(col, OutputFormatYAML)
	require.NoError(t, err)
	assert.Contains(t, string(yamlBytes), "node-1:")

	jsonBytes, err := MarshalCollection(col, OutputFormatJSON)
	require.NoError(t, err)
	assert.Contains(t, string(jsonBytes), `"pid": 100`)

	textBytes, err := MarshalCollection(col, OutputFormatText)
	require.NoError(t, err)
	assert.Contains(t, string(textBytes), "CLUSTER SPDK STATUS: HEALTHY")
}

package types

const (
	SpdkStatusHealthy    = "Healthy"
	SpdkStatusWarning    = "Warning"
	SpdkStatusStalled    = "Stalled"
	SpdkStatusDeadlocked = "Deadlocked"
)

// SpdkCollection holds cluster-wide or single-node SPDK target memory and iobuf diagnostics.
// Summary, DeadlockCycles, and CrossNodeDependencies are placed ahead of Nodes so YAML/JSON
// outputs surface the high-level diagnosis first while preserving all raw node data below.
type SpdkCollection struct {
	Summary               *SpdkSummary               `json:"summary,omitempty" yaml:"summary,omitempty"`
	DeadlockCycles        [][]string                 `json:"deadlockCycles,omitempty" yaml:"deadlockCycles,omitempty"`
	CrossNodeDependencies []*SpdkCrossNodeDependency `json:"crossNodeDependencies,omitempty" yaml:"crossNodeDependencies,omitempty"`
	Nodes                 map[string]*SpdkNodeInfo   `json:"nodes" yaml:"nodes"`
}

// SpdkSummary provides a high-level cluster and per-node diagnosis synthesized from raw SPDK data.
type SpdkSummary struct {
	Status   string             `json:"status" yaml:"status"`
	Findings []string           `json:"findings,omitempty" yaml:"findings,omitempty"`
	Nodes    []*SpdkNodeSummary `json:"nodes,omitempty" yaml:"nodes,omitempty"`
}

// SpdkNodeSummary summarizes the data-plane SPDK state on one node for quick triage.
type SpdkNodeSummary struct {
	Node                      string   `json:"node" yaml:"node"`
	Status                    string   `json:"status" yaml:"status"`
	PID                       int      `json:"pid,omitempty" yaml:"pid,omitempty"`
	PerBlobChannelSupported   bool     `json:"perBlobChannelSupported" yaml:"perBlobChannelSupported"`
	IobufLargeFree            uint64   `json:"iobufLargeFree" yaml:"iobufLargeFree"`
	IobufWaiters              int      `json:"iobufWaiters" yaml:"iobufWaiters"`
	BsChannelFreeReqs         int      `json:"bsChannelFreeReqs" yaml:"bsChannelFreeReqs"`
	BsChannelTotalReqs        int      `json:"bsChannelTotalReqs" yaml:"bsChannelTotalReqs"`
	BsChannelNeedClusterAlloc int      `json:"bsChannelNeedClusterAlloc" yaml:"bsChannelNeedClusterAlloc"`
	BsChannelAffectedLvols    int      `json:"bsChannelAffectedLvols" yaml:"bsChannelAffectedLvols"`
	BdevNoMemIOCount          int      `json:"bdevNoMemIOCount" yaml:"bdevNoMemIOCount"`
	BdevNoMemBdevCount        int      `json:"bdevNoMemBdevCount" yaml:"bdevNoMemBdevCount"`
	BlockingRequests          []string `json:"blockingRequests,omitempty" yaml:"blockingRequests,omitempty"`
	Reason                    string   `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// SpdkCrossNodeDependency describes an in-flight SPDK request on FromNode waiting on a remote NVMe-oF target on ToNode.
type SpdkCrossNodeDependency struct {
	FromNode              string `json:"fromNode" yaml:"fromNode"`
	ToNode                string `json:"toNode,omitempty" yaml:"toNode,omitempty"`
	OpType                string `json:"opType,omitempty" yaml:"opType,omitempty"`
	TargetAddress         string `json:"targetAddress" yaml:"targetAddress"`
	LocalLvol             string `json:"localLvol,omitempty" yaml:"localLvol,omitempty"`
	RemoteSnapshotLvol    string `json:"remoteSnapshotLvol,omitempty" yaml:"remoteSnapshotLvol,omitempty"`
	BackingDevice         string `json:"backingDevice" yaml:"backingDevice"`
	IOUnit                uint64 `json:"ioUnit,omitempty" yaml:"ioUnit,omitempty"`
	TargetStalled         bool   `json:"targetStalled,omitempty" yaml:"targetStalled,omitempty"`
	TargetReason          string `json:"targetReason,omitempty" yaml:"targetReason,omitempty"`
	RemoteNoMemReadCount  int    `json:"remoteNoMemReadCount,omitempty" yaml:"remoteNoMemReadCount,omitempty"`
	RemoteNoMemMaxRetries uint16 `json:"remoteNoMemMaxRetries,omitempty" yaml:"remoteNoMemMaxRetries,omitempty"`
}

// SpdkNodeInfo holds SPDK diagnostics collected on a single node.
type SpdkNodeInfo struct {
	Node      string             `json:"node,omitempty" yaml:"node,omitempty"`
	Processes []*SpdkProcessInfo `json:"processes" yaml:"processes"`
	Warnings  []string           `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Errors    []string           `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// SpdkProcessInfo holds memory, iobuf, blobstore channel, and RPC diagnostics for one spdk_tgt process.
type SpdkProcessInfo struct {
	PID                     int                      `json:"pid" yaml:"pid"`
	Cmdline                 string                   `json:"cmdline,omitempty" yaml:"cmdline,omitempty"`
	Executable              string                   `json:"executable,omitempty" yaml:"executable,omitempty"`
	Version                 string                   `json:"version,omitempty" yaml:"version,omitempty"`
	PerBlobChannelSupported bool                     `json:"perBlobChannelSupported" yaml:"perBlobChannelSupported"`
	DWARFLoaded             bool                     `json:"dwarfLoaded" yaml:"dwarfLoaded"`
	Iobuf                   *SpdkIobufInfo           `json:"iobuf,omitempty" yaml:"iobuf,omitempty"`
	NvmfPollGroups          []*SpdkNvmfPollGroupStat `json:"nvmfPollGroups,omitempty" yaml:"nvmfPollGroups,omitempty"`
	Threads                 []*SpdkThreadInfo        `json:"threads,omitempty" yaml:"threads,omitempty"`
	Warnings                []string                 `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	Errors                  []string                 `json:"errors,omitempty" yaml:"errors,omitempty"`
}

// SpdkIobufInfo holds global g_iobuf pool configuration, live DPDK ring free counts, and RPC module statistics.
type SpdkIobufInfo struct {
	SmallPoolCount uint64                 `json:"smallPoolCount" yaml:"smallPoolCount"`
	SmallBufSize   uint32                 `json:"smallBufSize" yaml:"smallBufSize"`
	SmallRingFree  uint32                 `json:"smallRingFree" yaml:"smallRingFree"`
	LargePoolCount uint64                 `json:"largePoolCount" yaml:"largePoolCount"`
	LargeBufSize   uint32                 `json:"largeBufSize" yaml:"largeBufSize"`
	LargeRingFree  uint32                 `json:"largeRingFree" yaml:"largeRingFree"`
	ModuleStats    []*SpdkIobufModuleStat `json:"moduleStats,omitempty" yaml:"moduleStats,omitempty"`
}

// SpdkIobufModuleStat holds cumulative per-module iobuf allocation statistics from iobuf_get_stats RPC.
type SpdkIobufModuleStat struct {
	Module    string            `json:"module" yaml:"module"`
	SmallPool SpdkIobufPoolStat `json:"smallPool" yaml:"smallPool"`
	LargePool SpdkIobufPoolStat `json:"largePool" yaml:"largePool"`
}

// SpdkIobufPoolStat holds cumulative allocation counters for an iobuf pool.
type SpdkIobufPoolStat struct {
	CacheSize uint32 `json:"cacheSize" yaml:"cacheSize"`
	Cache     uint64 `json:"cache" yaml:"cache"`
	Main      uint64 `json:"main" yaml:"main"`
	Retry     uint64 `json:"retry" yaml:"retry"`
}

// SpdkNvmfPollGroupStat holds NVMe-oF target poll group counters from nvmf_get_stats RPC.
type SpdkNvmfPollGroupStat struct {
	Name               string `json:"name" yaml:"name"`
	CurrentAdminQpairs uint32 `json:"currentAdminQpairs" yaml:"currentAdminQpairs"`
	CurrentIOQpairs    uint32 `json:"currentIOQpairs" yaml:"currentIOQpairs"`
	PendingBdevIO      uint64 `json:"pendingBdevIO" yaml:"pendingBdevIO"`
	CompletedNvmeIO    uint64 `json:"completedNvmeIO" yaml:"completedNvmeIO"`
}

// SpdkThreadInfo holds memory and poller state for an SPDK thread (such as nvmf_tgt_poll_group_000 or app_thread).
type SpdkThreadInfo struct {
	Name                 string               `json:"name" yaml:"name"`
	ID                   uint64               `json:"id" yaml:"id"`
	BdevNoMemPollerCount int                  `json:"bdevNoMemPollerCount" yaml:"bdevNoMemPollerCount"`
	ActivePollers        []string             `json:"activePollers,omitempty" yaml:"activePollers,omitempty"`
	TimedPollerCounts    map[string]int       `json:"timedPollerCounts,omitempty" yaml:"timedPollerCounts,omitempty"`
	TimedPollers         []string             `json:"timedPollers,omitempty" yaml:"timedPollers,omitempty"`
	IobufChannel         *SpdkThreadIobufInfo `json:"iobufChannel,omitempty" yaml:"iobufChannel,omitempty"`
	BlobstoreChannels    []*SpdkBsChannelInfo `json:"blobstoreChannels,omitempty" yaml:"blobstoreChannels,omitempty"`
	BdevNoMemResources   []*SpdkBdevNoMemInfo `json:"bdevNoMemResources,omitempty" yaml:"bdevNoMemResources,omitempty"`
}

// SpdkThreadIobufInfo holds live per-thread iobuf wait queues and per-module lockless cache occupancy.
type SpdkThreadIobufInfo struct {
	SmallWaiters          int                           `json:"smallWaiters" yaml:"smallWaiters"`
	LargeWaiters          int                           `json:"largeWaiters" yaml:"largeWaiters"`
	TotalFreeLargeBuffers uint64                        `json:"totalFreeLargeBuffers,omitempty" yaml:"totalFreeLargeBuffers,omitempty"`
	Modules               []*SpdkThreadIobufModuleCache `json:"modules,omitempty" yaml:"modules,omitempty"`
}

// SpdkThreadIobufModuleCache holds live cache counts and retry counters for one iobuf module on a thread.
type SpdkThreadIobufModuleCache struct {
	Module          string `json:"module" yaml:"module"`
	SmallCacheCount uint32 `json:"smallCacheCount" yaml:"smallCacheCount"`
	SmallCacheSize  uint32 `json:"smallCacheSize" yaml:"smallCacheSize"`
	SmallRetry      uint64 `json:"smallRetry,omitempty" yaml:"smallRetry,omitempty"`
	LargeCacheCount uint32 `json:"largeCacheCount" yaml:"largeCacheCount"`
	LargeCacheSize  uint32 `json:"largeCacheSize" yaml:"largeCacheSize"`
	LargeRetry      uint64 `json:"largeRetry,omitempty" yaml:"largeRetry,omitempty"`
}

// SpdkBsChannelInfo holds live spdk_bs_channel pool and cluster-allocation serialization state.
type SpdkBsChannelInfo struct {
	Device                 string                 `json:"device" yaml:"device"`
	Address                string                 `json:"address" yaml:"address"`
	LvolName               string                 `json:"lvolName,omitempty" yaml:"lvolName,omitempty"`
	BlobID                 string                 `json:"blobID,omitempty" yaml:"blobID,omitempty"`
	TotalReqs              int                    `json:"totalReqs" yaml:"totalReqs"`
	FreeReqs               int                    `json:"freeReqs" yaml:"freeReqs"`
	NeedClusterAlloc       int                    `json:"needClusterAlloc" yaml:"needClusterAlloc"`
	QueuedIO               int                    `json:"queuedIO" yaml:"queuedIO"`
	NeedClusterAllocByLvol map[string]int         `json:"needClusterAllocByLvol,omitempty" yaml:"needClusterAllocByLvol,omitempty"`
	ActiveRequests         []*SpdkBsActiveRequest `json:"activeRequests,omitempty" yaml:"activeRequests,omitempty"`
}

// SpdkBsActiveRequest describes an in-flight spdk_bs_request_set that is neither free nor waiting in need_cluster_alloc.
type SpdkBsActiveRequest struct {
	Index               int    `json:"index" yaml:"index"`
	OpType              string `json:"opType" yaml:"opType"`
	LvolName            string `json:"lvolName,omitempty" yaml:"lvolName,omitempty"`
	BlobID              string `json:"blobID,omitempty" yaml:"blobID,omitempty"`
	IOUnit              uint64 `json:"ioUnit,omitempty" yaml:"ioUnit,omitempty"`
	NewCluster          uint64 `json:"newCluster,omitempty" yaml:"newCluster,omitempty"`
	BackingDevice       string `json:"backingDevice,omitempty" yaml:"backingDevice,omitempty"`
	RemoteTargetAddress string `json:"remoteTargetAddress,omitempty" yaml:"remoteTargetAddress,omitempty"`
	RemoteTargetNode    string `json:"remoteTargetNode,omitempty" yaml:"remoteTargetNode,omitempty"`
}

// SpdkBdevNoMemInfo describes a spdk_bdev_shared_resource with queued nomem_io requests.
type SpdkBdevNoMemInfo struct {
	IOOutstanding  uint64                `json:"ioOutstanding" yaml:"ioOutstanding"`
	NoMemThreshold uint64                `json:"noMemThreshold" yaml:"noMemThreshold"`
	NoMemIOCount   int                   `json:"noMemIOCount" yaml:"noMemIOCount"`
	ByBdev         []*SpdkBdevNoMemEntry `json:"byBdev,omitempty" yaml:"byBdev,omitempty"`
}

// SpdkBdevNoMemEntry summarizes queued nomem_io requests for one bdev.
type SpdkBdevNoMemEntry struct {
	BdevName   string `json:"bdevName" yaml:"bdevName"`
	LvolName   string `json:"lvolName,omitempty" yaml:"lvolName,omitempty"`
	IOType     string `json:"ioType,omitempty" yaml:"ioType,omitempty"`
	Count      int    `json:"count" yaml:"count"`
	MaxRetries uint16 `json:"maxRetries" yaml:"maxRetries"`
}

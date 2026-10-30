// Package dataengine calculates the SPDK iobuf pool sizes and the hugepage memory of the Longhorn V2 data engine.
//
// For each setting it gives two values. "At least" is the smallest value with which spdk_tgt starts and every per-thread
// iobuf cache is filled. "Highest" is the value that covers the theoretical peak, with every queue of every engine full,
// so that the engines can never take all buffers and the replicas always get one, whatever the workload. Both follow from
// the SPDK source (lib/thread/iobuf.c, lib/nvmf/transport.c, lib/nvmf/tcp.c) and were checked against spdk_tgt of
// longhorn/spdk v26.05. The memory is computed from the SPDK source too, with no measured coefficient; the measurements of
// the heap are only used to test it.
//
// The estimate is for one node, with the number of engines and replicas on it.
package dataengine

import (
	"fmt"
	"math"
	"math/big"
	"slices"
	"strings"

	"github.com/pkg/errors"
)

const (
	// Longhorn setting defaults. They are also the minimum values the settings accept.
	LonghornMinLargePool = 1024
	LonghornMinSmallPool = 8192

	// SPDK minimum pool sizes (lib/thread/iobuf.c).
	spdkMinLargePool = 8
	spdkMinSmallPool = 64

	// Per-thread iobuf cache of each of the bdev and the accel module.
	moduleSmallCache = 128
	moduleLargeCache = 16

	// Per-thread iobuf cache of the ublk module (lib/ublk/ublk.c).
	ublkSmallCache = 128
	ublkLargeCache = 32

	// Without an explicit cache size the nvmf transport keeps half of each pool in per-poll-group caches.
	nvmfCacheDivisor = 2

	// A thread cache can hold up to cache size + 31 buffers before it gives a batch back to the shared ring (iobuf.c).
	iobufCacheOverflow = 31

	// Limits seen by the target: queues of one controller and entries of one queue.
	maxQueuesPerController = 127
	maxQueueEntries        = 127

	largeBufferKiB = 132
	smallBufferKiB = 8

	// The memory size is the heap plus this headroom, rounded up.
	memoryHeadroomPercent  = 10
	hugepageGranularityMiB = 128
	poolGranularity        = 1024

	DefaultNrIoQueues = 4
)

// queueCounts are the numbers of I/O queues per volume that the output compares, besides the requested one.
var queueCounts = []int{1, 2, 4, 8}

// Input holds the parameters of the calculation.
type Input struct {
	// Cores is the number of SPDK cores (bit count of data-engine-cpu-mask). With more than one core the lowest one is
	// the admin core and the others host the nvmf poll groups.
	Cores int `json:"cores" yaml:"cores"`

	// Engines is the number of engines on the node, the volumes that are attached to it. The front end of each has
	// nr-io-queues queue pairs on the node.
	Engines int `json:"engines" yaml:"engines"`
	// Replicas is the number of replicas on the node, of any volume. The engine of each connects to its replica with one
	// queue pair per SPDK core.
	Replicas int `json:"replicas" yaml:"replicas"`

	// NrIoQueues is default-nvme-tcp-nr-io-queues. 0 means the kernel default, one queue per host CPU.
	NrIoQueues int `json:"nrIoQueues" yaml:"nrIoQueues"`
	// HostCPUs is the number of CPUs of a node, used only when NrIoQueues is 0.
	HostCPUs int `json:"hostCpus,omitempty" yaml:"hostCpus,omitempty"`

	// UblkCores is the number of cores of the ublk target, 0 when the ublk frontend is not used.
	UblkCores int `json:"ublkCores,omitempty" yaml:"ublkCores,omitempty"`
}

// DefaultInput returns an input with the documented defaults. Cores and the engines or replicas must be set.
func DefaultInput() Input {
	return Input{NrIoQueues: DefaultNrIoQueues}
}

// Derived holds the values computed from the input.
type Derived struct {
	PollGroups      int `json:"pollGroups" yaml:"pollGroups"`
	EnginesPerNode  int `json:"enginesPerNode" yaml:"enginesPerNode"`
	ReplicasPerNode int `json:"replicasPerNode" yaml:"replicasPerNode"`
	QueuesPerVolume int `json:"queuesPerVolume" yaml:"queuesPerVolume"`
	FrontendQpairs  int `json:"frontendQpairs" yaml:"frontendQpairs"`
	BackendQpairs   int `json:"backendQpairs" yaml:"backendQpairs"`
	QpairsOfNode    int `json:"qpairsOfNode" yaml:"qpairsOfNode"`

	// PeakInflight is the most commands the engines of the node can hold at the same time: a queue holds at most 127.
	PeakInflight int `json:"peakInflight" yaml:"peakInflight"`
}

// Step is one line of the calculation, so that the user can follow how a value was obtained.
type Step struct {
	Name    string `json:"name" yaml:"name"`
	Formula string `json:"formula" yaml:"formula"`
	Values  string `json:"values" yaml:"values"`
	Basis   string `json:"basis" yaml:"basis"`
}

// Where a number of a step comes from.
const (
	BasisSource = "SPDK source"
	BasisInput  = "input"
	BasisDesign = "design choice"
)

// PoolSizes is a pair of small and large pool sizes.
type PoolSizes struct {
	Small int `json:"small" yaml:"small"`
	Large int `json:"large" yaml:"large"`
}

// Range is the lowest and the highest value of a setting.
type Range struct {
	AtLeast int `json:"atLeast" yaml:"atLeast"`
	Highest int `json:"highest" yaml:"highest"`
}

// MemoryBreakdown is the estimated hugepage heap of one node in MiB, and the memory size derived from it.
type MemoryBreakdown struct {
	BaseMiB           float64 `json:"baseMiB" yaml:"baseMiB"`
	LargePoolMiB      float64 `json:"largePoolMiB" yaml:"largePoolMiB"`
	SmallPoolMiB      float64 `json:"smallPoolMiB" yaml:"smallPoolMiB"`
	FrontendQpairsMiB float64 `json:"frontendQpairsMiB" yaml:"frontendQpairsMiB"`
	BackendQpairsMiB  float64 `json:"backendQpairsMiB" yaml:"backendQpairsMiB"`
	HeapMiB           float64 `json:"heapMiB" yaml:"heapMiB"`

	// MemoryMiB is the heap plus the headroom, rounded up.
	MemoryMiB int `json:"memoryMiB" yaml:"memoryMiB"`
}

// QueueRow is the highest values for one number of I/O queues per volume.
type QueueRow struct {
	NrIoQueues int  `json:"nrIoQueues" yaml:"nrIoQueues"`
	Requested  bool `json:"requested,omitempty" yaml:"requested,omitempty"`
	LargePool  int  `json:"largePool" yaml:"largePool"`
	SmallPool  int  `json:"smallPool" yaml:"smallPool"`
	MemoryMiB  int  `json:"memoryMiB" yaml:"memoryMiB"`
}

// Result is the output of Calculate.
type Result struct {
	Input   Input   `json:"input" yaml:"input"`
	Derived Derived `json:"derived" yaml:"derived"`

	// CacheFitMinimum is the smallest pool for which every per-thread cache can be populated, with a single nvmf
	// transport, before the minimum of the Longhorn settings is applied.
	CacheFitMinimum PoolSizes `json:"cacheFitMinimum" yaml:"cacheFitMinimum"`

	// The settings. At least: spdk_tgt starts and every cache is filled, but the caches take the pool and I/O stalls under
	// load. Highest: the engines can never take every buffer, whatever the workload.
	LargePool Range `json:"data-engine-iobuf-large-pool-size" yaml:"data-engine-iobuf-large-pool-size"`
	SmallPool Range `json:"data-engine-iobuf-small-pool-size" yaml:"data-engine-iobuf-small-pool-size"`
	MemoryMiB Range `json:"data-engine-memory-size" yaml:"data-engine-memory-size"`

	// UsableLargeAtHighest and UsableSmallAtHighest are the usable buffers of the highest pools.
	UsableLargeAtHighest int `json:"usableLargeBuffersAtHighest" yaml:"usableLargeBuffersAtHighest"`
	UsableSmallAtHighest int `json:"usableSmallBuffersAtHighest" yaml:"usableSmallBuffersAtHighest"`

	MemoryAtLeast MemoryBreakdown `json:"memoryAtLeast" yaml:"memoryAtLeast"`
	MemoryHighest MemoryBreakdown `json:"memoryHighest" yaml:"memoryHighest"`

	// ByNrIoQueues compares the highest values for other numbers of I/O queues per volume.
	ByNrIoQueues []QueueRow `json:"byNrIoQueues" yaml:"byNrIoQueues"`

	Steps    []Step   `json:"steps" yaml:"steps"`
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// ParseCPUMask returns the number of cores selected by a hexadecimal CPU mask such as 0xFF.
func ParseCPUMask(mask string) (int, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(mask), "0x"), "0X")
	if trimmed == "" {
		return 0, errors.Errorf("invalid CPU mask %q", mask)
	}
	value, ok := new(big.Int).SetString(trimmed, 16)
	if !ok || value.Sign() < 0 {
		return 0, errors.Errorf("invalid CPU mask %q: expected a hexadecimal number", mask)
	}
	count := 0
	for i := 0; i < value.BitLen(); i++ {
		if value.Bit(i) == 1 {
			count++
		}
	}
	if count == 0 {
		return 0, errors.Errorf("CPU mask %q selects no core", mask)
	}
	return count, nil
}

// Validate checks the input.
func (in Input) Validate() error {
	if in.Cores < 1 {
		return errors.New("the number of SPDK cores must be at least 1")
	}
	if in.Engines < 0 || in.Replicas < 0 {
		return errors.New("the number of engines and replicas cannot be negative")
	}
	if in.Engines+in.Replicas < 1 {
		return errors.New("set the number of engines or of replicas on the node")
	}
	if in.NrIoQueues < 0 {
		return errors.New("the number of NVMe I/O queues cannot be negative")
	}
	if in.NrIoQueues == 0 && in.HostCPUs < 1 {
		return errors.New("nr-io-queues 0 means one queue per host CPU, so the number of host CPUs is required")
	}
	if in.UblkCores < 0 {
		return errors.New("ublk cores cannot be negative")
	}
	return nil
}

func ceilDiv(a, b int) int {
	return (a + b - 1) / b
}

func roundUp(value, granularity int) int {
	return ceilDiv(value, granularity) * granularity
}

func pollGroups(cores int) int {
	return max(cores-1, 1)
}

// spdkThreads returns the number of SPDK threads that own a bdev and an accel cache. With a single core the admin
// thread and the poll group thread run on the same core, so there are still two of them.
func spdkThreads(cores int) int {
	return max(cores, 2)
}

// cachesFit reports whether the per-thread caches fit in a pool of the given size. The nvmf transport reserves
// floor(floor(pool / 2) / pollGroups) buffers for each poll group, the bdev and the accel module reserve perModule
// buffers for each thread, and the ublk module perUblk buffers for each ublk core.
func cachesFit(pool, cores, perModule, ublkCores, perUblk int) bool {
	groups := pollGroups(cores)
	nvmf := (pool / nvmfCacheDivisor / groups) * groups
	return nvmf+2*perModule*spdkThreads(cores)+perUblk*ublkCores <= pool
}

// cacheFitMinimum returns the smallest pool from which every larger pool also fits. The fit is not monotonic
// because of the rounding of the nvmf cache: with 8 cores a small pool of 4085 fits and 4088 does not.
func cacheFitMinimum(cores, perModule, ublkCores, perUblk, spdkMin int) int {
	fixed := 2*perModule*spdkThreads(cores) + perUblk*ublkCores

	// The nvmf caches take at most half of the pool, so every pool of at least 2 * fixed fits.
	lastFail := 0
	for pool := 2*fixed - 1; pool >= 1; pool-- {
		if !cachesFit(pool, cores, perModule, ublkCores, perUblk) {
			lastFail = pool
			break
		}
	}
	return max(lastFail+1, spdkMin)
}

// usable returns the number of buffers that the busiest poll group can count on when the load is skewed. Of the pool,
// the bdev, accel and ublk caches of every thread are taken at start-up, and so are the nvmf caches of all poll groups.
// A poll group serves its requests from its own cache first and then from the shared ring (lib/thread/iobuf.c:
// spdk_iobuf_get), and buffers only return to the shared ring when its cache is over-full, so the caches of the other
// poll groups cannot be used by the busiest one.
func usable(pool, cores, ublkCores, perModule, perUblk int) int {
	groups := pollGroups(cores)
	nvmfCache := pool / nvmfCacheDivisor / groups
	return pool - 2*perModule*spdkThreads(cores) - perUblk*ublkCores - (groups-1)*nvmfCache
}

func usableLarge(pool, cores, ublkCores int) int {
	return usable(pool, cores, ublkCores, moduleLargeCache, ublkLargeCache)
}

func usableSmall(pool, cores, ublkCores int) int {
	return usable(pool, cores, ublkCores, moduleSmallCache, ublkSmallCache)
}

// peakTarget is the number of usable buffers with which the engines of the node can never take them all, even with every
// queue full: the front end holds at most nr-io-queues x 127 commands per engine, and the caches of the other poll groups
// can keep up to 31 more buffers each than their size.
func peakTarget(d Derived) int {
	return d.PeakInflight + iobufCacheOverflow*(d.PollGroups-1)
}

// smallestPool returns the smallest pool, in steps of step buffers and not below from, whose usable buffers reach target
// and whose caches fit.
func smallestPool(target, cores, ublkCores, perModule, perUblk, step, from int) int {
	pool := roundUp(max(from, 1), step)
	for limit := 8*target + 64*1024; pool < limit; pool += step {
		if usable(pool, cores, ublkCores, perModule, perUblk) >= target && cachesFit(pool, cores, perModule, ublkCores, perUblk) {
			return pool
		}
	}
	return pool
}

// estimateHeap returns the heap for the given pools and derives the memory size from it.
func estimateHeap(in Input, d Derived, largePool, smallPool int) MemoryBreakdown {
	m := MemoryBreakdown{
		BaseMiB:           fixedHeapMiB(in.Cores),
		LargePoolMiB:      poolHeapMiB(largePool, largeBufferKiB),
		SmallPoolMiB:      poolHeapMiB(smallPool, smallBufferKiB),
		FrontendQpairsMiB: frontendHeapMiB(d.EnginesPerNode, d.QueuesPerVolume),
		BackendQpairsMiB:  backendHeapMiB(d.ReplicasPerNode, in.Cores),
	}
	m.HeapMiB = m.BaseMiB + m.LargePoolMiB + m.SmallPoolMiB + m.FrontendQpairsMiB + m.BackendQpairsMiB
	m.MemoryMiB = roundUp(int(math.Ceil(m.HeapMiB*(1+float64(memoryHeadroomPercent)/100))), hugepageGranularityMiB)
	return m
}

// layout derives the engines, replicas and queue pairs of the node for the given number of I/O queues per volume.
func layout(in Input, nrIoQueues int) Derived {
	d := Derived{PollGroups: pollGroups(in.Cores)}

	// The engines and the replicas on the node, as given.
	d.EnginesPerNode = in.Engines
	d.ReplicasPerNode = in.Replicas

	d.QueuesPerVolume = nrIoQueues
	if d.QueuesPerVolume == 0 {
		d.QueuesPerVolume = in.HostCPUs
	}
	d.QueuesPerVolume = min(d.QueuesPerVolume, maxQueuesPerController)
	d.FrontendQpairs = d.EnginesPerNode * d.QueuesPerVolume
	d.BackendQpairs = d.ReplicasPerNode * in.Cores
	d.QpairsOfNode = d.FrontendQpairs + d.BackendQpairs
	d.PeakInflight = d.EnginesPerNode * d.QueuesPerVolume * maxQueueEntries
	return d
}

// highestPools returns the pools that cover the theoretical peak for the given layout, not below floor.
func highestPools(in Input, d Derived, floor PoolSizes) PoolSizes {
	target := peakTarget(d)
	return PoolSizes{
		Small: smallestPool(target, in.Cores, in.UblkCores, moduleSmallCache, ublkSmallCache, poolGranularity, floor.Small),
		Large: smallestPool(target, in.Cores, in.UblkCores, moduleLargeCache, ublkLargeCache, poolGranularity, floor.Large),
	}
}

// Calculate computes the pool sizes and the memory size.
func Calculate(in Input) (*Result, error) {
	if err := in.Validate(); err != nil {
		return nil, err
	}

	d := layout(in, in.NrIoQueues)
	res := &Result{Input: in, Derived: d}

	res.CacheFitMinimum = PoolSizes{
		Small: cacheFitMinimum(in.Cores, moduleSmallCache, in.UblkCores, ublkSmallCache, spdkMinSmallPool),
		Large: cacheFitMinimum(in.Cores, moduleLargeCache, in.UblkCores, ublkLargeCache, spdkMinLargePool),
	}
	atLeast := PoolSizes{
		Small: max(res.CacheFitMinimum.Small, LonghornMinSmallPool),
		Large: max(res.CacheFitMinimum.Large, LonghornMinLargePool),
	}
	highest := highestPools(in, d, atLeast)

	res.LargePool = Range{AtLeast: atLeast.Large, Highest: highest.Large}
	res.SmallPool = Range{AtLeast: atLeast.Small, Highest: highest.Small}
	res.UsableLargeAtHighest = usableLarge(highest.Large, in.Cores, in.UblkCores)
	res.UsableSmallAtHighest = usableSmall(highest.Small, in.Cores, in.UblkCores)

	res.MemoryAtLeast = estimateHeap(in, d, atLeast.Large, atLeast.Small)
	res.MemoryHighest = estimateHeap(in, d, highest.Large, highest.Small)
	res.MemoryMiB = Range{AtLeast: res.MemoryAtLeast.MemoryMiB, Highest: res.MemoryHighest.MemoryMiB}

	queues := slices.Clone(queueCounts)
	if !slices.Contains(queues, d.QueuesPerVolume) {
		queues = append(queues, d.QueuesPerVolume)
		slices.Sort(queues)
	}
	for _, q := range queues {
		dq := layout(in, q)
		pools := highestPools(in, dq, atLeast)
		res.ByNrIoQueues = append(res.ByNrIoQueues, QueueRow{
			NrIoQueues: q,
			Requested:  q == d.QueuesPerVolume,
			LargePool:  pools.Large,
			SmallPool:  pools.Small,
			MemoryMiB:  estimateHeap(in, dq, pools.Large, pools.Small).MemoryMiB,
		})
	}

	res.Steps = buildSteps(res)
	res.Warnings = warnings(res)
	return res, nil
}

func buildSteps(r *Result) []Step {
	in, d := r.Input, r.Derived
	threads := spdkThreads(in.Cores)
	nvmfLarge := r.LargePool.Highest / nvmfCacheDivisor / d.PollGroups
	nvmfSmall := r.SmallPool.Highest / nvmfCacheDivisor / d.PollGroups
	mAt, mHi := r.MemoryAtLeast, r.MemoryHighest

	return []Step{
		{
			Name:    "SPDK threads and poll groups",
			Formula: "poll groups G = cores - 1 (the first core is the admin core); threads T = cores",
			Values:  fmt.Sprintf("G = %d, T = %d", d.PollGroups, threads),
			Basis:   BasisSource,
		},
		{
			Name:    "Engines and replicas on the node",
			Formula: "engines V and replicas R on the node, as given",
			Values:  fmt.Sprintf("V = %d, R = %d", d.EnginesPerNode, d.ReplicasPerNode),
			Basis:   BasisInput,
		},
		{
			Name:    "NVMe/TCP queue pairs on the node",
			Formula: "front end V x nr-io-queues (the kernel initiator opens that many I/O queues per controller); back end R x cores (bdev_nvme opens an I/O qpair for each thread that uses the controller, one thread per core; nvmf_get_stats showed the same count on 4, 8 and 16 cores)",
			Values:  fmt.Sprintf("%d x %d + %d x %d = %d", d.EnginesPerNode, d.QueuesPerVolume, d.ReplicasPerNode, in.Cores, d.QpairsOfNode),
			Basis:   BasisSource,
		},
		{
			Name: "At least: pool that fits every per-thread cache",
			Formula: fmt.Sprintf("smallest P with floor(floor(P / 2) / G) x G + 2 x cache x T + ublk <= P, at least the %d / %d of the Longhorn settings; cache is %d small and %d large "+
				"for each of the bdev and the accel module; the nvmf transport keeps half of each pool in its poll group caches (lib/thread/iobuf.c, lib/nvmf/transport.c). "+
				"At this value spdk_tgt starts, but the caches take the pool and I/O stalls under load", LonghornMinSmallPool, LonghornMinLargePool, moduleSmallCache, moduleLargeCache),
			Values: fmt.Sprintf("small %d (cache fit %d), large %d (cache fit %d)", r.SmallPool.AtLeast, r.CacheFitMinimum.Small, r.LargePool.AtLeast, r.CacheFitMinimum.Large),
			Basis:  BasisSource,
		},
		{
			Name: "Theoretical peak of commands holding a buffer",
			Formula: fmt.Sprintf("a write takes one buffer when it starts and keeps it until all replicas acknowledged it (lib/nvmf/tcp.c, lib/nvmf/transport.c): a %d KiB large buffer, or a %d KiB small buffer "+
				"when the command is at most that long; a queue holds at most %d commands, so F = V x nr-io-queues x %d, whatever the workload", largeBufferKiB, smallBufferKiB, maxQueueEntries, maxQueueEntries),
			Values: fmt.Sprintf("F = %d x %d x %d = %d", d.EnginesPerNode, d.QueuesPerVolume, maxQueueEntries, d.PeakInflight),
			Basis:  BasisSource,
		},
		{
			Name: "Usable buffers of a pool",
			Formula: "U(P) = P - 2 x cache x T - ublk - (G - 1) x floor(floor(P / 2) / G): the busiest poll group cannot use the caches of the other poll groups " +
				"(lib/thread/iobuf.c: a freed buffer goes back to the ring only when a cache is 32 over its size)",
			Values: fmt.Sprintf("large U(%d) = %d - %d - %d x %d = %d; small U(%d) = %d - %d - %d x %d = %d",
				r.LargePool.Highest, r.LargePool.Highest, 2*moduleLargeCache*threads+ublkLargeCache*in.UblkCores, d.PollGroups-1, nvmfLarge, r.UsableLargeAtHighest,
				r.SmallPool.Highest, r.SmallPool.Highest, 2*moduleSmallCache*threads+ublkSmallCache*in.UblkCores, d.PollGroups-1, nvmfSmall, r.UsableSmallAtHighest),
			Basis: BasisSource,
		},
		{
			Name: "Highest: pools for the theoretical peak",
			Formula: fmt.Sprintf("smallest multiple of %d with U(P) >= F + %d x (G - 1), for the large and for the small pool: with it the engines can never take every buffer, "+
				"so the replicas always get one and every write can finish, whatever the workload; %d is how far a poll group cache can exceed its size",
				poolGranularity, iobufCacheOverflow, iobufCacheOverflow),
			Values: fmt.Sprintf("U(P) >= %d + %d x %d = %d  ->  large %d, small %d", d.PeakInflight, iobufCacheOverflow, d.PollGroups-1, peakTarget(d), r.LargePool.Highest, r.SmallPool.Highest),
			Basis:  BasisSource,
		},
		{
			Name: "Fixed hugepage memory",
			Formula: fmt.Sprintf("object pools: bdev_io %d x %d B + message %d x %d B + fsdev_io %d x %d B + event %d x %d B (+ %d lcore caches); each object is rounded as rte_mempool does "+
				"(64 B header, cache line, count of lines coprime with 4 memory channels) and each pool has a ring of the next power of two above its count; "+
				"2 rings of %d entries per core (the event ring of the reactor, the message ring of the thread); %d buffers of 1 MiB (bdev zero buffer, FTL write and read buffers); "+
				"sizeof: spdk_bdev_io 1216 + the largest driver context ftl_io 280, spdk_msg 24, spdk_fsdev_io 368 + aio_fsdev_io 32, spdk_event 32 "+
				"(lib/bdev/bdev.c, lib/thread/thread.c, lib/fsdev/fsdev.c, lib/event/reactor.c, lib/ftl/ftl_core.c)",
				bdevIoPoolCount, mempoolObjectBytes(bdevIoBytes), msgPoolCount, mempoolObjectBytes(msgBytes), fsdevIoPoolCount, mempoolObjectBytes(fsdevIoBytes),
				eventPoolCount, mempoolObjectBytes(eventBytes), maxLcores, coreRingEntries, oneMiBBuffers),
			Values: fmt.Sprintf("bdev_io %.1f + message %.1f + fsdev_io %.1f + event %.1f + rings %.1f + buffers %d = %.1f MiB",
				float64(mempoolBytes(bdevIoPoolCount, bdevIoBytes))/mib, float64(mempoolBytes(msgPoolCount, msgBytes))/mib, float64(mempoolBytes(fsdevIoPoolCount, fsdevIoBytes))/mib,
				float64(mempoolBytes(eventPoolCount, eventBytes)+192+maxLcores*mempoolCacheBytes)/mib, float64(in.Cores*ringsPerCore*ringBytes(coreRingEntries))/mib, oneMiBBuffers, mAt.BaseMiB),
			Basis: BasisSource,
		},
		{
			Name: "Hugepage memory of the queue pairs",
			Formula: fmt.Sprintf("target side of a queue pair: %d entries x %d B in-capsule buffer + (2 x %d + 1) PDUs x %d B (lib/nvmf/tcp.c nvmf_tcp_qpair_init_mem_resource); "+
				"initiator side: (%d + 2) PDUs x %d B + %d requests x %d B (lib/nvme/nvme_tcp.c nvme_tcp_alloc_reqs, nvme_qpair.c); the admin queue has %d entries. "+
				"Front end: V x (nr-io-queues + 1 admin) target queue pairs. Back end: R x (cores + 1) target queue pairs plus R x (cores I/O + 1 admin) initiator queue pairs, "+
				"because every replica is connected to one engine. The request arrays of the queue pairs are calloc memory, not hugepages",
				queueEntries, inCapsuleBytes, queueEntries, pduBytes, queueEntries, pduBytes, initiatorRequests, roundUp(nvmeRequestBytes, cacheLineBytes), adminQueueEntries),
			Values: fmt.Sprintf("target %.0f KiB, initiator I/O %.0f KiB, initiator admin %.0f KiB; front end %.0f MiB; back end %.0f MiB",
				float64(targetQpairBytes())/1024, float64(initiatorQpairBytes(queueEntries, initiatorRequests))/1024, float64(initiatorQpairBytes(adminQueueEntries, adminQueueEntries))/1024,
				mAt.FrontendQpairsMiB, mAt.BackendQpairsMiB),
			Basis: BasisSource,
		},
		{
			Name: "Hugepage heap",
			Formula: fmt.Sprintf("fixed + large pool (pool x %d KiB + ring) + small pool (pool x %d KiB + ring) + front end + back end [SPDK source]. Not counted, about 3 MiB at idle: four allocations of 0.8 MiB "+
				"that were not traced to the source; the headroom covers them. Blob cluster arrays use realloc (lib/blob/blobstore.c), not hugepages, so the size and the snapshots of the volumes do not matter",
				largeBufferKiB, smallBufferKiB),
			Values: fmt.Sprintf("at least %.0f + %.0f + %.0f + %.0f + %.0f = %.0f MiB; highest %.0f + %.0f + %.0f + %.0f + %.0f = %.0f MiB",
				mAt.BaseMiB, mAt.LargePoolMiB, mAt.SmallPoolMiB, mAt.FrontendQpairsMiB, mAt.BackendQpairsMiB, mAt.HeapMiB,
				mHi.BaseMiB, mHi.LargePoolMiB, mHi.SmallPoolMiB, mHi.FrontendQpairsMiB, mHi.BackendQpairsMiB, mHi.HeapMiB),
			Basis: BasisSource,
		},
		{
			Name:    "Memory size",
			Formula: fmt.Sprintf("roundup(heap x (1 + %d%% headroom), %d MiB)", memoryHeadroomPercent, hugepageGranularityMiB),
			Values:  fmt.Sprintf("at least %.0f x %.2f -> %d MiB; highest %.0f x %.2f -> %d MiB", mAt.HeapMiB, 1+float64(memoryHeadroomPercent)/100, mAt.MemoryMiB, mHi.HeapMiB, 1+float64(memoryHeadroomPercent)/100, mHi.MemoryMiB),
			Basis:   BasisDesign,
		},
	}
}

func warnings(r *Result) []string {
	var w []string
	if r.Input.NrIoQueues == 0 {
		w = append(w, "nr-io-queues 0 lets the kernel open one queue per host CPU, which multiplies the front-end qpairs and the peak. Consider a small explicit value.")
	}
	if r.Input.UblkCores > 0 {
		w = append(w, "The ublk caches (128 small and 32 large buffers per ublk core) come from the SPDK source and were not measured.")
	}
	w = append(w,
		"Between at least and highest the result depends on the workload, which the command does not know: at least is where spdk_tgt starts but I/O stalls under load, highest is where no workload can exhaust the pools. Use the highest values when the memory allows.",
		"The highest values grow with nr-io-queues (engines x queues x 127), so a smaller default-nvme-tcp-nr-io-queues needs less memory; see the table. The effect of fewer queues on the throughput of a single volume was not measured.",
		"The estimate assumes a single NVMe/TCP transport. Two transports reserve twice the caches and make spdk_tgt fail at any pool size.",
		"The memory is computed from the SPDK source. On 12 configurations (4 to 16 cores, 16 to 50 replicas, 1 to 4 queues) it is up to 1% below the heap that spdk_tgt reported, about 3 MiB of it allocations that are not traced to the source; the headroom covers it.")
	return w
}

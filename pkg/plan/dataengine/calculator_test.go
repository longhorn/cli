package dataengine

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// busiestNode is the node with the most engines (17) of the three-node cluster with 50 volumes and 3 replicas used in the
// experiments: it held 17 engines and 50 replicas.
func busiestNode() Input {
	in := DefaultInput()
	in.Cores = 8
	in.Engines = 17
	in.Replicas = 50
	return in
}

func TestParseCPUMask(t *testing.T) {
	for _, test := range []struct {
		mask     string
		expected int
		wantErr  bool
	}{
		{mask: "0xFF", expected: 8},
		{mask: "FF", expected: 8},
		{mask: "0x3", expected: 2},
		{mask: "0x1", expected: 1},
		{mask: "0xFF0000000000", expected: 8},
		{mask: "0x1FFFFFFFFFFFFFFFFF", expected: 69},
		{mask: "0x0", wantErr: true},
		{mask: "0x", wantErr: true},
		{mask: "", wantErr: true},
		{mask: "0xZZ", wantErr: true},
	} {
		t.Run(test.mask, func(t *testing.T) {
			got, err := ParseCPUMask(test.mask)
			if test.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.expected, got)
		})
	}
}

// Every boundary below was checked against spdk_tgt of longhorn/spdk v26.05: the minimum and every larger pool in the
// swept window pass, and the minimum minus one fails (small and large pools, window of 8 on each side of the minimum).
func TestCacheFitMinimum(t *testing.T) {
	for _, test := range []struct {
		cores        int
		small, large int
	}{
		{cores: 1, small: 1023, large: 127},
		{cores: 2, small: 1023, large: 127},
		{cores: 3, small: 1534, large: 190},
		{cores: 4, small: 2047, large: 254},
		{cores: 5, small: 2556, large: 316},
		{cores: 6, small: 3071, large: 382},
		{cores: 7, small: 3580, large: 446},
		{cores: 8, small: 4092, large: 508},
		{cores: 9, small: 4600, large: 568},
		{cores: 10, small: 5116, large: 635},
		{cores: 11, small: 5626, large: 702},
		{cores: 12, small: 6141, large: 758},
		{cores: 13, small: 6652, large: 824},
		{cores: 14, small: 7159, large: 890},
		{cores: 15, small: 7676, large: 956},
		{cores: 16, small: 8191, large: 1022},
		{cores: 17, small: 8688, large: 1072},
		{cores: 20, small: 10231, large: 1267},
	} {
		in := DefaultInput()
		in.Cores = test.cores
		in.Engines = 1
		in.Replicas = 1
		r, err := Calculate(in)
		require.NoError(t, err)
		assert.Equal(t, test.small, r.CacheFitMinimum.Small, "small, %d cores", test.cores)
		assert.Equal(t, test.large, r.CacheFitMinimum.Large, "large, %d cores", test.cores)
	}
}

func TestCachesFitIsNotMonotonic(t *testing.T) {
	// With 8 cores 4085 fits, 4088 to 4091 do not, 4092 fits (checked against spdk_tgt).
	assert.True(t, cachesFit(4085, 8, moduleSmallCache, 0, 0))
	assert.False(t, cachesFit(4088, 8, moduleSmallCache, 0, 0))
	assert.False(t, cachesFit(4090, 8, moduleSmallCache, 0, 0))
	assert.True(t, cachesFit(4092, 8, moduleSmallCache, 0, 0))

	assert.True(t, cachesFit(501, 8, moduleLargeCache, 0, 0))
	assert.False(t, cachesFit(504, 8, moduleLargeCache, 0, 0))
	assert.True(t, cachesFit(508, 8, moduleLargeCache, 0, 0))
}

func TestDefaultPoolsFitUpToSixteenCores(t *testing.T) {
	for cores := 2; cores <= 16; cores++ {
		assert.True(t, cachesFit(LonghornMinSmallPool, cores, moduleSmallCache, 0, 0), "small, %d cores", cores)
		assert.True(t, cachesFit(LonghornMinLargePool, cores, moduleLargeCache, 0, 0), "large, %d cores", cores)
	}
	assert.False(t, cachesFit(LonghornMinSmallPool, 17, moduleSmallCache, 0, 0))
	assert.False(t, cachesFit(LonghornMinLargePool, 17, moduleLargeCache, 0, 0))
}

func TestUblkRaisesTheMinimum(t *testing.T) {
	base := busiestNode()
	withUblk := base
	withUblk.UblkCores = 7

	a, err := Calculate(base)
	require.NoError(t, err)
	b, err := Calculate(withUblk)
	require.NoError(t, err)

	assert.Greater(t, b.CacheFitMinimum.Small, a.CacheFitMinimum.Small)
	assert.Greater(t, b.CacheFitMinimum.Large, a.CacheFitMinimum.Large)
	assert.Contains(t, strings.Join(b.Warnings, "\n"), "ublk caches")
	assert.NotContains(t, strings.Join(a.Warnings, "\n"), "ublk caches")
}

func TestAtLeastIsTheCacheFitMinimumOrTheLonghornMinimum(t *testing.T) {
	for _, test := range []struct {
		cores        int
		small, large int
	}{
		{cores: 1, small: 8192, large: 1024},
		{cores: 8, small: 8192, large: 1024},
		{cores: 16, small: 8192, large: 1024},
		{cores: 17, small: 8688, large: 1072},
		{cores: 20, small: 10231, large: 1267},
	} {
		in := DefaultInput()
		in.Cores = test.cores
		in.Engines = 1
		in.Replicas = 1
		r, err := Calculate(in)
		require.NoError(t, err)
		assert.Equal(t, test.small, r.SmallPool.AtLeast, "small, %d cores", test.cores)
		assert.Equal(t, test.large, r.LargePool.AtLeast, "large, %d cores", test.cores)
	}
}

// usableLarge against the runs on 17 engines with 4 queues, 1 MiB writes on every volume: with U above the peak of about
// 2200 buffers in flight the runs had almost no waits; at 0.55 of it they still ran, at 0.51 they timed out under 4 jobs,
// and at 0.41 and less they degraded or stopped.
func TestUsableLargeAgainstMeasuredRuns(t *testing.T) {
	for _, test := range []struct {
		cores, pool, usable int
		outcome             string
	}{
		{cores: 8, pool: 5120, usable: 2674, outcome: "111 waits, ran"},
		{cores: 8, pool: 3072, usable: 1502, outcome: "1.5 M waits, ran"},
		{cores: 8, pool: 2560, usable: 1212, outcome: "2.9 M waits, ran"},
		{cores: 8, pool: 2048, usable: 916, outcome: "630 kernel NVMe timeouts"},
		{cores: 8, pool: 1024, usable: 330, outcome: "stopped"},
		{cores: 4, pool: 5120, usable: 3286, outcome: "0 waits, ran"},
		{cores: 4, pool: 3072, usable: 1920, outcome: "1.0 M waits, ran"},
		{cores: 4, pool: 2048, usable: 1238, outcome: "1.8 M waits, ran"},
		{cores: 16, pool: 3072, usable: 1132, outcome: "timeouts under 4 jobs"},
	} {
		assert.Equal(t, test.usable, usableLarge(test.pool, test.cores, 0), "%d cores, pool %d (%s)", test.cores, test.pool, test.outcome)
	}
}

func TestPoolSearchReturnsTheSmallestPool(t *testing.T) {
	for _, per := range []int{moduleLargeCache, moduleSmallCache} {
		for _, cores := range []int{1, 2, 4, 8, 16, 24} {
			for _, target := range []int{100, 500, 2345, 8822} {
				pool := smallestPool(target, cores, 0, per, 0, 1, 1)
				assert.GreaterOrEqual(t, usable(pool, cores, 0, per, 0), target, "%d cores, target %d", cores, target)
				for smaller := 1; smaller < pool; smaller++ {
					if usable(smaller, cores, 0, per, 0) >= target && cachesFit(smaller, cores, per, 0, 0) {
						t.Fatalf("per-module cache %d, %d cores, target %d: pool %d is enough but %d was returned", per, cores, target, smaller, pool)
					}
				}
			}
		}
	}
}

// 17 engines with 8 cores. The expected pools were also computed independently: the peak is engines x queues x 127 and
// the target adds 31 for each of the 6 other poll group caches.
func TestHighestCoversThePeak(t *testing.T) {
	for _, test := range []struct {
		queues       int
		peak, target int
		large, small int
	}{
		{queues: 1, peak: 2159, target: 2345, large: 5120, small: 8192},
		{queues: 2, peak: 4318, target: 4504, large: 9216, small: 12288},
		{queues: 4, peak: 8636, target: 8822, large: 16384, small: 19456},
	} {
		in := busiestNode()
		in.NrIoQueues = test.queues

		r, err := Calculate(in)
		require.NoError(t, err)

		assert.Equal(t, test.peak, r.Derived.PeakInflight, "%d queues", test.queues)
		assert.Equal(t, test.target, peakTarget(r.Derived), "%d queues", test.queues)
		assert.Equal(t, test.large, r.LargePool.Highest, "large, %d queues", test.queues)
		assert.Equal(t, test.small, r.SmallPool.Highest, "small, %d queues", test.queues)
		assert.GreaterOrEqual(t, r.UsableLargeAtHighest, test.target)
		assert.GreaterOrEqual(t, r.UsableSmallAtHighest, test.target)
	}
}

func TestHighestIsTheSmallestMultipleOfTheGranularity(t *testing.T) {
	for _, cores := range []int{2, 4, 8, 16} {
		for _, queues := range []int{1, 2, 4} {
			in := busiestNode()
			in.Cores, in.NrIoQueues = cores, queues
			r, err := Calculate(in)
			require.NoError(t, err)
			target := peakTarget(r.Derived)

			assert.Zero(t, r.LargePool.Highest%poolGranularity)
			assert.Zero(t, r.SmallPool.Highest%poolGranularity)
			assert.GreaterOrEqual(t, r.LargePool.Highest, r.LargePool.AtLeast)
			assert.GreaterOrEqual(t, r.SmallPool.Highest, r.SmallPool.AtLeast)

			for pool := r.LargePool.AtLeast; pool < r.LargePool.Highest; pool++ {
				if pool%poolGranularity == 0 && usableLarge(pool, cores, 0) >= target && cachesFit(pool, cores, moduleLargeCache, 0, 0) {
					t.Fatalf("%d cores, %d queues: large pool %d covers the peak but %d was returned", cores, queues, pool, r.LargePool.Highest)
				}
			}
			for pool := r.SmallPool.AtLeast; pool < r.SmallPool.Highest; pool++ {
				if pool%poolGranularity == 0 && usableSmall(pool, cores, 0) >= target && cachesFit(pool, cores, moduleSmallCache, 0, 0) {
					t.Fatalf("%d cores, %d queues: small pool %d covers the peak but %d was returned", cores, queues, pool, r.SmallPool.Highest)
				}
			}
		}
	}
}

func TestMemorySizes(t *testing.T) {
	r, err := Calculate(busiestNode())
	require.NoError(t, err)

	// 17 engines with 4 queues and 50 replicas on 8 cores: 17 x 4 + 50 x 8 queue pairs, the 468 measured on that node.
	assert.Equal(t, 468, r.Derived.QpairsOfNode)
	assert.InDelta(t, 937.6, r.MemoryAtLeast.HeapMiB, 0.1)
	assert.InDelta(t, 3006.0, r.MemoryHighest.HeapMiB, 0.1)
	assert.Equal(t, Range{AtLeast: 1152, Highest: 3328}, r.MemoryMiB)
	assert.Equal(t, Range{AtLeast: 1024, Highest: 16384}, r.LargePool)
	assert.Equal(t, Range{AtLeast: 8192, Highest: 19456}, r.SmallPool)
	assert.Zero(t, r.MemoryAtLeast.MemoryMiB%hugepageGranularityMiB)
	assert.Zero(t, r.MemoryHighest.MemoryMiB%hugepageGranularityMiB)
	assert.GreaterOrEqual(t, float64(r.MemoryHighest.MemoryMiB), r.MemoryHighest.HeapMiB*1.1)
}

func TestTheOutputComparesNumbersOfQueues(t *testing.T) {
	r, err := Calculate(busiestNode())
	require.NoError(t, err)

	require.Len(t, r.ByNrIoQueues, 4)
	for i, want := range []QueueRow{
		{NrIoQueues: 1, LargePool: 5120, SmallPool: 8192, MemoryMiB: 1664},
		{NrIoQueues: 2, LargePool: 9216, SmallPool: 12288, MemoryMiB: 2304},
		{NrIoQueues: 4, Requested: true, LargePool: 16384, SmallPool: 19456, MemoryMiB: 3328},
		{NrIoQueues: 8, LargePool: 31744, SmallPool: 34816, MemoryMiB: 5760},
	} {
		assert.Equal(t, want, r.ByNrIoQueues[i])
	}

	// The requested value is added in order when it is not one of the usual ones.
	in := busiestNode()
	in.NrIoQueues = 3
	r, err = Calculate(in)
	require.NoError(t, err)
	var queues []int
	for _, row := range r.ByNrIoQueues {
		queues = append(queues, row.NrIoQueues)
		assert.Equal(t, row.NrIoQueues == 3, row.Requested)
	}
	assert.Equal(t, []int{1, 2, 3, 4, 8}, queues)
}

func TestHighestGrowsWithTheLoad(t *testing.T) {
	var previous Result
	for _, engines := range []int{1, 3, 10, 17, 25, 50, 100} {
		in := busiestNode()
		in.Engines, in.Replicas, in.NrIoQueues = engines, 3*engines, 2
		r, err := Calculate(in)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, r.LargePool.Highest, previous.LargePool.Highest, "%d engines", engines)
		assert.GreaterOrEqual(t, r.SmallPool.Highest, previous.SmallPool.Highest, "%d engines", engines)
		assert.GreaterOrEqual(t, r.MemoryMiB.Highest, previous.MemoryMiB.Highest, "%d engines", engines)
		previous = *r
	}

	previous = Result{}
	for _, cores := range []int{2, 4, 8, 12, 16, 24} {
		in := busiestNode()
		in.Cores, in.NrIoQueues = cores, 2
		r, err := Calculate(in)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, r.LargePool.Highest, previous.LargePool.Highest, "%d cores", cores)
		assert.GreaterOrEqual(t, r.SmallPool.Highest, previous.SmallPool.Highest, "%d cores", cores)
		assert.GreaterOrEqual(t, r.MemoryMiB.Highest, previous.MemoryMiB.Highest, "%d cores", cores)
		previous = *r
	}
}

func TestEnginesAndReplicasAreCountedSeparately(t *testing.T) {
	in := DefaultInput()
	in.Cores = 8
	in.Engines = 10
	in.Replicas = 30
	in.NrIoQueues = 1

	r, err := Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, 10, r.Derived.EnginesPerNode)
	assert.Equal(t, 30, r.Derived.ReplicasPerNode)
	assert.Equal(t, 10*127, r.Derived.PeakInflight)
	assert.Equal(t, 10+30*8, r.Derived.QpairsOfNode)

	// Replicas only enter the queue pairs of the back end, and so the memory, not the pools.
	in.Replicas = 60
	more, err := Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, r.LargePool, more.LargePool)
	assert.Equal(t, r.SmallPool, more.SmallPool)
	assert.Equal(t, 10+60*8, more.Derived.QpairsOfNode)
	assert.Greater(t, more.MemoryHighest.HeapMiB, r.MemoryHighest.HeapMiB)
	assert.InDelta(t, backendHeapMiB(60, 8)-backendHeapMiB(30, 8), more.MemoryHighest.HeapMiB-r.MemoryHighest.HeapMiB, 0.01)
}

func TestANodeWithOnlyReplicasOrOnlyEngines(t *testing.T) {
	in := DefaultInput()
	in.Cores = 8

	in.Replicas = 20
	r, err := Calculate(in)
	require.NoError(t, err)
	assert.Zero(t, r.Derived.PeakInflight)
	assert.Equal(t, 20*8, r.Derived.QpairsOfNode)
	assert.Equal(t, r.LargePool.AtLeast, r.LargePool.Highest)

	in.Replicas = 0
	in.Engines = 5
	r, err = Calculate(in)
	require.NoError(t, err)
	assert.Equal(t, 5*4, r.Derived.QpairsOfNode)
	assert.Equal(t, 5*4*127, r.Derived.PeakInflight)
}

func TestKernelDefaultQueuesUseTheHostCPUs(t *testing.T) {
	in := busiestNode()
	in.NrIoQueues = 0
	in.HostCPUs = 64

	d := layout(in, in.NrIoQueues)
	assert.Equal(t, 64, d.QueuesPerVolume)
	assert.Equal(t, 17*64, d.FrontendQpairs)

	in.HostCPUs = 200
	assert.Equal(t, maxQueuesPerController, layout(in, in.NrIoQueues).QueuesPerVolume)

	r, err := Calculate(in)
	require.NoError(t, err)
	assert.Contains(t, strings.Join(r.Warnings, "\n"), "nr-io-queues 0")
}

// The derived sizes of the object pools against the memory zones that spdk_tgt of longhorn/spdk v26.05 reports
// (env_dpdk_get_mem_stats, 8 cores): the zone of the objects and the ring of free objects.
func TestMempoolsAgainstTheMemoryZones(t *testing.T) {
	assert.Equal(t, 1600, mempoolObjectBytes(bdevIoBytes))
	assert.Equal(t, 192, mempoolObjectBytes(msgBytes))
	assert.Equal(t, 576, mempoolObjectBytes(fsdevIoBytes))
	assert.Equal(t, 192, mempoolObjectBytes(eventBytes))

	// Zone sizes in bytes: 104915200 + 524672, 50334720 + 2097536, 37757952 + 524672. The zone of the objects is a few
	// dozen objects larger than count x size, which is under 0.1 MiB.
	assert.InDelta(t, 104915200+524672, mempoolBytes(bdevIoPoolCount, bdevIoBytes), 64*1024)
	assert.InDelta(t, 50334720+2097536, mempoolBytes(msgPoolCount, msgBytes), 64*1024)
	assert.InDelta(t, 37757952+524672, mempoolBytes(fsdevIoPoolCount, fsdevIoBytes), 64*1024)

	// The memory zones of the rings: the iobuf pools (8192 and 5120 buffers) and the rings of 65536 entries.
	assert.Equal(t, 131456, ringBytes(8192))
	assert.Equal(t, 65920, ringBytes(5120))
	assert.Equal(t, 1048960, ringBytes(coreRingEntries))
	assert.Equal(t, 1056960, 192+maxLcores*mempoolCacheBytes)
}

// The heap that spdk_tgt allocated in the hugepages (env_dpdk_get_mem_stats), measured on 3 nodes with 50 volumes of 6
// GiB: 12 measurements on 4, 8 and 16 cores, with 16 to 50 replicas and 16 or 17 engines on the node, 1 to 4 queues per
// volume, large pools of 2048 to 9216 and small pools of 8192 to 16384. The model is computed from the SPDK source, with
// nothing fitted on them. It is below the measurements by up to 1%: about 3 MiB of allocations at idle are not
// traced to the source, and the rest is the allocator rounding of the queue pair buffers.
func TestHeapAgainstMeasurements(t *testing.T) {
	for _, test := range []struct {
		cores, engines, replicas, queues, large, small int
		measured                                       float64
	}{
		{cores: 8, engines: 17, replicas: 50, queues: 4, large: 4096, small: 8192, measured: 1339},
		{cores: 8, engines: 17, replicas: 50, queues: 4, large: 8192, small: 8192, measured: 1867},
		{cores: 8, engines: 17, replicas: 50, queues: 4, large: 5120, small: 8192, measured: 1471},
		{cores: 8, engines: 17, replicas: 50, queues: 1, large: 2048, small: 8192, measured: 1036},
		{cores: 8, engines: 17, replicas: 50, queues: 4, large: 2048, small: 8192, measured: 1075},
		{cores: 4, engines: 17, replicas: 50, queues: 4, large: 5120, small: 8192, measured: 1250},
		{cores: 16, engines: 17, replicas: 50, queues: 4, large: 5120, small: 16384, measured: 1979},
		{cores: 8, engines: 17, replicas: 50, queues: 2, large: 9216, small: 8192, measured: 1973},
		{cores: 8, engines: 17, replicas: 50, queues: 2, large: 9216, small: 12288, measured: 2005},
		{cores: 8, engines: 17, replicas: 16, queues: 4, large: 5120, small: 8192, measured: 1156},
		{cores: 8, engines: 17, replicas: 18, queues: 4, large: 5120, small: 8192, measured: 1169},
		{cores: 8, engines: 16, replicas: 16, queues: 4, large: 5120, small: 8192, measured: 1149},
		// The idle instance manager, with no volume, on 2, 4, 8 and 16 cores.
		{cores: 2, engines: 0, replicas: 0, queues: 4, large: 5120, small: 8192, measured: 925.6},
		{cores: 4, engines: 0, replicas: 0, queues: 4, large: 5120, small: 8192, measured: 929.6},
		{cores: 8, engines: 0, replicas: 0, queues: 4, large: 5120, small: 8192, measured: 937.6},
		{cores: 16, engines: 0, replicas: 0, queues: 4, large: 5120, small: 8192, measured: 953.6},
	} {
		in := DefaultInput()
		in.Cores, in.Engines, in.Replicas = test.cores, test.engines, test.replicas

		heap := estimateHeap(in, layout(in, test.queues), test.large, test.small).HeapMiB
		assert.Less(t, heap, test.measured, "%d cores, %d engines, %d replicas", test.cores, test.engines, test.replicas)
		assert.InDelta(t, test.measured, heap, 0.012*test.measured,
			"%d cores, %d engines, %d replicas, %d queues, large %d, small %d", test.cores, test.engines, test.replicas, test.queues, test.large, test.small)
	}
}

func TestQueuePairMemoryFollowsTheStructures(t *testing.T) {
	// 128 in-capsule buffers of 4096 B and 257 PDUs of 1088 B rounded to a page.
	assert.Equal(t, 128*4096+282624, targetQpairBytes())
	// 130 PDUs rounded to a page and 512 requests of 320 B; the admin queue has 34 PDUs and 32 requests.
	assert.Equal(t, 143360+512*320, initiatorQpairBytes(queueEntries, initiatorRequests))
	assert.Equal(t, 40960+32*320, initiatorQpairBytes(adminQueueEntries, adminQueueEntries))

	// An engine has its I/O queues and an admin queue on the target, a replica one queue per core and an admin queue.
	assert.InDelta(t, float64(17*5*targetQpairBytes())/mib, frontendHeapMiB(17, 4), 0.001)
	assert.Greater(t, backendHeapMiB(50, 8), frontendHeapMiB(17, 4))
}

func TestVolumeSizeAndSnapshotsDoNotChangeTheMemory(t *testing.T) {
	// Blob cluster arrays use realloc, not hugepages, so the memory has no term for them.
	r, err := Calculate(busiestNode())
	require.NoError(t, err)
	assert.InDelta(t, r.MemoryHighest.HeapMiB,
		r.MemoryHighest.BaseMiB+r.MemoryHighest.LargePoolMiB+r.MemoryHighest.SmallPoolMiB+r.MemoryHighest.FrontendQpairsMiB+r.MemoryHighest.BackendQpairsMiB, 0.001)
}

func TestValidate(t *testing.T) {
	for name, mutate := range map[string]func(*Input){
		"no cores":                func(in *Input) { in.Cores = 0 },
		"no engines and replicas": func(in *Input) { in.Engines = 0; in.Replicas = 0 },
		"negative engines":        func(in *Input) { in.Engines = -1 },
		"negative replicas":       func(in *Input) { in.Replicas = -1 },
		"negative queues":         func(in *Input) { in.NrIoQueues = -1 },
		"kernel queues, no CPUs":  func(in *Input) { in.NrIoQueues = 0; in.HostCPUs = 0 },
		"negative ublk cores":     func(in *Input) { in.UblkCores = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			in := busiestNode()
			mutate(&in)
			_, err := Calculate(in)
			assert.Error(t, err)
		})
	}

	in := DefaultInput()
	in.Cores = 8
	in.Engines = 1
	_, err := Calculate(in)
	assert.NoError(t, err, "a node with an engine and no replica")
}

func TestRender(t *testing.T) {
	r, err := Calculate(busiestNode())
	require.NoError(t, err)

	var text bytes.Buffer
	require.NoError(t, Render(&text, r, OutputFormatText))
	for _, want := range []string{
		"at least    highest",
		"Engines / replicas:         17 / 50",
		"data-engine-iobuf-large-pool-size       1024      16384",
		"data-engine-iobuf-small-pool-size       8192      19456",
		"data-engine-memory-size (MiB)           1152       3328",
		"8636 commands (17 engines x 4 queues x 127)",
		"queues  large pool  small pool  memory (MiB)",
		"<- requested",
		"Calculation",
		"Notes",
	} {
		assert.Contains(t, text.String(), want)
	}
	for _, unwanted := range []string{"budget", "headroom instead", "lowered", "one replica"} {
		assert.NotContains(t, text.String(), unwanted)
	}

	var jsonOut bytes.Buffer
	require.NoError(t, Render(&jsonOut, r, OutputFormatJSON))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &decoded))
	large, ok := decoded["data-engine-iobuf-large-pool-size"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 1024, large["atLeast"])
	assert.EqualValues(t, 16384, large["highest"])
	memory, ok := decoded["data-engine-memory-size"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 3328, memory["highest"])
	assert.NotEmpty(t, decoded["steps"])
	assert.Len(t, decoded["byNrIoQueues"], 4)

	var yamlOut bytes.Buffer
	require.NoError(t, Render(&yamlOut, r, OutputFormatYAML))
	var decodedYAML map[string]any
	require.NoError(t, yaml.Unmarshal(yamlOut.Bytes(), &decodedYAML))
	small, ok := decodedYAML["data-engine-iobuf-small-pool-size"].(map[string]any)
	require.True(t, ok)
	assert.EqualValues(t, 19456, small["highest"])

	assert.Error(t, Render(&bytes.Buffer{}, r, "xml"))
}

func TestStepsExplainTheCalculation(t *testing.T) {
	r, err := Calculate(busiestNode())
	require.NoError(t, err)

	require.NotEmpty(t, r.Steps)
	for _, step := range r.Steps {
		assert.NotEmpty(t, step.Name)
		assert.NotEmpty(t, step.Formula, step.Name)
		assert.NotEmpty(t, step.Values, step.Name)
		assert.NotEmpty(t, step.Basis, step.Name)
	}

	var out bytes.Buffer
	require.NoError(t, Render(&out, r, OutputFormatText))
	for _, want := range []string{
		"[SPDK source]", "[input]", "[design choice]",
		"V = 17, R = 50",
		"17 x 4 + 50 x 8 = 468",
		"F = 17 x 4 x 127 = 8636",
		"U(P) >= 8636 + 31 x 6 = 8822  ->  large 16384, small 19456",
		"large U(16384) = 16384 - 256 - 6 x 1170 = 9108",
	} {
		assert.Contains(t, out.String(), want)
	}
}

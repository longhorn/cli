package dataengine

// The hugepage heap of spdk_tgt is computed from the SPDK source, with no measured or fitted coefficient. The object
// sizes are sizeof() of the structures in spdk_tgt of longhorn/spdk v26.05 (built with --enable-debug --with-ublk, DPDK
// 26.03, 128 lcores), and the pool and ring sizes follow rte_mempool_create() and rte_ring_create(). Only the memory that
// spdk_tgt takes from the hugepages is counted: the pools, the rings, and the DMA buffers (spdk_zmalloc and spdk_malloc).
// The memory of malloc() and calloc(), such as the request arrays of a queue pair, and the cluster arrays of a blob
// (lib/blob/blobstore.c uses realloc), is not in the hugepages.
const (
	mib = 1024 * 1024

	// DPDK mempool and ring layout (lib/mempool/rte_mempool.c, lib/ring/rte_ring.c).
	cacheLineBytes     = 64
	mempoolHeaderBytes = 64 // struct rte_mempool_objhdr (24 bytes) rounded to a cache line
	memoryChannels     = 4  // the pool pads an object so that its size in cache lines is coprime with this (default without -n)
	ringHeaderBytes    = 384
	ringEntryBytes     = 8
	// The memory zone of a mempool with a per-lcore cache holds RTE_MAX_LCORE caches of 2 x 512 entries.
	mempoolCacheBytes = 64 + 2*512*ringEntryBytes
	maxLcores         = 128

	// Object pools of the application (lib/bdev/bdev.c, lib/thread/thread.c, lib/fsdev/fsdev.c, lib/event/reactor.c).
	//
	// bdev_io: sizeof(struct spdk_bdev_io) 1216, plus the largest driver context of the linked bdev modules: struct ftl_io
	// 280 (spdk_ftl_io_size), the others are at most 192 (raid, virtio_scsi) and 184 (nvme).
	bdevIoPoolCount = 64*1024 - 1
	bdevIoBytes     = 1216 + 280
	// spdk_msg: sizeof 24, pool of SPDK_MSG_MEMPOOL_SIZE without a cache.
	msgPoolCount = 262143
	msgBytes     = 24
	// fsdev_io: sizeof(struct spdk_fsdev_io) 368, plus struct aio_fsdev_io 32.
	fsdevIoPoolCount = 64*1024 - 1
	fsdevIoBytes     = 368 + 32
	// spdk_event: sizeof 32, pool of 2^14 - 1 events with a per-lcore cache.
	eventPoolCount = 1<<14 - 1
	eventBytes     = 32

	// Rings of 65536 entries: the event ring of each reactor (reactor.c) and the message ring of each thread (thread.c). One
	// thread runs on each core.
	coreRingEntries = 65536
	ringsPerCore    = 2

	// Buffers of 1 MiB: the bdev zero buffer (bdev.c) and the FTL write and read buffers (ftl_core.c).
	oneMiBBuffers = 3

	// NVMe/TCP queue pair, with the default transport options: 128 entries (lib/nvmf/tcp.c max queue depth), in-capsule
	// data of 4096 bytes. sizeof(struct nvme_tcp_pdu) is 1088 and sizeof(struct nvme_request) is 288.
	queueEntries     = 128
	inCapsuleBytes   = 4096
	pduBytes         = 1088
	nvmeRequestBytes = 288
	// The initiator asks for 256 entries and gets the 128 that the target allows (lib/nvme/nvme_ctrlr.c). Its request pool
	// has DEFAULT_IO_QUEUE_REQUESTS 512 entries, each padded to a cache line.
	initiatorRequests = 512
	// The admin queue has DEFAULT_ADMIN_QUEUE_SIZE 32 entries and as many requests.
	adminQueueEntries = 32
	pageBytes         = 4096
)

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// ringBytes is the memory zone of rte_ring_create(count): the slots are the next power of two above count.
func ringBytes(count int) int {
	slots := 1
	for slots < count+1 {
		slots <<= 1
	}
	return slots*ringEntryBytes + ringHeaderBytes
}

// mempoolObjectBytes is the size of an object of a mempool (rte_mempool_calc_obj_size): header, element rounded to 8
// bytes, padding to a cache line, then more cache lines until the count of lines is coprime with the memory channels.
func mempoolObjectBytes(element int) int {
	lines := ceilDiv(mempoolHeaderBytes+roundUp(element, 8), cacheLineBytes)
	for gcd(lines, memoryChannels) != 1 {
		lines++
	}
	return lines * cacheLineBytes
}

// mempoolBytes is the objects of a mempool and its ring of free objects.
func mempoolBytes(count, element int) int {
	return count*mempoolObjectBytes(element) + ringBytes(count)
}

// fixedHeapMiB is the heap of an idle instance manager without the iobuf pools: the object pools, and the rings of each core.
func fixedHeapMiB(cores int) float64 {
	b := mempoolBytes(bdevIoPoolCount, bdevIoBytes) +
		mempoolBytes(msgPoolCount, msgBytes) +
		mempoolBytes(fsdevIoPoolCount, fsdevIoBytes) +
		mempoolBytes(eventPoolCount, eventBytes) + 192 + maxLcores*mempoolCacheBytes +
		cores*ringsPerCore*(ringBytes(coreRingEntries)) +
		oneMiBBuffers*mib
	return float64(b) / mib
}

// poolHeapMiB is the buffers of an iobuf pool and its ring of free buffers.
func poolHeapMiB(count, bufferKiB int) float64 {
	return float64(count*bufferKiB*1024+ringBytes(count)) / mib
}

// targetQpairBytes is the DMA memory of one NVMe/TCP queue pair on the target side (nvmf_tcp_qpair_init_mem_resource): the
// in-capsule buffers, and 2 x 128 + 1 PDUs. It is the same for the I/O and the admin queue pairs of a controller.
func targetQpairBytes() int {
	return queueEntries*inCapsuleBytes + roundUp((2*queueEntries+1)*pduBytes, pageBytes)
}

// initiatorQpairBytes is the DMA memory of one NVMe/TCP queue pair on the initiator side (nvme_tcp_alloc_reqs,
// nvme_qpair_init_request_pool): entries + 2 PDUs, and the request pool.
func initiatorQpairBytes(entries, requests int) int {
	return roundUp((entries+2)*pduBytes, pageBytes) + requests*roundUp(nvmeRequestBytes, cacheLineBytes)
}

// frontendHeapMiB is the target side of the front end. Each engine has nr-io-queues I/O queue pairs and one admin queue pair.
func frontendHeapMiB(engines, queues int) float64 {
	return float64(engines*(queues+1)*targetQpairBytes()) / mib
}

// backendHeapMiB is the back end of the node: the target side of each replica (an I/O queue pair per core and an admin
// queue pair), and the initiator side of the same number of connections, as every replica is connected to one engine.
func backendHeapMiB(replicas, cores int) float64 {
	target := replicas * (cores + 1) * targetQpairBytes()
	initiator := replicas * (cores*initiatorQpairBytes(queueEntries, initiatorRequests) + initiatorQpairBytes(adminQueueEntries, adminQueueEntries))
	return float64(target+initiator) / mib
}

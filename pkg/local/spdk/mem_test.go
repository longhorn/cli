package spdk

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/longhorn/cli/pkg/types"
)

type sparseMem struct {
	pages map[uint64][]byte
}

const pageSize = 4096

func newSparseMem() *sparseMem {
	return &sparseMem{pages: make(map[uint64][]byte)}
}

func (s *sparseMem) writeBytes(addr uint64, data []byte) {
	for i, b := range data {
		cur := addr + uint64(i)
		pg := cur &^ (pageSize - 1)
		off := cur & (pageSize - 1)
		buf, ok := s.pages[pg]
		if !ok {
			buf = make([]byte, pageSize)
			s.pages[pg] = buf
		}
		buf[off] = b
	}
}

func (s *sparseMem) putU64(addr, val uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], val)
	s.writeBytes(addr, b[:])
}

func (s *sparseMem) putU32(addr uint64, val uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], val)
	s.writeBytes(addr, b[:])
}

func (s *sparseMem) putU16(addr uint64, val uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], val)
	s.writeBytes(addr, b[:])
}

func (s *sparseMem) putU8(addr uint64, val uint8) {
	s.writeBytes(addr, []byte{val})
}

func (s *sparseMem) putCString(addr uint64, str string) {
	b := append([]byte(str), 0)
	s.writeBytes(addr, b)
}

func (s *sparseMem) ReadAt(p []byte, off int64) (int, error) {
	if off < 0 {
		return 0, io.EOF
	}
	uoff := uint64(off)
	for i := range p {
		cur := uoff + uint64(i)
		pg := cur &^ (pageSize - 1)
		poff := cur & (pageSize - 1)
		buf, ok := s.pages[pg]
		if !ok {
			if i == 0 {
				return 0, io.ErrUnexpectedEOF
			}
			return i, io.ErrUnexpectedEOF
		}
		p[i] = buf[poff]
	}
	return len(p), nil
}

func TestInspectProcessMemory_SharedBsChannelDeadlock(t *testing.T) {
	mem := newSparseMem()
	layout := defaultSpdkStructLayout(false)
	syms := &spdkSymbols{
		gThreads:                0x500000,
		gIobuf:                  0x510000,
		gLvolStores:             0x520000,
		blobAllocCopyClusterCpl: 0x412340,
		perBlobChannelSupported: false,
	}

	// 1. Global iobuf + DPDK rings
	smallRing := uint64(0x200000100000)
	largeRing := uint64(0x200000200000)
	mem.putU64(syms.gIobuf+layout.iobufSmallPoolCount, 8192)
	mem.putU64(syms.gIobuf+layout.iobufLargePoolCount, 6144)
	mem.putU32(syms.gIobuf+layout.iobufSmallBufSize, 8192)
	mem.putU32(syms.gIobuf+layout.iobufLargeBufSize, 135168)
	mem.putU64(syms.gIobuf+layout.iobufNode+layout.iobufNodeSmallPool, smallRing)
	mem.putU64(syms.gIobuf+layout.iobufNode+layout.iobufNodeLargePool, largeRing)

	mem.putU32(smallRing+layout.rteRingMask, 16383)
	mem.putU32(smallRing+layout.rteRingCapacity, 8192)
	mem.putU32(smallRing+layout.rteRingProdTail, 8192)
	mem.putU32(smallRing+layout.rteRingConsHead, 0)

	// Test uint32 wraparound on largeRing: prodTail=500, consHead=0xfffffe0c -> diff = 1000
	mem.putU32(largeRing+layout.rteRingMask, 8191)
	mem.putU32(largeRing+layout.rteRingCapacity, 6144)
	mem.putU32(largeRing+layout.rteRingProdTail, 500)
	mem.putU32(largeRing+layout.rteRingConsHead, ^uint32(499))

	// 2. g_lvol_stores -> rebuild replica + canary replica
	lvsAddr := uint64(0x600000)
	lvolRebuild := uint64(0x601000)
	lvolCanary := uint64(0x602000)
	lvolPeerSnap := uint64(0x602800)
	blobRebuild := uint64(0x603000)
	blobCanary := uint64(0x604000)
	blobPeerSnap := uint64(0x605000)
	bdevPtr := uint64(0x901000)

	assert.Equal(t, 3, spdkBsCplTypeBlobBasic)

	mem.putU64(syms.gLvolStores, lvsAddr)
	mem.putU64(lvsAddr+layout.lvsLvols, lvolRebuild)
	mem.putU64(lvolRebuild+layout.lvolBlob, blobRebuild)
	mem.putU64(lvolRebuild+layout.lvolBlobID, 0x100000011)
	mem.putCString(lvolRebuild+layout.lvolName, "pvc-rebuild-r-49a7a136")
	mem.putU64(lvolRebuild+layout.lvolLinkNext, lvolCanary)

	mem.putU64(lvolCanary+layout.lvolBlob, blobCanary)
	mem.putU64(lvolCanary+layout.lvolBlobID, 0x100000022)
	mem.putCString(lvolCanary+layout.lvolName, "pvc-canary-r-99999999")
	mem.putU64(lvolCanary+layout.lvolLinkNext, lvolPeerSnap)

	mem.putU64(lvolPeerSnap+layout.lvolBlob, blobPeerSnap)
	mem.putU64(lvolPeerSnap+layout.lvolBlobID, 0x100000033)
	mem.putCString(lvolPeerSnap+layout.lvolUUIDStr, "1af7ab1b-ffe1-4687-95f6-2719672fcda0")
	mem.putCString(lvolPeerSnap+layout.lvolName, "pvc-peer-snap-rebuild")
	mem.putU64(lvolPeerSnap+layout.lvolBdev, bdevPtr)
	mem.putU64(lvolPeerSnap+layout.lvolLinkNext, 0)

	// 3. Thread nvmf_tgt_poll_group_000 with shared blobstore channel + bdev_mgr channel
	threadAddr := uint64(0x700000)
	mem.putU64(syms.gThreads, threadAddr)
	mem.putU64(threadAddr+layout.threadID, 2)
	mem.putCString(threadAddr+layout.threadName, "nvmf_tgt_poll_group_000")

	bsChanAddr := uint64(0x7f77442cdfb0)
	bsDevAddr := uint64(0x710000)
	bdevMgrChanAddr := uint64(0x7f7744300000)
	bdevMgrDevAddr := uint64(0x711000)

	mem.putU64(threadAddr+layout.threadIOChannels, bsChanAddr)
	mem.putU64(bsChanAddr+layout.ioChanDev, bsDevAddr)
	mem.putCString(bsDevAddr+layout.ioDevName, "blobstore")
	mem.putU64(bsChanAddr+layout.ioChanNode+8, bdevMgrChanAddr)

	// 512 request sets: idx 7 is active esnap_cow, remaining 511 are queued in need_cluster_alloc (freeReqs = 0)
	reqMem := uint64(0x800000)
	bsCtx := bsChanAddr + layout.ioChanCtx
	mem.putU64(bsCtx+layout.bsChanReqMem, reqMem)
	mem.putU64(bsCtx+layout.bsChanReqs, 0)

	var prevNeed uint64
	for idx := 0; idx < bsChannelMaxReqs; idx++ {
		r := reqMem + uint64(idx)*layout.bsReqSetSize
		if idx == 7 {
			copyCtx := uint64(0x880000)
			backChan := uint64(0x890000)
			backDev := uint64(0x891000)

			mem.putU32(r+layout.bsReqSetCplType, spdkBsCplTypeBlobBasic)
			mem.putU64(r+layout.bsReqSetCplCbFn, syms.blobAllocCopyClusterCpl)
			mem.putU64(r+layout.bsReqSetCplCbArg, copyCtx)
			mem.putU64(r+layout.bsReqSetBackChannel, backChan)

			mem.putU64(copyCtx+layout.copyCtxBlob, blobRebuild)
			mem.putU64(copyCtx+layout.copyCtxIOUnit, 4636672)
			mem.putU64(copyCtx+layout.copyCtxNewCluster, 102)

			mem.putU64(backChan+layout.ioChanDev, backDev)
			mem.putCString(backDev+layout.ioDevName, "bdev_pvc-rebuild-r-b5b5cfdc-snap-rebuild-464c20a6n1")
			continue
		}

		targetBlob := blobRebuild
		if idx >= 400 {
			targetBlob = blobCanary
		}
		mem.putU64(r+layout.bsReqSetUserOpBlob, targetBlob)
		if prevNeed == 0 {
			mem.putU64(bsCtx+layout.bsChanNeedClusterAlloc, r)
		} else {
			mem.putU64(prevNeed+layout.bsReqSetLinkNext, r)
		}
		prevNeed = r
	}
	mem.putU64(prevNeed+layout.bsReqSetLinkNext, 0)

	// 4. bdev_mgr channel with 2 queued nomem_io snapshot reads
	mem.putU64(bdevMgrChanAddr+layout.ioChanDev, bdevMgrDevAddr)
	mem.putCString(bdevMgrDevAddr+layout.ioDevName, "bdev_mgr")
	sharedRes := uint64(0x900000)
	bdevNameStr := uint64(0x901500)
	bio1 := uint64(0x910000)
	bio2 := uint64(0x911000)

	mem.putU64(bdevMgrChanAddr+layout.ioChanCtx+layout.bdevMgmtSharedResources, sharedRes)
	mem.putU64(sharedRes+layout.bdevSharedIOOutstanding, 512)
	mem.putU64(sharedRes+layout.bdevSharedNoMemThresh, 64)
	mem.putU64(sharedRes+layout.bdevSharedNoMemIO, bio1)

	mem.putCString(bdevNameStr, "1af7ab1b-ffe1-4687-95f6-2719672fcda0")
	mem.putU64(bdevPtr+layout.bdevNamePtr, bdevNameStr)

	mem.putU64(bio1+layout.bdevIOBdev, bdevPtr)
	mem.putU8(bio1+layout.bdevIOType, spdkBdevIOTypeRead)
	mem.putU16(bio1+layout.bdevIONumRetries, 36254)
	mem.putU64(bio1+layout.bdevIOLinkNext, bio2)

	mem.putU64(bio2+layout.bdevIOBdev, bdevPtr)
	mem.putU8(bio2+layout.bdevIOType, spdkBdevIOTypeRead)
	mem.putU16(bio2+layout.bdevIONumRetries, 36250)
	mem.putU64(bio2+layout.bdevIOLinkNext, 0)

	rpcSnap := &rpcSnapshot{
		threadPollers: map[string]*threadPollerSnapshot{
			"nvmf_tgt_poll_group_000": {
				bdevNoMemPollerCount: 1,
				timedPollers:         []string{"bdev_no_mem_poller", "nvmf_ctrlr_keep_alive_poll", "nvmf_ctrlr_keep_alive_poll"},
			},
		},
		nvmeCtrlrAddrs: map[string]string{
			"pvc-rebuild-r-b5b5cfdc-snap-rebuild-464c20a6": "10.42.2.10:20015",
		},
	}

	procInfo := &types.SpdkProcessInfo{PID: 23}
	inspectProcessMemory(mem, syms, layout, rpcSnap, false, procInfo)

	require.NotNil(t, procInfo.Iobuf)
	assert.Equal(t, uint32(1000), procInfo.Iobuf.LargeRingFree)
	assert.Equal(t, uint32(8192), procInfo.Iobuf.SmallRingFree)

	require.Len(t, procInfo.Threads, 1)
	th := procInfo.Threads[0]
	assert.Equal(t, 1, th.BdevNoMemPollerCount)
	assert.Len(t, th.TimedPollers, 3)
	assert.Equal(t, 2, th.TimedPollerCounts["nvmf_ctrlr_keep_alive_poll"])
	assert.Equal(t, 1, th.TimedPollerCounts["bdev_no_mem_poller"])
	require.Len(t, th.BlobstoreChannels, 1)

	bsCh := th.BlobstoreChannels[0]
	assert.Equal(t, 0, bsCh.FreeReqs)
	assert.Equal(t, 511, bsCh.NeedClusterAlloc)
	assert.Equal(t, 399, bsCh.NeedClusterAllocByLvol["pvc-rebuild-r-49a7a136"])
	assert.Equal(t, 112, bsCh.NeedClusterAllocByLvol["pvc-canary-r-99999999"])

	require.Len(t, bsCh.ActiveRequests, 1)
	act := bsCh.ActiveRequests[0]
	assert.Equal(t, 7, act.Index)
	assert.Equal(t, "esnap_cow", act.OpType)
	assert.Equal(t, "pvc-rebuild-r-49a7a136", act.LvolName)
	assert.Equal(t, "0x100000011", act.BlobID)
	assert.Equal(t, uint64(4636672), act.IOUnit)
	assert.Equal(t, "10.42.2.10:20015", act.RemoteTargetAddress)

	require.Len(t, th.BdevNoMemResources, 1)
	assert.Equal(t, 2, th.BdevNoMemResources[0].NoMemIOCount)
	require.Len(t, th.BdevNoMemResources[0].ByBdev, 1)
	assert.Equal(t, "1af7ab1b-ffe1-4687-95f6-2719672fcda0", th.BdevNoMemResources[0].ByBdev[0].BdevName)
	assert.Equal(t, "pvc-peer-snap-rebuild", th.BdevNoMemResources[0].ByBdev[0].LvolName)
	assert.Equal(t, "read", th.BdevNoMemResources[0].ByBdev[0].IOType)
	assert.Equal(t, uint16(36254), th.BdevNoMemResources[0].ByBdev[0].MaxRetries)

	assert.Len(t, procInfo.Warnings, 1)
	assert.Len(t, procInfo.Errors, 2)
}

func TestInspectProcessMemory_PerBlobIobufExhaustion(t *testing.T) {
	mem := newSparseMem()
	layout := defaultSpdkStructLayout(true)
	syms := &spdkSymbols{
		gThreads:                0x500000,
		gIobuf:                  0x510000,
		gLvolStores:             0x520000,
		perBlobChannelSupported: true,
	}

	largeRing := uint64(0x200000200000)
	mem.putU64(syms.gIobuf+layout.iobufLargePoolCount, 1024)
	mem.putU32(syms.gIobuf+layout.iobufLargeBufSize, 135168)
	mem.putU64(syms.gIobuf+layout.iobufNode+layout.iobufNodeLargePool, largeRing)
	mem.putU32(largeRing+layout.rteRingMask, 2047)
	mem.putU32(largeRing+layout.rteRingCapacity, 1024)
	mem.putU32(largeRing+layout.rteRingProdTail, 2784)
	mem.putU32(largeRing+layout.rteRingConsHead, 2784) // ring_free = 0

	// Thread with exhausted iobuf large_pool + 2 waiters, and 1 idle per-blob channel
	threadAddr := uint64(0x700000)
	mem.putU64(syms.gThreads, threadAddr)
	mem.putCString(threadAddr+layout.threadName, "nvmf_tgt_poll_group_000")

	iobufChan := uint64(0x720000)
	iobufDev := uint64(0x721000)
	idleBlobChan := uint64(0x730000)
	idleBlobDev := uint64(0x731000)
	idleBlobPtr := uint64(0x732000)

	mem.putU64(threadAddr+layout.threadIOChannels, iobufChan)
	mem.putU64(iobufChan+layout.ioChanDev, iobufDev)
	mem.putCString(iobufDev+layout.ioDevName, "iobuf")
	mem.putU64(iobufChan+layout.ioChanNode, idleBlobChan)

	// 2 waiters in large_queue
	w1 := uint64(0x740000)
	w2 := uint64(0x740100)
	iobufCtx := iobufChan + layout.ioChanCtx
	mem.putU64(iobufCtx+layout.iobufChanLargeQueue, w1)
	mem.putU64(w1+layout.iobufEntryStailqNext, w2)
	mem.putU64(w2+layout.iobufEntryStailqNext, 0)

	// nvmf_TCP spdk_iobuf_channel with large_cache = 0/512
	nvmfIch := uint64(0x750000)
	modStruct := uint64(0x751000)
	modNameStr := uint64(0x751100)
	mem.putCString(modNameStr, "nvmf_TCP")
	mem.putU64(modStruct, modNameStr)
	mem.putU64(iobufCtx+layout.iobufChanChannels+8, nvmfIch)
	mem.putU64(nvmfIch+layout.spdkIobufChanModule, modStruct)
	mem.putU64(nvmfIch+layout.spdkIobufChanParent, iobufChan)
	mem.putU32(nvmfIch+layout.spdkIobufChanCache+layout.poolCacheLargeCount, 0)
	mem.putU32(nvmfIch+layout.spdkIobufChanCache+layout.poolCacheLargeSize, 512)
	mem.putU64(nvmfIch+layout.spdkIobufChanCache+layout.poolCacheLargeRetry, 5409293)

	// Idle per-blob channel (all 512 reqs in free list)
	mem.putU64(idleBlobChan+layout.ioChanDev, idleBlobDev)
	mem.putU64(idleBlobDev+layout.ioDevPtr, idleBlobPtr)
	mem.putCString(idleBlobDev+layout.ioDevName, "blob")
	mem.putU64(idleBlobPtr+layout.blobID, 0x1000000aa)
	reqMem := uint64(0x800000)
	blobCtx := idleBlobChan + layout.ioChanCtx
	mem.putU64(blobCtx+layout.bsChanReqMem, reqMem)
	mem.putU64(blobCtx+layout.bsChanReqs, reqMem)
	for i := 0; i < bsChannelMaxReqs-1; i++ {
		cur := reqMem + uint64(i)*layout.bsReqSetSize
		nxt := reqMem + uint64(i+1)*layout.bsReqSetSize
		mem.putU64(cur+layout.bsReqSetLinkNext, nxt)
	}

	procFiltered := &types.SpdkProcessInfo{PID: 908}
	inspectProcessMemory(mem, syms, layout, nil, false, procFiltered)
	require.Len(t, procFiltered.Threads, 1)
	require.NotNil(t, procFiltered.Threads[0].IobufChannel)
	assert.Equal(t, 2, procFiltered.Threads[0].IobufChannel.LargeWaiters)
	assert.Equal(t, uint64(0), procFiltered.Threads[0].IobufChannel.TotalFreeLargeBuffers)
	assert.Empty(t, procFiltered.Threads[0].BlobstoreChannels)
	require.Len(t, procFiltered.Errors, 1)
	assert.Contains(t, procFiltered.Errors[0], "waiting for iobuf large_pool buffers")

	procAll := &types.SpdkProcessInfo{PID: 908}
	inspectProcessMemory(mem, syms, layout, nil, true, procAll)
	require.Len(t, procAll.Threads[0].BlobstoreChannels, 1)
	assert.Equal(t, "0x1000000aa", procAll.Threads[0].BlobstoreChannels[0].BlobID)
	assert.Equal(t, 512, procAll.Threads[0].BlobstoreChannels[0].FreeReqs)
}

func TestCollectSpdkRPC_UnixSocket(t *testing.T) {
	tmpDir, err := os.MkdirTemp("/tmp", "spdk-rpc-*")
	require.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	sockPath := filepath.Join(tmpDir, "spdk.sock")
	ln, err := net.Listen("unix", sockPath)
	require.NoError(t, err)
	defer func() {
		_ = ln.Close()
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				var req jsonRPCRequest
				if err := json.NewDecoder(c).Decode(&req); err != nil {
					return
				}
				var payload any
				switch req.Method {
				case "spdk_get_version":
					payload = map[string]any{"version": "SPDK v26.05"}
				case "iobuf_get_stats":
					payload = []map[string]any{
						{
							"module":     "nvmf_TCP",
							"small_pool": map[string]any{"cache_size": 4096, "cache": 10, "main": 0, "retry": 0},
							"large_pool": map[string]any{"cache_size": 3072, "cache": 999, "main": 0, "retry": 0},
						},
					}
				case "nvmf_get_stats":
					payload = map[string]any{
						"poll_groups": []map[string]any{
							{"name": "nvmf_tgt_poll_group_000", "current_admin_qpairs": 4, "current_io_qpairs": 12, "pending_bdev_io": 0, "completed_nvme_io": 500},
						},
					}
				case "thread_get_pollers":
					payload = map[string]any{
						"threads": []map[string]any{
							{
								"name":           "nvmf_tgt_poll_group_000",
								"active_pollers": []map[string]any{{"name": "nvmf_TCP"}},
								"timed_pollers":  []map[string]any{{"name": "bdev_no_mem_poller"}},
							},
						},
					}
				case "bdev_nvme_get_controllers":
					payload = []map[string]any{
						{
							"name": "pvc-123-snap-rebuild",
							"ctrlrs": []map[string]any{
								{"trid": map[string]any{"traddr": "10.42.1.5", "trsvcid": "20010"}},
							},
						},
					}
				}
				raw, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": payload})
				_, _ = c.Write(append(raw, '\n'))
			}(conn)
		}
	}()

	snap := collectSpdkRPC(sockPath)
	assert.Empty(t, snap.warnings)
	assert.Equal(t, "SPDK v26.05", snap.version)
	require.Len(t, snap.iobufStats, 1)
	assert.Equal(t, uint64(999), snap.iobufStats[0].LargePool.Cache)
	require.Len(t, snap.nvmfPollGroups, 1)
	assert.Equal(t, uint32(12), snap.nvmfPollGroups[0].CurrentIOQpairs)
	assert.Equal(t, 1, snap.threadPollers["nvmf_tgt_poll_group_000"].bdevNoMemPollerCount)
	assert.Equal(t, "10.42.1.5:20010", resolveBackingDeviceTarget("bdev_pvc-123-snap-rebuildn1", snap.nvmeCtrlrAddrs))
}

func TestDWARFMemberOffset(t *testing.T) {
	off, ok := dwarfMemberOffset(int64(96))
	assert.True(t, ok)
	assert.Equal(t, uint64(96), off)

	// DW_OP_plus_uconst (0x23) with ULEB128 for 1032 (0x88, 0x08)
	off, ok = dwarfMemberOffset([]byte{0x23, 0x88, 0x08})
	assert.True(t, ok)
	assert.Equal(t, uint64(1032), off)
}

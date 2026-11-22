package spdk

import (
	"encoding/binary"
	"fmt"
	"io"
	"sort"

	"github.com/longhorn/cli/pkg/types"
)

type memReader struct {
	r io.ReaderAt
}

func (m *memReader) u64(addr uint64) (uint64, bool) {
	if addr == 0 {
		return 0, false
	}
	var buf [8]byte
	if _, err := m.r.ReadAt(buf[:], int64(addr)); err != nil {
		return 0, false
	}
	return binary.LittleEndian.Uint64(buf[:]), true
}

func (m *memReader) u32(addr uint64) (uint32, bool) {
	if addr == 0 {
		return 0, false
	}
	var buf [4]byte
	if _, err := m.r.ReadAt(buf[:], int64(addr)); err != nil {
		return 0, false
	}
	return binary.LittleEndian.Uint32(buf[:]), true
}

func (m *memReader) u16(addr uint64) (uint16, bool) {
	if addr == 0 {
		return 0, false
	}
	var buf [2]byte
	if _, err := m.r.ReadAt(buf[:], int64(addr)); err != nil {
		return 0, false
	}
	return binary.LittleEndian.Uint16(buf[:]), true
}

func (m *memReader) u8(addr uint64) (uint8, bool) {
	if addr == 0 {
		return 0, false
	}
	var buf [1]byte
	if _, err := m.r.ReadAt(buf[:], int64(addr)); err != nil {
		return 0, false
	}
	return buf[0], true
}

func (m *memReader) cstr(addr uint64, maxLen int) string {
	if addr == 0 || maxLen <= 0 {
		return ""
	}
	buf := make([]byte, maxLen)
	n, err := m.r.ReadAt(buf, int64(addr))
	if n == 0 && err != nil {
		return ""
	}
	buf = buf[:n]
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func (m *memReader) walkList(headPtrAddr, nextOffset uint64, maxItems int) []uint64 {
	first, ok := m.u64(headPtrAddr)
	if !ok || first == 0 {
		return nil
	}
	res := make([]uint64, 0, 16)
	seen := make(map[uint64]struct{}, 16)
	cur := first
	for cur != 0 && len(res) < maxItems {
		if _, dup := seen[cur]; dup {
			break
		}
		seen[cur] = struct{}{}
		res = append(res, cur)
		next, ok := m.u64(cur + nextOffset)
		if !ok {
			break
		}
		cur = next
	}
	return res
}

func (m *memReader) walkRBTree(rootAddr, nodeOffset uint64, maxItems int) []uint64 {
	if rootAddr == 0 {
		return nil
	}
	res := make([]uint64, 0, 32)
	seen := make(map[uint64]struct{}, 32)
	stack := []uint64{rootAddr}
	for len(stack) > 0 && len(res) < maxItems {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if cur == 0 {
			continue
		}
		if _, dup := seen[cur]; dup {
			continue
		}
		seen[cur] = struct{}{}
		res = append(res, cur)

		if right, ok := m.u64(cur + nodeOffset + 8); ok && right != 0 {
			stack = append(stack, right)
		}
		if left, ok := m.u64(cur + nodeOffset); ok && left != 0 {
			stack = append(stack, left)
		}
	}
	return res
}

type lvolMeta struct {
	name   string
	blobID uint64
}

type lvolIndex struct {
	byBlobPtr map[uint64]lvolMeta
	byBdevPtr map[uint64]string
	byUUID    map[string]string
}

func inspectProcessMemory(
	rawMem io.ReaderAt,
	syms *spdkSymbols,
	layout *spdkStructLayout,
	rpcSnap *rpcSnapshot,
	includeIdleChannels bool,
	procInfo *types.SpdkProcessInfo,
) {
	m := &memReader{r: rawMem}

	if syms.gIobuf != 0 {
		procInfo.Iobuf = inspectGlobalIobuf(m, syms.gIobuf, layout, rpcSnap)
	} else if rpcSnap != nil && len(rpcSnap.iobufStats) > 0 {
		procInfo.Iobuf = &types.SpdkIobufInfo{
			ModuleStats: rpcSnap.iobufStats,
		}
	}

	lvolIdx := inspectLvolIndex(m, syms.gLvolStores, layout)

	threads := m.walkList(syms.gThreads, layout.threadTailqNext, 256)
	for _, th := range threads {
		tInfo := inspectThread(m, th, syms, layout, lvolIdx, rpcSnap, procInfo.Iobuf, includeIdleChannels)
		procInfo.Threads = append(procInfo.Threads, tInfo)

		if tInfo.BdevNoMemPollerCount > 0 {
			procInfo.Warnings = append(procInfo.Warnings,
				fmt.Sprintf("thread %s has %d active bdev_no_mem_poller(s)", tInfo.Name, tInfo.BdevNoMemPollerCount))
		}
		if tInfo.IobufChannel != nil && tInfo.IobufChannel.LargeWaiters > 0 {
			procInfo.Errors = append(procInfo.Errors,
				fmt.Sprintf("thread %s has %d request(s) blocked waiting for iobuf large_pool buffers (totalFreeLargeBuffers=%d)",
					tInfo.Name, tInfo.IobufChannel.LargeWaiters, tInfo.IobufChannel.TotalFreeLargeBuffers))
		}
		for _, bsCh := range tInfo.BlobstoreChannels {
			target := bsCh.Device
			if bsCh.LvolName != "" {
				target = fmt.Sprintf("%s (%s)", bsCh.Device, bsCh.LvolName)
			}
			if bsCh.FreeReqs == 0 && bsCh.TotalReqs > 0 {
				procInfo.Errors = append(procInfo.Errors,
					fmt.Sprintf("thread %s %s channel %s exhausted spdk_bs_request_set pool (freeReqs=0/%d, needClusterAlloc=%d)",
						tInfo.Name, target, bsCh.Address, bsCh.TotalReqs, bsCh.NeedClusterAlloc))
			} else if bsCh.FreeReqs > 0 && bsCh.NeedClusterAlloc > 0 && len(bsCh.ActiveRequests) > 0 &&
				(bsCh.NeedClusterAlloc >= bsCh.TotalReqs/2 || len(tInfo.BdevNoMemResources) > 0) {
				procInfo.Errors = append(procInfo.Errors,
					fmt.Sprintf("thread %s %s channel %s cluster allocation blocked (needClusterAlloc=%d, freeReqs=%d/%d, activeRequests=%d)",
						tInfo.Name, target, bsCh.Address, bsCh.NeedClusterAlloc, bsCh.FreeReqs, bsCh.TotalReqs, len(bsCh.ActiveRequests)))
			}
		}
		for _, nm := range tInfo.BdevNoMemResources {
			if nm.NoMemIOCount > 0 {
				procInfo.Errors = append(procInfo.Errors,
					fmt.Sprintf("thread %s has %d bdev I/O(s) queued in nomem_io (ioOutstanding=%d)",
						tInfo.Name, nm.NoMemIOCount, nm.IOOutstanding))
			}
		}
	}
}

func inspectGlobalIobuf(m *memReader, gIobufAddr uint64, layout *spdkStructLayout, rpcSnap *rpcSnapshot) *types.SpdkIobufInfo {
	smallCnt, _ := m.u64(gIobufAddr + layout.iobufSmallPoolCount)
	largeCnt, _ := m.u64(gIobufAddr + layout.iobufLargePoolCount)
	smallSz, _ := m.u32(gIobufAddr + layout.iobufSmallBufSize)
	largeSz, _ := m.u32(gIobufAddr + layout.iobufLargeBufSize)

	nodeBase := gIobufAddr + layout.iobufNode
	smallRing, _ := m.u64(nodeBase + layout.iobufNodeSmallPool)
	largeRing, _ := m.u64(nodeBase + layout.iobufNodeLargePool)

	info := &types.SpdkIobufInfo{
		SmallPoolCount: smallCnt,
		SmallBufSize:   smallSz,
		SmallRingFree:  readRteRingFree(m, smallRing, layout),
		LargePoolCount: largeCnt,
		LargeBufSize:   largeSz,
		LargeRingFree:  readRteRingFree(m, largeRing, layout),
	}
	if rpcSnap != nil {
		info.ModuleStats = rpcSnap.iobufStats
	}
	return info
}

func readRteRingFree(m *memReader, ringAddr uint64, layout *spdkStructLayout) uint32 {
	if ringAddr == 0 {
		return 0
	}
	mask, ok1 := m.u32(ringAddr + layout.rteRingMask)
	capVal, _ := m.u32(ringAddr + layout.rteRingCapacity)
	prodTail, ok2 := m.u32(ringAddr + layout.rteRingProdTail)
	consHead, ok3 := m.u32(ringAddr + layout.rteRingConsHead)
	if !ok1 || !ok2 || !ok3 {
		return 0
	}
	count := (prodTail - consHead) & mask
	if capVal > 0 && count > capVal {
		count = capVal
	}
	return count
}

func inspectLvolIndex(m *memReader, gLvolStoresAddr uint64, layout *spdkStructLayout) *lvolIndex {
	idx := &lvolIndex{
		byBlobPtr: make(map[uint64]lvolMeta),
		byBdevPtr: make(map[uint64]string),
		byUUID:    make(map[string]string),
	}
	if gLvolStoresAddr == 0 {
		return idx
	}
	for _, lvs := range m.walkList(gLvolStoresAddr, layout.lvsLinkNext, 256) {
		for _, lvol := range m.walkList(lvs+layout.lvsLvols, layout.lvolLinkNext, 8192) {
			name := m.cstr(lvol+layout.lvolName, layout.lvolNameSize)
			if blobPtr, ok := m.u64(lvol + layout.lvolBlob); ok && blobPtr != 0 {
				blobID, _ := m.u64(lvol + layout.lvolBlobID)
				idx.byBlobPtr[blobPtr] = lvolMeta{
					name:   name,
					blobID: blobID,
				}
			}
			if name != "" {
				if layout.lvolUUIDStrSize > 0 {
					if uuidStr := m.cstr(lvol+layout.lvolUUIDStr, layout.lvolUUIDStrSize); uuidStr != "" {
						idx.byUUID[uuidStr] = name
					}
				}
				if bdevPtr, ok := m.u64(lvol + layout.lvolBdev); ok && bdevPtr != 0 {
					idx.byBdevPtr[bdevPtr] = name
				}
			}
		}
	}
	return idx
}

func inspectThread(
	m *memReader,
	threadAddr uint64,
	syms *spdkSymbols,
	layout *spdkStructLayout,
	lvolIdx *lvolIndex,
	rpcSnap *rpcSnapshot,
	globalIobuf *types.SpdkIobufInfo,
	includeIdleChannels bool,
) *types.SpdkThreadInfo {
	tName := m.cstr(threadAddr+layout.threadName, layout.threadNameSize)
	tID, _ := m.u64(threadAddr + layout.threadID)

	tInfo := &types.SpdkThreadInfo{
		Name: tName,
		ID:   tID,
	}
	if rpcSnap != nil {
		if ps, ok := rpcSnap.threadPollers[tName]; ok {
			tInfo.BdevNoMemPollerCount = ps.bdevNoMemPollerCount
			tInfo.ActivePollers = ps.activePollers
			tInfo.TimedPollers = ps.timedPollers
			if len(ps.timedPollers) > 0 {
				counts := make(map[string]int)
				for _, tp := range ps.timedPollers {
					counts[tp]++
				}
				tInfo.TimedPollerCounts = counts
			}
		}
	}

	rbRoot, _ := m.u64(threadAddr + layout.threadIOChannels)
	channels := m.walkRBTree(rbRoot, layout.ioChanNode, 16384)
	for _, ch := range channels {
		dev, ok := m.u64(ch + layout.ioChanDev)
		if !ok || dev == 0 {
			continue
		}
		devName := m.cstr(dev+layout.ioDevName, layout.ioDevNameSize)
		ctx := ch + layout.ioChanCtx

		switch devName {
		case "iobuf":
			tInfo.IobufChannel = inspectThreadIobufChannel(m, ch, ctx, layout, globalIobuf)
		case "blobstore", "blob":
			bsInfo := inspectBsChannel(m, ch, dev, ctx, devName, syms, layout, lvolIdx, rpcSnap)
			if includeIdleChannels || devName == "blobstore" ||
				bsInfo.FreeReqs < bsInfo.TotalReqs || bsInfo.NeedClusterAlloc > 0 ||
				bsInfo.QueuedIO > 0 || len(bsInfo.ActiveRequests) > 0 {
				tInfo.BlobstoreChannels = append(tInfo.BlobstoreChannels, bsInfo)
			}
		case "bdev_mgr":
			tInfo.BdevNoMemResources = append(tInfo.BdevNoMemResources, inspectBdevMgmtChannel(m, ctx, layout, lvolIdx)...)
		}
	}

	sort.Slice(tInfo.BlobstoreChannels, func(i, j int) bool {
		if tInfo.BlobstoreChannels[i].Device != tInfo.BlobstoreChannels[j].Device {
			return tInfo.BlobstoreChannels[i].Device == "blobstore"
		}
		if tInfo.BlobstoreChannels[i].NeedClusterAlloc != tInfo.BlobstoreChannels[j].NeedClusterAlloc {
			return tInfo.BlobstoreChannels[i].NeedClusterAlloc > tInfo.BlobstoreChannels[j].NeedClusterAlloc
		}
		return tInfo.BlobstoreChannels[i].Address < tInfo.BlobstoreChannels[j].Address
	})

	return tInfo
}

func inspectThreadIobufChannel(
	m *memReader,
	ioChanAddr, ctxAddr uint64,
	layout *spdkStructLayout,
	globalIobuf *types.SpdkIobufInfo,
) *types.SpdkThreadIobufInfo {
	smallWaiters := len(m.walkList(ctxAddr+layout.iobufChanSmallQueue, layout.iobufEntryStailqNext, 16384))
	largeWaiters := len(m.walkList(ctxAddr+layout.iobufChanLargeQueue, layout.iobufEntryStailqNext, 16384))

	info := &types.SpdkThreadIobufInfo{
		SmallWaiters: smallWaiters,
		LargeWaiters: largeWaiters,
	}

	var nvmfLargeCache uint64
	hasNvmfTCP := false
	for idx := 0; idx < iobufMaxChannels; idx++ {
		ich, ok := m.u64(ctxAddr + layout.iobufChanChannels + uint64(idx)*8)
		if !ok || ich == 0 {
			continue
		}
		parent, ok := m.u64(ich + layout.spdkIobufChanParent)
		if !ok || parent != ioChanAddr {
			continue
		}

		modName := ""
		if modPtr, ok := m.u64(ich + layout.spdkIobufChanModule); ok && modPtr != 0 {
			if namePtr, ok := m.u64(modPtr); ok && namePtr != 0 {
				modName = m.cstr(namePtr, 64)
			}
		}
		if modName == "" {
			modName = fmt.Sprintf("module_%d", idx)
		}

		cacheBase := ich + layout.spdkIobufChanCache
		sCount, _ := m.u32(cacheBase + layout.poolCacheSmallCount)
		sSize, _ := m.u32(cacheBase + layout.poolCacheSmallSize)
		sRetry, _ := m.u64(cacheBase + layout.poolCacheSmallRetry)
		lCount, _ := m.u32(cacheBase + layout.poolCacheLargeCount)
		lSize, _ := m.u32(cacheBase + layout.poolCacheLargeSize)
		lRetry, _ := m.u64(cacheBase + layout.poolCacheLargeRetry)

		if modName == "nvmf_TCP" {
			hasNvmfTCP = true
			nvmfLargeCache = uint64(lCount)
		}

		info.Modules = append(info.Modules, &types.SpdkThreadIobufModuleCache{
			Module:          modName,
			SmallCacheCount: sCount,
			SmallCacheSize:  sSize,
			SmallRetry:      sRetry,
			LargeCacheCount: lCount,
			LargeCacheSize:  lSize,
			LargeRetry:      lRetry,
		})
	}

	if hasNvmfTCP && globalIobuf != nil {
		info.TotalFreeLargeBuffers = uint64(globalIobuf.LargeRingFree) + nvmfLargeCache
	}

	return info
}

func inspectBsChannel(
	m *memReader,
	ioChanAddr, ioDevAddr, ctxAddr uint64,
	devName string,
	syms *spdkSymbols,
	layout *spdkStructLayout,
	lvolIdx *lvolIndex,
	rpcSnap *rpcSnapshot,
) *types.SpdkBsChannelInfo {
	var blobMap map[uint64]lvolMeta
	if lvolIdx != nil {
		blobMap = lvolIdx.byBlobPtr
	}
	freeList := m.walkList(ctxAddr+layout.bsChanReqs, layout.bsReqSetLinkNext, bsChannelMaxReqs*2)
	needList := m.walkList(ctxAddr+layout.bsChanNeedClusterAlloc, layout.bsReqSetLinkNext, bsChannelMaxReqs*2)
	queuedList := m.walkList(ctxAddr+layout.bsChanQueuedIO, layout.bsReqSetLinkNext, bsChannelMaxReqs*2)

	freeSet := make(map[uint64]struct{}, len(freeList))
	for _, r := range freeList {
		freeSet[r] = struct{}{}
	}
	needSet := make(map[uint64]struct{}, len(needList))
	for _, r := range needList {
		needSet[r] = struct{}{}
	}
	queuedSet := make(map[uint64]struct{}, len(queuedList))
	for _, r := range queuedList {
		queuedSet[r] = struct{}{}
	}

	info := &types.SpdkBsChannelInfo{
		Device:           devName,
		Address:          fmt.Sprintf("0x%x", ioChanAddr),
		TotalReqs:        bsChannelMaxReqs,
		FreeReqs:         len(freeSet),
		NeedClusterAlloc: len(needSet),
		QueuedIO:         len(queuedSet),
	}

	if devName == "blob" {
		if blobPtr, ok := m.u64(ioDevAddr + layout.ioDevPtr); ok && blobPtr != 0 {
			info.LvolName, info.BlobID = resolveBlobIdentity(m, blobPtr, layout, blobMap)
		}
	}

	if len(needList) > 0 {
		byLvol := make(map[string]int)
		for _, opAddr := range needList {
			blobPtr, _ := m.u64(opAddr + layout.bsReqSetUserOpBlob)
			lvolName, blobID := resolveBlobIdentity(m, blobPtr, layout, blobMap)
			key := lvolName
			if key == "" {
				key = blobID
			}
			if key == "" {
				key = "unknown"
			}
			byLvol[key]++
		}
		info.NeedClusterAllocByLvol = byLvol
	}

	reqMem, ok := m.u64(ctxAddr + layout.bsChanReqMem)
	if !ok || reqMem == 0 {
		return info
	}

	var ctrlrAddrs map[string]string
	if rpcSnap != nil {
		ctrlrAddrs = rpcSnap.nvmeCtrlrAddrs
	}

	for idx := 0; idx < bsChannelMaxReqs; idx++ {
		r := reqMem + uint64(idx)*layout.bsReqSetSize
		if _, isFree := freeSet[r]; isFree {
			continue
		}
		if _, isNeed := needSet[r]; isNeed {
			continue
		}
		if _, isQueued := queuedSet[r]; isQueued {
			continue
		}

		active := inspectActiveBsRequest(m, idx, r, syms, layout, blobMap, ctrlrAddrs)
		info.ActiveRequests = append(info.ActiveRequests, active)
	}

	return info
}

func inspectActiveBsRequest(
	m *memReader,
	idx int,
	reqAddr uint64,
	syms *spdkSymbols,
	layout *spdkStructLayout,
	blobMap map[uint64]lvolMeta,
	ctrlrAddrs map[string]string,
) *types.SpdkBsActiveRequest {
	cplType, _ := m.u32(reqAddr + layout.bsReqSetCplType)
	cbFn, _ := m.u64(reqAddr + layout.bsReqSetCplCbFn)
	cbArg, _ := m.u64(reqAddr + layout.bsReqSetCplCbArg)
	backCh, _ := m.u64(reqAddr + layout.bsReqSetBackChannel)

	backingDev := ""
	if backCh != 0 {
		if bDev, ok := m.u64(backCh + layout.ioChanDev); ok && bDev != 0 {
			backingDev = m.cstr(bDev+layout.ioDevName, layout.ioDevNameSize)
		}
	}

	active := &types.SpdkBsActiveRequest{
		Index:               idx,
		OpType:              "sequence",
		BackingDevice:       backingDev,
		RemoteTargetAddress: resolveBackingDeviceTarget(backingDev, ctrlrAddrs),
	}

	expectedBlobBasic := layout.cplTypeBlobBasic
	if expectedBlobBasic == 0 {
		expectedBlobBasic = spdkBsCplTypeBlobBasic
	}
	expectedBlobHandle := layout.cplTypeBlobHandle
	if expectedBlobHandle == 0 {
		expectedBlobHandle = spdkBsCplTypeBlobHandle
	}

	if cplType == expectedBlobBasic && cbArg != 0 &&
		(syms.blobAllocCopyClusterCpl != 0 && cbFn == syms.blobAllocCopyClusterCpl) {
		active.OpType = "cluster_cow"
		if backingDev != "" && backingDev != "blob" {
			active.OpType = "esnap_cow"
		}
		blobPtr, _ := m.u64(cbArg + layout.copyCtxBlob)
		active.LvolName, active.BlobID = resolveBlobIdentity(m, blobPtr, layout, blobMap)
		active.IOUnit, _ = m.u64(cbArg + layout.copyCtxIOUnit)
		active.NewCluster, _ = m.u64(cbArg + layout.copyCtxNewCluster)
		return active
	}

	var blobPtr uint64
	if esnapBlob, _ := m.u64(reqAddr + layout.bsReqSetEsnapBlob); esnapBlob != 0 {
		blobPtr = esnapBlob
	} else if cplType == expectedBlobHandle {
		blobPtr, _ = m.u64(reqAddr + layout.bsReqSetCplBlob)
	} else {
		blobPtr, _ = m.u64(reqAddr + layout.bsReqSetUserOpBlob)
	}
	if blobPtr != 0 {
		active.LvolName, active.BlobID = resolveBlobIdentity(m, blobPtr, layout, blobMap)
	}

	return active
}

func resolveBlobIdentity(m *memReader, blobPtr uint64, layout *spdkStructLayout, blobMap map[uint64]lvolMeta) (string, string) {
	if blobPtr == 0 {
		return "", ""
	}
	if meta, ok := blobMap[blobPtr]; ok {
		return meta.name, fmt.Sprintf("0x%x", meta.blobID)
	}
	if id, ok := m.u64(blobPtr + layout.blobID); ok && id != 0 {
		return "", fmt.Sprintf("0x%x", id)
	}
	return "", ""
}

func inspectBdevMgmtChannel(m *memReader, ctxAddr uint64, layout *spdkStructLayout, lvolIdx *lvolIndex) []*types.SpdkBdevNoMemInfo {
	var out []*types.SpdkBdevNoMemInfo
	sharedResources := m.walkList(ctxAddr+layout.bdevMgmtSharedResources, layout.bdevSharedLinkNext, 4096)
	for _, sr := range sharedResources {
		nomemList := m.walkList(sr+layout.bdevSharedNoMemIO, layout.bdevIOLinkNext, 16384)
		if len(nomemList) == 0 {
			continue
		}
		ioOut, _ := m.u64(sr + layout.bdevSharedIOOutstanding)
		thresh, _ := m.u64(sr + layout.bdevSharedNoMemThresh)

		type bdevKey struct {
			name   string
			ioType string
		}
		grouped := make(map[bdevKey]*types.SpdkBdevNoMemEntry)
		for _, bio := range nomemList {
			bdevName := "unknown"
			lvolName := ""
			bdevPtr, hasBdevPtr := m.u64(bio + layout.bdevIOBdev)
			if hasBdevPtr && bdevPtr != 0 {
				if namePtr, ok := m.u64(bdevPtr + layout.bdevNamePtr); ok && namePtr != 0 {
					if s := m.cstr(namePtr, 256); s != "" {
						bdevName = s
					}
				}
				if lvolIdx != nil {
					lvolName = lvolIdx.byBdevPtr[bdevPtr]
				}
			}
			if lvolName == "" && lvolIdx != nil && bdevName != "unknown" {
				lvolName = lvolIdx.byUUID[bdevName]
			}
			rawType, _ := m.u8(bio + layout.bdevIOType)
			ioType := formatBdevIOType(rawType)
			retries, _ := m.u16(bio + layout.bdevIONumRetries)

			k := bdevKey{name: bdevName, ioType: ioType}
			entry, exists := grouped[k]
			if !exists {
				entry = &types.SpdkBdevNoMemEntry{
					BdevName: bdevName,
					LvolName: lvolName,
					IOType:   ioType,
				}
				grouped[k] = entry
			}
			entry.Count++
			if retries > entry.MaxRetries {
				entry.MaxRetries = retries
			}
		}

		entries := make([]*types.SpdkBdevNoMemEntry, 0, len(grouped))
		for _, e := range grouped {
			entries = append(entries, e)
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Count != entries[j].Count {
				return entries[i].Count > entries[j].Count
			}
			return entries[i].BdevName < entries[j].BdevName
		})

		out = append(out, &types.SpdkBdevNoMemInfo{
			IOOutstanding:  ioOut,
			NoMemThreshold: thresh,
			NoMemIOCount:   len(nomemList),
			ByBdev:         entries,
		})
	}
	return out
}

func formatBdevIOType(t uint8) string {
	switch t {
	case spdkBdevIOTypeRead:
		return "read"
	case spdkBdevIOTypeWrite:
		return "write"
	case spdkBdevIOTypeUnmap:
		return "unmap"
	case spdkBdevIOTypeFlush:
		return "flush"
	case spdkBdevIOTypeReset:
		return "reset"
	case spdkBdevIOTypeWriteZero:
		return "write_zeros"
	default:
		return fmt.Sprintf("type_%d", t)
	}
}

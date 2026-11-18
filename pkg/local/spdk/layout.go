package spdk

import (
	"bufio"
	"debug/dwarf"
	"debug/elf"
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	bsChannelMaxReqs = 512
	iobufMaxChannels = 64

	// enum spdk_bs_cpl_type: NONE(0), BS_BASIC(1), BS_HANDLE(2), BLOB_BASIC(3), BLOB_HANDLE(4)
	spdkBsCplTypeBlobBasic  = 3
	spdkBsCplTypeBlobHandle = 4

	spdkBdevIOTypeRead      = 1
	spdkBdevIOTypeWrite     = 2
	spdkBdevIOTypeUnmap     = 3
	spdkBdevIOTypeFlush     = 4
	spdkBdevIOTypeReset     = 5
	spdkBdevIOTypeWriteZero = 7
)

type spdkSymbols struct {
	gThreads                uint64
	gIobuf                  uint64
	gLvolStores             uint64
	blobAllocCopyClusterCpl uint64
	perBlobChannelSupported bool
}

type spdkStructLayout struct {
	dwarfLoaded bool

	// struct spdk_thread
	threadID         uint64
	threadIOChannels uint64
	threadTailqNext  uint64
	threadName       uint64
	threadNameSize   int

	// struct spdk_io_channel
	ioChanDev  uint64
	ioChanNode uint64 // RB_ENTRY: left(+0), right(+8)
	ioChanCtx  uint64 // sizeof(struct spdk_io_channel)

	// struct io_device
	ioDevPtr      uint64
	ioDevName     uint64
	ioDevNameSize int

	// struct iobuf / spdk_iobuf_opts / iobuf_node
	iobufSmallPoolCount uint64
	iobufLargePoolCount uint64
	iobufSmallBufSize   uint64
	iobufLargeBufSize   uint64
	iobufNode           uint64
	iobufNodeSmallPool  uint64
	iobufNodeLargePool  uint64

	// struct rte_ring
	rteRingMask     uint64
	rteRingCapacity uint64
	rteRingProdTail uint64
	rteRingConsHead uint64

	// struct iobuf_channel / spdk_iobuf_entry / spdk_iobuf_channel
	iobufChanSmallQueue  uint64
	iobufChanLargeQueue  uint64
	iobufChanChannels    uint64
	iobufEntryStailqNext uint64
	spdkIobufChanModule  uint64
	spdkIobufChanParent  uint64
	spdkIobufChanCache   uint64
	poolCacheSmallCount  uint64
	poolCacheSmallSize   uint64
	poolCacheSmallRetry  uint64
	poolCacheLargeOffset uint64
	poolCacheLargeCount  uint64
	poolCacheLargeSize   uint64
	poolCacheLargeRetry  uint64

	// struct spdk_bs_channel
	bsChanReqMem           uint64
	bsChanReqs             uint64
	bsChanNeedClusterAlloc uint64
	bsChanQueuedIO         uint64

	// struct spdk_bs_request_set
	bsReqSetSize        uint64
	bsReqSetCplType     uint64
	bsReqSetCplCbFn     uint64
	bsReqSetCplCbArg    uint64
	bsReqSetCplBlob     uint64
	bsReqSetBackChannel uint64
	bsReqSetEsnapBlob   uint64
	bsReqSetUserOpBlob  uint64
	bsReqSetLinkNext    uint64

	// enum spdk_bs_cpl_type values
	cplTypeBlobBasic  uint32
	cplTypeBlobHandle uint32

	// struct spdk_blob_copy_cluster_ctx
	copyCtxBlob       uint64
	copyCtxIOUnit     uint64
	copyCtxNewCluster uint64

	// struct spdk_blob
	blobID uint64

	// struct spdk_lvol_store & spdk_lvol
	lvsLvols        uint64
	lvsLinkNext     uint64
	lvolBlob        uint64
	lvolBlobID      uint64
	lvolUUIDStr     uint64
	lvolUUIDStrSize int
	lvolName        uint64
	lvolNameSize    int
	lvolBdev        uint64
	lvolLinkNext    uint64

	// struct spdk_bdev_mgmt_channel / spdk_bdev_shared_resource / spdk_bdev_io / spdk_bdev
	bdevMgmtSharedResources uint64
	bdevSharedIOOutstanding uint64
	bdevSharedNoMemIO       uint64
	bdevSharedNoMemThresh   uint64
	bdevSharedLinkNext      uint64
	bdevIOBdev              uint64
	bdevIOType              uint64
	bdevIONumRetries        uint64
	bdevIOLinkNext          uint64
	bdevNamePtr             uint64
}

func defaultSpdkStructLayout(perBlobSupported bool) *spdkStructLayout {
	needClusterAllocOff := uint64(56)
	queuedIOOff := uint64(72)
	if perBlobSupported {
		needClusterAllocOff = 96
		queuedIOOff = 112
	}

	return &spdkStructLayout{
		threadID:         176,
		threadIOChannels: 208,
		threadTailqNext:  216,
		threadName:       232,
		threadNameSize:   257,

		ioChanDev:  8,
		ioChanNode: 24,
		ioChanCtx:  96,

		ioDevPtr:      0,
		ioDevName:     8,
		ioDevNameSize: 257,

		iobufSmallPoolCount: 0,
		iobufLargePoolCount: 8,
		iobufSmallBufSize:   16,
		iobufLargeBufSize:   20,
		iobufNode:           72,
		iobufNodeSmallPool:  0,
		iobufNodeLargePool:  8,

		rteRingMask:     52,
		rteRingCapacity: 56,
		rteRingProdTail: 132,
		rteRingConsHead: 256,

		iobufChanSmallQueue:  0,
		iobufChanLargeQueue:  16,
		iobufChanChannels:    32,
		iobufEntryStailqNext: 16,
		spdkIobufChanModule:  0,
		spdkIobufChanParent:  8,
		spdkIobufChanCache:   16,
		poolCacheSmallCount:  24,
		poolCacheSmallSize:   28,
		poolCacheSmallRetry:  72,
		poolCacheLargeOffset: 80,
		poolCacheLargeCount:  104,
		poolCacheLargeSize:   108,
		poolCacheLargeRetry:  152,

		bsChanReqMem:           0,
		bsChanReqs:             8,
		bsChanNeedClusterAlloc: needClusterAllocOff,
		bsChanQueuedIO:         queuedIOOff,

		bsReqSetSize:        192,
		bsReqSetCplType:     0,
		bsReqSetCplCbFn:     8,
		bsReqSetCplCbArg:    16,
		bsReqSetCplBlob:     24,
		bsReqSetBackChannel: 56,
		bsReqSetEsnapBlob:   72,
		bsReqSetUserOpBlob:  120,
		bsReqSetLinkNext:    176,

		cplTypeBlobBasic:  spdkBsCplTypeBlobBasic,
		cplTypeBlobHandle: spdkBsCplTypeBlobHandle,

		copyCtxBlob:       0,
		copyCtxIOUnit:     16,
		copyCtxNewCluster: 24,

		blobID: 16,

		lvsLvols:        56,
		lvsLinkNext:     112,
		lvolBlob:        8,
		lvolBlobID:      16,
		lvolUUIDStr:     24,
		lvolUUIDStrSize: 37,
		lvolName:        82,
		lvolNameSize:    256,
		lvolBdev:        416,
		lvolLinkNext:    440,

		bdevMgmtSharedResources: 200,
		bdevSharedIOOutstanding: 8,
		bdevSharedNoMemIO:       16,
		bdevSharedNoMemThresh:   32,
		bdevSharedLinkNext:      72,
		bdevIOBdev:              0,
		bdevIOType:              8,
		bdevIONumRetries:        10,
		bdevIOLinkNext:          1032,
		bdevNamePtr:             8,
	}
}

func loadELFMetadata(exePath, mapsPath string) (*spdkSymbols, *spdkStructLayout, error) {
	ef, err := elf.Open(exePath)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to open ELF binary %s", exePath)
	}
	defer func() {
		_ = ef.Close()
	}()

	var loadBias uint64
	if ef.Type == elf.ET_DYN && mapsPath != "" {
		bias, biasErr := resolvePIELoadBias(ef, mapsPath)
		if biasErr == nil {
			loadBias = bias
		}
	}

	syms := &spdkSymbols{}
	elfSyms, err := ef.Symbols()
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to read symbol table from %s", exePath)
	}
	for _, s := range elfSyms {
		if s.Value == 0 {
			continue
		}
		switch s.Name {
		case "g_threads":
			syms.gThreads = loadBias + s.Value
		case "g_iobuf":
			syms.gIobuf = loadBias + s.Value
		case "g_lvol_stores":
			syms.gLvolStores = loadBias + s.Value
		case "blob_allocate_and_copy_cluster_cpl":
			syms.blobAllocCopyClusterCpl = loadBias + s.Value
		case "spdk_blob_alloc_io_channel":
			syms.perBlobChannelSupported = true
		}
	}
	if syms.gThreads == 0 {
		return nil, nil, errors.Errorf("symbol g_threads not found in %s", exePath)
	}

	layout := defaultSpdkStructLayout(syms.perBlobChannelSupported)
	if dw, dwErr := ef.DWARF(); dwErr == nil && dw != nil {
		applyDWARFLayout(dw, layout)
	}

	return syms, layout, nil
}

func resolvePIELoadBias(ef *elf.File, mapsPath string) (uint64, error) {
	var minLoadVaddr uint64
	foundLoad := false
	for _, prog := range ef.Progs {
		if prog.Type == elf.PT_LOAD {
			if !foundLoad || prog.Vaddr < minLoadVaddr {
				minLoadVaddr = prog.Vaddr
				foundLoad = true
			}
		}
	}
	if !foundLoad {
		return 0, errors.New("no PT_LOAD segment in ELF")
	}

	f, err := os.Open(mapsPath)
	if err != nil {
		return 0, err
	}
	defer func() {
		_ = f.Close()
	}()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		if !strings.HasSuffix(fields[5], "/spdk_tgt") && !strings.Contains(fields[5], "/spdk_tgt ") {
			continue
		}
		offset, err := strconv.ParseUint(fields[2], 16, 64)
		if err != nil || offset != 0 {
			continue
		}
		addrParts := strings.SplitN(fields[0], "-", 2)
		if len(addrParts) != 2 {
			continue
		}
		startAddr, err := strconv.ParseUint(addrParts[0], 16, 64)
		if err != nil {
			continue
		}
		return startAddr - minLoadVaddr, nil
	}
	return 0, errors.New("spdk_tgt base mapping not found")
}

type dwarfStructInfo struct {
	size   uint64
	fields map[string]uint64
}

var targetDWARFStructs = map[string]struct{}{
	"spdk_thread":                {},
	"spdk_io_channel":            {},
	"io_device":                  {},
	"iobuf":                      {},
	"spdk_iobuf_opts":            {},
	"iobuf_node":                 {},
	"rte_ring":                   {},
	"rte_ring_headtail":          {},
	"iobuf_channel":              {},
	"iobuf_channel_node":         {},
	"spdk_iobuf_entry":           {},
	"spdk_iobuf_channel":         {},
	"spdk_iobuf_node_cache":      {},
	"spdk_iobuf_pool_cache":      {},
	"spdk_iobuf_pool_stats":      {},
	"spdk_bs_channel":            {},
	"spdk_bs_request_set":        {},
	"spdk_bs_user_op_args":       {},
	"spdk_blob_copy_cluster_ctx": {},
	"spdk_blob":                  {},
	"spdk_lvol_store":            {},
	"spdk_lvol":                  {},
	"spdk_bdev_mgmt_channel":     {},
	"spdk_bdev_shared_resource":  {},
	"spdk_bdev_io":               {},
	"spdk_bdev":                  {},
}

func applyDWARFLayout(dw *dwarf.Data, layout *spdkStructLayout) {
	found := make(map[string]*dwarfStructInfo, len(targetDWARFStructs))
	cplEnumLoaded := false
	reader := dw.Reader()

	for {
		entry, err := reader.Next()
		if err != nil || entry == nil {
			break
		}
		if entry.Tag == dwarf.TagCompileUnit || entry.Tag == dwarf.TagPartialUnit {
			continue
		}
		if entry.Tag == dwarf.TagEnumerationType {
			enumName, _ := entry.Val(dwarf.AttrName).(string)
			if enumName == "spdk_bs_cpl_type" && entry.Children && !cplEnumLoaded {
				readCplTypeEnum(reader, layout)
				cplEnumLoaded = true
				continue
			}
			if entry.Children {
				reader.SkipChildren()
			}
			continue
		}
		if entry.Tag != dwarf.TagStructType {
			if entry.Children {
				reader.SkipChildren()
			}
			continue
		}

		name, _ := entry.Val(dwarf.AttrName).(string)
		if _, wanted := targetDWARFStructs[name]; !wanted || entry.Val(dwarf.AttrDeclaration) != nil || !entry.Children {
			if entry.Children {
				reader.SkipChildren()
			}
			continue
		}
		if _, already := found[name]; already {
			reader.SkipChildren()
			continue
		}

		byteSize, _ := dwarfInt64(entry.Val(dwarf.AttrByteSize))
		info := &dwarfStructInfo{
			size:   uint64(byteSize),
			fields: make(map[string]uint64),
		}
		readStructMembers(dw, reader, 0, info.fields, 0)
		found[name] = info

		if len(found) == len(targetDWARFStructs) && cplEnumLoaded {
			break
		}
	}

	if len(found) == 0 {
		return
	}
	layout.dwarfLoaded = true

	setField := func(structName, fieldName string, dst *uint64) {
		if st, ok := found[structName]; ok {
			if off, ok := st.fields[fieldName]; ok {
				*dst = off
			}
		}
	}

	setField("spdk_thread", "id", &layout.threadID)
	setField("spdk_thread", "io_channels", &layout.threadIOChannels)
	setField("spdk_thread", "tailq", &layout.threadTailqNext)
	setField("spdk_thread", "name", &layout.threadName)

	if st, ok := found["spdk_io_channel"]; ok {
		if st.size > 0 {
			layout.ioChanCtx = st.size
		}
		setField("spdk_io_channel", "dev", &layout.ioChanDev)
		setField("spdk_io_channel", "node", &layout.ioChanNode)
	}

	setField("io_device", "io_device", &layout.ioDevPtr)
	setField("io_device", "name", &layout.ioDevName)

	var iobufOptsOff uint64
	setField("iobuf", "opts", &iobufOptsOff)
	setField("iobuf", "node", &layout.iobufNode)
	if optsSt, ok := found["spdk_iobuf_opts"]; ok {
		if off, ok := optsSt.fields["small_pool_count"]; ok {
			layout.iobufSmallPoolCount = iobufOptsOff + off
		}
		if off, ok := optsSt.fields["large_pool_count"]; ok {
			layout.iobufLargePoolCount = iobufOptsOff + off
		}
		if off, ok := optsSt.fields["small_bufsize"]; ok {
			layout.iobufSmallBufSize = iobufOptsOff + off
		}
		if off, ok := optsSt.fields["large_bufsize"]; ok {
			layout.iobufLargeBufSize = iobufOptsOff + off
		}
	}
	setField("iobuf_node", "small_pool", &layout.iobufNodeSmallPool)
	setField("iobuf_node", "large_pool", &layout.iobufNodeLargePool)

	if ringSt, ok := found["rte_ring"]; ok {
		setField("rte_ring", "mask", &layout.rteRingMask)
		setField("rte_ring", "capacity", &layout.rteRingCapacity)
		tailInHT := uint64(4)
		headInHT := uint64(0)
		if htSt, ok := found["rte_ring_headtail"]; ok {
			if off, ok := htSt.fields["tail"]; ok {
				tailInHT = off
			}
			if off, ok := htSt.fields["head"]; ok {
				headInHT = off
			}
		}
		if prodOff, ok := ringSt.fields["prod"]; ok {
			layout.rteRingProdTail = prodOff + tailInHT
		}
		if consOff, ok := ringSt.fields["cons"]; ok {
			layout.rteRingConsHead = consOff + headInHT
		}
	}

	var chanNodeOff uint64
	setField("iobuf_channel", "node", &chanNodeOff)
	setField("iobuf_channel", "channels", &layout.iobufChanChannels)
	if cnSt, ok := found["iobuf_channel_node"]; ok {
		if off, ok := cnSt.fields["small_queue"]; ok {
			layout.iobufChanSmallQueue = chanNodeOff + off
		}
		if off, ok := cnSt.fields["large_queue"]; ok {
			layout.iobufChanLargeQueue = chanNodeOff + off
		}
	}
	setField("spdk_iobuf_entry", "stailq", &layout.iobufEntryStailqNext)
	setField("spdk_iobuf_channel", "module", &layout.spdkIobufChanModule)
	setField("spdk_iobuf_channel", "parent", &layout.spdkIobufChanParent)
	setField("spdk_iobuf_channel", "cache", &layout.spdkIobufChanCache)

	if ncSt, ok := found["spdk_iobuf_node_cache"]; ok {
		smallOff := ncSt.fields["small"]
		largeOff, hasLarge := ncSt.fields["large"]
		if hasLarge {
			layout.poolCacheLargeOffset = largeOff
		}
		if pcSt, ok := found["spdk_iobuf_pool_cache"]; ok {
			if off, ok := pcSt.fields["cache_count"]; ok {
				layout.poolCacheSmallCount = smallOff + off
				layout.poolCacheLargeCount = layout.poolCacheLargeOffset + off
			}
			if off, ok := pcSt.fields["cache_size"]; ok {
				layout.poolCacheSmallSize = smallOff + off
				layout.poolCacheLargeSize = layout.poolCacheLargeOffset + off
			}
			if statsOff, ok := pcSt.fields["stats"]; ok {
				if psSt, ok := found["spdk_iobuf_pool_stats"]; ok {
					if retryOff, ok := psSt.fields["retry"]; ok {
						layout.poolCacheSmallRetry = smallOff + statsOff + retryOff
						layout.poolCacheLargeRetry = layout.poolCacheLargeOffset + statsOff + retryOff
					}
				}
			}
		}
	}

	setField("spdk_bs_channel", "req_mem", &layout.bsChanReqMem)
	setField("spdk_bs_channel", "reqs", &layout.bsChanReqs)
	setField("spdk_bs_channel", "need_cluster_alloc", &layout.bsChanNeedClusterAlloc)
	setField("spdk_bs_channel", "queued_io", &layout.bsChanQueuedIO)

	if rsSt, ok := found["spdk_bs_request_set"]; ok {
		if rsSt.size > 0 {
			layout.bsReqSetSize = rsSt.size
		}
		setField("spdk_bs_request_set", "back_channel", &layout.bsReqSetBackChannel)
		setField("spdk_bs_request_set", "esnap_blob", &layout.bsReqSetEsnapBlob)
		setField("spdk_bs_request_set", "link", &layout.bsReqSetLinkNext)
		if uOff, ok := rsSt.fields["u"]; ok {
			if uoSt, ok := found["spdk_bs_user_op_args"]; ok {
				if blobOff, ok := uoSt.fields["blob"]; ok {
					layout.bsReqSetUserOpBlob = uOff + blobOff
				}
			}
		}
	}

	setField("spdk_blob_copy_cluster_ctx", "blob", &layout.copyCtxBlob)
	setField("spdk_blob_copy_cluster_ctx", "io_unit", &layout.copyCtxIOUnit)
	setField("spdk_blob_copy_cluster_ctx", "new_cluster", &layout.copyCtxNewCluster)

	setField("spdk_blob", "id", &layout.blobID)

	setField("spdk_lvol_store", "lvols", &layout.lvsLvols)
	setField("spdk_lvol_store", "link", &layout.lvsLinkNext)
	setField("spdk_lvol", "blob", &layout.lvolBlob)
	setField("spdk_lvol", "blob_id", &layout.lvolBlobID)
	setField("spdk_lvol", "uuid_str", &layout.lvolUUIDStr)
	setField("spdk_lvol", "name", &layout.lvolName)
	setField("spdk_lvol", "bdev", &layout.lvolBdev)
	setField("spdk_lvol", "link", &layout.lvolLinkNext)

	setField("spdk_bdev_mgmt_channel", "shared_resources", &layout.bdevMgmtSharedResources)
	setField("spdk_bdev_shared_resource", "io_outstanding", &layout.bdevSharedIOOutstanding)
	setField("spdk_bdev_shared_resource", "nomem_io", &layout.bdevSharedNoMemIO)
	setField("spdk_bdev_shared_resource", "nomem_threshold", &layout.bdevSharedNoMemThresh)
	setField("spdk_bdev_shared_resource", "link", &layout.bdevSharedLinkNext)

	if bioSt, ok := found["spdk_bdev_io"]; ok {
		setField("spdk_bdev_io", "bdev", &layout.bdevIOBdev)
		setField("spdk_bdev_io", "type", &layout.bdevIOType)
		setField("spdk_bdev_io", "num_retries", &layout.bdevIONumRetries)
		if internalOff, ok := bioSt.fields["internal.link"]; ok {
			layout.bdevIOLinkNext = internalOff
		}
	}
	setField("spdk_bdev", "name", &layout.bdevNamePtr)
}

func readCplTypeEnum(reader *dwarf.Reader, layout *spdkStructLayout) {
	for {
		child, err := reader.Next()
		if err != nil || child == nil || child.Tag == 0 {
			break
		}
		if child.Tag != dwarf.TagEnumerator {
			if child.Children {
				reader.SkipChildren()
			}
			continue
		}
		name, _ := child.Val(dwarf.AttrName).(string)
		val, ok := dwarfInt64(child.Val(dwarf.AttrConstValue))
		if ok && val >= 0 {
			switch name {
			case "SPDK_BS_CPL_TYPE_BLOB_BASIC":
				layout.cplTypeBlobBasic = uint32(val)
			case "SPDK_BS_CPL_TYPE_BLOB_HANDLE":
				layout.cplTypeBlobHandle = uint32(val)
			}
		}
		if child.Children {
			reader.SkipChildren()
		}
	}
}

func readStructMembers(dw *dwarf.Data, reader *dwarf.Reader, baseOffset uint64, fields map[string]uint64, depth int) {
	if depth > 4 {
		return
	}
	for {
		child, err := reader.Next()
		if err != nil || child == nil || child.Tag == 0 {
			break
		}
		if child.Tag != dwarf.TagMember {
			if child.Children {
				reader.SkipChildren()
			}
			continue
		}

		loc, _ := dwarfMemberOffset(child.Val(dwarf.AttrDataMemberLoc))
		memberOff := baseOffset + loc
		memberName, _ := child.Val(dwarf.AttrName).(string)

		if child.Children {
			reader.SkipChildren()
		}

		if memberName != "" {
			if _, exists := fields[memberName]; !exists {
				fields[memberName] = memberOff
			}
			// Flatten spdk_bdev_io.internal.link so we can read nomem_io's TAILQ link offset directly.
			if memberName == "internal" {
				if typeOff, ok := child.Val(dwarf.AttrType).(dwarf.Offset); ok {
					subFields := make(map[string]uint64)
					flattenTypeAtOffset(dw, typeOff, memberOff, subFields, depth+1)
					for k, v := range subFields {
						fields["internal."+k] = v
					}
				}
			}
			continue
		}

		// Anonymous struct or union member: flatten its fields into the parent struct.
		if typeOff, ok := child.Val(dwarf.AttrType).(dwarf.Offset); ok {
			flattenTypeAtOffset(dw, typeOff, memberOff, fields, depth+1)
		}
	}
}

func flattenTypeAtOffset(dw *dwarf.Data, typeOff dwarf.Offset, baseOffset uint64, fields map[string]uint64, depth int) {
	if depth > 4 {
		return
	}
	sub := dw.Reader()
	sub.Seek(typeOff)
	entry, err := sub.Next()
	if err != nil || entry == nil {
		return
	}
	for entry != nil && (entry.Tag == dwarf.TagTypedef || entry.Tag == dwarf.TagConstType || entry.Tag == dwarf.TagVolatileType) {
		nextOff, ok := entry.Val(dwarf.AttrType).(dwarf.Offset)
		if !ok {
			return
		}
		sub.Seek(nextOff)
		entry, err = sub.Next()
		if err != nil {
			return
		}
	}
	if entry == nil || (entry.Tag != dwarf.TagStructType && entry.Tag != dwarf.TagUnionType) || !entry.Children {
		return
	}
	readStructMembers(dw, sub, baseOffset, fields, depth)
}

func dwarfInt64(v any) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case uint64:
		return int64(val), true
	default:
		return 0, false
	}
}

func dwarfMemberOffset(v any) (uint64, bool) {
	switch val := v.(type) {
	case int64:
		return uint64(val), true
	case uint64:
		return val, true
	case []byte:
		// DWARF2/3 DW_OP_plus_uconst (0x23) followed by ULEB128 offset.
		if len(val) >= 2 && val[0] == 0x23 {
			var res uint64
			var shift uint
			for _, b := range val[1:] {
				res |= uint64(b&0x7f) << shift
				if b&0x80 == 0 {
					return res, true
				}
				shift += 7
			}
		}
		return 0, false
	default:
		return 0, false
	}
}

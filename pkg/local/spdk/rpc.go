package spdk

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/longhorn/cli/pkg/types"
)

const (
	rpcDialTimeout = 2 * time.Second
	rpcCallTimeout = 3 * time.Second
)

type threadPollerSnapshot struct {
	bdevNoMemPollerCount int
	activePollers        []string
	timedPollers         []string
}

type rpcSnapshot struct {
	version        string
	iobufStats     []*types.SpdkIobufModuleStat
	nvmfPollGroups []*types.SpdkNvmfPollGroupStat
	threadPollers  map[string]*threadPollerSnapshot
	nvmeCtrlrAddrs map[string]string
	warnings       []string
}

type jsonRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
}

type jsonRPCResponse struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func callSpdkRPC(sockPath, method string, out any) error {
	conn, err := net.DialTimeout("unix", sockPath, rpcDialTimeout)
	if err != nil {
		return err
	}
	defer func() {
		_ = conn.Close()
	}()

	if err := conn.SetDeadline(time.Now().Add(rpcCallTimeout)); err != nil {
		return err
	}

	reqBytes, err := json.Marshal(jsonRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  method,
	})
	if err != nil {
		return err
	}
	reqBytes = append(reqBytes, '\n')
	if _, err := conn.Write(reqBytes); err != nil {
		return err
	}

	dec := json.NewDecoder(conn)
	var resp jsonRPCResponse
	if err := dec.Decode(&resp); err != nil {
		return err
	}
	if resp.Error != nil {
		return errors.Errorf("RPC %s error %d: %s", method, resp.Error.Code, resp.Error.Message)
	}
	return json.Unmarshal(resp.Result, out)
}

func collectSpdkRPC(sockPath string) *rpcSnapshot {
	snap := &rpcSnapshot{
		threadPollers:  make(map[string]*threadPollerSnapshot),
		nvmeCtrlrAddrs: make(map[string]string),
	}

	var ver struct {
		Version string `json:"version"`
	}
	if err := callSpdkRPC(sockPath, "spdk_get_version", &ver); err != nil {
		snap.warnings = append(snap.warnings, fmt.Sprintf("SPDK RPC unavailable at %s: %v", sockPath, err))
		return snap
	}
	snap.version = ver.Version

	var rawIobuf []struct {
		Module    string `json:"module"`
		SmallPool struct {
			CacheSize uint32 `json:"cache_size"`
			Cache     uint64 `json:"cache"`
			Main      uint64 `json:"main"`
			Retry     uint64 `json:"retry"`
		} `json:"small_pool"`
		LargePool struct {
			CacheSize uint32 `json:"cache_size"`
			Cache     uint64 `json:"cache"`
			Main      uint64 `json:"main"`
			Retry     uint64 `json:"retry"`
		} `json:"large_pool"`
	}
	if err := callSpdkRPC(sockPath, "iobuf_get_stats", &rawIobuf); err != nil {
		snap.warnings = append(snap.warnings, fmt.Sprintf("RPC iobuf_get_stats failed: %v", err))
	} else {
		for _, item := range rawIobuf {
			snap.iobufStats = append(snap.iobufStats, &types.SpdkIobufModuleStat{
				Module: item.Module,
				SmallPool: types.SpdkIobufPoolStat{
					CacheSize: item.SmallPool.CacheSize,
					Cache:     item.SmallPool.Cache,
					Main:      item.SmallPool.Main,
					Retry:     item.SmallPool.Retry,
				},
				LargePool: types.SpdkIobufPoolStat{
					CacheSize: item.LargePool.CacheSize,
					Cache:     item.LargePool.Cache,
					Main:      item.LargePool.Main,
					Retry:     item.LargePool.Retry,
				},
			})
		}
	}

	var rawNvmf struct {
		PollGroups []struct {
			Name               string `json:"name"`
			CurrentAdminQpairs uint32 `json:"current_admin_qpairs"`
			CurrentIOQpairs    uint32 `json:"current_io_qpairs"`
			PendingBdevIO      uint64 `json:"pending_bdev_io"`
			CompletedNvmeIO    uint64 `json:"completed_nvme_io"`
		} `json:"poll_groups"`
	}
	if err := callSpdkRPC(sockPath, "nvmf_get_stats", &rawNvmf); err != nil {
		snap.warnings = append(snap.warnings, fmt.Sprintf("RPC nvmf_get_stats failed: %v", err))
	} else {
		for _, pg := range rawNvmf.PollGroups {
			snap.nvmfPollGroups = append(snap.nvmfPollGroups, &types.SpdkNvmfPollGroupStat{
				Name:               pg.Name,
				CurrentAdminQpairs: pg.CurrentAdminQpairs,
				CurrentIOQpairs:    pg.CurrentIOQpairs,
				PendingBdevIO:      pg.PendingBdevIO,
				CompletedNvmeIO:    pg.CompletedNvmeIO,
			})
		}
	}

	var rawPollers struct {
		Threads []struct {
			Name          string `json:"name"`
			ActivePollers []struct {
				Name string `json:"name"`
			} `json:"active_pollers"`
			TimedPollers []struct {
				Name string `json:"name"`
			} `json:"timed_pollers"`
		} `json:"threads"`
	}
	if err := callSpdkRPC(sockPath, "thread_get_pollers", &rawPollers); err != nil {
		snap.warnings = append(snap.warnings, fmt.Sprintf("RPC thread_get_pollers failed: %v", err))
	} else {
		for _, th := range rawPollers.Threads {
			ps := &threadPollerSnapshot{}
			for _, ap := range th.ActivePollers {
				ps.activePollers = append(ps.activePollers, ap.Name)
			}
			for _, tp := range th.TimedPollers {
				ps.timedPollers = append(ps.timedPollers, tp.Name)
				if tp.Name == "bdev_no_mem_poller" {
					ps.bdevNoMemPollerCount++
				}
			}
			snap.threadPollers[th.Name] = ps
		}
	}

	var rawCtrlrs []struct {
		Name   string `json:"name"`
		Ctrlrs []struct {
			Trid struct {
				Traddr  string `json:"traddr"`
				Trsvcid string `json:"trsvcid"`
			} `json:"trid"`
		} `json:"ctrlrs"`
	}
	if err := callSpdkRPC(sockPath, "bdev_nvme_get_controllers", &rawCtrlrs); err == nil {
		for _, c := range rawCtrlrs {
			for _, sub := range c.Ctrlrs {
				if sub.Trid.Traddr != "" {
					addr := sub.Trid.Traddr
					if sub.Trid.Trsvcid != "" {
						addr = fmt.Sprintf("%s:%s", sub.Trid.Traddr, sub.Trid.Trsvcid)
					}
					snap.nvmeCtrlrAddrs[c.Name] = addr
					break
				}
			}
		}
	}

	return snap
}

func resolveBackingDeviceTarget(backingDev string, ctrlrAddrs map[string]string) string {
	if backingDev == "" || len(ctrlrAddrs) == 0 {
		return ""
	}
	trimmed := strings.TrimPrefix(backingDev, "bdev_")
	if addr, ok := ctrlrAddrs[trimmed]; ok {
		return addr
	}
	// Strip NVMe namespace suffix such as "n1" in "bdev_<ctrlrName>n1".
	if idx := strings.LastIndexByte(trimmed, 'n'); idx > 0 {
		allDigits := true
		for _, r := range trimmed[idx+1:] {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			if addr, ok := ctrlrAddrs[trimmed[:idx]]; ok {
				return addr
			}
		}
	}
	return ""
}

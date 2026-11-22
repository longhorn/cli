package spdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

// Diagnoser collects live SPDK memory, iobuf, blobstore channel, and RPC diagnostics on a node.
type Diagnoser struct {
	LogLevel string

	CurrentNodeID  string
	OutputFilePath string
	ProcDir        string
	SpdkSocketPath string
	TargetPID      int
	IncludeAll     bool

	logger   *logrus.Entry
	nodeInfo types.SpdkNodeInfo
}

// Init initializes the local SPDK Diagnoser.
func (local *Diagnoser) Init() error {
	if local.OutputFilePath != "" {
		local.logger = logrus.WithField("output", local.OutputFilePath)
	} else {
		local.logger = logrus.WithField("output", "stdout")
	}

	if local.ProcDir == "" {
		local.ProcDir = consts.DefaultProcDirectory
	}
	if local.SpdkSocketPath == "" {
		local.SpdkSocketPath = consts.DefaultSpdkSocketPath
	}

	local.nodeInfo = types.SpdkNodeInfo{
		Node:      local.CurrentNodeID,
		Processes: []*types.SpdkProcessInfo{},
	}
	return nil
}

// Run discovers spdk_tgt processes and inspects their RPC state and /proc/<pid>/mem structures.
func (local *Diagnoser) Run() error {
	pids, err := local.discoverSpdkPIDs()
	if err != nil {
		return err
	}
	if len(pids) == 0 {
		local.nodeInfo.Warnings = append(local.nodeInfo.Warnings,
			fmt.Sprintf("no running spdk_tgt process found under %s", local.ProcDir))
		return nil
	}

	for _, pid := range pids {
		procInfo := local.inspectPID(pid)
		local.nodeInfo.Processes = append(local.nodeInfo.Processes, procInfo)
	}
	return nil
}

// Output writes the collected SpdkNodeInfo JSON to OutputFilePath or stdout.
func (local *Diagnoser) Output() error {
	jsonBytes, err := json.MarshalIndent(local.nodeInfo, "", "  ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal SPDK diagnostics to JSON")
	}
	return utils.HandleResult(jsonBytes, local.OutputFilePath, local.logger)
}

func (local *Diagnoser) discoverSpdkPIDs() ([]int, error) {
	if local.TargetPID > 0 {
		memPath := filepath.Join(local.ProcDir, strconv.Itoa(local.TargetPID), "mem")
		if _, err := os.Stat(memPath); err != nil {
			return nil, errors.Wrapf(err, "target SPDK PID %d not accessible at %s", local.TargetPID, memPath)
		}
		return []int{local.TargetPID}, nil
	}

	entries, err := os.ReadDir(local.ProcDir)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read proc directory %s", local.ProcDir)
	}

	var pids []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if local.isSpdkTargetProcess(pid) {
			pids = append(pids, pid)
		}
	}
	sort.Ints(pids)
	return pids, nil
}

func (local *Diagnoser) isSpdkTargetProcess(pid int) bool {
	pidDir := filepath.Join(local.ProcDir, strconv.Itoa(pid))
	if exeTarget, err := os.Readlink(filepath.Join(pidDir, "exe")); err == nil {
		cleanExe := strings.TrimSuffix(exeTarget, " (deleted)")
		if filepath.Base(cleanExe) == "spdk_tgt" {
			return true
		}
	}

	rawCmdline, err := os.ReadFile(filepath.Join(pidDir, "cmdline"))
	if err != nil || len(rawCmdline) == 0 {
		return false
	}
	args := bytes.Split(bytes.TrimRight(rawCmdline, "\x00"), []byte{0})
	if len(args) == 0 {
		return false
	}
	return filepath.Base(string(args[0])) == "spdk_tgt"
}

func (local *Diagnoser) inspectPID(pid int) *types.SpdkProcessInfo {
	pidDir := filepath.Join(local.ProcDir, strconv.Itoa(pid))
	exePath := filepath.Join(pidDir, "exe")
	mapsPath := filepath.Join(pidDir, "maps")
	memPath := filepath.Join(pidDir, "mem")

	procInfo := &types.SpdkProcessInfo{
		PID:     pid,
		Cmdline: readFormattedCmdline(filepath.Join(pidDir, "cmdline")),
	}
	if exeTarget, err := os.Readlink(exePath); err == nil {
		procInfo.Executable = exeTarget
	}

	sockPath := local.resolveSocketPath(pidDir)
	rpcSnap := collectSpdkRPC(sockPath)
	procInfo.Version = rpcSnap.version
	procInfo.NvmfPollGroups = rpcSnap.nvmfPollGroups
	procInfo.Warnings = append(procInfo.Warnings, rpcSnap.warnings...)

	syms, layout, err := loadELFMetadata(exePath, mapsPath)
	if err != nil {
		procInfo.Errors = append(procInfo.Errors, err.Error())
		return procInfo
	}
	procInfo.PerBlobChannelSupported = syms.perBlobChannelSupported
	procInfo.DWARFLoaded = layout.dwarfLoaded

	memFile, err := os.Open(memPath)
	if err != nil {
		procInfo.Errors = append(procInfo.Errors, errors.Wrapf(err, "failed to open %s", memPath).Error())
		return procInfo
	}
	defer func() {
		_ = memFile.Close()
	}()

	inspectProcessMemory(memFile, syms, layout, rpcSnap, local.IncludeAll, procInfo)
	return procInfo
}

func (local *Diagnoser) resolveSocketPath(pidDir string) string {
	nsRootSock := filepath.Join(pidDir, "root", strings.TrimPrefix(local.SpdkSocketPath, "/"))
	if _, err := os.Stat(nsRootSock); err == nil {
		return nsRootSock
	}
	return local.SpdkSocketPath
}

func readFormattedCmdline(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return ""
	}
	parts := bytes.Split(bytes.TrimRight(raw, "\x00"), []byte{0})
	strs := make([]string, 0, len(parts))
	for _, p := range parts {
		strs = append(strs, string(p))
	}
	return strings.Join(strs, " ")
}

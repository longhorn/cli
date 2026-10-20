package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	commontypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/cli/pkg/consts"
	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
)

// rdmaBaseModules are the kernel modules SPDK needs to create the NVMe-oF RDMA transport.
var rdmaBaseModules = []string{"ib_core", "ib_uverbs", "rdma_cm", "rdma_ucm"}

// rdmaProviderModules maps a NIC kernel driver to the module that registers its RDMA device.
var rdmaProviderModules = map[string]string{
	"mlx5_core": "mlx5_ib",
	"mlx4_core": "mlx4_ib",
	"bnxt_en":   "bnxt_re",
	"ice":       "irdma",
	"i40e":      "irdma",
	"qede":      "qedr",
}

// Per-reactor iobuf pool sizes that kept an RDMA engine node free of iobuf retries: with RDMA the
// engine node's target runs two transports (TCP frontend, RDMA local replica), and each takes a
// per-poll-group cache. 8192/1024 at 2 reactors and 32768/4096 at 8 reactors were both clean.
const (
	rdmaIobufSmallPerReactor = 4096
	rdmaIobufLargePerReactor = 512
)

const (
	settingNameDataEngineCPUMask        = "data-engine-cpu-mask"
	settingNameDataEngineIobufSmallPool = "data-engine-iobuf-small-pool-size"
	settingNameDataEngineIobufLargePool = "data-engine-iobuf-large-pool-size"
)

// rdmaHostStateScript prints the host RDMA state, one record per line:
//
//	netdrv <netdev> <driver>
//	port <ibdev> <port> <state> <link_layer>
//	rocev2 <ibdev> <port> <gid index> <netdev>
//	devnode <path>
//	netns <ib_core netns_mode>
const rdmaHostStateScript = `
for n in /sys/class/net/*; do
  d=$(readlink "$n/device/driver" 2>/dev/null) && echo "netdrv ${n##*/} ${d##*/}"
done
for dev in /sys/class/infiniband/*; do
  [ -d "$dev" ] || continue
  for p in "$dev"/ports/*; do
    state=$(awk '{print $2}' "$p/state" 2>/dev/null)
    echo "port ${dev##*/} ${p##*/} ${state:-UNKNOWN} $(cat "$p/link_layer" 2>/dev/null)"
    for g in "$p"/gid_attrs/types/*; do
      t=$(cat "$g" 2>/dev/null) || continue
      [ "$t" = "RoCE v2" ] || continue
      i=${g##*/}
      nd=$(cat "$p/gid_attrs/ndevs/$i" 2>/dev/null) || continue
      echo "rocev2 ${dev##*/} ${p##*/} $i $nd"
    done
  done
done
for f in /dev/infiniband/uverbs* /dev/infiniband/rdma_cm; do
  [ -e "$f" ] && echo "devnode $f"
done
echo "netns $(cat /sys/module/ib_core/parameters/netns_mode 2>/dev/null)"
`

type rdmaPort struct {
	device    string
	port      string
	state     string
	linkLayer string
}

type rdmaHostState struct {
	netDrivers map[string]string   // netdev -> kernel driver
	ports      []rdmaPort          // every port of every RDMA device
	roceV2     map[string][]string // "<ibdev>/<port>" -> netdevs that own a RoCE v2 GID
	devNodes   []string
	netnsMode  string
}

func parseRdmaHostState(output string) rdmaHostState {
	state := rdmaHostState{
		netDrivers: map[string]string{},
		roceV2:     map[string][]string{},
	}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "netdrv":
			if len(fields) == 3 {
				state.netDrivers[fields[1]] = fields[2]
			}
		case "port":
			if len(fields) >= 4 {
				port := rdmaPort{device: fields[1], port: fields[2], state: fields[3]}
				if len(fields) >= 5 {
					port.linkLayer = fields[4]
				}
				state.ports = append(state.ports, port)
			}
		case "rocev2":
			if len(fields) == 5 {
				key := fields[1] + "/" + fields[2]
				state.roceV2[key] = appendUnique(state.roceV2[key], fields[4])
			}
		case "devnode":
			if len(fields) == 2 {
				state.devNodes = append(state.devNodes, fields[1])
			}
		case "netns":
			if len(fields) == 2 {
				state.netnsMode = fields[1]
			}
		}
	}
	return state
}

// rdmaRequiredModules returns the base RDMA modules plus the provider module of every RDMA-capable
// NIC driver found on the host.
func rdmaRequiredModules(netDrivers map[string]string) []string {
	modules := append([]string{}, rdmaBaseModules...)
	for _, driver := range netDrivers {
		if provider, ok := rdmaProviderModules[driver]; ok {
			modules = appendUnique(modules, provider)
		}
	}
	return modules
}

// usableRoceV2Ports returns "<ibdev>/<port> (<netdevs>)" for ports that are ACTIVE, Ethernet and have
// a RoCE v2 GID.
func usableRoceV2Ports(state rdmaHostState) []string {
	var usable []string
	for _, p := range state.ports {
		netdevs := state.roceV2[p.device+"/"+p.port]
		if p.state == "ACTIVE" && p.linkLayer == "Ethernet" && len(netdevs) > 0 {
			usable = append(usable, fmt.Sprintf("%s/%s (%s)", p.device, p.port, strings.Join(netdevs, ",")))
		}
	}
	sort.Strings(usable)
	return usable
}

func (local *Checker) checkRdma() error {
	logrus.Info("Checking RDMA prerequisites")
	topic := formatTopic(consts.PreflightCheckTopicRDMA)

	output, err := local.packageManager.Execute([]string{}, "sh", []string{"-c", rdmaHostStateScript}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return wrapInternalError(topic, errors.Wrap(err, "failed to read RDMA host state"))
	}
	state := parseRdmaHostState(output)

	if err := local.checkRdmaModules(state); err != nil {
		return err
	}

	if len(state.ports) == 0 {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, "No RDMA devices found in /sys/class/infiniband. Check that the NIC supports RoCE and its RDMA provider module is loaded."))
	} else if usable := usableRoceV2Ports(state); len(usable) == 0 {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, "No ACTIVE Ethernet RDMA port with a RoCE v2 GID. Check the link state and that the port has an IP address."))
	} else {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, fmt.Sprintf("RoCE v2 ports: %s", strings.Join(usable, ", "))))
	}

	hasUverbs, hasRdmaCM := false, false
	for _, node := range state.devNodes {
		hasUverbs = hasUverbs || strings.HasPrefix(node, "/dev/infiniband/uverbs")
		hasRdmaCM = hasRdmaCM || node == "/dev/infiniband/rdma_cm"
	}
	if !hasUverbs || !hasRdmaCM {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, "Missing /dev/infiniband/uverbs* or /dev/infiniband/rdma_cm. Load ib_uverbs and rdma_ucm."))
	} else {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, "RDMA device nodes are present in /dev/infiniband"))
	}

	if state.netnsMode != "Y" {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf("ib_core netns_mode is %q, not shared (Y). Pods attached to the RDMA NIC through ipvlan/macvlan may not see the RDMA device.", state.netnsMode)))
	}

	if local.RdmaInterface != "" {
		if err := local.checkRdmaInterface(state); err != nil {
			return err
		}
	}

	return local.checkRdmaDataEngineSettings()
}

func (local *Checker) checkRdmaModules(state rdmaHostState) error {
	topic := formatTopic(consts.PreflightCheckTopicRDMA, consts.PreflightCheckTopicKernelModules)
	internalError := map[string]any{}
	for _, mod := range rdmaRequiredModules(state.netDrivers) {
		if err := local.packageManager.CheckModLoaded(mod); err != nil {
			if isExitCode(err, 1) || errors.Is(err, pkgmgr.ErrPackageNotInstalled) {
				local.collection.Log.Error = append(local.collection.Log.Error,
					wrapMsgWithTopic(topic, fmt.Sprintf("%s is not loaded", mod)))
				continue
			}
			internalError[mod] = err
			continue
		}
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, fmt.Sprintf("%s is loaded", mod)))
	}
	if len(internalError) > 0 {
		return wrapAggregatedInternalError(topic, "Failed to check modules:", internalError)
	}
	return nil
}

// checkRdmaInterface checks that the interface is backed by an RDMA device with an ACTIVE Ethernet
// port, and that the V2 data engine CPU mask is on the NIC's NUMA node.
func (local *Checker) checkRdmaInterface(state rdmaHostState) error {
	topic := formatTopic(consts.PreflightCheckTopicRDMA, local.RdmaInterface)
	iface := local.RdmaInterface

	script := fmt.Sprintf(`d=/sys/class/net/%[1]s/device
[ -d /sys/class/net/%[1]s ] || { echo missing; exit 0; }
echo "ibdev $(ls "$d/infiniband" 2>/dev/null | head -1)"
echo "numa $(cat "$d/numa_node" 2>/dev/null)"
for n in /sys/devices/system/node/node*; do [ -d "$n" ] && echo "node ${n##*node} $(cat "$n/cpulist")"; done`, iface)
	output, err := local.packageManager.Execute([]string{}, "sh", []string{"-c", script}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return wrapInternalError(topic, errors.Wrapf(err, "failed to read state of interface %s", iface))
	}
	if strings.TrimSpace(output) == "missing" {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, fmt.Sprintf("Interface %s does not exist", iface)))
		return nil
	}

	ibdev, nicNode, nodeCPUs := parseRdmaInterfaceState(output)
	if ibdev == "" {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, fmt.Sprintf("Interface %s is not backed by an RDMA device", iface)))
		return nil
	}

	active := false
	for _, p := range state.ports {
		if p.device == ibdev && p.state == "ACTIVE" && p.linkLayer == "Ethernet" {
			active = true
		}
	}
	if !active {
		local.collection.Log.Error = append(local.collection.Log.Error,
			wrapMsgWithTopic(topic, fmt.Sprintf("RDMA device %s of interface %s has no ACTIVE Ethernet port", ibdev, iface)))
	} else {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, fmt.Sprintf("Interface %s is backed by RDMA device %s (NUMA node %d)", iface, ibdev, nicNode)))
	}

	if nicNode < 0 {
		return nil
	}
	mask, found, err := local.getDataEngineSettingValue(settingNameDataEngineCPUMask)
	if err != nil {
		return wrapInternalError(topic, err)
	}
	if !found {
		return nil
	}
	cpus, err := cpusFromMask(mask)
	if err != nil {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf("Cannot parse %s %q: %v", settingNameDataEngineCPUMask, mask, err)))
		return nil
	}
	if remote := cpusOutsideNode(cpus, nodeCPUs[nicNode]); len(remote) > 0 {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf("%s %s includes CPUs %v that are not on NUMA node %d of %s; keep the V2 data engine on the NIC's NUMA node", settingNameDataEngineCPUMask, mask, remote, nicNode, iface)))
	} else {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, fmt.Sprintf("%s %s is on the NIC's NUMA node %d", settingNameDataEngineCPUMask, mask, nicNode)))
	}
	return nil
}

func parseRdmaInterfaceState(output string) (ibdev string, nicNode int, nodeCPUs map[int][]int) {
	nicNode = -1
	nodeCPUs = map[int][]int{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "ibdev":
			ibdev = fields[1]
		case "numa":
			if n, err := strconv.Atoi(fields[1]); err == nil {
				nicNode = n
			}
		case "node":
			if len(fields) == 3 {
				if n, err := strconv.Atoi(fields[1]); err == nil {
					if cpus, err := parseCPUList(fields[2]); err == nil {
						nodeCPUs[n] = cpus
					}
				}
			}
		}
	}
	return ibdev, nicNode, nodeCPUs
}

// checkRdmaDataEngineSettings warns when the V2 data engine iobuf pools are small for the number of
// reactors in the CPU mask. It is skipped when Longhorn is not installed.
func (local *Checker) checkRdmaDataEngineSettings() error {
	topic := formatTopic(consts.PreflightCheckTopicRDMA, "Iobuf")

	mask, found, err := local.getDataEngineSettingValue(settingNameDataEngineCPUMask)
	if err != nil {
		return wrapInternalError(topic, err)
	}
	if !found {
		local.collection.Log.Info = append(local.collection.Log.Info,
			wrapMsgWithTopic(topic, fmt.Sprintf("Longhorn settings not found in namespace %s; skipping the CPU mask and iobuf pool check", local.LonghornNamespace)))
		return nil
	}
	cpus, err := cpusFromMask(mask)
	if err != nil || len(cpus) == 0 {
		local.collection.Log.Warn = append(local.collection.Log.Warn,
			wrapMsgWithTopic(topic, fmt.Sprintf("Cannot parse %s %q", settingNameDataEngineCPUMask, mask)))
		return nil
	}
	reactors := len(cpus)
	wantSmall, wantLarge := requiredIobufPools(reactors)

	for _, s := range []struct {
		name string
		want int
	}{
		{settingNameDataEngineIobufSmallPool, wantSmall},
		{settingNameDataEngineIobufLargePool, wantLarge},
	} {
		value, found, err := local.getDataEngineSettingValue(s.name)
		if err != nil {
			return wrapInternalError(topic, err)
		}
		if !found {
			continue
		}
		got, err := strconv.Atoi(value)
		if err != nil {
			local.collection.Log.Warn = append(local.collection.Log.Warn,
				wrapMsgWithTopic(topic, fmt.Sprintf("Cannot parse %s %q", s.name, value)))
			continue
		}
		if got < s.want {
			local.collection.Log.Warn = append(local.collection.Log.Warn,
				wrapMsgWithTopic(topic, fmt.Sprintf("%s is %d; with %d reactors (%s %s) RDMA volumes need at least %d", s.name, got, reactors, settingNameDataEngineCPUMask, mask, s.want)))
		} else {
			local.collection.Log.Info = append(local.collection.Log.Info,
				wrapMsgWithTopic(topic, fmt.Sprintf("%s is %d, enough for %d reactors", s.name, got, reactors)))
		}
	}
	return nil
}

func requiredIobufPools(reactors int) (small, large int) {
	return rdmaIobufSmallPerReactor * reactors, rdmaIobufLargePerReactor * reactors
}

// getDataEngineSettingValue returns the V2 value of a Longhorn data engine setting. found is false when
// Longhorn (or the setting) is not installed.
func (local *Checker) getDataEngineSettingValue(name string) (value string, found bool, err error) {
	if local.kubeClient == nil {
		return "", false, nil
	}
	raw, err := local.kubeClient.CoreV1().RESTClient().Get().
		AbsPath("/apis/longhorn.io/v1beta2/namespaces", local.LonghornNamespace, "settings", name).
		DoRaw(context.TODO())
	if err != nil {
		if apierrors.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, errors.Wrapf(err, "failed to get Longhorn setting %s", name)
	}
	var setting struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &setting); err != nil {
		return "", false, errors.Wrapf(err, "failed to decode Longhorn setting %s", name)
	}
	return parseDataEngineSettingValue(setting.Value), true, nil
}

// parseDataEngineSettingValue returns the V2 value of a data engine specific setting, which is either a
// JSON map such as {"v2":"0x3"} or a plain value.
func parseDataEngineSettingValue(value string) string {
	var perEngine map[string]string
	if err := json.Unmarshal([]byte(value), &perEngine); err == nil {
		return perEngine["v2"]
	}
	return value
}

// cpusFromMask converts a hex CPU mask such as 0xff into the list of CPU indexes.
func cpusFromMask(mask string) ([]int, error) {
	m, ok := new(big.Int).SetString(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(mask)), "0x"), 16)
	if !ok {
		return nil, errors.Errorf("invalid CPU mask %q", mask)
	}
	var cpus []int
	for i := 0; i < m.BitLen(); i++ {
		if m.Bit(i) == 1 {
			cpus = append(cpus, i)
		}
	}
	return cpus, nil
}

// parseCPUList parses a kernel CPU list such as "0-3,8,10-11".
func parseCPUList(list string) ([]int, error) {
	var cpus []int
	for _, part := range strings.Split(strings.TrimSpace(list), ",") {
		if part == "" {
			continue
		}
		lo, hi, isRange := strings.Cut(part, "-")
		start, err := strconv.Atoi(lo)
		if err != nil {
			return nil, errors.Wrapf(err, "invalid CPU list %q", list)
		}
		end := start
		if isRange {
			if end, err = strconv.Atoi(hi); err != nil {
				return nil, errors.Wrapf(err, "invalid CPU list %q", list)
			}
		}
		for c := start; c <= end; c++ {
			cpus = append(cpus, c)
		}
	}
	return cpus, nil
}

func cpusOutsideNode(cpus, nodeCPUs []int) []int {
	onNode := map[int]bool{}
	for _, c := range nodeCPUs {
		onNode[c] = true
	}
	var outside []int
	for _, c := range cpus {
		if !onNode[c] {
			outside = append(outside, c)
		}
	}
	return outside
}

func appendUnique(list []string, item string) []string {
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}

// rdmaModulesLoadFile persists the RDMA modules across reboots.
const rdmaModulesLoadFile = "/etc/modules-load.d/longhorn-rdma.conf"

// installRdmaModules loads the RDMA kernel modules (base modules plus the provider of every
// RDMA-capable NIC on the host) and persists them in rdmaModulesLoadFile. No host packages are
// needed: the RDMA userspace ships in the instance-manager image.
func (local *Installer) installRdmaModules() error {
	output, err := local.packageManager.Execute([]string{}, "sh", []string{"-c", rdmaHostStateScript}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return errors.Wrap(err, "failed to read RDMA host state")
	}
	modules := rdmaRequiredModules(parseRdmaHostState(output).netDrivers)

	for _, mod := range modules {
		logrus.Infof("Probing module %s", mod)
		if _, err := local.packageManager.Modprobe(mod); err != nil {
			return errors.Wrapf(err, "failed to probe module %s", mod)
		}
		local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Successfully probed module %s", mod))
	}

	content := strings.Join(modules, "\n") + "\n"
	if _, err := local.packageManager.Execute([]string{}, "sh",
		[]string{"-c", fmt.Sprintf("mkdir -p /etc/modules-load.d && printf '%%s' '%s' > %s", content, rdmaModulesLoadFile)},
		commontypes.ExecuteNoTimeout); err != nil {
		return errors.Wrapf(err, "failed to write %s", rdmaModulesLoadFile)
	}
	local.collection.Log.Info = append(local.collection.Log.Info, fmt.Sprintf("Persisted RDMA modules in %s", rdmaModulesLoadFile))
	return nil
}

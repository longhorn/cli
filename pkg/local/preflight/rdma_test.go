package preflight

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/longhorn/cli/pkg/local/preflight/packagemanager/mocks"
	"github.com/longhorn/cli/pkg/types"
)

// Output of rdmaHostStateScript on an Ampere Altra node with a BlueField-2 (two mlx5 ports) after
// Longhorn instance managers attached ipvlan interfaces (lhnet1) to the first port.
const rdmaHostStateFixture = `netdrv enP1p1s0f0np0 mlx5_core
netdrv enP1p1s0f1np1 mlx5_core
netdrv enP2p2s0f0 igb
netdrv enP8p1s0f0 ixgbe
port mlx5_0 1 ACTIVE Ethernet
rocev2 mlx5_0 1 1 enP1p1s0f0np0
rocev2 mlx5_0 1 3 lhnet1
rocev2 mlx5_0 1 5 lhnet1
port mlx5_1 1 ACTIVE Ethernet
rocev2 mlx5_1 1 1 enP1p1s0f1np1
devnode /dev/infiniband/uverbs0
devnode /dev/infiniband/uverbs1
devnode /dev/infiniband/rdma_cm
netns Y
`

func TestParseRdmaHostState(t *testing.T) {
	state := parseRdmaHostState(rdmaHostStateFixture)

	assert.Equal(t, "mlx5_core", state.netDrivers["enP1p1s0f0np0"])
	assert.Len(t, state.ports, 2)
	assert.Equal(t, rdmaPort{device: "mlx5_0", port: "1", state: "ACTIVE", linkLayer: "Ethernet"}, state.ports[0])
	assert.Equal(t, []string{"enP1p1s0f0np0", "lhnet1"}, state.roceV2["mlx5_0/1"])
	assert.Len(t, state.devNodes, 3)
	assert.Equal(t, "Y", state.netnsMode)
	assert.Equal(t, []string{"mlx5_0/1 (enP1p1s0f0np0,lhnet1)", "mlx5_1/1 (enP1p1s0f1np1)"}, usableRoceV2Ports(state))
}

func TestUsableRoceV2Ports(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   []string
	}{
		{"port down", "port mlx5_0 1 DOWN Ethernet\nrocev2 mlx5_0 1 1 eth0\n", nil},
		{"infiniband link layer", "port mlx5_0 1 ACTIVE InfiniBand\nrocev2 mlx5_0 1 1 ib0\n", nil},
		{"no RoCE v2 GID", "port mlx5_0 1 ACTIVE Ethernet\n", nil},
		{"usable", "port mlx5_0 1 ACTIVE Ethernet\nrocev2 mlx5_0 1 1 eth0\n", []string{"mlx5_0/1 (eth0)"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, usableRoceV2Ports(parseRdmaHostState(tc.output)))
		})
	}
}

func TestRdmaRequiredModules(t *testing.T) {
	modules := rdmaRequiredModules(map[string]string{"eth0": "mlx5_core", "eth1": "mlx5_core", "eth2": "ixgbe", "eth3": "ice"})
	assert.ElementsMatch(t, []string{"ib_core", "ib_uverbs", "rdma_cm", "rdma_ucm", "mlx5_ib", "irdma"}, modules)
}

func TestParseDataEngineSettingValue(t *testing.T) {
	assert.Equal(t, "0xff", parseDataEngineSettingValue(`{"v2":"0xff"}`))
	assert.Equal(t, "8192", parseDataEngineSettingValue("8192"))
	assert.Equal(t, "", parseDataEngineSettingValue(`{"v1":"true"}`))
}

func TestCpusFromMask(t *testing.T) {
	cpus, err := cpusFromMask("0x3")
	assert.NoError(t, err)
	assert.Equal(t, []int{0, 1}, cpus)

	cpus, err = cpusFromMask("0xff")
	assert.NoError(t, err)
	assert.Len(t, cpus, 8)

	cpus, err = cpusFromMask("0x10000000000000000000") // CPU 76, beyond 64 bits
	assert.NoError(t, err)
	assert.Equal(t, []int{76}, cpus)

	_, err = cpusFromMask("zz")
	assert.Error(t, err)
}

func TestParseCPUList(t *testing.T) {
	cpus, err := parseCPUList("0-3,8,10-11")
	assert.NoError(t, err)
	assert.Equal(t, []int{0, 1, 2, 3, 8, 10, 11}, cpus)

	_, err = parseCPUList("0-x")
	assert.Error(t, err)
}

func TestParseRdmaInterfaceState(t *testing.T) {
	ibdev, node, nodeCPUs := parseRdmaInterfaceState("ibdev mlx5_0\nnuma 0\nnode 0 0-79\nnode 1 80-159\n")
	assert.Equal(t, "mlx5_0", ibdev)
	assert.Equal(t, 0, node)
	assert.Len(t, nodeCPUs[0], 80)
	assert.Equal(t, []int{80}, cpusOutsideNode([]int{0, 1, 80}, nodeCPUs[0]))
}

func TestRequiredIobufPools(t *testing.T) {
	small, large := requiredIobufPools(2)
	assert.Equal(t, 8192, small) // Longhorn defaults
	assert.Equal(t, 1024, large)

	small, large = requiredIobufPools(8)
	assert.Equal(t, 32768, small)
	assert.Equal(t, 4096, large)
}

func TestCheckRdma(t *testing.T) {
	notLoaded := exec.Command("sh", "-c", "exit 1").Run()

	tests := []struct {
		name          string
		hostState     string
		missingModule string
		wantErrSubstr []string
		wantWarn      int
	}{
		{
			name:      "healthy RoCE node",
			hostState: rdmaHostStateFixture,
		},
		{
			name:          "rdma_ucm not loaded",
			hostState:     rdmaHostStateFixture,
			missingModule: "rdma_ucm",
			wantErrSubstr: []string{"rdma_ucm is not loaded"},
		},
		{
			name:          "no RDMA devices",
			hostState:     "netdrv eth0 mlx5_core\nnetns Y\n",
			wantErrSubstr: []string{"No RDMA devices found", "Missing /dev/infiniband"},
		},
		{
			name:          "port down",
			hostState:     strings.Replace(rdmaHostStateFixture, "ACTIVE", "DOWN", 2),
			wantErrSubstr: []string{"No ACTIVE Ethernet RDMA port with a RoCE v2 GID"},
		},
		{
			name:      "exclusive netns mode",
			hostState: strings.Replace(rdmaHostStateFixture, "netns Y", "netns N", 1),
			wantWarn:  1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPM := new(mocks.MockPackageManager)
			mockPM.On("Execute", []string{}, "sh", mock.Anything, mock.Anything).Return(tc.hostState, nil).Once()
			mockPM.On("CheckModLoaded", mock.Anything).Return(func(mod string) error {
				if mod == tc.missingModule {
					return notLoaded
				}
				return nil
			})

			checker := &Checker{
				packageManager: mockPM,
				collection:     types.NodeCollection{Log: &types.LogCollection{}},
			}
			checker.LonghornNamespace = "longhorn-system"

			assert.NoError(t, checker.checkRdma())

			errs := strings.Join(checker.collection.Log.Error, "\n")
			if len(tc.wantErrSubstr) == 0 {
				assert.Empty(t, checker.collection.Log.Error)
			}
			for _, s := range tc.wantErrSubstr {
				assert.Contains(t, errs, s)
			}
			assert.Len(t, checker.collection.Log.Warn, tc.wantWarn)
			mockPM.AssertExpectations(t)
		})
	}
}

func TestInstallRdmaModules(t *testing.T) {
	mockPM := new(mocks.MockPackageManager)
	mockPM.On("Execute", []string{}, "sh", []string{"-c", rdmaHostStateScript}, mock.Anything).Return(rdmaHostStateFixture, nil).Once()
	for _, mod := range []string{"ib_core", "ib_uverbs", "rdma_cm", "rdma_ucm", "mlx5_ib"} {
		mockPM.On("Modprobe", mod).Return("", nil).Once()
	}
	mockPM.On("Execute", []string{}, "sh", mock.MatchedBy(func(args []string) bool {
		return len(args) == 2 && strings.Contains(args[1], rdmaModulesLoadFile) && strings.Contains(args[1], "mlx5_ib")
	}), mock.Anything).Return("", nil).Once()

	installer := &Installer{
		packageManager: mockPM,
		collection:     types.NodeCollection{Log: &types.LogCollection{}},
	}
	assert.NoError(t, installer.installRdmaModules())
	assert.Contains(t, strings.Join(installer.collection.Log.Info, "\n"), "Persisted RDMA modules")
	mockPM.AssertExpectations(t)
}

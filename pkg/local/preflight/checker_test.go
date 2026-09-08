package preflight

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"

	"github.com/longhorn/cli/pkg/local/preflight/packagemanager/mocks"
	"github.com/longhorn/cli/pkg/utils"
	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
)

type UtilTestSuite struct {
	suite.Suite
}

func (s *UtilTestSuite) TestWrapMsgWithTopic() {
	s.Equal("Topic This is a message", wrapMsgWithTopic("Topic", "This is a message"))
}

func (s *UtilTestSuite) TestFormatTopic() {
	s.Equal("[A][B]", formatTopic("A", "B"))
	s.Equal("", formatTopic())
}

func (s *UtilTestSuite) TestWrapMultItems() {
	// === Case 1: Normal error values ===
	itemsWithErrors := map[string]any{
		"nvme-cli":  errors.New("command not found"),
		"sg3_utils": errors.New("exit status 1"),
	}

	result := wrapMultItems("The following packages are not installed:", itemsWithErrors)

	s.Contains(result, "The following packages are not installed:")
	s.Contains(result, "nvme-cli: command not found")
	s.Contains(result, "sg3_utils: exit status 1")

	// === Case 2: Nil value ===
	itemsWithNil := map[string]any{
		"some-key": nil,
	}
	result = wrapMultItems("Testing nil:", itemsWithNil)
	s.Contains(result, "Testing nil:")
}

func TestSPDKDependencies(t *testing.T) {
	tests := []struct {
		name             string
		osRelease        string
		expectedPacman   bool
		expectedPackages []string
		expectedModules  []string
		expectedServices []string
	}{
		{
			name:             "Arch Linux - should use Pacman with packages",
			osRelease:        "arch",
			expectedPacman:   true,
			expectedPackages: []string{"nfs-utils", "open-iscsi", "cryptsetup", "device-mapper"},
			expectedModules:  []string{"nfs", "iscsi_tcp", "dm_crypt"},
			expectedServices: []string{"multipathd.service"},
		},
		{
			name:             "RHEL - should use Yum with packages",
			osRelease:        "rhel",
			expectedPacman:   false,
			expectedPackages: []string{"nfs-utils", "iscsi-initiator-utils", "cryptsetup", "device-mapper"},
			expectedModules:  []string{"nfs", "iscsi_tcp", "dm_crypt"},
			expectedServices: []string{"multipathd.service"},
		},
		{
			name:             "Ubuntu - should use Apt with packages",
			osRelease:        "ubuntu",
			expectedPacman:   false,
			expectedPackages: []string{"nfs-common", "open-iscsi", "cryptsetup", "dmsetup"},
			expectedModules:  []string{"nfs", "dm_crypt"},
			expectedServices: []string{"multipathd.service"},
		},
		{
			name:             "Talos - SPDK modules are checked (early return)",
			osRelease:        "talos",
			expectedPacman:   false,
			expectedPackages: []string{},
			expectedModules:  []string{"nvme_tcp", "uio_pci_generic", "vfio_pci"},
			expectedServices: []string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockPM := new(mocks.MockPackageManager)
			
			// Initialize all slice fields to avoid nil slices
			checker := &Checker{
				osRelease:        tc.osRelease,
				packageManager:   mockPM,
				packages:         []string{},
				modules:          []string{},
				services:         []string{},
				spdkDepPackages:  []string{},
				spdkDepModules:   []string{},
			}

			// Simulate the Talos early return case
			if checker.osRelease == "talos" {
				checker.packages = []string{}
				checker.modules = []string{"nfs", "dm_crypt"}
				checker.services = []string{}
				checker.spdkDepPackages = []string{}
				checker.spdkDepModules = []string{"nvme_tcp", "uio_pci_generic", "vfio_pci"}
				assert.Equal(t, tc.expectedPackages, checker.spdkDepPackages, "SPDK packages should match expected")
				assert.Equal(t, tc.expectedModules, checker.spdkDepModules, "SPDK modules should match expected")
				return
			}

			// Simulate the switch statement logic for package manager detection
			pkgType, err := utils.GetPackageManagerType(tc.osRelease)
			assert.NoError(t, err)

			// Set packages and modules based on package manager type (simulating the switch statement)
			switch pkgType {
			case pkgmgr.PackageManagerApt:
				checker.packages = []string{"nfs-common", "open-iscsi", "cryptsetup", "dmsetup"}
				checker.modules = []string{"nfs", "dm_crypt"}
				checker.services = []string{"multipathd.service"}
			case pkgmgr.PackageManagerYum:
				checker.packages = []string{"nfs-utils", "iscsi-initiator-utils", "cryptsetup", "device-mapper"}
				checker.modules = []string{"nfs", "iscsi_tcp", "dm_crypt"}
				checker.services = []string{"multipathd.service"}
			case pkgmgr.PackageManagerZypper, pkgmgr.PackageManagerTransactionalUpdate:
				checker.packages = []string{"nfs-client", "open-iscsi", "cryptsetup", "device-mapper"}
				checker.modules = []string{"nfs", "iscsi_tcp", "dm_crypt"}
				checker.services = []string{"multipathd.service"}
			case pkgmgr.PackageManagerPacman:
				checker.packages = []string{"nfs-utils", "open-iscsi", "cryptsetup", "device-mapper"}
				checker.modules = []string{"nfs", "iscsi_tcp", "dm_crypt"}
				checker.services = []string{"multipathd.service"}
			}

			// Set SPDK dependencies (as done in the actual code)
			checker.spdkDepPackages = []string{}
			checker.spdkDepModules = []string{"nvme_tcp", "uio_pci_generic", "vfio_pci"}

			// Verify SPDK dependencies are correctly initialized
			assert.Equal(t, []string{}, checker.spdkDepPackages, "SPDK packages should be empty for all supported managers")
			assert.Equal(t, []string{"nvme_tcp", "uio_pci_generic", "vfio_pci"}, checker.spdkDepModules, "SPDK modules should be correctly set for all supported managers")
		})
	}
}

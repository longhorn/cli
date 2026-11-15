package preflight

import (
	"strings"
	"testing"

	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
	remote "github.com/longhorn/cli/pkg/remote/preflight"
)

func TestV2InstallerRejectsInvalidCapabilityBeforeInitialization(t *testing.T) {
	installer := Installer{InstallerCmdOptions: remote.InstallerCmdOptions{DataEngineMode: "unknown"}}
	if err := installer.Init(); err == nil || !strings.Contains(err.Error(), "data-engine-mode") {
		t.Fatalf("invalid mode reached host or connection initialization: %v", err)
	}
}

func TestV2InstallerProfilesRemoveOnlyISCSI(t *testing.T) {
	for _, profile := range []pkgmgr.PackageManagerType{pkgmgr.PackageManagerApt, pkgmgr.PackageManagerYum, pkgmgr.PackageManagerZypper, pkgmgr.PackageManagerTransactionalUpdate, pkgmgr.PackageManagerBinary, pkgmgr.PackageManagerPacman} {
		installer := Installer{InstallerCmdOptions: remote.InstallerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048}}
		if err := installer.configureDependencies(profile, "fixture-kernel"); err != nil {
			t.Fatal(err)
		}
		for _, dependency := range append(append(append([]Package{}, installer.packages...), installer.modules...), installer.services...) {
			if dependency.Name == "open-iscsi" || dependency.Name == "iscsi-initiator-utils" || dependency.Name == "iscsi_tcp" || dependency.Name == "iscsid" {
				t.Fatalf("V2 profile retains %s", dependency.Name)
			}
			if !dependency.Required {
				t.Fatalf("required dependency became optional: %s", dependency.Name)
			}
		}
		if len(installer.packages) != 2 || len(installer.modules) != 2 || len(installer.services) != 0 || len(installer.spdkDepModules) != 3 {
			t.Fatalf("V2 profile removed non-iSCSI dependencies: %s", profile)
		}
	}
}

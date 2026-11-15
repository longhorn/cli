package preflight

import (
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/longhorn/cli/pkg/local/preflight/packagemanager/mocks"
	"github.com/longhorn/cli/pkg/types"

	remote "github.com/longhorn/cli/pkg/remote/preflight"
)

func TestV2SPDKSetupClearsInheritedOptionsAndVerifiesActualAllocation(t *testing.T) {
	t.Setenv("BASH_ENV", "/unqualified/startup")
	t.Setenv("NRHUGE", "0")
	t.Setenv("PCI_ALLOWED", "all")
	for _, test := range []struct {
		name, pages string
		setupError  bool
		valid       bool
	}{
		{"complete", "1024", false, true},
		{"setup-failed", "", true, false},
		{"partial-allocation", "1000", false, false},
		{"unreadable-allocation", "unknown", false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := new(mocks.MockPackageManager)
			installer := Installer{InstallerCmdOptions: remote.InstallerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048, AllowPci: "none"}, packageManager: manager, collection: types.NodeCollection{Log: &types.LogCollection{}}}
			environment, node, err := installer.V2SPDKSetupEnvironment()
			if err != nil {
				t.Fatal(err)
			}
			args := append([]string{"-i"}, environment...)
			args = append(args, "/bin/bash", "--noprofile", "--norc", "--", "/tmp/owned/scripts/setup.sh", "config")
			var setupErr error
			if test.setupError {
				setupErr = errors.New("native setup failed")
			}
			manager.On("Execute", []string(nil), "/usr/bin/env", mock.MatchedBy(func(actual []string) bool { return reflect.DeepEqual(actual, args) }), v2SPDKSetupTimeout).Return("", setupErr).Once()
			if !test.setupError {
				manager.On("Execute", []string(nil), "/usr/bin/env", mock.Anything, v2SPDKReadTimeout).Return(test.pages+"\n", nil).Once()
			}
			err = installer.executeV2SPDKSetup("/tmp/owned/scripts/setup.sh", environment, node)
			if (err == nil) != test.valid {
				t.Fatalf("incorrect setup verdict: %v", err)
			}
			if test.setupError && !errors.Is(err, setupErr) {
				t.Fatal("native setup failure was swallowed")
			}
			if (len(installer.collection.Log.Info) > 0) != test.valid {
				t.Fatal("setup reported success before actual allocation was verified")
			}
			manager.AssertExpectations(t)
		})
	}
}

func TestV2SPDKHostQualificationRejectsUnsupportedHooksBeforeWrites(t *testing.T) {
	manager := new(mocks.MockPackageManager)
	installer := Installer{InstallerCmdOptions: remote.InstallerCmdOptions{DataEngineMode: "v2-only", HugePageSize: 2048, AllowPci: "none"}, packageManager: manager}
	manager.On("Execute", []string(nil), "/usr/bin/env", mock.Anything, v2SPDKReadTimeout).Return("", errors.New("unqualified host hook or NUMA interface")).Once()
	if err := installer.qualifyV2SPDKHost(); err == nil {
		t.Fatal("unsupported host profile qualified")
	}
	manager.AssertExpectations(t)
}

package preflight

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/longhorn/cli/pkg/types"
)

func TestV2InstallerTransportReportsFailureWithoutAutomaticReplay(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "persistent-attempts")
	calls := 0
	installer := Installer{collection: types.NodeCollection{Log: &types.LogCollection{}}}
	run := func() error { calls++; return errors.New("installation pending a host reboot") }
	if err := runV2InstallAttempt(directory, "invocation-1", run, installer.recordV2InstallFailure); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(installer.collection.Log.Error) != 1 || !strings.Contains(installer.collection.Log.Error[0], "host reboot") {
		t.Fatal("failed installation did not produce its failure report")
	}
	// A new container/Pod uses the same persistent host marker. It must be
	// able to report failure without calling the mutating installer again.
	replacement := Installer{collection: types.NodeCollection{Log: &types.LogCollection{}}}
	if err := runV2InstallAttempt(directory, "invocation-1", run, replacement.recordV2InstallFailure); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(replacement.collection.Log.Error) != 1 || !strings.Contains(replacement.collection.Log.Error[0], "automatic replay is refused") {
		t.Fatal("container replacement replayed host writes or lost the refusal report")
	}
	if err := runV2InstallAttempt(directory, "explicit-new-invocation", run, replacement.recordV2InstallFailure); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal("an explicit new invocation could not run")
	}
}

func TestV2InstallerCannotMutateWithoutItsAttemptRecord(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(directory, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	runs, failures := 0, 0
	if err := runV2InstallAttempt(directory, "invocation", func() error { runs++; return nil }, func(error) { failures++ }); err != nil {
		t.Fatal(err)
	}
	if runs != 0 || failures != 1 {
		t.Fatal("host mutation ran without a persistent attempt record")
	}
}

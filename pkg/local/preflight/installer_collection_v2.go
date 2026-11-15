package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
)

var v2InstallAttemptID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// RunForCollection transports a failed or reboot-pending installation as a
// complete error report. The remote collector, not the init-container exit
// code, owns the verdict. A persistent per-invocation host marker prevents
// Kubernetes init retries or replacement Pods from repeating host writes.
func (local *Installer) RunForCollection() error {
	if err := local.ValidateDataEngineMode(); err != nil {
		return err
	}
	if local.DataEngineMode != string(types.DataEngineModeV2Only) || local.collection.Log == nil || local.packageManager == nil || local.kubeClient == nil {
		return fmt.Errorf("V2 collection requires an initialized installer")
	}
	if local.OutputFilePath != filepath.Join(consts.VolumeMountSharedDirectory, consts.FileNameOutputJSON) || len(local.AttemptID) > 63 || !strings.HasPrefix(local.AttemptID, consts.AppNamePreflightInstaller+"-") || !v2InstallAttemptID.MatchString(local.AttemptID) {
		return fmt.Errorf("V2 installer collection requires its identified invocation and shared output")
	}
	local.collection.DataEngineMode = string(types.DataEngineModeV2Only)
	if err := local.qualifyV2SPDKHost(); err != nil {
		local.recordV2InstallFailure(err)
		return nil
	}
	directory := filepath.Join(consts.VolumeMountHostDirectory, "var/lib/longhorn-cli/preflight-attempts")
	if err := qualifyV2AttemptFilesystem(directory); err != nil {
		local.recordV2InstallFailure(err)
		return nil
	}
	return runV2InstallAttempt(directory, local.AttemptID, local.Run, local.recordV2InstallFailure)
}

func (local *Installer) recordV2InstallFailure(err error) {
	local.collection.Log.Info = append(local.collection.Log.Info, "V2 prerequisite setup returned a failure report; readiness is not established")
	local.collection.Log.Error = append(local.collection.Log.Error, err.Error())
}

func runV2InstallAttempt(directory, attemptID string, run func() error, recordFailure func(error)) error {
	if err := claimV2InstallAttempt(directory, attemptID); err != nil {
		recordFailure(err)
		return nil
	}
	if err := run(); err != nil {
		recordFailure(err)
	}
	return nil
}

func claimV2InstallAttempt(directory, attemptID string) error {
	if err := prepareV2AttemptDirectory(directory); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(directory, attemptID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("V2 installation was already attempted on this host; automatic replay is refused; inspect the prior failure or reboot state before a new invocation")
	}
	if err != nil {
		return err
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	return syncV2AttemptDirectory(directory)
}

func syncV2AttemptDirectory(directory string) error {
	dir, err := os.Open(directory)
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func prepareV2AttemptDirectory(directory string) error {
	// The runtime path has two private levels below the existing /var/lib.
	// Persist each child's entry in its parent, including when a concurrent
	// invocation created it, before the host-changing callback can run.
	root := filepath.Dir(filepath.Dir(directory))
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("V2 attempt root is not an identified directory")
	}
	for _, path := range []string{filepath.Dir(directory), directory} {
		if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || !ok || int(stat.Uid) != os.Geteuid() {
			return fmt.Errorf("V2 attempt directory is not private and owned by this installer")
		}
		if err := errors.Join(syncV2AttemptDirectory(path), syncV2AttemptDirectory(filepath.Dir(path))); err != nil {
			return err
		}
	}
	return nil
}

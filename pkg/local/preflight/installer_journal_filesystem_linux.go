//go:build linux

package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func qualifyV2AttemptFilesystem(directory string) error {
	path := directory
	for {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) && filepath.Dir(path) != path {
			path = filepath.Dir(path)
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("V2 attempt store is not an identified directory")
		}
		var status unix.Statfs_t
		if err := unix.Statfs(path, &status); err != nil {
			return err
		}
		switch status.Type {
		case unix.EXT4_SUPER_MAGIC, unix.XFS_SUPER_MAGIC, unix.BTRFS_SUPER_MAGIC, unix.F2FS_SUPER_MAGIC:
			return nil
		default:
			return fmt.Errorf("V2 installer replay protection requires a qualified persistent ext, XFS, Btrfs or F2FS attempt store")
		}
	}
}

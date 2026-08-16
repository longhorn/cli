package kubernetes

import (
	"k8s.io/mount-utils"
)

func IsMountPointReadOnly(mp mount.MountPoint) bool {
	for _, opt := range mp.Opts {
		// "ro" is the standard read-only mount option. "emergency_ro" is the
		// ext4 emergency read-only state: since kernel v6.12 (commit
		// d3476f3dad4a "ext4: don't set SB_RDONLY after filesystem errors"),
		// errors=remount-ro no longer sets SB_RDONLY, so the mount keeps
		// showing "rw" while writes fail with EROFS and the state is only
		// visible as the "emergency_ro" option.
		if opt == "ro" || opt == "emergency_ro" {
			return true
		}
	}
	return false
}

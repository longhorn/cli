//go:build !linux

package replica

import (
	"fmt"
	"runtime"
)

// Recoverer is a no-op stand-in on non-Linux platforms. Replica recovery
// depends on Linux-specific sparse-file syscalls (FIEMAP, fallocate) with
// no portable equivalent, so this command is Linux-only.
type Recoverer struct {
	LogLevel      string
	Namespace     string
	CurrentNodeID string

	ReplicaDirectory string
	DryRun           bool
	Recovered        bool
}

func (r *Recoverer) Validate() error {
	return fmt.Errorf("replica recovery is only supported on Linux (current platform: %s/%s)", runtime.GOOS, runtime.GOARCH)
}

func (r *Recoverer) Init() error {
	return nil
}

func (r *Recoverer) Run() error {
	return nil
}

func (r *Recoverer) Close() error {
	return nil
}

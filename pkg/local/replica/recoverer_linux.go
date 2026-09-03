//go:build linux

package replica

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/coalesce"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/prune"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/sectormap"
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

	volMeta sectormap.VolumeMeta
	chain   *sectormap.Chain
}

func (r *Recoverer) Validate() error {
	if r.ReplicaDirectory == "" {
		return errors.Errorf("replica directory must be specified via --%s", consts.CmdOptReplicaDir)
	}
	info, err := os.Stat(r.ReplicaDirectory)
	if err != nil || !info.IsDir() {
		return errors.Wrapf(err, "replica directory %s is not accessible", r.ReplicaDirectory)
	}
	return nil
}

func (r *Recoverer) Init() error {
	var err error

	r.volMeta, err = sectormap.LoadVolumeMeta(r.ReplicaDirectory)
	if err != nil {
		return errors.Wrap(err, "failed to load volume metadata")
	}

	r.chain, err = sectormap.NewChain(r.ReplicaDirectory, r.volMeta)
	if err != nil {
		return errors.Wrap(err, "failed to build snapshot chain")
	}

	return nil
}

func (r *Recoverer) Run() (err error) {
	sMap, err := r.chain.BuildSectorLocationMap()
	if err != nil {
		return errors.Wrap(err, "failed to build sector location map")
	}

	logrus.Info("--- raw extents per file ---")
	if err := r.chain.DumpExtents(sMap); err != nil {
		return errors.Wrap(err, "failed to dump extents")
	}

	logrus.Info("--- resolved sector ranges ---")
	sectormap.PrintSectorLocationTable(sMap, r.chain.TotalSectors)

	logrus.Info("--- punching obsolete ranges in ancestors ---")
	r.Recovered, err = prune.PunchSnapshots(sMap, r.chain, r.DryRun)
	if err != nil {
		return errors.Wrap(err, "punch failed")
	}

	if r.Recovered {
		logrus.Info("--- promoting non-head data into head ---")
		if err := coalesce.PromoteToHead(sMap, r.chain, r.volMeta.Head, r.DryRun); err != nil {
			return errors.Wrap(err, "promote failed")
		}
	}

	return nil
}

func (r *Recoverer) Close() error {
	if r == nil || r.chain == nil {
		return nil
	}
	if err := r.chain.Close(); err != nil {
		logrus.Warnf("failed to close chain: %v", err)
		return err
	}
	return nil
}

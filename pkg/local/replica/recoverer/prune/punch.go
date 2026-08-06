package prune

import (
	"fmt"

	"github.com/dustin/go-humanize"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/common"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/sectormap"
	"github.com/longhorn/sparse-tools/sparse"
	"github.com/sirupsen/logrus"
)

type punchOp struct {
	fiemapFile sparse.FiemapFile
	punchTo    string
	offset     int64
	length     int64
	estimated  int64
}

// PunchSnapshots for every sector range with a resolved owner, it punches a hole for that range in
// every ancestor of the owner. Sectors with no resolved owner are left untouched.
func PunchSnapshots(smap *sectormap.SectorMapping, chain *sectormap.Chain, dryRun bool) (bool, error) {
	if chain.TotalSectors == 0 {
		return false, nil
	}
	totalSectors := chain.TotalSectors
	sectorSize := chain.SectorSize

	var ops []punchOp

	punchRun := func(runStart, runEnd int64, ownerIdx byte) error {
		if ownerIdx == 0 {
			// Implicitly owned by the base/oldest layer, nothing newer shadows it, nothing to punch.
			return nil
		}

		ownerName := smap.Names[ownerIdx]
		//ancestors, err := diskMetas.AncestorsOf(ownerName)
		ancestors := chain.Ancestors[ownerName]
		if ancestors == nil {
			// e.g. owner is the oldest snapshot, or a base image with no *.meta. Nothing older exists, so nothing to punch.
			logrus.Warnf("no metadata for %v; skipping punch for sectors [%d,%d)", ownerName, runStart, runEnd)
			return nil
		}
		if len(ancestors) == 0 {
			return nil
		}

		for _, ancestor := range ancestors {
			allocated := intersect(runStart, runEnd, smap.ExtentCache[ancestor])
			if len(allocated) == 0 {
				continue // already fully sparseHole here
			}

			file, err := chain.GetFile(ancestor)
			if err != nil {
				return fmt.Errorf("failed to get file %v: %w", ancestor, err)
			}

			blockSize, err := common.BlockSize(file)
			if err != nil {
				return fmt.Errorf("failed to get block size for %v: %w", ancestor, err)
			}

			fiemapFile := sparse.FiemapFile{File: file}
			for _, r := range allocated {
				offset := r.Start * sectorSize
				length := (r.End - r.Start) * sectorSize
				if dryRun {
					fmt.Printf("[dry-run] would punch %v at offset=%d length=%d\n", ancestor, offset, length)
					ops = append(ops, punchOp{
						fiemapFile: fiemapFile,
						punchTo:    ancestor,
						offset:     offset,
						length:     length,
						estimated:  common.EstimateReclaimable(offset, length, blockSize),
					})
				} else {
					if err := fiemapFile.PunchHole(offset, length); err != nil {
						return fmt.Errorf("failed to punch hole in %v at [%d,+%d): %w", ancestor, offset, length, err)
					}
					logrus.Infof("punched %v at [%d,+%d]", ancestor, offset, length)
				}
			}
		}
		return nil
	}

	runStart := int64(0)
	runOwnerIdx := smap.Location[0]

	for s := int64(1); s < totalSectors; s++ {
		idx := smap.Location[s]
		if idx != runOwnerIdx {
			if err := punchRun(runStart, s, runOwnerIdx); err != nil {
				return false, err
			}
			runStart, runOwnerIdx = s, idx
		}
	}

	if err := punchRun(runStart, totalSectors, runOwnerIdx); err != nil {
		return false, err
	}

	if dryRun {
		if len(ops) == 0 {
			fmt.Println("[dry-run] No holes to punch.")
			return false, nil
		}
		var totalEstimated int64
		for _, op := range ops {
			totalEstimated += op.estimated
		}
		logrus.Infof("Punching will reclaim approximately %v of disk space", humanize.Bytes(uint64(totalEstimated)))
		if !common.Confirm("Do you want to proceed with hole punching?") {
			fmt.Println("Operation canceled.")
			return false, nil
		}
		for _, op := range ops {
			if err := op.fiemapFile.PunchHole(op.offset, op.length); err != nil {
				return false, fmt.Errorf("failed to punch hole in %v at [%d,+%d): %w", op.punchTo, op.offset, op.length, err)
			}
			logrus.Infof("punched %v at [%d,+%d]", op.punchTo, op.offset, op.length)
		}
	}
	return true, nil
}

// intersect returns the portions of [runStart, runEnd) that overlap with sectorRanges,
// i.e. the parts of the run that are actually allocated in this file and thus worth punching.
func intersect(runStart, runEnd int64, ranges []sectormap.SectorRange) []sectormap.SectorRange {
	var out []sectormap.SectorRange
	for _, r := range ranges {
		s, e := max(runStart, r.Start), min(runEnd, r.End)
		if s < e {
			out = append(out, sectormap.SectorRange{Start: s, End: e})
		}
	}
	return out
}

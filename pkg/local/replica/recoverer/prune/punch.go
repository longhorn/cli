package prune

import (
	"fmt"
	"os"
	"sort"

	"github.com/dustin/go-humanize"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/common"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/sectormap"
	"github.com/longhorn/sparse-tools/sparse"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// PunchSnapshots for every sector range with a resolved owner, it punches a hole for that range in
// every ancestor of the owner. Sectors with no resolved owner are left untouched.
func PunchSnapshots(smap *sectormap.SectorMapping, chain *sectormap.Chain, dryRun bool) (bool, error) {
	if chain.TotalSectors == 0 {
		return false, nil
	}
	totalSectors := chain.TotalSectors
	sectorSize := chain.SectorSize

	var totalEstimated int64
	var anyOps bool

	// TODO: Add direct deletion for obsoleteFiles now.

	punchRun := func(runStart, runEnd int64, ownerIdx byte, execute bool) error {
		if ownerIdx == 0 {
			// Implicitly owned by the base/oldest layer, nothing newer shadows it, nothing to punch.
			return nil
		}

		ownerName := smap.OwnerFiles[ownerIdx]
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

			blockSize, err := blockSize(file)
			if err != nil {
				return fmt.Errorf("failed to get block size for %v: %w", ancestor, err)
			}

			fiemapFile := sparse.FiemapFile{File: file}
			for _, r := range allocated {
				offset := r.Start * sectorSize
				length := (r.End - r.Start) * sectorSize
				if execute {
					if err := fiemapFile.PunchHole(offset, length); err != nil {
						return fmt.Errorf("failed to punch hole in %v at [%d,+%d): %w", ancestor, offset, length, err)
					}
					logrus.Infof("punched %v at [%d,+%d]", ancestor, offset, length)
				} else {
					anyOps = true
					estimateSize := estimateReclaimable(offset, length, blockSize)
					totalEstimated += estimateSize
					fmt.Printf("[dry-run] would punch %v at offset=%d length=%d (~%v)\n", ancestor, offset, length, humanize.Bytes(uint64(estimateSize)))
				}
			}
		}
		return nil
	}

	scan := func(execute bool) error {
		runStart := int64(0)
		runOwnerIdx := smap.Location[0]
		for s := int64(1); s < totalSectors; s++ {
			if idx := smap.Location[s]; idx != runOwnerIdx {
				if err := punchRun(runStart, s, runOwnerIdx, execute); err != nil {
					return err
				}
				runStart, runOwnerIdx = s, idx
			}
		}
		return punchRun(runStart, totalSectors, runOwnerIdx, execute)
	}

	// Scan 1: compute + report, nothing retained beyond a running total.
	if err := scan(false); err != nil {
		return false, err
	}

	// Calculate estimatedReclaimable size for obsoleteFiles too
	var obsoleteEstimate int64
	for _, fName := range smap.ObsoleteFiles {
		if info, err := os.Stat(fName); err == nil {
			obsoleteEstimate += info.Size()
			fmt.Printf("[dry-run] would delete obsolete file %v (~%v)\n", fName, humanize.Bytes(uint64(info.Size())))
		}
	}
	totalEstimated += obsoleteEstimate

	// If no operations are to be performed (i.e. nothing is allocated), and no obsoleteFile exist, nothing to do.
	if !anyOps && len(smap.ObsoleteFiles) == 0 {
		fmt.Println("Nothing to do.")
		return false, nil
	}

	if dryRun {
		logrus.Infof("Will reclaim approximately %v of disk space", humanize.Bytes(uint64(totalEstimated)))
		if !common.Confirm("Do you want to proceed with hole punching and deletion?") {
			fmt.Println("Operation canceled.")
			return false, nil
		}
	}

	// Scan 2: execute for real, walking the same runs again.
	if err := deleteObsoleteFiles(smap.ObsoleteFiles); err != nil {
		return false, err
	}
	if err := scan(true); err != nil {
		return false, err
	}
	return true, nil
}

// intersect returns the portions of [runStart, runEnd) that overlap with sectorRanges,
// i.e. the parts of the run that are actually allocated in this file and thus worth punching.
func intersect(runStart, runEnd int64, ranges []sectormap.SectorRange) []sectormap.SectorRange {
	i := sort.Search(len(ranges), func(i int) bool {
		return ranges[i].End > runStart
	})
	var out []sectormap.SectorRange
	for ; i < len(ranges); i++ {
		r := ranges[i]
		if r.Start >= runEnd {
			break
		}
		s, e := max(runStart, r.Start), min(runEnd, r.End)
		if s < e {
			out = append(out, sectormap.SectorRange{Start: s, End: e})
		}
	}
	return out
}

func deleteObsoleteFiles(obsoleteFiles []string) error {
	for _, file := range obsoleteFiles {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove obsolete file %v: %w", file, err)
		}
	}
	return nil
}

// blockSize returns the filesystem block size backing given file.
func blockSize(file *os.File) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Fstatfs(int(file.Fd()), &st); err != nil {
		return 0, fmt.Errorf("failed to statfs: %w", err)
	}
	return int64(st.Bsize), nil
}

// estimateReclaimable returns the number of bytes covered by blocks that are fully
// contained within [offset, offset+length).
func estimateReclaimable(offset, length, blockSize int64) int64 {
	end := offset + length
	alignedStart := (offset + blockSize - 1) / blockSize * blockSize
	alignedEnd := end / blockSize * blockSize

	if alignedEnd <= alignedStart {
		return 0
	}
	return alignedEnd - alignedStart
}

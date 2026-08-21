package coalesce

import (
	"fmt"
	"io"
	"os"

	"github.com/longhorn/cli/pkg/local/replica/recoverer/common"
	"github.com/longhorn/cli/pkg/local/replica/recoverer/sectormap"
	"github.com/longhorn/sparse-tools/sparse"
	"github.com/pkg/errors"
)

// PromoteToHead copies every sector range whose current owner is NOT head into head,
// then punches a hole in that source range so the copy is not duplicated on disk.
func PromoteToHead(smap *sectormap.SectorMapping, chain *sectormap.Chain, headName string, dryRun bool) error {
	location := smap.Location
	names := smap.OwnerFiles

	totalSectors := int64(len(location))
	if totalSectors == 0 {
		return nil
	}

	if dryRun {
		if !common.Confirm("Do you want to proceed with promoting sectors to head?") {
			fmt.Println("Operation canceled.")
			return nil
		}
	}

	headFile, err := chain.GetFile(headName)
	if err != nil {
		return errors.Wrap(err, "failed to get head layer file")
	}

	promoteRun := func(runStart, runEnd int64, ownerIdx byte) error {
		if ownerIdx == 0 || names[ownerIdx] == headName {
			// Already head, or unresolved (implicitly head/backing), nothing to promote.
			return nil
		}
		ownerName := names[ownerIdx]

		srcFile, err := chain.GetFile(names[ownerIdx])
		if err != nil {
			return fmt.Errorf("failed to get file: %w", err)
		}

		for chunkBeg := runStart; chunkBeg < runEnd; chunkBeg += promoteChunkSectors {
			chunkEnd := chunkBeg + promoteChunkSectors
			if chunkEnd > runEnd {
				chunkEnd = runEnd
			}

			offset := chunkBeg * sectorSize
			length := (chunkEnd - chunkBeg) * sectorSize

			if err := executePromote(srcFile, headFile, ownerName, offset, length); err != nil {
				return err
			}

		}
		return nil
	}

	runStart := int64(0)
	runOwnerIdx := location[0]
	for s := int64(1); s < totalSectors; s++ {
		idx := location[s]
		if idx != runOwnerIdx {
			if err := promoteRun(runStart, s, runOwnerIdx); err != nil {
				return err
			}
			runStart, runOwnerIdx = s, idx
		}
	}
	return promoteRun(runStart, totalSectors, runOwnerIdx)
}

func executePromote(srcFile *os.File, headFile *os.File, ownerName string, offset, length int64) error {
	buf := make([]byte, length)
	if _, err := srcFile.ReadAt(buf, offset); err != nil && err != io.EOF {
		return fmt.Errorf("failed to read %s at [%d,+%d): %w", ownerName, offset, length, err)
	}

	if _, err := headFile.WriteAt(buf, offset); err != nil {
		return fmt.Errorf("failed to write head at [%d,+%d): %w", offset, length, err)
	}

	// Data is now durably in head, reclaim the now-redundant copy in the source.
	fiemapFile := sparse.NewFiemapFile(srcFile)
	if err := fiemapFile.PunchHole(offset, length); err != nil {
		return fmt.Errorf("failed to punch hole in %v at [%d,+%d): %w", ownerName, offset, length, err)
	}
	return nil
}

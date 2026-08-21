package sectormap

import (
	"fmt"
	"os"
)

// DumpExtents inspects and outputs physical extent mappings for all files in the storage chain.
// It processes the optional base backing file first, followed by each sequence layer ordered from
// newest to oldest. Returns an error if any file handle is missing or if extent dumping fails.
func (c *Chain) DumpExtents() error {
	newestToOldest := c.Sequence
	totalSectors := c.TotalSectors

	for _, fName := range newestToOldest {
		f, err := c.GetFile(fName)
		if err != nil {
			return fmt.Errorf("no open file for file %s", fName)
		}
		if err := dumpExtentsForFile(fName, f, totalSectors); err != nil {
			return err
		}
	}
	return nil
}

func dumpExtentsForFile(name string, f *os.File, totalSectors int64) error {
	extents, err := getAllExtents(f, uint64(totalSectors)*sectorSize)
	if err != nil {
		return fmt.Errorf("failed to read extents for %s: %w", name, err)
	}
	fmt.Printf("=== %s: %d extents ===\n", name, len(extents))
	for _, e := range extents {
		startSector := int64(e.Logical) / sectorSize
		lengthSectors := int64(e.Length) / sectorSize
		fmt.Printf("  logical=%d length=%d  -> sectors [%d, %d)\n",
			e.Logical, e.Length, startSector, startSector+lengthSectors)
	}
	return nil
}

// PrintSectorLocationTable walks the resolved location[] table and prints
// collapsed [start,end): owner ranges instead of one line per sector.
// This is the view you actually want to eyeball for correctness.
func PrintSectorLocationTable(smap *SectorMapping, totalSectors int64) {
	if totalSectors == 0 {
		return
	}

	fmt.Println("### does ownerfiles have head??? ", smap.OwnerFiles)
	location, names := smap.Location, smap.OwnerFiles

	runStart := int64(0)
	runOwnerIdx := location[0]

	for s := int64(1); s < totalSectors; s++ {
		idx := location[s]
		if idx != runOwnerIdx {
			if runOwnerIdx != 0 {
				fmt.Printf("[%d, %d): %s  (%d sectors)\n", runStart, s, names[runOwnerIdx], s-runStart)
			}
			runStart, runOwnerIdx = s, idx
		}
	}
	if runOwnerIdx != 0 {
		fmt.Printf("[%d, %d): %s  (%d sectors)\n", runStart, totalSectors, names[runOwnerIdx], totalSectors-runStart)
	}
}

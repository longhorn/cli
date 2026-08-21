package sectormap

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
)

// DumpExtents inspects and outputs physical extent mappings for all files in the storage chain.
// It processes the optional base backing file first, followed by each sequence layer ordered from
// newest to oldest. Returns an error if any file handle is missing or if extent dumping fails.
func (c *Chain) DumpExtents(smap *SectorMapping) error {
	newestToOldest := c.Sequence
	totalSectors := c.TotalSectors

	for _, fName := range newestToOldest {
		f, err := c.GetFile(fName)
		if err != nil {
			return fmt.Errorf("no open file for file %s", fName)
		}
		if err := dumpExtentsForFile(smap, fName, f, totalSectors); err != nil {
			return err
		}
	}
	return nil
}

func dumpExtentsForFile(smap *SectorMapping, name string, f *os.File, totalSectors int64) error {
	if existsIn(smap.ObsoleteFiles, name) {
		if fInfo, err := os.Stat(name); err == nil {
			logrus.Infof("file %v is expired (size %v), skipping extent retrieval", name, fInfo.Size())
		}
		return nil
	}

	if ranges, ok := smap.ExtentCache[name]; ok {
		fmt.Printf("=== %s: %d cached sector ranges ===\n", name, len(ranges))
		for _, r := range ranges {
			fmt.Printf("  sectors [%d, %d)\n", r.Start, r.End)
		}
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

func existsIn(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

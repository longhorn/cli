package sectormap

import (
	"fmt"
	"os"

	"github.com/rancher/go-fibmap"
)

// BuildSectorLocationMap resolves the first owner of each sector by scanning
// layer files from newest to oldest. It returns a per-sector location map,
// where each byte is an index into the returned names slice (0 = unclaimed,
// 1+ = layer files in scan order).
func (c *Chain) BuildSectorLocationMap() (*SectorMapping, error) {
	newestToOldest := c.Sequence
	totalSectors := c.TotalSectors
	if len(newestToOldest) == 0 {
		return &SectorMapping{}, fmt.Errorf("no layers provided")
	}
	if len(newestToOldest) > 254 {
		// byte can hold indices 1-255; Longhorn caps chains well below this anyway
		return &SectorMapping{}, fmt.Errorf("chain too long for a byte index: %d layers", len(newestToOldest))
	}

	location := make([]byte, totalSectors) // zero-value = sectorNil
	ownerFNames := []string{""}            // index 0 = reserved
	extentCache := make(map[string][]SectorRange, len(newestToOldest))

	var obsoleteFNames []string
	nextIndex := byte(1)
	remaining := totalSectors

	for _, fName := range newestToOldest {
		if remaining == 0 {
			obsoleteFNames = append(obsoleteFNames, fName)
			continue
		}

		idx := nextIndex
		ownerFNames = append(ownerFNames, fName)
		nextIndex++

		file, ok := c.Layers[fName]
		if !ok {
			return &SectorMapping{}, fmt.Errorf("no open file for %s", fName)
		}

		extents, err := getAllExtents(file.File, uint64(totalSectors)*sectorSize)
		if err != nil {
			return &SectorMapping{}, fmt.Errorf("failed to read extents for %s: %w", fName, err)
		}

		secRanges := make([]SectorRange, 0, len(extents))
		for _, extent := range extents {
			startSector := int64(extent.Logical) / sectorSize
			endSector := (int64(extent.Logical) + int64(extent.Length)) / sectorSize
			if endSector > totalSectors {
				endSector = totalSectors
			}
			for s := startSector; s < endSector; s++ {
				if location[s] == 0 {
					// enters only when unclaimed
					location[s] = idx
					remaining--
				}
			}
			secRanges = append(secRanges, SectorRange{startSector, endSector})
		}
		extentCache[fName] = secRanges
	}

	return &SectorMapping{Location: location, LocationFileNames: ownerFNames, ExtentCache: extentCache, ObsoleteFileNames: obsoleteFNames}, nil
}

// getAllExtents pulls the complete FIEMAP extent list for a file, paging
// through multiple ioctl calls if needed.
func getAllExtents(f *os.File, length uint64) ([]fibmap.Extent, error) {
	var all []fibmap.Extent
	start := uint64(0)

	for {
		extents, errno := fibmap.Fiemap(f.Fd(), start, length-start, maxExtentsPerCall)
		if errno != 0 {
			return nil, fmt.Errorf("fiemap errno: %v", errno)
		}
		if len(extents) == 0 {
			return all, nil
		}
		all = append(all, extents...)

		last := extents[len(extents)-1]
		if last.Flags&fibmap.FIEMAP_EXTENT_LAST != 0 {
			return all, nil
		}
		start = last.Logical + last.Length
	}
}

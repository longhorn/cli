package sectormap

import (
	"os"

	"github.com/longhorn/longhorn-engine/pkg/types"
)

type SectorSize int64
type LayerID uint8

type MetaFileMap map[string]types.DiskInfo

type SectorRange struct {
	Start, End int64 // sectors, half-open [Start, End)
}

type Layer struct {
	Name     string
	File     *os.File
	FileMeta *types.DiskInfo
	Extents  []SectorRange
}

type Chain struct {
	Layers          map[string]*Layer
	Sequence        []string
	Ancestors       map[string][]string
	BackingFileName string
	TotalSectors    int64
	SectorSize      int64
	dir             string
}

type VolumeMeta struct {
	Size            int64  `json:"Size"`
	Head            string `json:"Head"`
	Dirty           bool   `json:"Dirty"`
	Parent          string `json:"Parent"`
	SectorSize      int64  `json:"SectorSize"`
	BackingFilePath string `json:"BackingFilePath"`
}

type SectorMapping struct {
	Location          []byte
	LocationFileNames []string // list of files that own ≥1 sector. Indexed by Location values
	TotalSectors      int64
	SectorSize        int64
	ExtentCache       map[string][]SectorRange
	ObsoleteFileNames []string
}

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
	Layers       map[string]*Layer
	Sequence     []string
	Ancestors    map[string][]string
	BackingFile  string
	TotalSectors int64
	SectorSize   int64
}

type VolumeMeta struct {
	Size            int64  `json:"Size"`
	Head            string `json:"Head"`
	Dirty           bool   `json:"Dirty"`
	Parent          string `json:"Parent"`
	SectorSize      int64  `json:"SectorSize"`
	BackingFileName string `json:"BackingFileName"`
}

type SectorMapping struct {
	Location     []byte
	Names        []string
	TotalSectors int64
	SectorSize   int64
	ExtentCache  map[string][]SectorRange
}

package sectormap // LoadVolumeMeta reads and parses volume.meta from the replica dir.
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/longhorn/longhorn-engine/pkg/types"
)

func LoadVolumeMeta(dir string) (VolumeMeta, error) {
	var vm VolumeMeta
	data, err := os.ReadFile(filepath.Join(dir, "volume.meta"))
	if err != nil {
		return vm, fmt.Errorf("failed to read volume.meta: %w", err)
	}
	if err := json.Unmarshal(data, &vm); err != nil {
		return vm, fmt.Errorf("failed to unmarshal volume.meta: %w", err)
	}
	return vm, nil
}

// LoadDiskMetas loops through *.meta files in dir (skipping volume.meta, as is
// handled by loadVolumeMeta), unmarshals each into a types.DiskInfo, and
// returns a MetaFileMap keyed by the meta file's base name.
func LoadDiskMetas(dir string) (MetaFileMap, error) {
	metaFiles, err := filepath.Glob(filepath.Join(dir, "*.meta"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob *.meta files in %s: %w", dir, err)
	}

	metas := make(MetaFileMap)
	for _, metaFile := range metaFiles {
		// volume.meta is handled by LoadVolumeMeta
		if strings.HasSuffix(metaFile, "volume.meta") {
			continue
		}
		data, err := os.ReadFile(metaFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %v", metaFile, err)
		}
		var metaData types.DiskInfo
		if err := json.Unmarshal(data, &metaData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal meta file %s: %v", metaFile, err)
		}
		// Reset the DiskInfo.Name field to the actual filename (of metaFile), since
		// it's persisted as the oldHead's name rather than snapshot file's own name.
		// This is tracked for a proper fix in https://github.com/longhorn/longhorn/issues/13728.
		metaData.Name = strings.TrimSuffix(filepath.Base(metaFile), ".meta")
		metas[filepath.Base(metaFile)] = metaData
	}

	return metas, nil
}

package sectormap

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"

	"github.com/longhorn/longhorn-engine/pkg/types"
)

// NewChain initializes and returns a storage Chain constructed from the given directory and volume metadata.
// It loads layer metadata, orders the backing chain starting from vm.Head, opens all layer files for
// read/write access, and populates the ancestor dependency map.
func NewChain(dir string, vm VolumeMeta) (c *Chain, err error) {
	metas, err := LoadDiskMetas(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load disk metas: %w", err)
	}

	c = &Chain{
		Layers:          make(map[string]*Layer),
		Ancestors:       make(map[string][]string),
		BackingFileName: vm.BackingFilePath,
		TotalSectors:    vm.Size / vm.SectorSize,
		SectorSize:      vm.SectorSize,
		dir:             dir,
	}
	defer func() {
		if r := recover(); r != nil {
			closeErr := c.Close()
			c = nil
			err = errors.Join(fmt.Errorf("panic building chain: %v", r), closeErr)
			return
		}
		if err != nil {
			closeErr := c.Close()
			c = nil
			err = errors.Join(err, closeErr)
		}
	}()

	if err = c.OrderChain(metas, vm.Head+".meta"); err != nil {
		return nil, err
	}
	if err = c.OpenLayers(os.O_RDWR); err != nil {
		return nil, err
	}
	c.buildAncestors()
	return c, nil
}

// OrderChain traverses layer metadata starting from headMetaFile down to the base parent.
// It populates c.Layers with initial layer structures, assigns file metadata, and updates
// c.Sequence with layer names ordered from newest to oldest.
func (c *Chain) OrderChain(metas MetaFileMap, headMetaFile string) error {
	var newestToOldest []string

	curr, ok := metas[headMetaFile]
	if !ok {
		return fmt.Errorf("head %s not found", headMetaFile)
	}
	for {
		newestToOldest = append(newestToOldest, curr.Name)
		l := &Layer{Name: curr.Name}
		m := metas[curr.Name]
		l.FileMeta = &m
		c.Layers[curr.Name] = l

		if curr.Parent == "" || curr.Parent == c.BackingFileName {
			break
		}
		next, ok := metas[curr.Parent+".meta"]
		if !ok {
			return fmt.Errorf("parent %s referenced by %s but missing", curr.Parent, curr.Name)
		}
		curr = next
	}

	c.Sequence = newestToOldest
	return nil
}

// OpenLayers opens OS file descriptors for each layer in c.Sequence using the provided flag
func (c *Chain) OpenLayers(flag int) error {
	for _, name := range c.Sequence {
		file, err := os.OpenFile(filepath.Join(c.dir, name), flag, 0)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", name, err)
		}
		c.Layers[name].File = file
	}

	return nil
}

// RemoveObsoleteLayers removes specified layer files from the chain, relinking surviving layers
// and persisting updated metadata to dir.
func (c *Chain) RemoveObsoleteLayers(obsoleteFiles []string) error {
	if len(obsoleteFiles) == 0 {
		return nil
	}

	obsoleteSet := make(map[string]struct{}, len(obsoleteFiles))
	for _, file := range obsoleteFiles {
		obsoleteSet[file] = struct{}{}
	}

	// Filter obsolete entries out of c.Sequence
	newSequence := make([]string, 0, len(c.Sequence)-len(obsoleteFiles))
	for _, name := range c.Sequence {
		// All obsolete snapshots are adjacent, so once we hit the first obsolete name,
		// everything after it in newest-to-oldest order is obsolete too.
		if _, exists := obsoleteSet[name]; exists {
			break
		}
		newSequence = append(newSequence, name)
	}

	// The new oldest surviving layer is exactly the one whose Parent pointed into
	// the (to-be-deleted) obsolete run; relink it to become the chain's new base.
	if len(newSequence) > 0 {
		newBaseSnapName := newSequence[len(newSequence)-1]
		layer, ok := c.Layers[newBaseSnapName]
		if !ok || layer.FileMeta == nil {
			return fmt.Errorf("missing layer/meta for %s", newBaseSnapName)
		}

		if _, parentObsolete := obsoleteSet[layer.FileMeta.Parent]; !parentObsolete {
			return fmt.Errorf("invariant violated: new base %s's parent %q is not found as obsolete",
				newBaseSnapName, layer.FileMeta.Parent)
		}
		logrus.Infof("relinking %s: parent %q is obsolete, clearing to make it the new chain base", newBaseSnapName, layer.FileMeta.Parent)
		layer.FileMeta.Parent = c.BackingFileName

		if err := writeMetaAtomic(c.dir, newBaseSnapName, layer.FileMeta); err != nil {
			return fmt.Errorf("failed to persist relinked meta for %s: %w", newBaseSnapName, err)
		}
	}

	// close & delete the obsolete layers
	for fName := range obsoleteSet {
		if l, ok := c.Layers[fName]; ok && l.File != nil {
			if err := l.File.Close(); err != nil {
				logrus.Warnf("failed to close layer for %v: %v", fName, err)
			}
			l.File = nil
		}
		delete(c.Layers, fName)
	}

	c.Sequence = newSequence
	c.buildAncestors()

	// once the cached meta has been updated, delete the files from disk
	if err := deleteObsoleteFiles(obsoleteFiles); err != nil {
		return fmt.Errorf("failed to delete obsolete files: %w", err)
	}
	return nil
}

// Close closes every open layer. It is safe to call more than
// once and on a nil Chain, so the constructor's cleanup defer
// and a caller's defer can both fire.
func (c *Chain) Close() error {
	if c == nil {
		return nil
	}
	var errs []error
	for _, l := range c.Layers {
		if l.File == nil {
			continue
		}
		if err := l.File.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close %s: %w", l.Name, err))
		}
		l.File = nil
	}
	return errors.Join(errs...)
}

// GetFile returns the open file for name, or an error if it isn't in the chain
// or hasn't been opened.
func (c *Chain) GetFile(name string) (*os.File, error) {
	l, ok := c.Layers[name]
	if !ok {
		return nil, fmt.Errorf("no layer named %s in chain", name)
	}
	if l.File == nil {
		return nil, fmt.Errorf("layer %s has no open file", name)
	}
	return l.File, nil
}

// buildAncestors populates the Ancestors map, mapping each layer name to a
// slice of all underlying parent/ancestor layer names ordered from immediate parent to root.
func (c *Chain) buildAncestors() {
	for i, file := range c.Sequence {
		if i == len(c.Sequence)-1 {
			break
		}
		c.Ancestors[file] = c.Sequence[i+1:]
	}
}

// writeMetaAtomic writes meta as JSON to name+".meta" in dir, atomically:
// write to a temp file in the same directory, fsync it, then rename over
// the original. rename is atomic on the same filesystem, so a crash mid-write
// leaves either the old or the new content, never a half-written file.
func writeMetaAtomic(dir, name string, meta *types.DiskInfo) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("failed to marshal meta for %s: %w", name, err)
	}

	target := filepath.Join(dir, name+".meta")
	tmp, err := os.CreateTemp(dir, name+".meta.tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for %s: %w", name, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = os.Remove(tmpName)
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write meta for %s: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync meta for %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp meta file: %w", err)
	}
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("failed to rename temp meta file into place: %w", err)
	}
	return nil
}

// deleteObsoleteFiles deletes all the listed files and their respective *.meta files
func deleteObsoleteFiles(obsoleteFiles []string) error {
	for _, file := range obsoleteFiles {
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("failed to remove obsolete file %v: %w", file, err)
		}
		if err := os.Remove(file + ".meta"); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove obsolete meta file %v: %w", file+".meta", err)
		}
	}
	return nil
}

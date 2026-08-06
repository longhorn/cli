package sectormap

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
		Layers:       make(map[string]*Layer),
		Ancestors:    make(map[string][]string),
		BackingFile:  vm.BackingFileName,
		TotalSectors: vm.Size / vm.SectorSize,
		SectorSize:   vm.SectorSize,
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
	if err = c.OpenLayers(dir, os.O_RDWR); err != nil {
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

		// TODO: check for curr.Removed scenario
		//if curr.Removed {
		//	fmt.Printf("warning: %s is marked removed; verify the disk file still exists\n", curr.Name)
		//}

		if curr.Parent == "" {
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

// OpenLayers opens OS file descriptors for each layer in c.Sequence using the provided flag,
// and opens the optional backing file in read-only mode.
func (c *Chain) OpenLayers(dir string, flag int) error {
	//files := make(LayerFileMap, len(c.Sequence))
	for _, name := range c.Sequence {
		file, err := os.OpenFile(filepath.Join(dir, name), flag, 0)
		if err != nil {
			return fmt.Errorf("failed to open %s: %w", name, err)
		}
		c.Layers[name].File = file
	}

	if c.BackingFile == "" {
		return nil
	}
	// read-only: the backing image is never written or punched
	f, err := os.Open(filepath.Join(dir, c.BackingFile))
	if err != nil {
		return fmt.Errorf("failed to open backing file %s: %w", c.BackingFile, err)
	}
	c.Layers[c.BackingFile] = &Layer{Name: c.BackingFile, File: f}

	return nil
}

// Close closes every open layer, including the backing image. It is safe to
// call more than once and on a nil Chain, so the constructor's cleanup defer
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

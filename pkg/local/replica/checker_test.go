package replica

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	lhmgrutil "github.com/longhorn/longhorn-manager/util"
)

type CheckerTestSuite struct {
	suite.Suite

	replicaDirectory string
}

func (s *CheckerTestSuite) SetupTest() {
	s.replicaDirectory = s.T().TempDir()
}

func (s *CheckerTestSuite) writeVolumeMeta(head string) {
	volumeMeta := &lhmgrutil.VolumeMeta{
		Size:       1073741824,
		Head:       head,
		SectorSize: 512,
	}
	content, err := json.Marshal(volumeMeta)
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, volumeMetadataFile), content, 0600))
}

func (s *CheckerTestSuite) writeDiskMeta(diskName, parent string) {
	diskMeta := &diskMetadata{
		Name:   diskName,
		Parent: parent,
	}
	content, err := json.Marshal(diskMeta)
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, diskName+".meta"), content, 0600))
}

func (s *CheckerTestSuite) writeDiskImage(diskName string) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, diskName), []byte{}, 0600))
}

func (s *CheckerTestSuite) writeDisk(diskName, parent string) {
	s.writeDiskImage(diskName)
	s.writeDiskMeta(diskName, parent)
}

func (s *CheckerTestSuite) TestHealthyChain() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "")
	s.writeDisk("volume-snap-b.img", "volume-snap-a.img")
	s.writeDisk("volume-head-001.img", "volume-snap-b.img")

	chain, checkErrors, warnings := validateSnapshotChain(s.replicaDirectory)

	s.Empty(checkErrors)
	s.Empty(warnings)
	s.Equal([]string{"volume-head-001.img", "volume-snap-b.img", "volume-snap-a.img"}, chain)
}

func (s *CheckerTestSuite) TestHealthySnapshotTree() {
	// A snapshot tree with a branch (for example after a revert) is not broken:
	// both branches share the root, and the head is on one of them.
	s.writeVolumeMeta("volume-head-002.img")
	s.writeDisk("volume-snap-root.img", "")
	s.writeDisk("volume-snap-branch.img", "volume-snap-root.img")
	s.writeDisk("volume-head-002.img", "volume-snap-root.img")

	chain, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Empty(checkErrors)
	s.Equal([]string{"volume-head-002.img", "volume-snap-root.img"}, chain)
}

func (s *CheckerTestSuite) TestMissingParent() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-b.img", "volume-snap-a.img")
	s.writeDisk("volume-head-001.img", "volume-snap-b.img")

	chain, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 1)
	s.Contains(checkErrors[0], "broken snapshot chain")
	s.Contains(checkErrors[0], "volume-snap-a.img")
	s.Equal([]string{"volume-head-001.img", "volume-snap-b.img"}, chain)
}

func (s *CheckerTestSuite) TestMissingDiskFile() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "")
	s.writeDiskMeta("volume-snap-b.img", "volume-snap-a.img")
	s.writeDisk("volume-head-001.img", "volume-snap-b.img")

	_, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 1)
	s.Contains(checkErrors[0], "disk file volume-snap-b.img is missing")
}

func (s *CheckerTestSuite) TestMissingMetadataFile() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "")
	s.writeDiskImage("volume-snap-b.img")
	s.writeDisk("volume-head-001.img", "volume-snap-b.img")

	chain, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 2)
	s.Contains(checkErrors[0], "metadata file volume-snap-b.img.meta is missing")
	s.Contains(checkErrors[1], "broken snapshot chain")
	// The chain walk stops at the disk with the missing metadata.
	s.Equal([]string{"volume-head-001.img"}, chain)
}

func (s *CheckerTestSuite) TestMissingVolumeHead() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "")

	chain, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 2)
	s.Contains(checkErrors[0], "volume head volume-head-001.img")
	s.Contains(checkErrors[1], "volume head file volume-head-001.img")
	s.Empty(chain)
}

func (s *CheckerTestSuite) TestChainLoop() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "volume-snap-b.img")
	s.writeDisk("volume-snap-b.img", "volume-snap-a.img")
	s.writeDisk("volume-head-001.img", "volume-snap-a.img")

	_, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 1)
	s.Contains(checkErrors[0], "loop")
}

func (s *CheckerTestSuite) TestExtraVolumeHead() {
	s.writeVolumeMeta("volume-head-002.img")
	s.writeDisk("volume-snap-a.img", "")
	s.writeDisk("volume-head-002.img", "volume-snap-a.img")
	s.writeDisk("volume-head-001.img", "volume-snap-a.img")

	_, checkErrors, warnings := validateSnapshotChain(s.replicaDirectory)

	s.Empty(checkErrors)
	s.Len(warnings, 1)
	s.Contains(warnings[0], "unexpected volume head file volume-head-001.img")
}

func (s *CheckerTestSuite) TestCorruptedDiskMetadata() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "")
	s.writeDiskImage("volume-snap-b.img")
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, "volume-snap-b.img.meta"), []byte("{invalid json"), 0600))
	s.writeDisk("volume-head-001.img", "volume-snap-b.img")

	_, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.NotEmpty(checkErrors)
	s.Contains(checkErrors[0], "failed to parse disk metadata volume-snap-b.img.meta")
}

func (s *CheckerTestSuite) TestMetadataNameMismatch() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDiskImage("volume-snap-a.img")
	diskMeta := &diskMetadata{Name: "volume-snap-other.img"}
	content, err := json.Marshal(diskMeta)
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, "volume-snap-a.img.meta"), content, 0600))
	s.writeDisk("volume-head-001.img", "volume-snap-a.img")

	_, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 1)
	s.Contains(checkErrors[0], "mismatching disk name")
}

func (s *CheckerTestSuite) TestMissingVolumeMeta() {
	s.writeDisk("volume-snap-a.img", "")

	chain, checkErrors, _ := validateSnapshotChain(s.replicaDirectory)

	s.Len(checkErrors, 1)
	s.Contains(checkErrors[0], "failed to read volume.meta")
	s.Empty(chain)
}

func (s *CheckerTestSuite) TestIgnoresChecksumAndUnrelatedFiles() {
	s.writeVolumeMeta("volume-head-001.img")
	s.writeDisk("volume-snap-a.img", "")
	s.writeDisk("volume-head-001.img", "volume-snap-a.img")
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, "volume-snap-a.img.checksum"), []byte("{}"), 0600))
	s.Require().NoError(os.WriteFile(filepath.Join(s.replicaDirectory, "revision.counter"), []byte{}, 0600))

	_, checkErrors, warnings := validateSnapshotChain(s.replicaDirectory)

	s.Empty(checkErrors)
	s.Empty(warnings)
}

func TestCheckerTestSuite(t *testing.T) {
	suite.Run(t, new(CheckerTestSuite))
}

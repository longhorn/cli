package replica

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	lhmgrutil "github.com/longhorn/longhorn-manager/util"

	commonio "github.com/longhorn/go-common-libs/io"

	"github.com/longhorn/cli/pkg/consts"
	remote "github.com/longhorn/cli/pkg/remote/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
	utilslonghorn "github.com/longhorn/cli/pkg/utils/longhorn"
)

const (
	diskImageSuffix    = ".img"
	diskMetadataSuffix = ".img.meta"

	volumeHeadPrefix     = "volume-head-"
	volumeSnapshotPrefix = "volume-snap-"
	volumeMetadataFile   = "volume.meta"
)

// diskMetadata mirrors the metadata the Longhorn engine persists next to each
// disk file (volume head or snapshot) in the replica data directory.
type diskMetadata struct {
	Name        string
	Parent      string
	Removed     bool
	UserCreated bool
	Created     string
	Labels      map[string]string
}

// Checker provide functions for the replica checker.
type Checker struct {
	remote.CheckerCmdOptions

	logger *logrus.Entry

	OutputFilePath string
	CurrentNodeID  string

	replicasDirectory string
	replicaNames      []string

	collection types.ReplicaCheckCollection
}

// Init initializes the Checker.
func (local *Checker) Init() error {
	var err error

	if len(local.OutputFilePath) != 0 {
		local.logger = logrus.WithField("output", local.OutputFilePath)
	} else {
		local.logger = logrus.WithField("output", "stdout")
	}

	local.LonghornDataDirectory, err = utilslonghorn.GetDataDirectory(local.logger, consts.VolumeMountHostDirectory, local.LonghornDataDirectory)
	if err != nil {
		return errors.Wrap(err, "failed to get Longhorn data directory")
	}

	local.logger = local.logger.WithField("data-dir", local.LonghornDataDirectory)

	local.replicasDirectory = filepath.Join(local.LonghornDataDirectory, "replicas")

	local.collection.Replicas = make(map[string][]*types.ReplicaCheckInfo)

	return nil
}

// Run checks the snapshot chain integrity of the replicas in the data directory.
func (local *Checker) Run() error {
	var err error

	log := local.logger
	if local.VolumeName != "" {
		log = log.WithField("volume", local.VolumeName)
	}
	if local.ReplicaName != "" {
		log = log.WithField("replica", local.ReplicaName)
	}

	local.replicaNames, err = getReplicaNamesInDirectory(log, local.replicasDirectory, local.VolumeName, local.ReplicaName)
	if err != nil {
		return err
	}

	for _, replicaName := range local.replicaNames {
		replicaCheckInfo, err := local.checkReplica(replicaName)
		if err != nil {
			return errors.Wrapf(err, "failed to check replica %s", replicaName)
		}

		if replicaCheckInfo == nil {
			continue
		}

		local.collection.Replicas[replicaName] = append(local.collection.Replicas[replicaName], replicaCheckInfo)
	}

	return nil
}

// Output converts the collection to JSON and output to stdout or the output file.
func (local *Checker) Output() error {
	local.logger.Tracef("Outputting replica checker results")

	jsonBytes, err := json.Marshal(local.collection)
	if err != nil {
		return errors.Wrap(err, "failed to convert replica check collections to JSON")
	}

	return utils.HandleResult(jsonBytes, local.OutputFilePath, local.logger)
}

// checkReplica checks the snapshot chain integrity of a single replica directory.
func (local *Checker) checkReplica(replicaName string) (*types.ReplicaCheckInfo, error) {
	log := local.logger

	log.Infof("Checking snapshot chain for replica %s", replicaName)

	replicaCheckInfo := &types.ReplicaCheckInfo{}
	replicaCheckInfo.Node = local.CurrentNodeID
	replicaCheckInfo.VolumeName = replicaName[:strings.LastIndex(replicaName, "-")]

	replicaDirectory := filepath.Join(local.replicasDirectory, replicaName)
	replicaCheckInfo.Directory = strings.TrimPrefix(replicaDirectory, consts.VolumeMountHostDirectory)

	isEmpty, err := commonio.IsDirectoryEmpty(replicaDirectory)
	if err != nil {
		replicaCheckInfo.Errors = append(replicaCheckInfo.Errors, errors.Wrapf(err, "failed to check if directory %s is empty", replicaCheckInfo.Directory).Error())
		return replicaCheckInfo, nil
	}

	if isEmpty {
		log.Warnf("Replica directory %s is empty", replicaCheckInfo.Directory)
		replicaCheckInfo.Warnings = append(replicaCheckInfo.Warnings, "replica directory is empty")
		return replicaCheckInfo, nil
	}

	isReplicaInUse, err := isReplicaDirectoryInUse(replicaDirectory)
	if err != nil {
		replicaCheckInfo.Warnings = append(replicaCheckInfo.Warnings, errors.Wrapf(err, "failed to check if replica %s is in use", replicaName).Error())
	} else if isReplicaInUse {
		replicaCheckInfo.Warnings = append(replicaCheckInfo.Warnings, "replica is in use; findings may be transient while the engine is modifying the snapshot chain")
	}

	chain, checkErrors, warnings := validateSnapshotChain(replicaDirectory)
	replicaCheckInfo.SnapshotChain = chain
	replicaCheckInfo.Errors = append(replicaCheckInfo.Errors, checkErrors...)
	replicaCheckInfo.Warnings = append(replicaCheckInfo.Warnings, warnings...)

	return replicaCheckInfo, nil
}

// validateSnapshotChain inspects the disk files in the given replica directory
// and returns the volume head chain (from the head to the root), along with any
// integrity errors and warnings found.
func validateSnapshotChain(replicaDirectory string) (chain []string, checkErrors []string, warnings []string) {
	volumeMeta := &lhmgrutil.VolumeMeta{}
	content, err := os.ReadFile(filepath.Join(replicaDirectory, volumeMetadataFile))
	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to read %s: %v", volumeMetadataFile, err))
		return nil, checkErrors, warnings
	}
	if err := json.Unmarshal(content, volumeMeta); err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to parse %s: %v", volumeMetadataFile, err))
		return nil, checkErrors, warnings
	}

	if volumeMeta.Error != "" {
		checkErrors = append(checkErrors, fmt.Sprintf("%s records an error: %s", volumeMetadataFile, volumeMeta.Error))
	}
	if volumeMeta.Rebuilding {
		warnings = append(warnings, "replica is marked as rebuilding; the snapshot chain may be incomplete until the rebuild finishes")
	}

	disks, images, headImages, diskReadErrors := readDiskMetadataFiles(replicaDirectory)
	checkErrors = append(checkErrors, diskReadErrors...)

	// Every disk file must have a metadata file, and vice versa.
	for _, diskName := range sortedKeys(disks) {
		if !images[diskName] {
			checkErrors = append(checkErrors, fmt.Sprintf("disk metadata %s%s exists, but disk file %s is missing", diskName, ".meta", diskName))
		}
		if metaName := disks[diskName].Name; metaName != "" && metaName != diskName {
			checkErrors = append(checkErrors, fmt.Sprintf("disk metadata %s%s declares mismatching disk name %s", diskName, ".meta", metaName))
		}
	}
	for _, imageName := range sortedKeys(images) {
		if _, ok := disks[imageName]; !ok {
			checkErrors = append(checkErrors, fmt.Sprintf("disk file %s exists, but its metadata file %s%s is missing", imageName, imageName, ".meta"))
		}
	}

	// Every parent reference must resolve, including the ones on snapshot tree
	// branches that are not part of the volume head chain.
	for _, diskName := range sortedKeys(disks) {
		parent := disks[diskName].Parent
		if parent == "" {
			continue
		}
		if _, ok := disks[parent]; !ok {
			checkErrors = append(checkErrors, fmt.Sprintf("broken snapshot chain: disk %s references parent %s, but the parent metadata file is missing", diskName, parent))
		}
	}

	head := volumeMeta.Head
	if head == "" {
		checkErrors = append(checkErrors, fmt.Sprintf("%s does not specify a volume head", volumeMetadataFile))
		return nil, checkErrors, warnings
	}

	if _, ok := disks[head]; !ok {
		checkErrors = append(checkErrors, fmt.Sprintf("broken snapshot chain: volume head %s declared in %s is missing its metadata file", head, volumeMetadataFile))
	}
	if !images[head] {
		checkErrors = append(checkErrors, fmt.Sprintf("broken snapshot chain: volume head file %s declared in %s is missing", head, volumeMetadataFile))
	}

	sort.Strings(headImages)
	for _, headImage := range headImages {
		if headImage != head {
			warnings = append(warnings, fmt.Sprintf("found unexpected volume head file %s; the active volume head is %s", headImage, head))
		}
	}

	// Walk the chain from the volume head to the root. Dangling parents and
	// missing files are already reported above, so the walk only needs to
	// detect loops and record the reachable chain.
	visited := map[string]bool{}
	current := head
	for current != "" {
		if visited[current] {
			checkErrors = append(checkErrors, fmt.Sprintf("snapshot chain contains a loop at disk %s", current))
			break
		}
		visited[current] = true

		diskMeta, ok := disks[current]
		if !ok {
			break
		}

		chain = append(chain, current)
		current = diskMeta.Parent
	}

	return chain, checkErrors, warnings
}

// readDiskMetadataFiles reads the disk files in the given replica directory and
// returns the parsed disk metadata, the set of disk image files, and the volume
// head image files found.
func readDiskMetadataFiles(replicaDirectory string) (disks map[string]*diskMetadata, images map[string]bool, headImages []string, checkErrors []string) {
	disks = map[string]*diskMetadata{}
	images = map[string]bool{}

	entries, err := os.ReadDir(replicaDirectory)
	if err != nil {
		checkErrors = append(checkErrors, fmt.Sprintf("failed to list replica directory: %v", err))
		return disks, images, headImages, checkErrors
	}

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, volumeHeadPrefix) && !strings.HasPrefix(name, volumeSnapshotPrefix) {
			continue
		}

		switch {
		case strings.HasSuffix(name, diskMetadataSuffix):
			content, err := os.ReadFile(filepath.Join(replicaDirectory, name))
			if err != nil {
				checkErrors = append(checkErrors, fmt.Sprintf("failed to read disk metadata %s: %v", name, err))
				continue
			}

			diskMeta := &diskMetadata{}
			if err := json.Unmarshal(content, diskMeta); err != nil {
				checkErrors = append(checkErrors, fmt.Sprintf("failed to parse disk metadata %s: %v", name, err))
				continue
			}

			disks[strings.TrimSuffix(name, ".meta")] = diskMeta

		case strings.HasSuffix(name, diskImageSuffix):
			images[name] = true
			if strings.HasPrefix(name, volumeHeadPrefix) {
				headImages = append(headImages, name)
			}
		}
	}

	return disks, images, headImages, checkErrors
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

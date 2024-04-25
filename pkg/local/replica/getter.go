package replica

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	lhmgrutil "github.com/longhorn/longhorn-manager/util"

	lhgoio "github.com/longhorn/go-common-libs/io"
	lhgotypes "github.com/longhorn/go-common-libs/types"

	"github.com/longhorn/cli/pkg/consts"
	remote "github.com/longhorn/cli/pkg/remote/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
	utilslonghorn "github.com/longhorn/cli/pkg/utils/longhorn"
)

// Getter provide functions for the replica getter.
type Getter struct {
	remote.GetterCmdOptions

	logger *logrus.Entry

	OutputFilePath string
	CurrentNodeID  string

	replicasDirectory string
	replicaNames      []string

	collection types.ReplicaCollection
}

// Init initializes the Getter.
func (local *Getter) Init() error {
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

	local.collection.Replicas = make(map[string][]*types.ReplicaInfo)

	return nil
}

// Run collects the replica information and outputs the result to stdout or a file.
func (local *Getter) Run() error {
	var err error

	local.replicaNames, err = local.getReplicaNamesInDirectory()
	if err != nil {
		return err
	}

	for _, replicaName := range local.replicaNames {
		replicaInfo, err := local.getReplicaInfo(replicaName)
		if err != nil {
			return errors.Wrapf(err, "failed to get replica info for %s", replicaName)
		}

		if replicaInfo == nil {
			continue
		}

		local.collection.Replicas[replicaName] = append(local.collection.Replicas[replicaName], replicaInfo)
	}

	return nil
}

// Output converts the collection to JSON and output to stdout or the output file.
func (local *Getter) Output() error {
	local.logger.Tracef("Outputting replica getter results")

	jsonBytes, err := json.Marshal(local.collection)
	if err != nil {
		return errors.Wrap(err, "failed to convert replica collections to JSON")
	}

	return utils.HandleResult(jsonBytes, local.OutputFilePath, local.logger)
}

// getReplicaNamesInDirectory returns a list of replica names in the given directory that match the given volume name.
// If the volume name is empty, it returns all replica names in the given directory.
func (local *Getter) getReplicaNamesInDirectory() ([]string, error) {
	log := local.logger
	replicasDirectory := local.replicasDirectory

	if local.VolumeName != "" {
		log = log.WithField("volume", local.VolumeName)
	}
	if local.ReplicaName != "" {
		log = log.WithField("replica", local.ReplicaName)
	}
	log.Infof("Searching for replicas in %s", replicasDirectory)

	filePaths, err := lhgoio.FindFiles(replicasDirectory, "", 1)
	if err != nil {
		return nil, err
	}

	var replicaNames []string

	for _, filePath := range filePaths {
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return nil, err
		}

		if !fileInfo.IsDir() {
			continue
		}

		replicaName := filepath.Base(filePath)

		// Skip the replicas directory itself.
		if filePath == replicasDirectory {
			continue
		}

		if local.ReplicaName != "" && local.ReplicaName != replicaName {
			continue
		}

		volumeName := replicaName[:strings.LastIndex(replicaName, "-")]
		if local.VolumeName != "" && local.VolumeName != volumeName {
			continue
		}

		replicaNames = append(replicaNames, replicaName)
	}

	return replicaNames, nil
}

func (local *Getter) getReplicaInfo(replicaName string) (replicaInfo *types.ReplicaInfo, err error) {
	log := local.logger

	log.Infof("Getting replica info for %s", replicaName)

	replicaInfo = &types.ReplicaInfo{}
	replicaInfo.Node = local.CurrentNodeID

	replicaDirectory := filepath.Join(local.replicasDirectory, replicaName)
	replicaInfo.Directory = strings.TrimPrefix(replicaDirectory, consts.VolumeMountHostDirectory)

	isEmpty, err := lhgoio.IsDirectoryEmpty(replicaDirectory)
	if err != nil {
		replicaInfo.Error = errors.Wrapf(err, "failed to check if directory %s is empty", replicaInfo.Directory).Error()
		return replicaInfo, nil
	}

	if isEmpty {
		log.Warnf("Replica directory %s is empty", replicaInfo.Directory)
		return nil, nil
	}

	replicaInfo.VolumeName = replicaName[:strings.LastIndex(replicaName, "-")]
	replicaInfo.Metadata, err = lhmgrutil.GetVolumeMeta(filepath.Join(replicaInfo.Directory, "volume.meta"))
	if err != nil {
		replicaInfo.Error = errors.Wrapf(err, "failed to get volume metadata for %s", replicaName).Error()
		return replicaInfo, nil
	}

	isReplicaInUse, err := local.isReplicaInUse(replicaName)
	if err != nil {
		replicaInfo.Error = errors.Wrapf(err, "failed to check if replica %s is in use", replicaName).Error()
		return replicaInfo, nil
	}
	replicaInfo.IsInUse = &isReplicaInUse

	return replicaInfo, nil
}

func (local *Getter) isReplicaInUse(name string) (bool, error) {
	replicaDirectory := filepath.Join(local.replicasDirectory, name)

	// Check if replica path exists
	if _, err := os.Stat(replicaDirectory); os.IsNotExist(err) {
		return false, errors.Wrapf(err, "replica directory %s does not exist", replicaDirectory)
	}

	logrus.Tracef("Listing open files in %s", replicaDirectory)

	openedFiles, err := lhgoio.ListOpenFiles(lhgotypes.HostProcDirectory, replicaDirectory)
	if err != nil {
		return false, errors.Wrapf(err, "failed to list open files in %s", replicaDirectory)
	}

	return len(openedFiles) != 0, nil
}

package longhorn

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	commonio "github.com/longhorn/go-common-libs/io"

	"github.com/longhorn/cli/pkg/consts"
)

func FindDataDirectory(logger *logrus.Entry, hostDirectory string) (string, error) {
	logger.Debug("Finding Longhorn data directory")

	foundFiles, err := commonio.FindFiles(hostDirectory, consts.LonghornDiskConfigFile, 0)
	if err != nil {
		return "", errors.Wrapf(err, "failed to find %s in %s", consts.LonghornDiskConfigFile, consts.VolumeMountHostDirectory)
	}

	if len(foundFiles) == 0 {
		return "", errors.Wrapf(err, "cannot find %s in %s", consts.LonghornDiskConfigFile, consts.VolumeMountHostDirectory)
	}

	// Blindly return the one with the shortest path. Best effort.
	sort.Slice(foundFiles, func(i, j int) bool {
		return len(foundFiles[i]) < len(foundFiles[j])
	})

	dataDir := filepath.Dir(foundFiles[0])
	logger.Debugf("Found Longhorn data directory %s", dataDir)

	return dataDir, nil
}

func GetDataDirectory(logger *logrus.Entry, hostDirectory, inputDataDirectory string) (dataDir string, err error) {
	if inputDataDirectory == "" {
		logger.Debugf("Longhorn data directory is not specified, searching the directory where %s is located", consts.LonghornDiskConfigFile)
		dataDir, err = FindDataDirectory(logger, hostDirectory)
		if err != nil {
			return "", err
		}
	} else {
		dataDir = filepath.Join(hostDirectory, inputDataDirectory)
		// check if directory exists
		_, err := os.Stat(dataDir)
		if err != nil {
			if os.IsNotExist(err) {
				dataDir, err = FindDataDirectory(logger, hostDirectory)
				if err != nil {
					return "", err
				}
			} else {
				return "", errors.Wrapf(err, "directory %s does not exist", dataDir)
			}
		}
	}

	return dataDir, nil
}

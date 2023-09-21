package installer

import (
	"os"
	"path/filepath"
	"strings"

	cp "github.com/otiai10/copy"
	"github.com/sirupsen/logrus"

	lhtypes "github.com/longhorn/go-common-libs/types"
)

const (
	spdkPath       = "/host/tmp/longhorn-spdk"
	spdkPathOnHost = "/tmp/longhorn-spdk"
)

func (i *Installer) ConfigureSPDKEnv() error {
	// Blindly remove the SPDK source code directory if it exists
	if err := os.RemoveAll(spdkPath); err != nil {
		return err
	}
	if err := cp.Copy("/spdk", spdkPath); err != nil {
		return err
	}
	defer os.RemoveAll(spdkPath)

	// Configure SPDK environment
	logrus.Infof("Configuring SPDK environment")
	args := getArgsForConfiguringSPDKEnv()
	if _, err := i.command.Execute("bash", args, lhtypes.ExecuteNoTimeout); err != nil {
		logrus.WithError(err).Errorf("Failed to configure SPDK environment")
	} else {
		logrus.Infof("Successfully configured SPDK environment")
	}

	return nil
}

func getArgsForConfiguringSPDKEnv() []string {
	args := []string{filepath.Join(spdkPathOnHost, "scripts/setup.sh")}
	value := os.Getenv("SPDK_OPTIONS")
	if value != "" {
		logrus.Infof("Configuring SPDK environment with custom options: %v", os.Getenv("SPDK_OPTIONS"))
		customOptions := strings.Split(value, " ")
		args = append(args, customOptions...)
	}
	return args
}

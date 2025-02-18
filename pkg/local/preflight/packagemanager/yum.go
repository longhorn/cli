package packagemanager

import (
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

type YumPackageManager struct {
	executor *commonns.Executor
}

func NewYumPackageManager(executor *commonns.Executor) *YumPackageManager {
	return &YumPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *YumPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, "yum", []string{"update", "-y"}, commontypes.ExecuteNoTimeout)
}

// StartPackageSession start a session to install/uninstall packages in a unique transaction
func (c *YumPackageManager) StartPackageSession() (string, error) {
	return "", nil
}

// InstallPackage executes the installation command
func (c *YumPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "yum", []string{"install", name, "-y"}, commontypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *YumPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "yum", []string{"remove", name, "-y"}, commontypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *YumPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *YumPackageManager) Modprobe(module string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", []string{module}, commontypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *YumPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, commontypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *YumPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, commontypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *YumPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, commontypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *YumPackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute([]string{}, "rpm", []string{"-q", name}, commontypes.ExecuteNoTimeout)
}

// NeedReboot tells if a reboot is needed after package installation
func (c *YumPackageManager) NeedReboot() bool {
	return false
}

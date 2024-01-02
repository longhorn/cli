package pkgmgr

import (
	"time"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

type YumPackageManager struct {
	executor *lhns.Executor
}

func NewYumPackageManager(executor *lhns.Executor) *YumPackageManager {
	return &YumPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *YumPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute("yum", []string{"update", "-y"}, lhtypes.ExecuteNoTimeout)
}

// InstallPackage executes the installation command
func (c *YumPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute("yum", []string{"install", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *YumPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute("yum", []string{"remove", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *YumPackageManager) Execute(binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *YumPackageManager) Modprobe(module string) (string, error) {
	return c.executor.Execute("modprobe", []string{module}, lhtypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *YumPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute("grep", []string{module, "/proc/modules"}, lhtypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *YumPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute("systemctl", []string{"-q", "enable", name}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute("systemctl", []string{"start", name}, lhtypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *YumPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute("systemctl", []string{"status", "--no-pager", name}, lhtypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *YumPackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute("rpm", []string{"-q", name}, lhtypes.ExecuteNoTimeout)
}

package pkgmgr

import (
	"time"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

type AptPackageManager struct {
	executor *lhns.Executor
}

func NewAptPackageManager(executor *lhns.Executor) *AptPackageManager {
	return &AptPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *AptPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, "apt", []string{"update", "-y"}, lhtypes.ExecuteNoTimeout)
}

// InstallPackage executes the installation command
func (c *AptPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "apt", []string{"install", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *AptPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "apt", []string{"remove", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *AptPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *AptPackageManager) Modprobe(module string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", []string{module}, lhtypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *AptPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, lhtypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *AptPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, lhtypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *AptPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, lhtypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *AptPackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute([]string{}, "dpkg-query", []string{"-l", name}, lhtypes.ExecuteNoTimeout)
}

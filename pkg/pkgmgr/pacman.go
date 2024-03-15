package pkgmgr

import (
	"time"

	lhns "github.com/longhorn/go-common-libs/ns"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

type PacmanPackageManager struct {
	executor *lhns.Executor
}

func NewPacmanPackageManager(executor *lhns.Executor) *PacmanPackageManager {
	return &PacmanPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *PacmanPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-Syu", "--noconfirm"}, lhtypes.ExecuteNoTimeout)
}

// InstallPackage executes the installation command
func (c *PacmanPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-S", "--noconfirm", name}, lhtypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *PacmanPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-R", "--noconfirm", name}, lhtypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *PacmanPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *PacmanPackageManager) Modprobe(module string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", []string{module}, lhtypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *PacmanPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, lhtypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *PacmanPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, lhtypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *PacmanPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, lhtypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *PacmanPackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-Q", name}, lhtypes.ExecuteNoTimeout)
}

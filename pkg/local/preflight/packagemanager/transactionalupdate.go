package packagemanager

import (
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

const (
	packageCommand = "transactional-update"
)

type TransactionalUpdatePackageManager struct {
	executor *commonns.Executor
}

func NewTransactionalUpdatePackageManager(executor *commonns.Executor) *TransactionalUpdatePackageManager {
	return &TransactionalUpdatePackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *TransactionalUpdatePackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, packageCommand, []string{"pkg", "update", "-y"}, commontypes.ExecuteNoTimeout)
}

// InstallPackage executes the installation command
func (c *TransactionalUpdatePackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, packageCommand, []string{"--non-interactive", "pkg", "install", name}, commontypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *TransactionalUpdatePackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, packageCommand, []string{"--non-interactive", "pkg", "remove", name}, commontypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *TransactionalUpdatePackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *TransactionalUpdatePackageManager) Modprobe(module string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", []string{module}, commontypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *TransactionalUpdatePackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, commontypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *TransactionalUpdatePackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, commontypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *TransactionalUpdatePackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, commontypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *TransactionalUpdatePackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute([]string{}, "rpm", []string{"-q", name}, commontypes.ExecuteNoTimeout)
}

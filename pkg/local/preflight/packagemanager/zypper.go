package packagemanager

import (
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

type ZypperPackageManager struct {
	executor *commonns.Executor
}

func NewZypperPackageManager(executor *commonns.Executor) *ZypperPackageManager {
	return &ZypperPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *ZypperPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, "zypper", []string{"update", "-y"}, commontypes.ExecuteNoTimeout)
}

// StartPackageSession start a session to install/uninstall packages in a unique transaction
func (c *ZypperPackageManager) StartPackageSession() (string, error) {
	return "", nil
}

// InstallPackage executes the installation command
func (c *ZypperPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "zypper", []string{"--non-interactive", "install", name}, commontypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *ZypperPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "zypper", []string{"--non-interactive", "remove", name}, commontypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *ZypperPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *ZypperPackageManager) Modprobe(module string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", []string{module}, commontypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *ZypperPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, commontypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *ZypperPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, commontypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *ZypperPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, commontypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *ZypperPackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute([]string{}, "rpm", []string{"-q", name}, commontypes.ExecuteNoTimeout)
}

// NeedReboot tells if a reboot is needed after package installation
func (c *ZypperPackageManager) NeedReboot() bool {
	return false
}

package packagemanager

import (
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

type PacmanPackageManager struct {
	executor *commonns.Executor
}

func NewPacmanPackageManager(executor *commonns.Executor) *PacmanPackageManager {
	return &PacmanPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *PacmanPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-Syu", "--noconfirm"}, commontypes.ExecuteNoTimeout)
}

// StartPackageSession start a session to install/uninstall packages in a unique transaction
func (c *PacmanPackageManager) StartPackageSession() (string, error) {
	return "", nil
}

// InstallPackage executes the installation command
func (c *PacmanPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-S", "--noconfirm", name}, commontypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *PacmanPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-R", "--noconfirm", name}, commontypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *PacmanPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *PacmanPackageManager) Modprobe(module string, opts ...string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", append(opts, module), commontypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *PacmanPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, commontypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *PacmanPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, commontypes.ExecuteNoTimeout)
}

// RestartService executes the service restart command
func (c *PacmanPackageManager) RestartService(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"restart", name}, commontypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *PacmanPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, commontypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *PacmanPackageManager) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute([]string{}, "pacman", []string{"-Q", name}, commontypes.ExecuteNoTimeout)
}

// NeedReboot tells if a reboot is needed after package installation
func (c *PacmanPackageManager) NeedReboot() bool {
	return false
}

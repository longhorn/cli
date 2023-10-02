package yum

import (
	"time"

	lhns "github.com/longhorn/go-common-libs/namespace"
	lhtypes "github.com/longhorn/go-common-libs/types"
)

type Command struct {
	executor *lhns.Executor
}

func NewCommand(executor *lhns.Executor) *Command {
	return &Command{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *Command) UpdatePackageList() (string, error) {
	return c.executor.Execute("yum", []string{"update", "-y"}, lhtypes.ExecuteNoTimeout)
}

// InstallPackage executes the installation command
func (c *Command) InstallPackage(name string) (string, error) {
	return c.executor.Execute("yum", []string{"install", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *Command) UninstallPackage(name string) (string, error) {
	return c.executor.Execute("yum", []string{"remove", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *Command) Execute(binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *Command) Modprobe(module string) (string, error) {
	return c.executor.Execute("modprobe", []string{module}, lhtypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *Command) CheckModLoaded(module string) error {
	_, err := c.executor.Execute("grep", []string{module, "/proc/modules"}, lhtypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *Command) StartService(name string) (string, error) {
	output, err := c.executor.Execute("systemctl", []string{"-q", "enable", name}, lhtypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute("systemctl", []string{"start", name}, lhtypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *Command) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute("systemctl", []string{"status", "--no-pager", name}, lhtypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *Command) CheckPackageInstalled(name string) (string, error) {
	return c.executor.Execute("rpm", []string{"-q", name}, lhtypes.ExecuteNoTimeout)
}

package apt

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
	return c.executor.Execute("apt", []string{"update", "-y"}, lhtypes.ExecuteNoTimeout)
}

// InstallPackage executes the installation command
func (c *Command) InstallPackage(name string) (string, error) {
	return c.executor.Execute("apt", []string{"install", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *Command) UninstallPackage(name string) (string, error) {
	return c.executor.Execute("apt", []string{"remove", name, "-y"}, lhtypes.ExecuteNoTimeout)
}

// ListPackages lists all installed packages
func (c *Command) ListPackages() (string, error) {
	return c.executor.Execute("apt", []string{"list", "--installed"}, lhtypes.ExecuteNoTimeout)
}

// PipInstallPackage executes the pip installation command
func (c *Command) PipInstallPackage(name string) (string, error) {
	return c.executor.Execute("pip3", []string{"install", name}, lhtypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *Command) Execute(binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(binary, args, timeout)
}

func (c *Command) Modprobe(module string) (string, error) {
	return c.executor.Execute("modprobe", []string{module}, lhtypes.ExecuteNoTimeout)
}

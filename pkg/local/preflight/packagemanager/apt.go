package packagemanager

import (
	"strings"
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

type AptPackageManager struct {
	executor *commonns.Executor
}

func NewAptPackageManager(executor *commonns.Executor) *AptPackageManager {
	return &AptPackageManager{
		executor: executor,
	}
}

// UpdatePackageList updates list of available packages
func (c *AptPackageManager) UpdatePackageList() (string, error) {
	return c.executor.Execute([]string{}, "apt", []string{"update", "-y"}, commontypes.ExecuteNoTimeout)
}

// StartPackageSession start a session to install/uninstall packages in a unique transaction
func (c *AptPackageManager) StartPackageSession() (string, error) {
	return "", nil
}

// InstallPackage executes the installation command
func (c *AptPackageManager) InstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "apt", []string{"install", name, "-y"}, commontypes.ExecuteNoTimeout)
}

// UninstallPackage executes the uninstallation command
func (c *AptPackageManager) UninstallPackage(name string) (string, error) {
	return c.executor.Execute([]string{}, "apt", []string{"remove", name, "-y"}, commontypes.ExecuteNoTimeout)
}

// Execute executes the given command with the specified environment variables, binary, and arguments.
func (c *AptPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

// Modprobe executes the modprobe command
func (c *AptPackageManager) Modprobe(module string, opts ...string) (string, error) {
	return c.executor.Execute([]string{}, "modprobe", append(opts, module), commontypes.ExecuteNoTimeout)
}

// CheckModLoaded checks if a module is loaded
func (c *AptPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, commontypes.ExecuteNoTimeout)
	return err
}

// StartService executes the service start command
func (c *AptPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}

	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, commontypes.ExecuteNoTimeout)
}

// RestartService executes the service restart command
func (c *AptPackageManager) RestartService(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"restart", name}, commontypes.ExecuteNoTimeout)
}

// GetServiceStatus executes the service status command
func (c *AptPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, commontypes.ExecuteNoTimeout)
}

// CheckPackageInstalled checks if a package is installed
func (c *AptPackageManager) CheckPackageInstalled(name string) (output string, err error) {
	// Check man 1 dpkg-query for status flags.
	// example for an installed package:
	// $ dpkg-query -f='${binary:Package} ${db:Status-Abbrev}' -W nfs-common
	// nfs-common ii
	output, err = c.executor.Execute([]string{}, "dpkg-query", []string{"-f=${binary:Package} ${db:Status-Abbrev}", "-W", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return
	}
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) == 2 {
		if fields[0] == name && fields[1] == "ii" {
			return output, nil
		}
	}
	return output, ErrPackageNotInstalled
}

// NeedReboot tells if a reboot is needed after package installation
func (c *AptPackageManager) NeedReboot() bool {
	return false
}

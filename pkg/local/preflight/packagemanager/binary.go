package packagemanager

import (
	"fmt"
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

type BinaryPackageManager struct {
	executor *commonns.Executor
}

func NewBinaryPackageManager(executor *commonns.Executor) *BinaryPackageManager {
	return &BinaryPackageManager{
		executor: executor,
	}
}

// Map logical package names to host executable binaries
var packageToBinaryMap = map[string]string{
	"nfs-client":    "mount.nfs4",
	"open-iscsi":    "iscsiadm",
	"cryptsetup":    "cryptsetup",
	"device-mapper": "dmsetup",
}

func (c *BinaryPackageManager) UpdatePackageList() (string, error) {
	return "", nil
}

func (c *BinaryPackageManager) StartPackageSession() (string, error) {
	return "", nil
}

func (c *BinaryPackageManager) InstallPackage(name string) (string, error) {
	return "", fmt.Errorf("package installation is not supported on binary-only/immutable host systems")
}

func (c *BinaryPackageManager) UninstallPackage(name string) (string, error) {
	return "", fmt.Errorf("package uninstallation is not supported on binary-only/immutable host systems")
}

func (c *BinaryPackageManager) Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error) {
	return c.executor.Execute(envs, binary, args, timeout)
}

func (c *BinaryPackageManager) Modprobe(module string, args ...string) (string, error) {
	cmdArgs := append([]string{module}, args...)
	return c.executor.Execute([]string{}, "modprobe", cmdArgs, commontypes.ExecuteNoTimeout)
}

func (c *BinaryPackageManager) CheckModLoaded(module string) error {
	_, err := c.executor.Execute([]string{}, "grep", []string{module, "/proc/modules"}, commontypes.ExecuteNoTimeout)
	return err
}

func (c *BinaryPackageManager) StartService(name string) (string, error) {
	output, err := c.executor.Execute([]string{}, "systemctl", []string{"-q", "enable", name}, commontypes.ExecuteNoTimeout)
	if err != nil {
		return output, err
	}
	return c.executor.Execute([]string{}, "systemctl", []string{"start", name}, commontypes.ExecuteNoTimeout)
}

func (c *BinaryPackageManager) RestartService(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"restart", name}, commontypes.ExecuteNoTimeout)
}

func (c *BinaryPackageManager) GetServiceStatus(name string) (string, error) {
	return c.executor.Execute([]string{}, "systemctl", []string{"status", "--no-pager", name}, commontypes.ExecuteNoTimeout)
}

// CheckPackageInstalled executes 'which' in the host namespace to verify binary existence
func (c *BinaryPackageManager) CheckPackageInstalled(name string) (string, error) {
	binary, ok := packageToBinaryMap[name]
	if !ok {
		binary = name
	}

	// Explicitly supply system PATH so 'which' searches /sbin and /usr/sbin
	envs := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"}
	return c.executor.Execute(envs, "which", []string{binary}, commontypes.ExecuteNoTimeout)
}

func (c *BinaryPackageManager) NeedReboot() bool {
	return false
}

package packagemanager

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	commonns "github.com/longhorn/go-common-libs/ns"
	commontypes "github.com/longhorn/go-common-libs/types"
)

type PackageManagerType string

const (
	PackageManagerUnknown             = PackageManagerType("")
	PackageManagerApt                 = PackageManagerType("apt")
	PackageManagerYum                 = PackageManagerType("yum")
	PackageManagerZypper              = PackageManagerType("zypper")
	PackageManagerTransactionalUpdate = PackageManagerType("transactional-update")
	PackageManagerPacman              = PackageManagerType("pacman")
	PackageManagerBinary              = PackageManagerType("binary")
	// PackageManagerQlist            = PackageManagerType("qlist")
)

var (
	ErrPackageNotInstalled = errors.New("package not installed")
	ServiceNotFoundRegex   = regexp.MustCompile(`Unit \S+\.service (not found|could not be found)`)
)

type PackageManager interface {
	UpdatePackageList() (string, error)
	StartPackageSession() (string, error)
	InstallPackage(name string) (string, error)
	UninstallPackage(name string) (string, error)
	Modprobe(module string, opts ...string) (string, error)
	CheckModLoaded(module string) error
	StartService(name string) (string, error)
	RestartService(name string) (string, error)
	GetServiceStatus(name string) (string, error)
	CheckPackageInstalled(name string) (string, error)
	Execute(envs []string, binary string, args []string, timeout time.Duration) (string, error)
	NeedReboot() bool
}

func New(pkgMgrType PackageManagerType, executor *commonns.Executor) (PackageManager, error) {
	switch pkgMgrType {
	case PackageManagerApt:
		return NewAptPackageManager(executor), nil
	case PackageManagerYum:
		return NewYumPackageManager(executor), nil
	case PackageManagerZypper:
		// Elemental3: Check if zypper is actually executable in the host namespace. If not, fallback to binary.
		if _, err := executor.Execute([]string{}, "zypper", []string{"--version"}, commontypes.ExecuteNoTimeout); err != nil {
			return NewBinaryPackageManager(executor), nil
		}
		return NewZypperPackageManager(executor), nil
	case PackageManagerTransactionalUpdate:
		return NewTransactionalUpdatePackageManager(executor), nil
	case PackageManagerPacman:
		return NewPacmanPackageManager(executor), nil
	case PackageManagerBinary:
		return NewBinaryPackageManager(executor), nil
	default:
		return nil, fmt.Errorf("unknown package manager type: %s", pkgMgrType)
	}
}

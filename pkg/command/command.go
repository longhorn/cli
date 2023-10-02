package command

import "time"

type CommandInterface interface {
	UpdatePackageList() (string, error)
	InstallPackage(name string) (string, error)
	UninstallPackage(name string) (string, error)
	Modprobe(module string) (string, error)
	CheckModLoaded(module string) error
	StartService(name string) (string, error)
	GetServiceStatus(name string) (string, error)
	CheckPackageInstalled(name string) (string, error)
	Execute(binary string, args []string, timeout time.Duration) (string, error)
}

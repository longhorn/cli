package app

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/longhorn/longhorn-preflight/pkg/installer"
	"github.com/longhorn/longhorn-preflight/pkg/pkgmgr"
)

func PreflightInstallCmd(pkgMgrType pkgmgr.PackageManagerType) cli.Command {
	return cli.Command{
		Name:  "install",
		Flags: []cli.Flag{},
		Usage: "Install and configure prerequisites",
		Action: func(c *cli.Context) {
			if err := install(c, pkgMgrType); err != nil {
				logrus.WithError(err).Fatalf("Failed to run command")
			}
		},
	}
}

func install(_ *cli.Context, pkgMgrType pkgmgr.PackageManagerType) error {
	ins, err := installer.NewInstaller(pkgMgrType)
	if err != nil {
		return err
	}

	if os.Getenv("UPDATE_PACKAGE_LIST") == "true" {
		ins.UpdatePackageList()
	}

	ins.StartServices()
	ins.ProbeModules(false)
	ins.InstallPackages(false)

	if os.Getenv("ENABLE_SPDK") == "true" {
		ins.InstallPackages(true)
		ins.ProbeModules(true)
		err := ins.ConfigureSPDKEnv()
		if err != nil {
			return err
		}
	}

	return nil
}

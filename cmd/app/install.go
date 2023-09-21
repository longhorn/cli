package app

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/longhorn/longhorn-preflight/pkg/installer"
	"github.com/longhorn/longhorn-preflight/pkg/types"
)

func PreflightInstallCmd(packageManager types.PackageManager) cli.Command {
	return cli.Command{
		Name:  "install",
		Flags: []cli.Flag{},
		Usage: "Install and configure prerequisites",
		Action: func(c *cli.Context) {
			if err := install(c, packageManager); err != nil {
				logrus.WithError(err).Fatalf("Failed to run command")
			}
		},
	}
}

func install(c *cli.Context, packageManager types.PackageManager) error {
	installer, err := installer.NewInstaller(packageManager)
	if err != nil {
		return err
	}

	if os.Getenv("UPDATE_PACKAGE_LIST") == "true" {
		installer.UpdatePackageList()
	}

	installer.StartServices()
	installer.ProbeModules(false)
	installer.InstallPackages(false)

	if os.Getenv("ENABLE_SPDK") == "true" {
		installer.InstallPackages(true)
		installer.ProbeModules(true)
		installer.ConfigureSPDKEnv()
	}

	return nil
}

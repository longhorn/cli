package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/longhorn/longhorn-preflight/cmd/app"
	"github.com/longhorn/longhorn-preflight/pkg/utils"
)

func main() {
	a := cli.NewApp()
	a.Name = "longhorn-preflight"
	a.Usage = "longhorn-preflight helps users install prerequisites and check environment before installing Longhorn system"

	platform, err := utils.GetOSRelease()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get OS release")
	}

	logrus.Infof("Detected platform: %s", platform)

	packageManager, err := utils.GetPackageManager(platform)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to get package manager")
	}

	a.Flags = []cli.Flag{}
	a.Commands = []cli.Command{
		app.PreflightInstallCmd(packageManager),
		app.PreflightCheckCmd(packageManager),
	}
	if err := a.Run(os.Args); err != nil {
		logrus.WithError(err).Fatal("Failed to execute command")
	}
}

package app

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/longhorn/cli/pkg/checker"
	"github.com/longhorn/cli/pkg/pkgmgr"
)

func PreflightCheckCmd(pkgMgrType pkgmgr.PackageManagerType) cli.Command {
	return cli.Command{
		Name:  "check",
		Flags: []cli.Flag{},
		Usage: "Check environment",
		Action: func(c *cli.Context) {
			if err := check(c, pkgMgrType); err != nil {
				logrus.WithError(err).Fatalf("Failed to run command")
			}

		},
	}
}

func check(_ *cli.Context, pkgMgrType pkgmgr.PackageManagerType) error {
	ckr, err := checker.NewChecker(pkgMgrType)
	if err != nil {
		return err
	}

	ckr.CheckIscsidService()
	ckr.CheckMultipathService()
	ckr.CheckNFSv4Support()
	ckr.CheckPackagesInstalled(false)

	if os.Getenv("ENABLE_SPDK") == "true" {
		instructionSets := map[string][]string{
			"amd64": {"sse4_2"},
		}
		ckr.CheckCpuInstructionSet(instructionSets)

		ckr.CheckHugePages()
		ckr.CheckPackagesInstalled(true)
		ckr.CheckModulesLoaded(true)
	}

	return nil
}

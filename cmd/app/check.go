package app

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"

	"github.com/longhorn/longhorn-preflight/pkg/checker"
	"github.com/longhorn/longhorn-preflight/pkg/types"
)

func PreflightCheckCmd(packageManager types.PackageManager) cli.Command {
	return cli.Command{
		Name:  "check",
		Flags: []cli.Flag{},
		Usage: "Check environment",
		Action: func(c *cli.Context) {
			if err := check(c, packageManager); err != nil {
				logrus.WithError(err).Fatalf("Failed to run command")
			}

		},
	}
}

func check(c *cli.Context, packageManager types.PackageManager) error {
	checker, err := checker.NewChecker(packageManager)
	if err != nil {
		return err
	}

	checker.CheckIscsidService()
	checker.CheckMultipathService()
	checker.CheckNFSv4Support()
	checker.CheckPackagesInstalled(false)

	if os.Getenv("ENABLE_SPDK") == "true" {
		instructionSets := map[string][]string{
			"amd64": {"sse4_2"},
		}
		checker.CheckCpuInstructionSet(instructionSets)

		checker.CheckHugePages()
		checker.CheckPackagesInstalled(true)
		checker.CheckModulesLoaded(true)
	}

	return nil
}

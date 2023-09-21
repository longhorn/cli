package app

import (
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli"
)

func PreflightCheckCmd() cli.Command {
	return cli.Command{
		Name:  "check",
		Flags: []cli.Flag{},
		Usage: "Check environment",
		Action: func(c *cli.Context) {
			if err := check(c); err != nil {
				logrus.WithError(err).Fatalf("Failed to run command")
			}

		},
	}
}

func check(c *cli.Context) error {
	return nil
}

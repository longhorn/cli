package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/kubectl/pkg/util/templates"

	localsubcmd "github.com/longhorn/cli/cmd/local/subcmd"
	remotesubcmd "github.com/longhorn/cli/cmd/remote/subcmd"
	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func main() {
	if err := newCmdLonghornctlLocal().Execute(); err != nil {
		logrus.WithError(err).Fatal("Failed to execute command")
		os.Exit(1)
	}
}

func newCmdLonghornctlLocal() *cobra.Command {
	globalOpts := &types.GlobalCmdOptions{}

	cmd := &cobra.Command{
		Use:   consts.CmdLonghornctlLocal,
		Short: "Command-line interface for Longhorn.",
		Long:  "A CLI tool (local) for troubleshooting and managing Longhorn operations.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			err := utils.SetLog(globalOpts.LogLevel)
			if err != nil {
				logrus.WithError(err).Warn("Failed to set log level")
			}
		},
	}

	cmd.CompletionOptions.HiddenDefaultCmd = true

	cmd.PersistentFlags().StringVarP(&globalOpts.LogLevel, consts.CmdOptLogLevel, "l", "info", "log level (trace, debug, info, warn, error, fatal, panic)")

	groups := templates.CommandGroups{
		{
			Message: "Install And Uninstall Commands:",
			Commands: []*cobra.Command{
				localsubcmd.NewCmdInstall(globalOpts),
			},
		},
		{
			Message: "Operation Commands:",
			Commands: []*cobra.Command{
				localsubcmd.NewCmdTrim(globalOpts),
			},
		},
		{
			Message: "Troubleshoot Commands:",
			Commands: []*cobra.Command{
				localsubcmd.NewCmdCheck(globalOpts),
				localsubcmd.NewCmdGet(globalOpts),
			},
		},
	}
	groups.Add(cmd)

	cmd.AddCommand(localsubcmd.NewCmdVersion())
	cmd.AddCommand(remotesubcmd.NewCmdGlobalOptions())

	filters := []string{"options"}
	templates.ActsAsRootCommand(cmd, filters, groups...)

	return cmd
}

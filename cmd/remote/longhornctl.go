package main

import (
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"k8s.io/kubectl/pkg/util/templates"

	"github.com/longhorn/cli/cmd/remote/subcmd"
	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func main() {
	if err := newCmdLonghornctl().Execute(); err != nil {
		logrus.Fatal(err)
		os.Exit(1)
	}
}

func newCmdLonghornctl() *cobra.Command {
	globalOpts := &types.GlobalCmdOptions{}

	cmd := &cobra.Command{
		Use:   consts.CmdLonghornctlRemote,
		Short: "Longhorn commandline interface.",
		Long:  "CLI for Longhorn troubleshooting and operations.",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			err := utils.SetLog(globalOpts.LogLevel)
			if err != nil {
				logrus.WithError(err).Warn("Failed to set log level")
			}
		},
	}

	cmd.CompletionOptions.HiddenDefaultCmd = true

	cmd.PersistentFlags().StringVarP(&globalOpts.LogLevel, consts.CmdOptLogLevel, "l", "info", "log level (trace, debug, info, warn, error, fatal, panic)")
	cmd.PersistentFlags().StringVar(&globalOpts.KubeConfigPath, consts.CmdOptKubeConfigPath, os.Getenv(consts.EnvKubeConfigPath), "Kubernetes config (kubeconfig) path")
	cmd.PersistentFlags().StringVar(&globalOpts.Image, consts.CmdOptImage, consts.ImageLonghornctl, "Image containing longhornctl-local")

	groups := templates.CommandGroups{
		{
			Message: "Install And Uninstall Commands:",
			Commands: []*cobra.Command{
				subcmd.NewCmdInstall(globalOpts),
			},
		},
		{
			Message: "Operation Commands:",
			Commands: []*cobra.Command{
				subcmd.NewCmdTrim(globalOpts),
				subcmd.NewCmdExport(globalOpts),
			},
		},
		{
			Message: "Troubleshoot Commands:",
			Commands: []*cobra.Command{
				subcmd.NewCmdCheck(globalOpts),
				subcmd.NewCmdGet(globalOpts),
			},
		},
	}
	groups.Add(cmd)

	cmd.AddCommand(subcmd.NewCmdGlobalOptions())

	filters := []string{"options"}
	templates.ActsAsRootCommand(cmd, filters, groups...)

	return cmd
}

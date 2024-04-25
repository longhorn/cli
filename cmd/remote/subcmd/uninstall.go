package subcmd

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/preflight"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdUninstall(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdUninstall,
		Short: "Uninstall Longhorn extensions",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(NewCmdUninstallPreflight(globalOpts))

	return cmd
}

func NewCmdUninstallPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var preflightUninstaller = preflight.Uninstaller{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Uninstall Longhorn preflight",
		Long: `Removed Kubernetes resources created by Longhorn install preflight command.
You can use this command for cleaning up resources after a preflight check or if you decided not to proceed with Longhorn installation.

Note: This command exclusively removes preflight-related resources and does not uninstall Longhorn itself.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			preflightUninstaller.KubeConfigPath = globalOpts.KubeConfigPath
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Initializing preflight uninstaller")
			err := preflightUninstaller.Init()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight uninstaller"))
			}

			logrus.Info("Running preflight uninstaller")
			err = preflightUninstaller.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight uninstaller"))
			}
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Completed preflight uninstaller")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	return cmd
}

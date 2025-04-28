package subcmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/pkg/errors"

	"github.com/longhorn/cli/pkg/consts"
	local "github.com/longhorn/cli/pkg/local/preflight"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdInstall(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdInstall,
		Short: "Longhorn installation operations",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.AddCommand(newCmdInstallPreflight(globalOpts))

	return cmd
}

func newCmdInstallPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localInstaller = local.Installer{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Install and configure prerequisites",
		Long: `This command prepares your system for Longhorn deployment by installing the necessary dependencies.
These dependencies ensure your Kubernetes cluster meets the requirements for successful Longhorn operation.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			localInstaller.LogLevel = globalOpts.LogLevel

			if err := localInstaller.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight installer"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			if err := localInstaller.Run(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight installer"))
			}

			logrus.Info("Successfully completed preflight installation")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			if err := localInstaller.Output(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output preflight checker collection"))
			}

			logrus.Info("Successfully output preflight installer collection")
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVarP(&localInstaller.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")
	cmd.Flags().BoolVar(&localInstaller.UpdatePackages, consts.CmdOptUpdatePackages, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvUpdatePackageList), true), "Update packages before installing required dependencies.")
	cmd.Flags().BoolVar(&localInstaller.EnableSpdk, consts.CmdOptEnableSpdk, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvEnableSpdk), false), "Enable installation of SPDK required packages, modules, and setup.")
	cmd.Flags().StringVar(&localInstaller.SpdkOptions, consts.CmdOptSpdkOptions, os.Getenv(consts.EnvSpdkOptions), fmt.Sprintf("Specify a comma-separated (%s) list of custom options for configuring SPDK environment.", consts.CmdOptSeperator))
	cmd.Flags().IntVar(&localInstaller.HugePageSize, consts.CmdOptHugePageSize, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvHugePageSize), 2048), "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().StringVar(&localInstaller.AllowPci, consts.CmdOptAllowPci, os.Getenv(consts.EnvPciAllowed), fmt.Sprintf("Specify a comma-separated (%s) list of allowed PCI devices. By default, all PCI devices are blocked by a non-valid address.", consts.CmdOptSeperator))
	cmd.Flags().StringVar(&localInstaller.DriverOverride, consts.CmdOptDriverOverride, os.Getenv(consts.EnvDriverOverride), "Userspace driver for device bindings. Override default driver for PCI devices.")

	return cmd
}

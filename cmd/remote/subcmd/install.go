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

func NewCmdInstall(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdInstall,
		Short: "Longhorn installation operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(NewCmdInstallPreflight(globalOpts))

	return cmd
}

func NewCmdInstallPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var preflightInstaller = preflight.Installer{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Install Longhorn preflight",
		Long: `This command prepares your system for Longhorn deployment. It automates the installation of the necessary dependencies.
These dependencies ensure your cluster meets the necessary requirements for successful Longhorn operation.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			preflightInstaller.Image = globalOpts.Image
			preflightInstaller.KubeConfigPath = globalOpts.KubeConfigPath
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Initializing preflight installer")
			err := preflightInstaller.Init()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight installer"))
			}

			logrus.Info("Running preflight installer")
			err = preflightInstaller.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight installer"))
			}
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			if preflightInstaller.OperatingSystem == "" {
				logrus.Info("Cleaning up preflight installer")
				if err := preflightInstaller.Cleanup(); err != nil {
					utils.CheckErr(errors.Wrapf(err, "Failed to cleanup preflight installer"))
				}
			}

			logrus.Infof("Completed preflight installer. Use '%s %s %s' to check the result.", consts.CmdLonghornctlRemote, consts.SubCmdCheck, consts.SubCmdPreflight)
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&preflightInstaller.OperatingSystem, consts.CmdOptOperatingSystem, "", "Specify the operating system (\"\", cos). Leave this empty to use the package manager for installation.")
	cmd.Flags().BoolVar(&preflightInstaller.UpdatePackages, consts.CmdOptUpdatePackages, true, "Update packages before installing required dependencies.")
	cmd.Flags().BoolVar(&preflightInstaller.EnableSpdk, consts.CmdOptEnableSpdk, true, "Enable installation of SPDK required packages, modules, and setup.")
	cmd.Flags().StringVar(&preflightInstaller.SpdkOptions, consts.CmdOptSpdkOptions, "", "Specify a space-separated list of custom options for configuring SPDK environment.")
	cmd.Flags().IntVar(&preflightInstaller.HugePageSize, consts.CmdOptHugePageSize, 1024, "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().BoolVar(&preflightInstaller.AllowPci, consts.CmdOptAllowPci, true, "Specify a space-separated list of allowed PCI devices. By default, all PCI devices are blocked by a non-valid address.")
	cmd.Flags().StringVar(&preflightInstaller.DriverOverride, consts.CmdOptDriverOverride, "uio_pci_generic", "User space driver for device bindings.")

	return cmd
}

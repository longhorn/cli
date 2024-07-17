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

func NewCmdCheck(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdCheck,
		Short: "Longhorn checking operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdCheckPreflight(globalOpts))

	return cmd
}

func newCmdCheckPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var preflightChecker = preflight.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Check Longhorn preflight",
		Long: `This command verifies your Kubernetes cluster environment. It performs a series of checks to ensure your cluster meets the requirements for Longhorn to function properly.
These checks can help to identify issues that might prevent Longhorn from functioning properly.`,
		Example: `$ longhornctl check preflight
INFO[2024-07-16T17:17:38+08:00] Initializing preflight checker
INFO[2024-07-16T17:17:38+08:00] Cleaning up preflight checker
INFO[2024-07-16T17:17:38+08:00] Running preflight checker
INFO[2024-07-16T17:17:42+08:00] Retrieved preflight checker result:
ip-10-0-2-123:
  info:
  - Service iscsid is running
  - NFS4 is supported
  - Package nfs-client is installed
  - Package open-iscsi is installed
ip-10-0-2-142:
  info:
  - Service iscsid is running
  - NFS4 is supported
  - Package nfs-client is installed
  - Package open-iscsi is installed
ip-10-0-2-217:
  info:
  - Service iscsid is running
  - NFS4 is supported
  - Package nfs-client is installed
  - Package open-iscsi is installed
INFO[2024-07-16T17:17:42+08:00] Cleaning up preflight checker
INFO[2024-07-16T17:17:42+08:00] Completed preflight checker`,

		PreRun: func(cmd *cobra.Command, args []string) {
			preflightChecker.Image = globalOpts.Image
			preflightChecker.KubeConfigPath = globalOpts.KubeConfigPath

			logrus.Info("Initializing preflight checker")
			if err := preflightChecker.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight checker"))
			}

			logrus.Info("Cleaning up preflight checker")
			if err := preflightChecker.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup preflight checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running preflight checker")
			output, err := preflightChecker.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight checker"))
			}

			logrus.Infof("Retrieved preflight checker result:\n%v", output)
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Cleaning up preflight checker")
			if err := preflightChecker.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup preflight checker"))
			}

			logrus.Info("Completed preflight checker")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().BoolVar(&preflightChecker.EnableSpdk, consts.CmdOptEnableSpdk, false, "Enable checking of SPDK required packages, modules, and setup.")
	cmd.Flags().IntVar(&preflightChecker.HugePageSize, consts.CmdOptHugePageSize, 1024, "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().StringVar(&preflightChecker.UioDriver, consts.CmdOptUioDriver, "uio_pci_generic", "User space I/O driver for SPDK.")

	return cmd
}

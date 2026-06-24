package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/preflight"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/upgrade"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdCheck(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdCheck,
		Short: "Longhorn checking operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdCheckPreflight(globalOpts))
	cmd.AddCommand(newCmdCheckUpgrade(globalOpts))

	return cmd
}

func newCmdCheckPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var preflightChecker = preflight.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Run a preflight check for Longhorn",
		Long:  `This command verifies your Kubernetes cluster environment to ensure it meets Longhorn's requirements. It performs a series of checks that can help identify potential issues that may prevent Longhorn from functioning correctly.`,
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
			preflightChecker.ImageRegistry = globalOpts.ImageRegistry
			preflightChecker.ImagePullSecret = globalOpts.ImagePullSecret
			preflightChecker.KubeConfigPath = globalOpts.KubeConfigPath
			preflightChecker.NodeSelector = globalOpts.NodeSelector
			preflightChecker.Tolerations = globalOpts.Tolerations
			preflightChecker.Namespace = globalOpts.Namespace

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
	cmd.Flags().IntVar(&preflightChecker.HugePageSize, consts.CmdOptHugePageSize, 2048, "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().StringVar(&preflightChecker.UserspaceDriver, consts.CmdOptUserspaceDriver, "", "Userspace I/O driver for SPDK.")

	return cmd
}

func newCmdCheckUpgrade(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var checker = upgrade.Checker{}
	var runErr error

	cmd := &cobra.Command{
		Use:   consts.SubCmdUpgrade,
		Short: "Check V2 instance-manager live-upgrade readiness",
		Long: `This command performs a point-in-time readiness check for the V2 instance-manager rolling live upgrade.
Run it immediately before the upgrade operation and avoid concurrent volume changes while checking.`,
		Args: cobra.NoArgs,

		PreRun: func(cmd *cobra.Command, args []string) {
			checker.LogLevel = globalOpts.LogLevel
			checker.KubeConfigPath = globalOpts.KubeConfigPath
			checker.Namespace = globalOpts.Namespace

			logrus.Info("Initializing upgrade checker")
			if err := checker.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize upgrade checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running upgrade checker")
			runErr = checker.Run()
			if runErr == nil {
				logrus.Info("Completed upgrade checker")
			}
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Outputting upgrade check result")
			if err := checker.Output(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output upgrade checker result"))
			}

			if runErr != nil {
				os.Exit(1)
			}
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVarP(&checker.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")

	return cmd
}

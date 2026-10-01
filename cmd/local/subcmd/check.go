package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	local "github.com/longhorn/cli/pkg/local/preflight"
	localreplica "github.com/longhorn/cli/pkg/local/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdCheck(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdCheck,
		Short: "Longhorn checking operations",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.AddCommand(newCmdCheckPreflight(globalOpts))
	cmd.AddCommand(newCmdCheckReplica(globalOpts))

	return cmd
}

func newCmdCheckReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localChecker = localreplica.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdReplica,
		Short: "Check Longhorn replica integrity",
		Long: `This command checks the integrity of the snapshot chains in the Longhorn replica data directories.
It identifies broken snapshot chains, for example snapshots referencing a missing parent, disk files without metadata files, and metadata files without disk files.
The results are presented by the replica data directory names, not the actual Custom Resource (CR) names.

By default, this command checks all Longhorn replicas in the data directory.
You can narrow down the results by using the following options:
- --name: Specify the Longhorn replica data directory name to check a specific replica.
- --volume-name: Filter replicas by the volume they belong to.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			localChecker.LogLevel = globalOpts.LogLevel

			err := localChecker.Init()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize replica checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			err := localChecker.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run replica checker"))
			}

			logrus.Info("Successfully checked replica integrity")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			err := localChecker.Output()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output replica checker collection"))
			}

			logrus.Info("Successfully output replica checker collection")
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVar(&localChecker.CurrentNodeID, consts.CmdOptNodeId, os.Getenv(consts.EnvCurrentNodeID), "Current node ID.")
	cmd.Flags().StringVarP(&localChecker.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")
	cmd.Flags().StringVar(&localChecker.ReplicaName, consts.CmdOptName, os.Getenv(consts.EnvLonghornReplicaName), "Specify the name of the replica to check.")
	cmd.Flags().StringVar(&localChecker.VolumeName, consts.CmdOptLonghornVolumeName, os.Getenv(consts.EnvLonghornVolumeName), "Specify the name of the volume to check its replicas.")
	cmd.Flags().StringVar(&localChecker.LonghornDataDirectory, consts.CmdOptLonghornDataDirectory, os.Getenv(consts.EnvLonghornDataDirectory), "Specify the Longhorn data directory. If not provided, the default will be attempted, or it will fall back to the directory of longhorn-disk.cfg.")

	return cmd
}

func newCmdCheckPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localChecker = local.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Run a preflight check for Longhorn",
		Long:  `This command verifies your Kubernetes cluster environment to ensure it meets Longhorn's requirements. It performs a series of checks that can help identify potential issues that may prevent Longhorn from functioning correctly.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			localChecker.LogLevel = globalOpts.LogLevel

			if err := localChecker.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			if err := localChecker.Run(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight checker"))
			}

			logrus.Info("Successfully checked preflight environment")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			if err := localChecker.Output(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output preflight checker collection"))
			}

			logrus.Info("Successfully output preflight checker collection")
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVarP(&localChecker.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")
	cmd.Flags().BoolVar(&localChecker.EnableSpdk, consts.CmdOptEnableSpdk, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvEnableSpdk), false), "Enable checking of SPDK required packages, modules, and setup.")
	cmd.Flags().IntVar(&localChecker.HugePageSize, consts.CmdOptHugePageSize, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvHugePageSize), 2048), "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().StringVar(&localChecker.UserspaceDriver, consts.CmdOptUserspaceDriver, os.Getenv(consts.EnvUserspaceDriver), "Userspace I/O driver for SPDK.")

	return cmd
}

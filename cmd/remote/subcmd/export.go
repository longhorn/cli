package subcmd

import (
	"fmt"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdExport(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdExport,
		Short: "Export Longhorn resources",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdExportReplica(globalOpts))

	return cmd
}

func newCmdExportReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var replicaExporter = replica.Exporter{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdReplica,
		Short: "Export Longhorn replica",
		Long: `This command exports the data from a specified Longhorn replica data directory to a directory on its host machine.
It provides data recovery capabilities when the Longhorn system is unavailable.

To perform an export, provide the name of the replica data directory to the --name option.

To find available replica data directory names, use the following command:
> longhornctl get replica

After the export is completed, you can access the exported data at the specified location on the node provided in the output.

To terminate the replica exporter and stop the replica export process, use the 'stop' subcommand with the original command. For example:
> longhornctl export replica <options> stop`,

		PreRun: func(cmd *cobra.Command, args []string) {
			replicaExporter.Image = globalOpts.Image
			replicaExporter.KubeConfigPath = globalOpts.KubeConfigPath

			utils.CheckErr(replicaExporter.Validate())

			logrus.Info("Initializing replica exporter")
			if err := replicaExporter.Init(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to initialize replica exporter"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running replica exporter")
			result, err := replicaExporter.Run()
			if err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to run replica exporter"))
			}

			logrus.Infof("Exported replica:\n %v", result)
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Infof("Completed replica exporter. Use '%s %s %s %s' to stop exporting replica.", consts.CmdLonghornctlRemote, consts.SubCmdExport, consts.SubCmdReplica, consts.SubCmdStop)
		},
	}

	cmd.AddCommand(newCmdExportReplicaStop(globalOpts))

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	// Use SetFlagHidden to include these option flags in the child subcommand.
	// This allows the user to use `export replica --<option> <value> stop` without
	// having to remove the irrelevant (--<option> <value>) option flags.
	// Note: cmd.PersistentFlags() is not used because the options won't display
	// in this command's help menu.
	cmd.Flags().StringVar(&replicaExporter.EngineImage, consts.CmdOptLonghornEngineImage, consts.ImageEngine, "Engine image to use to create volume from the replica.")
	cmd.Flags().StringVar(&replicaExporter.ReplicaName, consts.CmdOptName, "", fmt.Sprintf("Specify the replica directory name to export. The replica data directory name is not the same as the Kubernetes Replica custom resource (CR) object name. To retrieve the replica directory name, use '%s %s %s'.", consts.CmdLonghornctlRemote, consts.SubCmdGet, consts.SubCmdReplica))
	cmd.Flags().StringVar(&replicaExporter.LonghornDataDirectory, consts.CmdOptLonghornDataDirectory, "/var/lib/longhorn", "Specify the Longhorn data directory. If not provided, the default will be attempted, or it will fall back to the directory of longhorn-disk.cfg.")
	cmd.Flags().StringVar(&replicaExporter.HostTargetDirectory, consts.CmdOptTargetDirectory, "", "Target directory on the host machine where the exported data will be mounted.")

	return cmd
}

func newCmdExportReplicaStop(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var replicaExporter = replica.Exporter{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdStop,
		Short: "Stop exporting Longhorn replica",
		Long:  `This command terminates the replica exporter, stopping the export process for the replica.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			replicaExporter.KubeConfigPath = globalOpts.KubeConfigPath

			if err := replicaExporter.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize replica exporter"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Stopping replica exporter")

			err := replicaExporter.Cleanup()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to stop replica exporter"))
			}

			logrus.Info("Successfully stopped exporting replica")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	// Include flags from the parent command for user convenience. This allows
	// the `stop` subcommand to be appended directly to the `export replica` command
	// without having to remove the irrelevant option flags.
	utils.SetFlagHidden(cmd, consts.CmdOptLonghornEngineImage)
	utils.SetFlagHidden(cmd, consts.CmdOptName)
	utils.SetFlagHidden(cmd, consts.CmdOptLonghornDataDirectory)
	utils.SetFlagHidden(cmd, consts.CmdOptTargetDirectory)

	return cmd
}

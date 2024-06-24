package subcmd

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdGet(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdGet,
		Short: "Longhorn information gathering operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(NewCmdGetReplica(globalOpts))

	return cmd
}

func NewCmdGetReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var replicaGetter = replica.Getter{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdReplica,
		Short: "Get Longhorn replica information",
		Long: `This command retrieves detailed information about Longhorn replicas.
The information can be used for troubleshooting and gaining insights into the state of your Longhorn replicas.

By default, the command retrieves information about all Longhorn replicas in the system.
You can optionally filter the results using the option flags to narrow down the information returned:
- --name: Specify a specific Longhorn replica name to retrieve details for.
- --volume-name: Filter replicas based on the volume they belong to.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			replicaGetter.Image = globalOpts.Image
			replicaGetter.KubeConfigPath = globalOpts.KubeConfigPath
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Initializing replica getter")
			if err := replicaGetter.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize replica getter"))
			}

			logrus.Info("Running replica getter")
			output, err := replicaGetter.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run replica getter"))
			}

			logrus.Infof("Retrieved replica information:\n %v", output)
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Cleaning up replica getter")
			if err := replicaGetter.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup replica getter"))
			}

			logrus.Info("Completed replica getter")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&replicaGetter.ReplicaName, consts.CmdOptName, "", "Specify the name of the replica to retrieve information (optional).")
	cmd.Flags().StringVar(&replicaGetter.VolumeName, consts.CmdOptLonghornVolumeName, "", "Specify the name of the volume to retrieve replica information (optional).")
	cmd.Flags().StringVar(&replicaGetter.LonghornDataDirectory, consts.CmdOptLonghornDataDirectory, "/var/lib/longhorn", "Specify the Longhorn data directory. If not provided, the default will be attempted, or it will fall back to the directory of longhorn-disk.cfg (optional).")

	return cmd
}

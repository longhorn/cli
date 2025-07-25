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

	cmd.AddCommand(newCmdGetReplica(globalOpts))

	return cmd
}

func newCmdGetReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var replicaGetter = replica.Getter{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdReplica,
		Short: "Retrieve Longhorn replica information",
		Long: `This command retrieves detailed information about Longhorn replicas, which is useful for troubleshooting and understanding their state.
The information is presented by the replica data directory names, not the actual Custom Resource (CR) names.

By default, this command retrieves information for all Longhorn replicas in the data directory.
You can narrow down the results by using the following options:
- --name: Specify the Longhorn replica data directory name to retrieve details for a specific replica.
- --volume-name: Filter replicas by the volume they belong to.`,
		Example: `$ longhornctl get replica
INFO[2024-07-16T17:23:47+08:00] Initializing replica getter
INFO[2024-07-16T17:23:47+08:00] Cleaning up replica getter
INFO[2024-07-16T17:23:47+08:00] Running replica getter
INFO[2024-07-16T17:23:51+08:00] Retrieved replica information:
 replicas:
    pvc-48a6457d-585e-423b-b530-bbc68a5f948a-0e2603a7:
        - node: ip-10-0-2-123
          directory: /var/lib/longhorn/replicas/pvc-48a6457d-585e-423b-b530-bbc68a5f948a-0e2603a7
          isInUse: true
          volumeName: pvc-48a6457d-585e-423b-b530-bbc68a5f948a
          metadata:
            size: 10737418240
            head: volume-head-000.img
            dirty: true
            rebuilding: false
            error: ""
            parent: ""
            sectorsize: 512
            backingfilepath: ""
            backingfile: null
INFO[2024-07-16T17:23:51+08:00] Cleaning up replica getter
INFO[2024-07-16T17:23:51+08:00] Completed replica getter`,

		PreRun: func(cmd *cobra.Command, args []string) {
			replicaGetter.Image = globalOpts.Image
			replicaGetter.KubeConfigPath = globalOpts.KubeConfigPath
			replicaGetter.NodeSelector = globalOpts.NodeSelector

			logrus.Info("Initializing replica getter")
			if err := replicaGetter.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize replica getter"))
			}

			logrus.Info("Cleaning up replica getter")
			if err := replicaGetter.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup replica getter"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
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

	cmd.Flags().StringVar(&replicaGetter.ReplicaName, consts.CmdOptName, "", "Specify the name of the replica to retrieve information.")
	cmd.Flags().StringVar(&replicaGetter.VolumeName, consts.CmdOptLonghornVolumeName, "", "Specify the name of the volume to retrieve replica information.")
	cmd.Flags().StringVar(&replicaGetter.LonghornDataDirectory, consts.CmdOptLonghornDataDirectory, "/var/lib/longhorn", "Specify the Longhorn data directory. If not provided, the default will be attempted, or it will fall back to the directory of longhorn-disk.cfg.")

	return cmd
}

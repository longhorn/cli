package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	local "github.com/longhorn/cli/pkg/local/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdGet(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdGet,
		Short: "Longhorn information gathering operations",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.AddCommand(newCmdGetReplica(globalOpts))

	return cmd
}

func newCmdGetReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localGetter = local.Getter{}

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
			localGetter.LogLevel = globalOpts.LogLevel

			err := localGetter.Init()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize replica getter"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			err := localGetter.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run replica getter"))
			}

			logrus.Info("Successfully get replica information")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			err := localGetter.Output()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output replica getter collection"))
			}

			logrus.Info("Successfully output replica getter collection")
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVar(&localGetter.CurrentNodeID, consts.CmdOptNodeId, os.Getenv(consts.EnvCurrentNodeID), "Current node ID.")
	cmd.Flags().StringVarP(&localGetter.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")
	cmd.Flags().StringVar(&localGetter.ReplicaName, consts.CmdOptName, os.Getenv(consts.EnvLonghornReplicaName), "Specify the name of the replica to retrieve information.")
	cmd.Flags().StringVar(&localGetter.VolumeName, consts.CmdOptLonghornVolumeName, os.Getenv(consts.EnvLonghornVolumeName), "Specify the name of the volume to retrieve replica information.")
	cmd.Flags().StringVar(&localGetter.LonghornDataDirectory, consts.CmdOptLonghornDataDirectory, os.Getenv(consts.EnvLonghornDataDirectory), "Specify the Longhorn data directory. If not provided, the default will be attempted, or it will fall back to the directory of longhorn-disk.cfg.")

	return cmd
}

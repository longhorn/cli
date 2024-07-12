package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	local "github.com/longhorn/cli/pkg/local/volume"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdTrim(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdTrim,
		Short: "Longhorn trimming operations",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.AddCommand(newCmdTrimVolume(globalOpts))

	return cmd
}

func newCmdTrimVolume(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localTrimmer = local.Trimmer{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdVolume,
		Short: "Trim a Longhon volume",
		Long: `This command helps to reclaim storage space on a Longhorn volume. It achieves this by removing unused data blocks associated with data that has been deleted from the volume.
This is useful after you've deleted files or applications from the volume but haven't seen a corresponding reduction in storage consumption.

To use this command, you'll need to specify the following:
- --name: Specify a specific Longhorn volume you want to trim.

By regularly trimming your Longhorn volumes, you can ensure efficient storage management with your system.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			localTrimmer.LogLevel = globalOpts.LogLevel

			utils.CheckErr(localTrimmer.Validate())

			err := localTrimmer.Init()
			if err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to initialize trimmer for volume %s", localTrimmer.VolumeName))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			err := localTrimmer.Run()
			if err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to run trimmer for volume %s", localTrimmer.VolumeName))
			}

			logrus.Infof("Successfully trimmed volume %s", localTrimmer.VolumeName)
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVar(&localTrimmer.CurrentNodeID, consts.CmdOptNodeId, os.Getenv(consts.EnvCurrentNodeID), "Current node ID.")
	cmd.Flags().StringVar(&localTrimmer.LonghornNamespace, consts.CmdOptLonghornNamespace, os.Getenv(consts.EnvLonghornNamespace), "Namespace where Longhorn is deployed within the Kubernetes cluster.")
	cmd.Flags().StringVar(&localTrimmer.VolumeName, consts.CmdOptLonghornVolumeName, os.Getenv(consts.EnvLonghornVolumeName), "Name of the Longhorn volum to be trimmed.")

	return cmd
}

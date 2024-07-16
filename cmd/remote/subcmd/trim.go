package subcmd

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/volume"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdTrim(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdTrim,
		Short: "Longhorn trimming operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdTrimVolume(globalOpts))

	return cmd
}

func newCmdTrimVolume(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var volumeTrimmer = volume.Trimmer{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdVolume,
		Short: "Trim a Longhorn volume",
		Long: `This command helps to reclaim storage space on a Longhorn volume. It achieves this by removing unused data blocks associated with data that has been deleted from the volume.
This is useful after you've deleted files or applications from the volume but haven't seen a corresponding reduction in storage consumption.

To use this command, you'll need to specify the following:
- --name: Specify a specific Longhorn volume you want to trim.

By regularly trimming your Longhorn volumes, you can ensure efficient storage management with your system.`,
		Example: `$ longhornctl trim volume --name="pvc-48a6457d-585e-423b-b530-bbc68a5f948a"
INFO[2024-07-16T17:31:59+08:00] Initializing volume trimmer
INFO[2024-07-16T17:31:59+08:00] Cleaning volume trimmer
INFO[2024-07-16T17:31:59+08:00] Running volume trimmer                        volume=pvc-48a6457d-585e-423b-b530-bbc68a5f948a
INFO[2024-07-16T17:32:01+08:00] Cleaning volume trimmer                       volume=pvc-48a6457d-585e-423b-b530-bbc68a5f948a
INFO[2024-07-16T17:32:01+08:00] Completed volume trimmer                      volume=pvc-48a6457d-585e-423b-b530-bbc68a5f948a`,

		PreRun: func(cmd *cobra.Command, args []string) {
			volumeTrimmer.Image = globalOpts.Image
			volumeTrimmer.KubeConfigPath = globalOpts.KubeConfigPath

			utils.CheckErr(volumeTrimmer.Validate())

			logrus.Info("Initializing volume trimmer")
			if err := volumeTrimmer.Init(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to initialize volum trimmer for volume %s", volumeTrimmer.VolumeName))
			}

			logrus.Info("Cleaning volume trimmer")
			if err := volumeTrimmer.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup volume trimmer"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			log := logrus.WithField("volume", volumeTrimmer.VolumeName)

			log.Info("Running volume trimmer")
			if err := volumeTrimmer.Run(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to run volume trimmer for volume %s", volumeTrimmer.VolumeName))
			}
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			log := logrus.WithField("volume", volumeTrimmer.VolumeName)
			log.Info("Cleaning volume trimmer")
			if err := volumeTrimmer.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup volume trimmer"))
			}

			log.Info("Completed volume trimmer")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&volumeTrimmer.LonghornNamespace, consts.CmdOptLonghornNamespace, "longhorn-system", "Namespace where Longhorn is deployed within the Kubernetes cluster.")
	cmd.Flags().StringVar(&volumeTrimmer.VolumeName, consts.CmdOptName, "", "Name of the Longhorn volum to be trimmed.")

	return cmd
}

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

func NewCmdChecksum(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "checksum",
		Short: "Snapshot checksum operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)
	cmd.AddCommand(newCmdChecksumVolume(globalOpts))

	return cmd
}

func newCmdChecksumVolume(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var requester = &volume.ChecksumRequester{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdVolume,
		Short: "Trigger on-demand snapshot checksum calculation for a volume",
		Long: `
Trigger on-demand checksum calculation for all user-created snapshots of a Longhorn volume.

This updates the Volume CR so that the Longhorn Manager immediately schedules
checksum hashing tasks. SnapshotMonitor will calculate missing checksums in
the background without interrupting I/O.
`,
		Example: `$ longhornctl checksum volume --name=v1 --namespace=longhorn-system`,
		PreRun: func(cmd *cobra.Command, args []string) {
			requester.Image = globalOpts.Image
			requester.Namespace = globalOpts.Namespace
			requester.KubeConfigPath = globalOpts.KubeConfigPath

			if err := requester.Validate(); err != nil {
				utils.CheckErr(err)
			}

			if err := requester.Init(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "failed to initialize checksum requester"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Triggering on-demand snapshot checksum calculation")

			if err := requester.Run(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "failed to request checksum"))
			}

			logrus.Info("Checksum request submitted")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Cleaning volume on-demand checksum requester")
			if err := requester.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "failed to cleanup volume checksum requester"))
			}

			logrus.Info("Completed volume on-demand checksum requester")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)
	cmd.Flags().StringVar(&requester.VolumeName, consts.CmdOptName, "", "Name of the Longhorn volume")
	cmd.Flags().BoolVar(&requester.AllVolumes, consts.CmdOptAll, false, "Apply to all volumes")
	cmd.Flags().StringVar(&requester.NodeID, consts.CmdOptNodeId, "", "Compute snapshots for all volumes on the specified node")

	return cmd
}

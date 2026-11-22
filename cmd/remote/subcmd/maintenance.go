package subcmd

import (
	"context"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/maintenance"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdMaintenance(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdMaintenance,
		Short: "Longhorn node maintenance operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdEvictNode(globalOpts))
	cmd.AddCommand(newCmdEvictDisk(globalOpts))

	return cmd
}

func newCmdEvictNode(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var nodeEvictor = maintenance.Evictor{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdEvictNode,
		Short: "Evict all replicas from a node",
		Long: `Prepare a Longhorn node for maintenance by cordoning the Kubernetes node, disabling replica scheduling on it and requesting eviction of all its replicas.

Specify the following option:
- --node-id: The name of the node to evict.

With --wait, the command blocks until no replicas remain scheduled on the node.
Once eviction is done, drain the node with the printed kubectl command before stopping or restarting it.`,
		Example: `$ longhornctl maintenance evict-node --node-id="worker-1" --wait`,

		PreRun: func(cmd *cobra.Command, args []string) {
			nodeEvictor.Image = globalOpts.Image
			nodeEvictor.ImageRegistry = globalOpts.ImageRegistry
			nodeEvictor.ImagePullSecret = globalOpts.ImagePullSecret
			nodeEvictor.KubeConfigPath = globalOpts.KubeConfigPath
			nodeEvictor.NodeSelector = globalOpts.NodeSelector
			nodeEvictor.Tolerations = globalOpts.Tolerations
			nodeEvictor.Namespace = globalOpts.Namespace

			utils.CheckErr(nodeEvictor.Validate())

			logrus.Info("Initializing node evictor")
			if err := nodeEvictor.Init(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to initialize node evictor for node %s", nodeEvictor.NodeID))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			log := logrus.WithField("node", nodeEvictor.NodeID)

			log.Info("Running node evictor")
			if err := nodeEvictor.EvictNode(context.Background()); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to evict node %s", nodeEvictor.NodeID))
			}

			log.Info("Completed node evictor")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&nodeEvictor.NodeID, consts.CmdOptNodeId, "", "Name of the node to evict.")
	cmd.Flags().BoolVar(&nodeEvictor.Wait, consts.CmdOptWait, false, "Wait until all replicas have left the node.")

	return cmd
}

func newCmdEvictDisk(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var diskEvictor = maintenance.Evictor{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdEvictDisk,
		Short: "Evict all replicas from a disk",
		Long: `Prepare a single Longhorn disk for maintenance by disabling replica scheduling on it and requesting eviction of all its replicas.

Specify the following options:
- --node-id: The name of the node the disk belongs to.
- --disk-uuid: The UUID of the disk to evict.

With --wait, the command blocks until no replicas remain scheduled on the disk.`,
		Example: `$ longhornctl maintenance evict-disk --node-id="worker-1" --disk-uuid="a1b2c3d4-e5f6-7890-abcd-ef1234567890" --wait`,

		PreRun: func(cmd *cobra.Command, args []string) {
			diskEvictor.Image = globalOpts.Image
			diskEvictor.ImageRegistry = globalOpts.ImageRegistry
			diskEvictor.ImagePullSecret = globalOpts.ImagePullSecret
			diskEvictor.KubeConfigPath = globalOpts.KubeConfigPath
			diskEvictor.NodeSelector = globalOpts.NodeSelector
			diskEvictor.Tolerations = globalOpts.Tolerations
			diskEvictor.Namespace = globalOpts.Namespace

			utils.CheckErr(diskEvictor.ValidateDiskEviction())

			logrus.Info("Initializing disk evictor")
			if err := diskEvictor.Init(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to initialize disk evictor for disk %s", diskEvictor.DiskUUID))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			log := logrus.WithFields(logrus.Fields{"node": diskEvictor.NodeID, "disk": diskEvictor.DiskUUID})

			log.Info("Running disk evictor")
			if err := diskEvictor.EvictDisk(context.Background()); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to evict disk %s", diskEvictor.DiskUUID))
			}

			log.Info("Completed disk evictor")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&diskEvictor.NodeID, consts.CmdOptNodeId, "", "Name of the node the disk belongs to.")
	cmd.Flags().StringVar(&diskEvictor.DiskUUID, consts.CmdOptDiskUUID, "", "UUID of the disk to evict.")
	cmd.Flags().BoolVar(&diskEvictor.Wait, consts.CmdOptWait, false, "Wait until all replicas have left the disk.")

	return cmd
}

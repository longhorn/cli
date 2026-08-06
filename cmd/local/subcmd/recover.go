package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"

	localRep "github.com/longhorn/cli/pkg/local/replica"
)

func NewCmdRecover(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdRecover,
		Short: "Longhorn recovery operations",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)
	cmd.AddCommand(newCmdRecoverReplica(globalOpts))

	return cmd
}

func newCmdRecoverReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localRecoverer = localRep.Recoverer{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdReplica,
		Short: "Recover disk space for a single v1 replica",
		Long: `This command reclaims disk space on a single Longhorn v1 replica directory by:
- scanning its snapshot chain for sectors that are fully overlapped (shadowed) by newer snapshots or the volume head,
- then punching holes in the older, now-redundant snapshot files.
It is particularly useful after extensive snapshot churn, where older snapshot files continue to consume disk space even though every sector they hold has been superseded elsewhere in the chain.

To use this command, specify the following option:
- --` + consts.CmdOptReplicaDir + `: The path to the replica data directory you wish to recover.
- --dryRun (default false): set to true, if you wish to preview changes before applying them.

WARNING: This command modifies snapshot files in place via hole punching. Ensure the replica is not attached to a running engine before recovering it, and review the dry-run output carefully before confirming.`,

		Run: func(cmd *cobra.Command, args []string) {
			localRecoverer.LogLevel = globalOpts.LogLevel
			localRecoverer.Namespace = os.Getenv(consts.EnvLonghornNamespace)

			utils.CheckErr(localRecoverer.Validate())

			err := localRecoverer.Init()
			if err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to initialize recoverer for replica directory %s", localRecoverer.ReplicaDirectory))
			}
			defer func() {
				_ = localRecoverer.Close()
			}()
			err = localRecoverer.Run()

			if err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to run recoverer for replica directory %s", localRecoverer.ReplicaDirectory))
			}

			if localRecoverer.Recovered {
				logrus.Infof("Successfully recovered replica directory %s", localRecoverer.ReplicaDirectory)
			}
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVar(&localRecoverer.ReplicaDirectory, consts.CmdOptReplicaDir, os.Getenv(consts.EnvReplicaDirectory), "Path to the replica data directory to recover.")
	cmd.Flags().BoolVar(&localRecoverer.DryRun, "dry-run", false, "preview changes without applying them")

	return cmd
}

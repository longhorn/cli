package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"

	localspdk "github.com/longhorn/cli/pkg/local/spdk"
)

func NewCmdDiagnose(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdDiagnose,
		Short: "Diagnose the health of the Longhorn system",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.AddCommand(newCmdDiagnoseV2DataEngine(globalOpts))

	return cmd
}

func newCmdDiagnoseV2DataEngine(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localDiagnoser = localspdk.Diagnoser{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdV2DataEngine,
		Short: "Inspect V2 Data Engine SPDK target memory, iobuf pools, and blobstore channels",
		Long: `This command performs a lockless, read-only inspection of running V2 Data Engine spdk_tgt processes on the node.

It combines SPDK Unix-socket JSON-RPC queries with ELF/DWARF-guided /proc/<pid>/mem inspection to report:
- Global iobuf small/large pool configuration, live DPDK ring free counts, and per-module retry statistics.
- Per-thread iobuf wait queues and lockless module caches (including nvmf_TCP large_pool availability).
- Blobstore and per-blob spdk_bs_channel request-set pools (freeReqs, needClusterAlloc queues, and active in-flight CoW / external snapshot reads).
- Bdev shared resources with queued nomem_io requests and active bdev_no_mem_poller counts.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			localDiagnoser.LogLevel = globalOpts.LogLevel

			if err := localDiagnoser.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize V2 Data Engine diagnoser"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			if err := localDiagnoser.Run(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run V2 Data Engine diagnoser"))
			}

			logrus.Info("Successfully collected V2 Data Engine SPDK diagnostics")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			if err := localDiagnoser.Output(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output V2 Data Engine SPDK diagnostics"))
			}

			logrus.Info("Successfully output V2 Data Engine SPDK diagnostics")
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	defaultPID := utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvSpdkPid), 0)
	defaultProcDir := os.Getenv(consts.EnvProcDir)
	if defaultProcDir == "" {
		defaultProcDir = consts.DefaultProcDirectory
	}
	defaultSock := os.Getenv(consts.EnvSpdkSocket)
	if defaultSock == "" {
		defaultSock = consts.DefaultSpdkSocketPath
	}

	cmd.Flags().StringVar(&localDiagnoser.CurrentNodeID, consts.CmdOptNodeId, os.Getenv(consts.EnvCurrentNodeID), "Current node ID.")
	cmd.Flags().StringVarP(&localDiagnoser.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")
	cmd.Flags().StringVar(&localDiagnoser.ProcDir, consts.CmdOptProcDir, defaultProcDir, "Path to the procfs directory used to discover and inspect spdk_tgt.")
	cmd.Flags().StringVar(&localDiagnoser.SpdkSocketPath, consts.CmdOptSpdkSocket, defaultSock, "Path to the SPDK JSON-RPC Unix domain socket inside the target mount namespace.")
	cmd.Flags().IntVar(&localDiagnoser.TargetPID, consts.CmdOptPid, defaultPID, "Inspect a specific spdk_tgt PID instead of scanning procfs.")
	cmd.Flags().BoolVar(&localDiagnoser.IncludeAll, consts.CmdOptAll, false, "Include idle per-blob channels with 0 queued or active requests.")

	return cmd
}

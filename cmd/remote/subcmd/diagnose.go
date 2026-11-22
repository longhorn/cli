package subcmd

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/diagnose"
	"github.com/longhorn/cli/pkg/remote/spdk"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdDiagnose(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdDiagnose,
		Short: "Diagnose the health of the Longhorn system",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdDiagnoseSystem(globalOpts))
	cmd.AddCommand(newCmdDiagnoseV2DataEngine(globalOpts))

	return cmd
}

func newCmdDiagnoseSystem(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var diagnoser = diagnose.Diagnoser{}
	var results diagnose.Results
	var outputFormat string

	cmd := &cobra.Command{
		Use:   consts.SubCmdSystem,
		Short: "Diagnose the health of Longhorn system components and volumes",
		Long: `This command checks whether the Longhorn system is healthy and fully functional. It is useful for verifying a Longhorn deployment, for example in automation.

The following checks are performed, ordered so that a component is checked before the components depending on it:
- LonghornManager: The longhorn-manager DaemonSet pods are ready.
- LonghornBackend: The longhorn-backend Service has ready endpoints.
- LonghornNodes: The Longhorn nodes are ready. Other node conditions and disks that are not ready are reported as warnings.
- EngineImages: The engine images are deployed on all nodes.
- InstanceManagers: The instance managers are running.
- ShareManagers: The share managers carry the required discovery labels.
- BackingImageManagers: The backing image managers carry the required discovery labels.
- CSIDriver: The Longhorn CSIDriver (driver.longhorn.io) is registered.
- CSIPlugin: The longhorn-csi-plugin DaemonSet pods are ready.
- CSIAttacher, CSIProvisioner, CSIResizer, CSISnapshotter: The CSI sidecar Deployments have ready pods.
- LonghornUI: The longhorn-ui Deployment has ready pods.
- Volumes: No Longhorn volume is faulted. Degraded volumes are reported as warnings.

The command only reads the cluster state. It does not create or modify any resource.

The results are printed to stdout in YAML or JSON, and the logs are printed to stderr.

The command exits with a non-zero status if any check fails.`,
		Example: `$ longhornctl diagnose system
$ longhornctl diagnose system -o json 2>/dev/null | jq '.[] | select(.status == "fail")'`,

		PreRun: func(cmd *cobra.Command, args []string) {
			if !slices.Contains(diagnose.OutputFormats, outputFormat) {
				utils.CheckErr(errors.Errorf("Invalid --%s %q, must be one of %v", consts.CmdOptOutput, outputFormat, diagnose.OutputFormats))
			}

			diagnoser.KubeConfigPath = globalOpts.KubeConfigPath
			diagnoser.Namespace = globalOpts.Namespace

			logrus.Info("Initializing Longhorn diagnoser")
			if err := diagnoser.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize Longhorn diagnoser"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running Longhorn diagnoser")
			results = diagnoser.Run()

			output, err := results.Marshal(outputFormat)
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to marshal Longhorn diagnoser result"))
			}

			if _, err := cmd.OutOrStdout().Write(output); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to write Longhorn diagnoser result"))
			}
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			if failed := results.Failed(); len(failed) > 0 {
				utils.CheckErr(errors.Errorf("Longhorn system is unhealthy, failed checks: %v", strings.Join(failed, ", ")))
			}

			logrus.Info("Longhorn system is healthy")
		},
	}

	cmd.PersistentFlags().StringVarP(&globalOpts.LogLevel, consts.CmdOptLogLevel, "l", globalOpts.LogLevel, "Log level")
	cmd.PersistentFlags().StringVar(&globalOpts.KubeConfigPath, consts.CmdOptKubeConfigPath, globalOpts.KubeConfigPath, "Kubernetes config (kubeconfig) path. If not set, $KUBECONFIG or ~/.kube/config is used")
	cmd.PersistentFlags().StringVar(&globalOpts.Namespace, consts.CmdOptNamespace, globalOpts.Namespace, "The namespace where Longhorn is installed.")
	cmd.Flags().StringVarP(&outputFormat, consts.CmdOptOutput, "o", diagnose.OutputFormatYAML, fmt.Sprintf("Output format of the results. One of: %v.", strings.Join(diagnose.OutputFormats, ", ")))

	// Shadow the inherited options for deploying pods, which are not used by this command.
	for _, option := range []string{consts.CmdOptImage, consts.CmdOptImageRegistry, consts.CmdOptImagePullSecret, consts.CmdOptNodeSelector, consts.CmdOptTolerations} {
		utils.SetFlagHidden(cmd, option)
	}

	return cmd
}

func newCmdDiagnoseV2DataEngine(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var spdkDiagnoser = spdk.Diagnoser{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdV2DataEngine,
		Short: "Diagnose Longhorn V2 Data Engine SPDK memory, iobuf, blobstore channels, and cross-node deadlocks",
		Long: `This command inspects running V2 Data Engine spdk_tgt processes inside instance-manager pods across Longhorn nodes without pausing the SPDK reactor (using read-only /proc/<pid>/mem pread and SPDK JSON-RPC via a temporary read-only DaemonSet).

It collects, correlates, and diagnoses:
- Global iobuf pool sizes, live DPDK rte_ring free counts, and per-module allocation/retry counters.
- Per-thread iobuf wait queues and lockless module caches (such as nvmf_TCP large_pool availability).
- Blobstore and per-blob spdk_bs_channel request-set pools (freeReqs, needClusterAlloc queues, and active in-flight CoW / external snapshot reads).
- Bdev shared resources with queued nomem_io requests and active bdev_no_mem_poller counts.
- Cross-node NVMe-oF snapshot read dependencies and distributed deadlock cycles across stalled nodes.

The results are printed to stdout in YAML, JSON, or human-readable text (-o text), and logs are printed to stderr.
YAML and JSON outputs place the high-level cluster summary, deadlock cycles, and cross-node dependencies first while preserving all raw per-node SPDK data underneath.

The command exits with a non-zero status if any V2 Data Engine SPDK target is stalled or deadlocked.`,
		Example: `$ longhornctl diagnose v2-data-engine
$ longhornctl diagnose v2-data-engine -o text
$ longhornctl diagnose v2-data-engine --node-id=worker-1 -o json 2>/dev/null | jq '.nodes["worker-1"]'`,

		PreRun: func(cmd *cobra.Command, args []string) {
			spdkDiagnoser.Image = globalOpts.Image
			spdkDiagnoser.ImageRegistry = globalOpts.ImageRegistry
			spdkDiagnoser.ImagePullSecret = globalOpts.ImagePullSecret
			spdkDiagnoser.KubeConfigPath = globalOpts.KubeConfigPath
			spdkDiagnoser.NodeSelector = globalOpts.NodeSelector
			spdkDiagnoser.Tolerations = globalOpts.Tolerations
			spdkDiagnoser.Namespace = globalOpts.Namespace
			spdkDiagnoser.LogLevel = globalOpts.LogLevel

			utils.CheckErr(spdkDiagnoser.Validate())

			logrus.Info("Initializing V2 Data Engine diagnoser")
			if err := spdkDiagnoser.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize V2 Data Engine diagnoser"))
			}

			logrus.Info("Cleaning up V2 Data Engine diagnoser")
			if err := spdkDiagnoser.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to cleanup V2 Data Engine diagnoser"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running V2 Data Engine diagnoser")
			output, err := spdkDiagnoser.Run()
			if err != nil {
				if cleanupErr := spdkDiagnoser.Cleanup(); cleanupErr != nil {
					logrus.WithError(cleanupErr).Warn("Failed to cleanup V2 Data Engine diagnoser after error")
				}
				utils.CheckErr(errors.Wrap(err, "Failed to run V2 Data Engine diagnoser"))
			}

			if _, err := cmd.OutOrStdout().Write(output); err != nil {
				if cleanupErr := spdkDiagnoser.Cleanup(); cleanupErr != nil {
					logrus.WithError(cleanupErr).Warn("Failed to cleanup V2 Data Engine diagnoser after output error")
				}
				utils.CheckErr(errors.Wrap(err, "Failed to write V2 Data Engine diagnoser output"))
			}
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Cleaning up V2 Data Engine diagnoser")
			if err := spdkDiagnoser.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to cleanup V2 Data Engine diagnoser"))
			}

			if reason := spdkDiagnoser.UnhealthyReason(); reason != "" {
				utils.CheckErr(errors.Errorf("Longhorn V2 Data Engine SPDK target is unhealthy (%s)", reason))
			}

			logrus.Info("Completed V2 Data Engine diagnoser")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&spdkDiagnoser.NodeID, consts.CmdOptNodeId, "", "Inspect V2 Data Engine SPDK target processes only on the specified node.")
	cmd.Flags().StringVarP(&spdkDiagnoser.OutputFormat, consts.CmdOptOutput, "o", spdk.OutputFormatYAML, fmt.Sprintf("Output format of the results. One of: %v.", strings.Join(spdk.OutputFormats, ", ")))
	cmd.Flags().BoolVar(&spdkDiagnoser.IncludeAll, consts.CmdOptAll, false, "Include idle per-blob channels with 0 queued or active requests.")

	return cmd
}

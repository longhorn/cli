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
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdDiagnose(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var diagnoser = diagnose.Diagnoser{}
	var results diagnose.Results
	var outputFormat string

	cmd := &cobra.Command{
		Use:   consts.SubCmdDiagnose,
		Short: "Diagnose the health of the Longhorn system",
		Long: `This command checks whether the Longhorn system is healthy and fully functional. It is useful for verifying a Longhorn deployment, for example in automation.

The following checks are performed, ordered so that a component is checked before the components depending on it:
- LonghornManager: The longhorn-manager DaemonSet pods are ready.
- LonghornBackend: The longhorn-backend Service has ready endpoints.
- LonghornNodes: The Longhorn nodes are ready. Other node conditions and disks that are not ready are reported as warnings.
- EngineImages: The engine images are deployed on all nodes.
- InstanceManagers: The instance managers are running.
- CSIDriver: The Longhorn CSIDriver (driver.longhorn.io) is registered.
- CSIPlugin: The longhorn-csi-plugin DaemonSet pods are ready.
- CSIAttacher, CSIProvisioner, CSIResizer, CSISnapshotter: The CSI sidecar Deployments have ready pods.
- LonghornUI: The longhorn-ui Deployment has ready pods.
- Volumes: No Longhorn volume is faulted. Degraded volumes are reported as warnings.

The command only reads the cluster state. It does not create or modify any resource.

The results are printed to stdout in YAML or JSON, and the logs are printed to stderr.

The command exits with a non-zero status if any check fails.`,
		Example: `$ longhornctl diagnose
$ longhornctl diagnose -o json 2>/dev/null | jq '.[] | select(.status == "fail")'`,

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
	cmd.PersistentFlags().StringVar(&globalOpts.KubeConfigPath, consts.CmdOptKubeConfigPath, globalOpts.KubeConfigPath, "Kubernetes config (kubeconfig) path")
	cmd.PersistentFlags().StringVar(&globalOpts.Namespace, consts.CmdOptNamespace, globalOpts.Namespace, "The namespace where Longhorn is installed.")
	cmd.Flags().StringVarP(&outputFormat, consts.CmdOptOutput, "o", diagnose.OutputFormatYAML, fmt.Sprintf("Output format of the results. One of: %v.", strings.Join(diagnose.OutputFormats, ", ")))

	// Shadow the inherited options for deploying pods, which are not used by this command.
	for _, option := range []string{consts.CmdOptImage, consts.CmdOptImageRegistry, consts.CmdOptImagePullSecret, consts.CmdOptNodeSelector, consts.CmdOptTolerations} {
		utils.SetFlagHidden(cmd, option)
	}

	return cmd
}

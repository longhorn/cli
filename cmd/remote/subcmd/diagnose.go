package subcmd

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/diagnose"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdDiagnose(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var diagnoser = diagnose.Diagnoser{}
	var results diagnose.Results

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

The command exits with a non-zero status if any check fails.`,
		Example: `$ longhornctl diagnose`,

		PreRun: func(cmd *cobra.Command, args []string) {
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

			output, err := yaml.Marshal(results)
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to marshal Longhorn diagnoser result"))
			}

			logrus.Infof("Retrieved Longhorn diagnoser result:\n%v", string(output))
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

	// Shadow the inherited options for deploying pods, which are not used by this command.
	for _, option := range []string{consts.CmdOptImage, consts.CmdOptImageRegistry, consts.CmdOptImagePullSecret, consts.CmdOptNodeSelector, consts.CmdOptTolerations} {
		utils.SetFlagHidden(cmd, option)
	}

	return cmd
}

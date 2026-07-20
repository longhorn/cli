package subcmd

import (
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/remote/preflight"
	"github.com/longhorn/cli/pkg/remote/replica"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdCheck(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdCheck,
		Short: "Longhorn checking operations",
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.AddCommand(newCmdCheckPreflight(globalOpts))
	cmd.AddCommand(newCmdCheckReplica(globalOpts))

	return cmd
}

func newCmdCheckReplica(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var replicaChecker = replica.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdReplica,
		Short: "Check Longhorn replica integrity",
		Long: `This command checks the integrity of the snapshot chains in the Longhorn replica data directories on each node.
It identifies broken snapshot chains, for example snapshots referencing a missing parent, disk files without metadata files, and metadata files without disk files.
The results are presented by the replica data directory names, not the actual Custom Resource (CR) names.

By default, this command checks all Longhorn replicas in the data directory.
You can narrow down the results by using the following options:
- --name: Specify the Longhorn replica data directory name to check a specific replica.
- --volume-name: Filter replicas by the volume they belong to.`,
		Example: `$ longhornctl check replica
INFO[2024-07-16T17:23:47+08:00] Initializing replica checker
INFO[2024-07-16T17:23:47+08:00] Cleaning up replica checker
INFO[2024-07-16T17:23:47+08:00] Running replica checker
INFO[2024-07-16T17:23:51+08:00] Retrieved replica check results:
 replicas:
    pvc-48a6457d-585e-423b-b530-bbc68a5f948a-0e2603a7:
        - node: ip-10-0-2-123
          directory: /var/lib/longhorn/replicas/pvc-48a6457d-585e-423b-b530-bbc68a5f948a-0e2603a7
          volumeName: pvc-48a6457d-585e-423b-b530-bbc68a5f948a
          snapshotChain:
            - volume-head-001.img
            - volume-snap-40b3b028-b3b3-4a35-a806-8bea77f27c00.img
          errors:
            - 'broken snapshot chain: disk volume-snap-40b3b028-b3b3-4a35-a806-8bea77f27c00.img references parent volume-snap-6f244bbe-2857-46e4-92e2-eb1e16a63ba1.img, but the parent metadata file is missing'
INFO[2024-07-16T17:23:51+08:00] Cleaning up replica checker
INFO[2024-07-16T17:23:51+08:00] Completed replica checker`,

		PreRun: func(cmd *cobra.Command, args []string) {
			replicaChecker.Image = globalOpts.Image
			replicaChecker.ImageRegistry = globalOpts.ImageRegistry
			replicaChecker.ImagePullSecret = globalOpts.ImagePullSecret
			replicaChecker.KubeConfigPath = globalOpts.KubeConfigPath
			replicaChecker.NodeSelector = globalOpts.NodeSelector
			replicaChecker.Tolerations = globalOpts.Tolerations
			replicaChecker.Namespace = globalOpts.Namespace

			logrus.Info("Initializing replica checker")
			if err := replicaChecker.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize replica checker"))
			}

			logrus.Info("Cleaning up replica checker")
			if err := replicaChecker.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup replica checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running replica checker")
			output, err := replicaChecker.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run replica checker"))
			}

			logrus.Infof("Retrieved replica check results:\n %v", output)
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Cleaning up replica checker")
			if err := replicaChecker.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup replica checker"))
			}

			logrus.Info("Completed replica checker")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().StringVar(&replicaChecker.ReplicaName, consts.CmdOptName, "", "Specify the name of the replica to check.")
	cmd.Flags().StringVar(&replicaChecker.VolumeName, consts.CmdOptLonghornVolumeName, "", "Specify the name of the volume to check its replicas.")
	cmd.Flags().StringVar(&replicaChecker.LonghornDataDirectory, consts.CmdOptLonghornDataDirectory, "/var/lib/longhorn", "Specify the Longhorn data directory. If not provided, the default will be attempted, or it will fall back to the directory of longhorn-disk.cfg.")

	return cmd
}

func newCmdCheckPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var preflightChecker = preflight.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Run a preflight check for Longhorn",
		Long:  `This command verifies your Kubernetes cluster environment to ensure it meets Longhorn's requirements. It performs a series of checks that can help identify potential issues that may prevent Longhorn from functioning correctly.`,
		Example: `$ longhornctl check preflight
INFO[2024-07-16T17:17:38+08:00] Initializing preflight checker
INFO[2024-07-16T17:17:38+08:00] Cleaning up preflight checker
INFO[2024-07-16T17:17:38+08:00] Running preflight checker
INFO[2024-07-16T17:17:42+08:00] Retrieved preflight checker result:
ip-10-0-2-123:
  info:
  - Service iscsid is running
  - NFS4 is supported
  - Package nfs-client is installed
  - Package open-iscsi is installed
ip-10-0-2-142:
  info:
  - Service iscsid is running
  - NFS4 is supported
  - Package nfs-client is installed
  - Package open-iscsi is installed
ip-10-0-2-217:
  info:
  - Service iscsid is running
  - NFS4 is supported
  - Package nfs-client is installed
  - Package open-iscsi is installed
INFO[2024-07-16T17:17:42+08:00] Cleaning up preflight checker
INFO[2024-07-16T17:17:42+08:00] Completed preflight checker`,

		PreRun: func(cmd *cobra.Command, args []string) {
			preflightChecker.Image = globalOpts.Image
			preflightChecker.ImageRegistry = globalOpts.ImageRegistry
			preflightChecker.ImagePullSecret = globalOpts.ImagePullSecret
			preflightChecker.KubeConfigPath = globalOpts.KubeConfigPath
			preflightChecker.NodeSelector = globalOpts.NodeSelector
			preflightChecker.Tolerations = globalOpts.Tolerations
			preflightChecker.Namespace = globalOpts.Namespace

			logrus.Info("Initializing preflight checker")
			if err := preflightChecker.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight checker"))
			}

			logrus.Info("Cleaning up preflight checker")
			if err := preflightChecker.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup preflight checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			logrus.Info("Running preflight checker")
			output, err := preflightChecker.Run()
			if err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight checker"))
			}

			logrus.Infof("Retrieved preflight checker result:\n%v", output)
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			logrus.Info("Cleaning up preflight checker")
			if err := preflightChecker.Cleanup(); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to cleanup preflight checker"))
			}

			logrus.Info("Completed preflight checker")
		},
	}

	utils.SetGlobalOptionsRemote(cmd, globalOpts)

	cmd.Flags().BoolVar(&preflightChecker.EnableSpdk, consts.CmdOptEnableSpdk, false, "Enable checking of SPDK required packages, modules, and setup.")
	cmd.Flags().IntVar(&preflightChecker.HugePageSize, consts.CmdOptHugePageSize, 2048, "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().StringVar(&preflightChecker.UserspaceDriver, consts.CmdOptUserspaceDriver, "", "Userspace I/O driver for SPDK.")

	return cmd
}

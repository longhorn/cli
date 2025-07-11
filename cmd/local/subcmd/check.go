package subcmd

import (
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	local "github.com/longhorn/cli/pkg/local/preflight"
	"github.com/longhorn/cli/pkg/types"
	"github.com/longhorn/cli/pkg/utils"
)

func NewCmdCheck(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdCheck,
		Short: "Longhorn checking operations",
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.AddCommand(newCmdCheckPreflight(globalOpts))

	return cmd
}

func newCmdCheckPreflight(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	var localChecker = local.Checker{}

	cmd := &cobra.Command{
		Use:   consts.SubCmdPreflight,
		Short: "Run a preflight check for Longhorn",
		Long:  `This command verifies your Kubernetes cluster environment to ensure it meets Longhorn's requirements. It performs a series of checks that can help identify potential issues that may prevent Longhorn from functioning correctly.`,

		PreRun: func(cmd *cobra.Command, args []string) {
			localChecker.LogLevel = globalOpts.LogLevel

			if err := localChecker.Init(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to initialize preflight checker"))
			}
		},

		Run: func(cmd *cobra.Command, args []string) {
			if err := localChecker.Run(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to run preflight checker"))
			}

			logrus.Info("Successfully checked preflight environment")
		},

		PostRun: func(cmd *cobra.Command, args []string) {
			if err := localChecker.Output(); err != nil {
				utils.CheckErr(errors.Wrap(err, "Failed to output preflight checker collection"))
			}

			logrus.Info("Successfully output preflight checker collection")
		},
	}

	utils.SetGlobalOptionsLocal(cmd, globalOpts)

	cmd.Flags().StringVarP(&localChecker.OutputFilePath, consts.CmdOptOutputFile, "o", os.Getenv(consts.EnvOutputFilePath), "Output the result to a file, default to stdout.")
	cmd.Flags().BoolVar(&localChecker.EnableSpdk, consts.CmdOptEnableSpdk, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvEnableSpdk), false), "Enable checking of SPDK required packages, modules, and setup.")
	cmd.Flags().IntVar(&localChecker.HugePageSize, consts.CmdOptHugePageSize, utils.ConvertStringToTypeOrDefault(os.Getenv(consts.EnvHugePageSize), 2048), "Specify the huge page size in MiB for SPDK.")
	cmd.Flags().StringVar(&localChecker.UserspaceDriver, consts.CmdOptUserspaceDriver, os.Getenv(consts.EnvUserspaceDriver), "Userspace I/O driver for SPDK.")

	return cmd
}

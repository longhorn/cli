package subcmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
)

func bindLocalDataEngineMode(cmd *cobra.Command, target *string) {
	value := os.Getenv(consts.EnvDataEngineMode)
	if value == "" {
		value = string(types.DataEngineModeV1Capable)
	}
	cmd.Flags().StringVar(target, consts.CmdOptDataEngineMode, value, "Installation capability: v1-capable or v2-only. V2-only omits iSCSI and requires SPDK qualification.")
}

func validateLocalDataEngineEnvironment(cmd *cobra.Command, value string) error {
	mode, err := types.ParseDataEngineMode(value)
	if err != nil {
		return err
	}
	if mode == types.DataEngineModeV2Only && !cmd.Flags().Changed(consts.CmdOptHugePageSize) {
		if raw, present := os.LookupEnv(consts.EnvHugePageSize); present {
			if _, err := strconv.Atoi(raw); err != nil {
				return fmt.Errorf("v2-only hugepage environment must contain an explicit integer")
			}
		}
	}
	return nil
}

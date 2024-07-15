package subcmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/longhorn/cli/meta"
	"github.com/longhorn/cli/pkg/consts"
)

func NewCmdVersion() *cobra.Command {
	cmd := &cobra.Command{
		Use:   consts.SubCmdVersion,
		Short: fmt.Sprintf("Print %s version", consts.CmdLonghornctlRemote),

		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(meta.Version)
		},
	}

	return cmd
}

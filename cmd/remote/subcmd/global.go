package subcmd

import (
	"github.com/spf13/cobra"

	"k8s.io/kubectl/pkg/util/templates"

	"github.com/longhorn/cli/pkg/utils"
)

var KubeConfigPath string

func NewCmdGlobalOptions() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "global-options",
		Short: "Print global options inherited by all subcommands",
		Long:  `This command allows you to view the global options that apply to all subcommands.`,
		Run: func(cmd *cobra.Command, args []string) {
			utils.CheckErr(cmd.Usage())
		},
	}
	templates.UseOptionsTemplates(cmd)
	return cmd
}

package subcmd

import (
	"os"

	"github.com/longhorn/cli/pkg/utils"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

func NewCmdDoc() *cobra.Command {
	return &cobra.Command{
		Use:   "doc [output directory]",
		Short: "Generate markdown documentation for the CLI",
		Long:  "Generate markdown documentation for the CLI and save it to the specified output directory. This allows you to easily access the documentation in markdown format.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			outputDir := args[0]
			if err := generateMarkdownDocs(cmd.Parent(), outputDir); err != nil {
				utils.CheckErr(errors.Wrapf(err, "Failed to generate markdown documentation"))
			}
		},
	}
}

func generateMarkdownDocs(cmd *cobra.Command, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return doc.GenMarkdownTree(cmd, dir)
}

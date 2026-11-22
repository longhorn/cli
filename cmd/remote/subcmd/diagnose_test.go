package subcmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/longhorn/cli/pkg/consts"
	"github.com/longhorn/cli/pkg/types"
)

func newTestRootWithDiagnose(globalOpts *types.GlobalCmdOptions) *cobra.Command {
	root := &cobra.Command{Use: consts.CmdLonghornctlRemote}
	root.PersistentFlags().StringVarP(&globalOpts.LogLevel, consts.CmdOptLogLevel, "l", "info", "log level")
	root.PersistentFlags().StringVar(&globalOpts.KubeConfigPath, consts.CmdOptKubeConfigPath, "", "kubeconfig")
	root.PersistentFlags().StringVar(&globalOpts.Image, consts.CmdOptImage, consts.ImageLonghornCli, "image")
	root.PersistentFlags().StringVar(&globalOpts.ImageRegistry, consts.CmdOptImageRegistry, "", "image-registry")
	root.PersistentFlags().StringVar(&globalOpts.ImagePullSecret, consts.CmdOptImagePullSecret, "", "image-pull-secret")
	root.PersistentFlags().StringVar(&globalOpts.NodeSelector, consts.CmdOptNodeSelector, "", "node-selector")
	root.PersistentFlags().StringVar(&globalOpts.Tolerations, consts.CmdOptTolerations, "", "tolerations")
	root.PersistentFlags().StringVar(&globalOpts.Namespace, consts.CmdOptNamespace, consts.NamespaceLonghorn, "namespace")
	root.AddCommand(NewCmdDiagnose(globalOpts))
	return root
}

func TestDiagnoseSystemAndV2DataEngineSubcommands(t *testing.T) {
	globalOpts := &types.GlobalCmdOptions{}
	root := newTestRootWithDiagnose(globalOpts)

	// 1. Bare "diagnose" acts as a pure command group advertising "system" and "v2-data-engine".
	var diagHelp bytes.Buffer
	root.SetOut(&diagHelp)
	root.SetArgs([]string{"diagnose"})
	require.NoError(t, root.Execute())
	assert.Contains(t, diagHelp.String(), "system")
	assert.Contains(t, diagHelp.String(), "v2-data-engine")

	// 2. "diagnose system --help" hides DaemonSet pod flags while keeping API check flags.
	globalOpts = &types.GlobalCmdOptions{}
	root = newTestRootWithDiagnose(globalOpts)
	var sysHelp bytes.Buffer
	root.SetOut(&sysHelp)
	root.SetArgs([]string{"diagnose", "system", "--help"})
	require.NoError(t, root.Execute())
	assert.Contains(t, sysHelp.String(), "--kubeconfig ")
	assert.Contains(t, sysHelp.String(), "One of: yaml, json.")
	assert.NotContains(t, sysHelp.String(), "--image ")
	assert.NotContains(t, sysHelp.String(), "--tolerations")

	// 3. "diagnose v2-data-engine --help" exposes all DaemonSet and V2 Data Engine flags, with NO aliases.
	globalOpts = &types.GlobalCmdOptions{}
	root = newTestRootWithDiagnose(globalOpts)
	var v2Help bytes.Buffer
	root.SetOut(&v2Help)
	root.SetArgs([]string{"diagnose", "v2-data-engine", "--help"})
	require.NoError(t, root.Execute())
	assert.Contains(t, v2Help.String(), "--image ")
	assert.Contains(t, v2Help.String(), "--tolerations ")
	assert.Contains(t, v2Help.String(), "--node-id ")
	assert.Contains(t, v2Help.String(), "--all")
	assert.Contains(t, v2Help.String(), "One of: yaml, json, text.")
	assert.NotContains(t, v2Help.String(), "Aliases:")

	// 4. Confirm "v2" and "spdk" are NOT registered aliases under "diagnose".
	globalOpts = &types.GlobalCmdOptions{}
	root = newTestRootWithDiagnose(globalOpts)
	v2Cmd, _, err := root.Find([]string{"diagnose", "v2-data-engine"})
	require.NoError(t, err)
	assert.Empty(t, v2Cmd.Aliases)
	for _, disallowed := range []string{"v2", "spdk"} {
		found, _, _ := root.Find([]string{"diagnose", disallowed})
		assert.NotEqual(t, v2Cmd, found)
	}

	// 5. Parsing flags on "diagnose v2-data-engine" populates globalOpts cleanly.
	require.NoError(t, v2Cmd.ParseFlags([]string{
		"--image", "derekbit/longhorn-cli:iobuf2",
		"--tolerations", "node-role.kubernetes.io/control-plane:NoSchedule",
		"--kubeconfig", "/tmp/kubeconfig",
		"--node-id", "hp-23-tink-system",
		"-o", "text",
		"--all",
	}))
	assert.Equal(t, "derekbit/longhorn-cli:iobuf2", globalOpts.Image)
	assert.Equal(t, "node-role.kubernetes.io/control-plane:NoSchedule", globalOpts.Tolerations)
	assert.Equal(t, "/tmp/kubeconfig", globalOpts.KubeConfigPath)
}

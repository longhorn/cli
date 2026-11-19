package kubernetes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTestKubeconfig(t *testing.T, path, serverURL string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	content := `apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: ` + serverURL + `
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user:
    token: dummy-token
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

func TestGetKubeConfigPrecedence(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("KUBECONFIG", "")
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	// 1. Missing default ~/.kube/config and no env/flag returns an actionable error listing the checked path.
	homeCfg := filepath.Join(tmpHome, ".kube", "config")
	_, err := NewKubeClient("", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kubeconfig found (checked: "+homeCfg+")")
	assert.NotContains(t, err.Error(), "KUBERNETES_MASTER")

	// 2. Falls back to ~/.kube/config when --kubeconfig and KUBECONFIG are omitted.
	writeTestKubeconfig(t, homeCfg, "https://127.0.0.1:6443")

	cfg, err := GetKubeConfigFromPath("")
	require.NoError(t, err)
	assert.Equal(t, "https://127.0.0.1:6443", cfg.Host)

	client, err := NewKubeClient("", "")
	require.NoError(t, err)
	require.NotNil(t, client)

	// 3. KUBECONFIG path list overrides ~/.kube/config, and the first file wins when merging.
	envCfg1 := filepath.Join(tmpHome, "env1.yaml")
	envCfg2 := filepath.Join(tmpHome, "env2.yaml")
	writeTestKubeconfig(t, envCfg1, "https://127.0.0.2:6443")
	writeTestKubeconfig(t, envCfg2, "https://127.0.0.4:6443")
	t.Setenv("KUBECONFIG", envCfg1+string(os.PathListSeparator)+envCfg2)

	cfg, err = GetKubeConfigFromPath("")
	require.NoError(t, err)
	assert.Equal(t, "https://127.0.0.2:6443", cfg.Host)

	// 4. KUBECONFIG pointing only to missing files reports every checked path instead of falling back to ~/.kube/config.
	missing1 := filepath.Join(tmpHome, "missing1.yaml")
	missing2 := filepath.Join(tmpHome, "missing2.yaml")
	t.Setenv("KUBECONFIG", missing1+string(os.PathListSeparator)+missing2)

	_, err = GetKubeConfigFromPath("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no kubeconfig found (checked: "+missing1+", "+missing2+")")

	// 5. Explicit --kubeconfig overrides both KUBECONFIG and ~/.kube/config.
	explicitCfg := filepath.Join(tmpHome, "explicit.yaml")
	writeTestKubeconfig(t, explicitCfg, "https://127.0.0.3:6443")

	cfg, err = GetKubeConfigFromPath(explicitCfg)
	require.NoError(t, err)
	assert.Equal(t, "https://127.0.0.3:6443", cfg.Host)

	// 6. Explicit --kubeconfig pointing to a missing file fails immediately.
	_, err = GetKubeConfigFromPath(filepath.Join(tmpHome, "missing.yaml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provided kubeconfig path does not exist")
}

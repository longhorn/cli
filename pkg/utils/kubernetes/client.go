package kubernetes

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	kubeclient "k8s.io/client-go/kubernetes"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	kubeConfigHint = `Make sure to either:
  - Place a valid kubeconfig at ~/.kube/config
  - Set the environment variable: export KUBECONFIG=/path/to/config
  - Or use: --kubeconfig=/path/to/config`
)

func NewKubeClient(masterUrl string, kubeconfigPath string) (kubeClient *kubeclient.Clientset, err error) {
	kubeconfig, err := getKubeConfig(masterUrl, kubeconfigPath)
	if err != nil {
		return nil, err
	}

	kubeClient, err = kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w\n\n%s", err, kubeConfigHint)
	}

	return kubeClient, nil
}

func GetKubeConfigFromPath(kubeconfigPath string) (*rest.Config, error) {
	return getKubeConfig("", kubeconfigPath)
}

func getKubeConfig(masterUrl string, kubeconfigPath string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("provided kubeconfig path does not exist: %s\n\n%s", kubeconfigPath, kubeConfigHint)
		}
		loadingRules.ExplicitPath = kubeconfigPath
	} else if os.Getenv(clientcmd.RecommendedConfigPathEnvVar) == "" {
		// Resolve ~/.kube/config at call time; clientcmd.RecommendedHomeFile is computed once at package init.
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			loadingRules.Precedence = []string{
				filepath.Join(homeDir, clientcmd.RecommendedHomeDir, clientcmd.RecommendedFileName),
			}
		}
	}

	overrides := &clientcmd.ConfigOverrides{}
	if masterUrl != "" {
		overrides.ClusterInfo = clientcmdapi.Cluster{Server: masterUrl}
	}

	kubeconfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		if kubeconfigPath != "" {
			return nil, fmt.Errorf("failed to load kubeconfig from path '%s': %w\n\n%s", kubeconfigPath, err, kubeConfigHint)
		}
		if clientcmd.IsEmptyConfig(err) {
			return nil, fmt.Errorf("no kubeconfig found (checked: %s) and in-cluster configuration is unavailable\n\n%s",
				strings.Join(loadingRules.GetLoadingPrecedence(), ", "), kubeConfigHint)
		}
		return nil, fmt.Errorf("failed to load kubeconfig: %w\n\n%s", err, kubeConfigHint)
	}

	return kubeconfig, nil
}

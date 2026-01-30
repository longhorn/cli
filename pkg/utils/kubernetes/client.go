package kubernetes

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	kubeclient "k8s.io/client-go/kubernetes"
)

const (
	kubeConfigHint = `Make sure to either:
  - Set the environment variable: export KUBECONFIG=/path/to/config
  - Or use: --kubeconfig=/path/to/config`
)

func NewKubeClient(masterUrl string, kubeconfigPath string) (kubeClient *kubeclient.Clientset, err error) {
	if masterUrl == "" && kubeconfigPath == "" {
		return nil, fmt.Errorf("no kubeconfig path provided.\n\n%s", kubeConfigHint)
	}

	kubeconfig, err := GetKubeConfigFromPath(kubeconfigPath)
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
	if kubeconfigPath != "" {
		if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("provided kubeconfig path does not exist: %s\n\n%s", kubeconfigPath, kubeConfigHint)
		}
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from path '%s': %w\n\n%s", kubeconfigPath, err, kubeConfigHint)
	}

	return kubeconfig, nil
}

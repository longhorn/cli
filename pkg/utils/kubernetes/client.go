package kubernetes

import (
	"context"
	"fmt"
	"os"

	"github.com/pkg/errors"
	"k8s.io/client-go/tools/clientcmd"

	lhClient "github.com/longhorn/longhorn-manager/util/client"
	kubeclient "k8s.io/client-go/kubernetes"
)

func NewKubeClient(masterUrl string, kubeconfigPath string) (kubeClient *kubeclient.Clientset, err error) {
	const kubeConfigHint = `Make sure to either:
  - Set the environment variable: export KUBECONFIG=/path/to/config
  - Or use: --kubeconfig=/path/to/config`

	if masterUrl == "" && kubeconfigPath == "" {
		return nil, fmt.Errorf("no kubeconfig path provided.\n\n%s", kubeConfigHint)
	}

	if kubeconfigPath != "" {
		if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("provided kubeconfig path does not exist: %s\n\n%s", kubeconfigPath, kubeConfigHint)
		}
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags(masterUrl, kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig from path '%s': %w\n\n%s", kubeconfigPath, err, kubeConfigHint)
	}

	kubeClient, err = kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w\n\n%s", err, kubeConfigHint)
	}

	return kubeClient, nil
}

func NewLonghornClient(kubeconfigPath string) (*lhClient.Clients, context.CancelFunc, error) {
	ctx, cancel := context.WithCancel(context.Background())

	lhClient, err := lhClient.NewClients(kubeconfigPath, true, ctx.Done())
	if err != nil {
		return nil, cancel, errors.Wrap(err, "failed to create Longhorn clientset")
	}

	return lhClient, cancel, nil
}

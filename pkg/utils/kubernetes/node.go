package kubernetes

import (
	"context"
	"os"

	"k8s.io/apimachinery/pkg/api/resource"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
)

// GetHugePagesCapacity returns hugepages-2Mi capacity of current node
func GetHugePagesCapacity(kubeClient *kubeclient.Clientset) (*resource.Quantity, error) {
	currentNodeID := os.Getenv(consts.EnvCurrentNodeID)
	node, err := kubeClient.CoreV1().Nodes().Get(context.TODO(), currentNodeID, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	return node.Status.Capacity.Name("hugepages-2Mi", resource.BinarySI), nil
}

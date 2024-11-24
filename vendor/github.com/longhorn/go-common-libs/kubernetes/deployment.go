package kubernetes

import (
	"context"

	"github.com/sirupsen/logrus"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
)

// GetDeployment returns the Deployment with the given name in the given namespace.
func GetDeployment(kubeClient kubeclient.Interface, namespace, name string) (*appsv1.Deployment, error) {
	log := logrus.WithFields(logrus.Fields{
		"kind":      "Deployment",
		"namespace": namespace,
		"name":      name,
	})
	log.Trace("Getting resource")

	return kubeClient.AppsV1().Deployments(namespace).Get(context.Background(), name, metav1.GetOptions{})
}

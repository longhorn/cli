package kubernetes

import (
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	commonkube "github.com/longhorn/go-common-libs/kubernetes"
)

// CreateRbac creates a new ServiceAccount, ClusterRole, and ClusterRoleBinding
func CreateRbac(kubeClient *kubeclient.Clientset, namespace, name string, rbacRules []rbacv1.PolicyRule) error {
	newServiceAccount := newServiceAccount(namespace, name)
	_, err := commonkube.CreateServiceAccount(kubeClient, newServiceAccount)
	if err != nil {
		return err
	}

	newClusterRole := newClusterRole(name, rbacRules)
	_, err = commonkube.CreateClusterRole(kubeClient, newClusterRole)
	if err != nil {
		return err
	}

	newClusterRoleBinding := newClusterRoleBinding(namespace, name)
	_, err = commonkube.CreateClusterRoleBinding(kubeClient, newClusterRoleBinding)
	if err != nil {
		return err
	}

	return nil
}

// DeleteRbac deletes ServiceAccount, ClusterRole, and ClusterRoleBinding
func DeleteRbac(kubeClient *kubeclient.Clientset, namespace, name string) error {
	if err := commonkube.DeleteClusterRoleBinding(kubeClient, name); err != nil {
		return err
	}

	if err := commonkube.DeleteClusterRole(kubeClient, name); err != nil {
		return err
	}

	return commonkube.DeleteServiceAccount(kubeClient, namespace, name)
}

func newClusterRole(name string, rules []rbacv1.PolicyRule) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: rules,
	}
}

func newClusterRoleBinding(namespace, name string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      name,
				Namespace: namespace,
			},
		},
	}
}

func newServiceAccount(namespace, name string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

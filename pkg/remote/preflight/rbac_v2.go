package preflight

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	kubeclient "k8s.io/client-go/kubernetes"
)

type v2RBACReceipt struct {
	Namespace, Name                                 string
	ServiceAccount, ClusterRole, ClusterRoleBinding k8stypes.UID
}

// Never adopt an AlreadyExists resource, or delete an object whose creation
// did not return its UID. Partial receipts retain ownership of partial creates.
func createV2RBAC(ctx context.Context, client kubeclient.Interface, namespace, name string, rules []rbacv1.PolicyRule) (*v2RBACReceipt, error) {
	receipt := &v2RBACReceipt{Namespace: namespace, Name: name}
	account, err := client.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}, metav1.CreateOptions{})
	if err != nil {
		return receipt, err
	}
	if account.UID == "" || account.Name != name || account.Namespace != namespace {
		return receipt, fmt.Errorf("V2 checker service account creation returned no identified receipt")
	}
	receipt.ServiceAccount = account.UID
	role, err := client.RbacV1().ClusterRoles().Create(ctx, &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: name}, Rules: rules}, metav1.CreateOptions{})
	if err != nil {
		return receipt, err
	}
	if role.UID == "" || role.Name != name {
		return receipt, fmt.Errorf("V2 checker role creation returned no identified receipt")
	}
	receipt.ClusterRole = role.UID
	binding, err := client.RbacV1().ClusterRoleBindings().Create(ctx, &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name}, RoleRef: rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "ClusterRole", Name: name}, Subjects: []rbacv1.Subject{{Kind: "ServiceAccount", Name: name, Namespace: namespace}}}, metav1.CreateOptions{})
	if err != nil {
		return receipt, err
	}
	if binding.UID == "" || binding.Name != name {
		return receipt, fmt.Errorf("V2 checker binding creation returned no identified receipt")
	}
	receipt.ClusterRoleBinding = binding.UID
	return receipt, nil
}

func cleanupV2RBAC(ctx context.Context, client kubeclient.Interface, receipt *v2RBACReceipt) error {
	if receipt == nil {
		return nil
	}
	var result error
	remove := func(uid k8stypes.UID, deleteObject func(metav1.DeleteOptions) error) {
		if uid == "" {
			return
		}
		err := deleteObject(metav1.DeleteOptions{Preconditions: &metav1.Preconditions{UID: &uid}})
		if err != nil && !apierrors.IsNotFound(err) {
			result = errors.Join(result, err)
		}
	}
	remove(receipt.ClusterRoleBinding, func(options metav1.DeleteOptions) error {
		return client.RbacV1().ClusterRoleBindings().Delete(ctx, receipt.Name, options)
	})
	remove(receipt.ClusterRole, func(options metav1.DeleteOptions) error {
		return client.RbacV1().ClusterRoles().Delete(ctx, receipt.Name, options)
	})
	remove(receipt.ServiceAccount, func(options metav1.DeleteOptions) error {
		return client.CoreV1().ServiceAccounts(receipt.Namespace).Delete(ctx, receipt.Name, options)
	})
	return result
}

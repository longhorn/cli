package preflight

import (
	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	lhgokube "github.com/longhorn/go-common-libs/kubernetes"

	"github.com/longhorn/cli/pkg/consts"
)

// Uninstaller provide functions for the preflight uninstall.
type Uninstaller struct {
	UninstallerCmdOptions

	kubeClient *kubeclient.Clientset
}

// UninstallerCmdOptions holds the options for the command.
type UninstallerCmdOptions struct {
	LogLevel       string
	KubeConfigPath string
}

// Init initializes the Uninstaller.
func (remote *Uninstaller) Init() error {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", remote.KubeConfigPath)
	if err != nil {
		return err
	}

	kubeClient, err := kubeclient.NewForConfig(kubeconfig)
	if err != nil {
		return err
	}
	remote.kubeClient = kubeClient

	return nil
}

// Run deletes the Kubernetes resources created by Longhorn install preflight command.
func (remote *Uninstaller) Run() error {
	uninstallNames := []string{
		consts.AppNamePreflightChecker,
		consts.AppNamePreflightInstaller,
		consts.AppNamePreflightContainerOptimizedOS,
	}

	for _, name := range uninstallNames {
		logrus.Infof("Uninstalling %v", name)

		switch name {
		case consts.AppNamePreflightChecker:
			err := lhgokube.DeleteClusterRoleBinding(remote.kubeClient, name)
			if err != nil {
				return err
			}

			err = lhgokube.DeleteClusterRole(remote.kubeClient, name)
			if err != nil {
				return err
			}

			err = lhgokube.DeleteServiceAccount(remote.kubeClient, metav1.NamespaceDefault, name)
			if err != nil {
				return err
			}

			err = lhgokube.DeleteDaemonSet(remote.kubeClient, metav1.NamespaceDefault, name)
			if err != nil {
				return err
			}

		case consts.AppNamePreflightInstaller:
			err := lhgokube.DeleteDaemonSet(remote.kubeClient, metav1.NamespaceDefault, name)
			if err != nil {
				return err
			}

		case consts.AppNamePreflightContainerOptimizedOS:
			err := lhgokube.DeleteDaemonSet(remote.kubeClient, metav1.NamespaceDefault, name)
			if err != nil {
				return err
			}

			err = lhgokube.DeleteConfigMap(remote.kubeClient, metav1.NamespaceDefault, name)
			if err != nil {
				return err
			}
		}
	}

	logrus.Info("Completed uninstall")
	return nil
}

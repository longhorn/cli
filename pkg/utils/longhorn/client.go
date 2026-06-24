package longhorn

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubeutils "github.com/longhorn/cli/pkg/utils/kubernetes"
	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
	lhclientset "github.com/longhorn/longhorn-manager/k8s/pkg/client/clientset/versioned"
	lhTypes "github.com/longhorn/longhorn-manager/types"
)

// LonghornClient is a lightweight client for interacting with Longhorn CRs via the Kubernetes API.
//
// It is implemented instead of reusing `lhClient.Clients` because the CLI only needs to operate
// on CRs (e.g., Volume, Snapshot), while the manager client includes additional components such
// as the DataStore (informer/cache) and manager API client, which are unnecessary and too heavy
// for short-lived, out-of-cluster CLI usage.
//
// This client keeps the dependency minimal and avoids global environment setup and informer
// lifecycle management.
type LonghornClient struct {
	namespace string
	clientset *lhclientset.Clientset
}

func NewLonghornClient(kubeconfigPath, namespace string) (*LonghornClient, error) {
	kubeconfig, err := kubeutils.GetKubeConfigFromPath(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	lhClient, err := lhclientset.NewForConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return &LonghornClient{
		namespace: namespace,
		clientset: lhClient,
	}, nil
}

func (s *LonghornClient) ListVolumes() (*longhorn.VolumeList, error) {
	return s.clientset.LonghornV1beta2().Volumes(s.namespace).List(context.Background(), metav1.ListOptions{})
}

func (s *LonghornClient) GetVolume(name string) (*longhorn.Volume, error) {
	return s.clientset.LonghornV1beta2().Volumes(s.namespace).Get(context.Background(), name, metav1.GetOptions{})
}

func (s *LonghornClient) UpdateVolume(volume *longhorn.Volume) (*longhorn.Volume, error) {
	return s.clientset.LonghornV1beta2().Volumes(s.namespace).Update(context.Background(), volume, metav1.UpdateOptions{})
}

func (s *LonghornClient) ListVolumeSnapshots(volumeName string) (*longhorn.SnapshotList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: lhTypes.GetVolumeLabels(volumeName),
	})
	if err != nil {
		return nil, err
	}

	return s.clientset.LonghornV1beta2().Snapshots(s.namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
}

func (s *LonghornClient) ListVolumesByDataEngine(dataEngine longhorn.DataEngineType) (*longhorn.VolumeList, error) {
	volumes, err := s.ListVolumes()
	if err != nil {
		return nil, err
	}

	filtered := &longhorn.VolumeList{}
	for _, volume := range volumes.Items {
		if volume.Spec.DataEngine != dataEngine {
			continue
		}
		filtered.Items = append(filtered.Items, volume)
	}

	return filtered, nil
}

func (s *LonghornClient) ListVolumeReplicas(volumeName string) (*longhorn.ReplicaList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: lhTypes.GetVolumeLabels(volumeName),
	})
	if err != nil {
		return nil, err
	}

	return s.clientset.LonghornV1beta2().Replicas(s.namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
}

func (s *LonghornClient) ListVolumeEngines(volumeName string) (*longhorn.EngineList, error) {
	selector, err := metav1.LabelSelectorAsSelector(&metav1.LabelSelector{
		MatchLabels: lhTypes.GetVolumeLabels(volumeName),
	})
	if err != nil {
		return nil, err
	}

	return s.clientset.LonghornV1beta2().Engines(s.namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: selector.String(),
	})
}

func (s *LonghornClient) ListInstanceManagers() (*longhorn.InstanceManagerList, error) {
	return s.clientset.LonghornV1beta2().InstanceManagers(s.namespace).List(context.Background(), metav1.ListOptions{})
}

func (s *LonghornClient) ListNodes() (*longhorn.NodeList, error) {
	return s.clientset.LonghornV1beta2().Nodes(s.namespace).List(context.Background(), metav1.ListOptions{})
}

func (s *LonghornClient) GetSetting(name string) (*longhorn.Setting, error) {
	return s.clientset.LonghornV1beta2().Settings(s.namespace).Get(context.Background(), name, metav1.GetOptions{})
}

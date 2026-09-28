package diagnose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"k8s.io/utils/ptr"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"

	"github.com/longhorn/cli/pkg/types"
)

func TestEvaluateDaemonSet(t *testing.T) {
	for _, test := range []struct {
		name     string
		status   appsv1.DaemonSetStatus
		expected types.LogCollection
	}{
		{
			name:     "no pods scheduled",
			expected: types.LogCollection{Error: []string{"DaemonSet longhorn-manager has no pods scheduled"}},
		},
		{
			name:     "not all pods ready",
			status:   appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 2},
			expected: types.LogCollection{Error: []string{"DaemonSet longhorn-manager has 2/3 pods ready"}},
		},
		{
			name:     "all pods ready",
			status:   appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 3},
			expected: types.LogCollection{Info: []string{"DaemonSet longhorn-manager has 3/3 pods ready"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemonSet := &appsv1.DaemonSet{
				ObjectMeta: metav1.ObjectMeta{Name: "longhorn-manager"},
				Status:     test.status,
			}

			result := &Result{}
			evaluateDaemonSet(daemonSet, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateDeployment(t *testing.T) {
	for _, test := range []struct {
		name     string
		replicas *int32
		ready    int32
		expected types.LogCollection
	}{
		{
			name:     "no ready pods",
			replicas: ptr.To(int32(3)),
			expected: types.LogCollection{Error: []string{"Deployment csi-attacher has 0/3 pods ready"}},
		},
		{
			name:     "scaled down",
			replicas: ptr.To(int32(0)),
			expected: types.LogCollection{Error: []string{"Deployment csi-attacher is scaled down to 0 replicas"}},
		},
		{
			name:     "not all pods ready",
			replicas: ptr.To(int32(3)),
			ready:    1,
			expected: types.LogCollection{Warn: []string{"Deployment csi-attacher has 1/3 pods ready"}},
		},
		{
			name:     "all pods ready",
			replicas: ptr.To(int32(3)),
			ready:    3,
			expected: types.LogCollection{Info: []string{"Deployment csi-attacher has 3/3 pods ready"}},
		},
		{
			name:     "default replicas",
			ready:    1,
			expected: types.LogCollection{Info: []string{"Deployment csi-attacher has 1/1 pods ready"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{Name: "csi-attacher"},
				Spec:       appsv1.DeploymentSpec{Replicas: test.replicas},
				Status:     appsv1.DeploymentStatus{ReadyReplicas: test.ready},
			}

			result := &Result{}
			evaluateDeployment(deployment, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateEndpointSlices(t *testing.T) {
	for _, test := range []struct {
		name           string
		endpointSlices []discoveryv1.EndpointSlice
		expected       types.LogCollection
	}{
		{
			name:     "no endpoint slices",
			expected: types.LogCollection{Error: []string{"Service longhorn-backend has no ready endpoints"}},
		},
		{
			name: "no ready endpoints",
			endpointSlices: []discoveryv1.EndpointSlice{
				{Endpoints: []discoveryv1.Endpoint{{Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false)}}}},
			},
			expected: types.LogCollection{Error: []string{"Service longhorn-backend has no ready endpoints"}},
		},
		{
			name: "ready endpoints",
			endpointSlices: []discoveryv1.EndpointSlice{
				{Endpoints: []discoveryv1.Endpoint{
					{Addresses: []string{"10.0.0.1"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(true)}},
					{Addresses: []string{"10.0.0.2"}, Conditions: discoveryv1.EndpointConditions{Ready: ptr.To(false)}},
				}},
				{Endpoints: []discoveryv1.Endpoint{{Addresses: []string{"10.0.0.3"}}}},
			},
			expected: types.LogCollection{Info: []string{"Service longhorn-backend has 2 ready endpoints"}},
		},
		{
			name: "dual-stack endpoints of the same pods",
			endpointSlices: []discoveryv1.EndpointSlice{
				{
					AddressType: discoveryv1.AddressTypeIPv4,
					Endpoints: []discoveryv1.Endpoint{
						{Addresses: []string{"10.0.0.1"}, TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "longhorn-system", Name: "longhorn-manager-a"}},
						{Addresses: []string{"10.0.0.2"}, TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "longhorn-system", Name: "longhorn-manager-b"}},
					},
				},
				{
					AddressType: discoveryv1.AddressTypeIPv6,
					Endpoints: []discoveryv1.Endpoint{
						{Addresses: []string{"fd00::1"}, TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "longhorn-system", Name: "longhorn-manager-a"}},
						{Addresses: []string{"fd00::2"}, TargetRef: &corev1.ObjectReference{Kind: "Pod", Namespace: "longhorn-system", Name: "longhorn-manager-b"}},
					},
				},
			},
			expected: types.LogCollection{Info: []string{"Service longhorn-backend has 2 ready endpoints"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateEndpointSlices("longhorn-backend", test.endpointSlices, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateNodes(t *testing.T) {
	readyCondition := longhorn.Condition{Type: longhorn.NodeConditionTypeReady, Status: longhorn.ConditionStatusTrue}
	readyDisk := &longhorn.DiskStatus{
		Conditions: []longhorn.Condition{
			{Type: longhorn.DiskConditionTypeReady, Status: longhorn.ConditionStatusTrue},
			{Type: longhorn.DiskConditionTypeSchedulable, Status: longhorn.ConditionStatusTrue},
		},
	}

	for _, test := range []struct {
		name     string
		nodes    []longhorn.Node
		expected types.LogCollection
	}{
		{
			name:     "no nodes",
			expected: types.LogCollection{Error: []string{"No Longhorn node is found"}},
		},
		{
			name: "all nodes ready",
			nodes: []longhorn.Node{
				newNode("node-1", []longhorn.Condition{readyCondition}, map[string]*longhorn.DiskStatus{"disk-1": readyDisk}),
				newNode("node-2", []longhorn.Condition{readyCondition}, map[string]*longhorn.DiskStatus{"disk-1": readyDisk}),
			},
			expected: types.LogCollection{Info: []string{"2/2 nodes are ready"}},
		},
		{
			name: "node not ready",
			nodes: []longhorn.Node{
				newNode("node-1", []longhorn.Condition{readyCondition}, nil),
				newNode("node-2", []longhorn.Condition{
					{Type: longhorn.NodeConditionTypeReady, Status: longhorn.ConditionStatusFalse, Reason: "ManagerPodDown", Message: "Node node-2 is down: the manager pod is not running"},
					{Type: longhorn.NodeConditionTypeSchedulable, Status: longhorn.ConditionStatusFalse},
				}, map[string]*longhorn.DiskStatus{"disk-1": {}}),
				newNode("node-3", nil, nil),
			},
			expected: types.LogCollection{
				Error: []string{
					"Node node-2 is not ready: Node node-2 is down: the manager pod is not running",
					"Node node-3 is not ready: status is Unknown",
				},
				Info: []string{"1/3 nodes are ready"},
			},
		},
		{
			name: "node condition false",
			nodes: []longhorn.Node{
				newNode("node-1", []longhorn.Condition{
					readyCondition,
					{Type: longhorn.NodeConditionTypeMountPropagation, Status: longhorn.ConditionStatusTrue},
					{Type: longhorn.NodeConditionTypeRequiredPackages, Status: longhorn.ConditionStatusFalse, Reason: "PackagesNotInstalled", Message: "Missing packages: [nfs-common]"},
					{Type: longhorn.NodeConditionTypeSchedulable, Status: longhorn.ConditionStatusFalse, Reason: "KubernetesNodeCordoned"},
					{Type: longhorn.NodeConditionTypeMultipathd, Status: longhorn.ConditionStatusUnknown},
				}, nil),
			},
			expected: types.LogCollection{
				Info: []string{"1/1 nodes are ready"},
				Warn: []string{
					"Node node-1 condition RequiredPackages is false: Missing packages: [nfs-common]",
					"Node node-1 condition Schedulable is false: KubernetesNodeCordoned",
				},
			},
		},
		{
			name: "disks not ready or schedulable",
			nodes: []longhorn.Node{
				newNode("node-1", []longhorn.Condition{readyCondition}, map[string]*longhorn.DiskStatus{
					"disk-3": {
						Conditions: []longhorn.Condition{
							{Type: longhorn.DiskConditionTypeReady, Status: longhorn.ConditionStatusTrue},
							{Type: longhorn.DiskConditionTypeSchedulable, Status: longhorn.ConditionStatusFalse, Message: "Disk disk-3 on node node-1 is not schedulable for more replica"},
						},
					},
					"disk-1": readyDisk,
					"disk-2": {
						Conditions: []longhorn.Condition{
							{Type: longhorn.DiskConditionTypeReady, Status: longhorn.ConditionStatusFalse, Reason: "DiskNotFound"},
						},
					},
					"disk-4": nil,
				}),
			},
			expected: types.LogCollection{
				Info: []string{"1/1 nodes are ready"},
				Warn: []string{
					"Disk disk-2 on node node-1 is not ready: DiskNotFound",
					"Disk disk-3 on node node-1 is not schedulable: Disk disk-3 on node node-1 is not schedulable for more replica",
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateNodes(test.nodes, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateEngineImages(t *testing.T) {
	for _, test := range []struct {
		name         string
		engineImages []longhorn.EngineImage
		expected     types.LogCollection
	}{
		{
			name:     "no engine images",
			expected: types.LogCollection{Error: []string{"No EngineImage is found"}},
		},
		{
			name: "deployed",
			engineImages: []longhorn.EngineImage{
				newEngineImage("ei-1", longhorn.EngineImageStatus{
					State:             longhorn.EngineImageStateDeployed,
					NodeDeploymentMap: map[string]bool{"node-1": true, "node-2": true},
				}),
			},
			expected: types.LogCollection{Info: []string{"EngineImage ei-1 (longhornio/longhorn-engine:ei-1) is deployed"}},
		},
		{
			name: "not deployed on some nodes",
			engineImages: []longhorn.EngineImage{
				newEngineImage("ei-1", longhorn.EngineImageStatus{
					State:             longhorn.EngineImageStateDeploying,
					NodeDeploymentMap: map[string]bool{"node-3": false, "node-1": true, "node-2": false},
				}),
				newEngineImage("ei-2", longhorn.EngineImageStatus{}),
			},
			expected: types.LogCollection{
				Error: []string{
					"EngineImage ei-1 (longhornio/longhorn-engine:ei-1) is deploying, not deployed on nodes [node-2 node-3]",
					"EngineImage ei-2 (longhornio/longhorn-engine:ei-2) is unknown, not deployed on nodes []",
				},
			},
		},
		{
			name: "incompatible",
			engineImages: []longhorn.EngineImage{
				newEngineImage("ei-1", longhorn.EngineImageStatus{
					State:        longhorn.EngineImageStateDeploying,
					Incompatible: true,
				}),
				newEngineImage("ei-2", longhorn.EngineImageStatus{
					State: longhorn.EngineImageStateDeployed,
				}),
			},
			expected: types.LogCollection{
				Info: []string{"EngineImage ei-2 (longhornio/longhorn-engine:ei-2) is deployed"},
				Warn: []string{"EngineImage ei-1 (longhornio/longhorn-engine:ei-1) is incompatible"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateEngineImages(test.engineImages, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateInstanceManagers(t *testing.T) {
	for _, test := range []struct {
		name             string
		instanceManagers []longhorn.InstanceManager
		expected         types.LogCollection
	}{
		{
			name:     "no instance managers",
			expected: types.LogCollection{Error: []string{"No InstanceManager is found"}},
		},
		{
			name: "all running",
			instanceManagers: []longhorn.InstanceManager{
				newInstanceManager("im-1", "node-1", longhorn.InstanceManagerStateRunning),
				newInstanceManager("im-2", "node-2", longhorn.InstanceManagerStateRunning),
			},
			expected: types.LogCollection{Info: []string{"2/2 instance managers are running"}},
		},
		{
			name: "not running",
			instanceManagers: []longhorn.InstanceManager{
				newInstanceManager("im-1", "node-1", longhorn.InstanceManagerStateRunning),
				newInstanceManager("im-2", "node-2", longhorn.InstanceManagerStateError),
				newInstanceManager("im-3", "node-3", ""),
			},
			expected: types.LogCollection{
				Error: []string{
					"InstanceManager im-2 on node node-2 is error",
					"InstanceManager im-3 on node node-3 is unknown",
				},
				Info: []string{"1/3 instance managers are running"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateInstanceManagers(test.instanceManagers, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestDescribeCondition(t *testing.T) {
	assert.Equal(t, "message", describeCondition(longhorn.Condition{Status: longhorn.ConditionStatusFalse, Reason: "reason", Message: "message"}))
	assert.Equal(t, "reason", describeCondition(longhorn.Condition{Status: longhorn.ConditionStatusFalse, Reason: "reason"}))
	assert.Equal(t, "status is False", describeCondition(longhorn.Condition{Status: longhorn.ConditionStatusFalse}))
	assert.Equal(t, "status is unknown", describeCondition(longhorn.Condition{}))
}

func newNode(name string, conditions []longhorn.Condition, disks map[string]*longhorn.DiskStatus) longhorn.Node {
	return longhorn.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: longhorn.NodeStatus{
			Conditions: conditions,
			DiskStatus: disks,
		},
	}
}

func newEngineImage(name string, status longhorn.EngineImageStatus) longhorn.EngineImage {
	return longhorn.EngineImage{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       longhorn.EngineImageSpec{Image: "longhornio/longhorn-engine:" + name},
		Status:     status,
	}
}

func newInstanceManager(name, nodeID string, state longhorn.InstanceManagerState) longhorn.InstanceManager {
	return longhorn.InstanceManager{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       longhorn.InstanceManagerSpec{NodeID: nodeID},
		Status:     longhorn.InstanceManagerStatus{CurrentState: state},
	}
}

func TestEvaluateVolumes(t *testing.T) {
	for _, test := range []struct {
		name     string
		volumes  []longhorn.Volume
		expected types.LogCollection
	}{
		{
			name:     "no volumes",
			expected: types.LogCollection{Info: []string{"0 volumes are found, 0 degraded, 0 faulted"}},
		},
		{
			name: "healthy and detached volumes",
			volumes: []longhorn.Volume{
				newVolume("vol-1", longhorn.VolumeRobustnessHealthy),
				newVolume("vol-2", longhorn.VolumeRobustnessUnknown),
			},
			expected: types.LogCollection{Info: []string{"2 volumes are found, 0 degraded, 0 faulted"}},
		},
		{
			name: "degraded and faulted volumes",
			volumes: []longhorn.Volume{
				newVolume("vol-3", longhorn.VolumeRobustnessFaulted),
				newVolume("vol-1", longhorn.VolumeRobustnessHealthy),
				newVolume("vol-4", longhorn.VolumeRobustnessDegraded),
				newVolume("vol-2", longhorn.VolumeRobustnessDegraded),
				newVolume("vol-0", longhorn.VolumeRobustnessFaulted),
			},
			expected: types.LogCollection{
				Info:  []string{"5 volumes are found, 2 degraded, 2 faulted"},
				Warn:  []string{"Volume vol-2 is degraded", "Volume vol-4 is degraded"},
				Error: []string{"Volume vol-0 is faulted", "Volume vol-3 is faulted"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateVolumes(test.volumes, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func newVolume(name string, robustness longhorn.VolumeRobustness) longhorn.Volume {
	return longhorn.Volume{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status:     longhorn.VolumeStatus{Robustness: robustness},
	}
}

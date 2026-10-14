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
	lhtypes "github.com/longhorn/longhorn-manager/types"

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
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: lhtypes.GetInstanceManagerLabels(nodeID, "", "", ""),
		},
		Spec:   longhorn.InstanceManagerSpec{NodeID: nodeID},
		Status: longhorn.InstanceManagerStatus{CurrentState: state},
	}
}

func TestCheckInstanceManagerLabels(t *testing.T) {
	newLabeledInstanceManager := func(mutate func(*longhorn.InstanceManager)) longhorn.InstanceManager {
		im := longhorn.InstanceManager{
			ObjectMeta: metav1.ObjectMeta{
				Name: "im-1",
				Labels: map[string]string{
					"longhorn.io/managed-by":            "longhorn-manager",
					"longhorn.io/component":             "instance-manager",
					"longhorn.io/instance-manager-type": "replica",
					"longhorn.io/node":                  "node-1",
				},
			},
			Spec: longhorn.InstanceManagerSpec{
				NodeID: "node-1",
				Type:   longhorn.InstanceManagerTypeReplica,
			},
		}
		if mutate != nil {
			mutate(&im)
		}
		return im
	}

	for _, test := range []struct {
		name     string
		mutate   func(*longhorn.InstanceManager)
		expected types.LogCollection
	}{
		{
			name:     "all critical labels present",
			mutate:   nil,
			expected: types.LogCollection{},
		},
		{
			name: "missing critical label",
			mutate: func(im *longhorn.InstanceManager) {
				delete(im.Labels, "longhorn.io/node")
			},
			expected: types.LogCollection{Error: []string{
				`InstanceManager im-1 is missing critical label "longhorn.io/node"`,
			}},
		},
		{
			name: "unexpected label value",
			mutate: func(im *longhorn.InstanceManager) {
				im.Labels["longhorn.io/instance-manager-type"] = "engine"
			},
			expected: types.LogCollection{Error: []string{
				`InstanceManager im-1 has unexpected label value longhorn.io/instance-manager-type="engine" (expected "replica")`,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			im := newLabeledInstanceManager(test.mutate)
			result := &Result{}
			checkInstanceManagerLabels(&im, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func newShareManager(name, image string, mutate func(*longhorn.ShareManager)) longhorn.ShareManager {
	sm := longhorn.ShareManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: lhtypes.GetShareManagerLabels(name, image),
		},
		Spec: longhorn.ShareManagerSpec{Image: image},
	}
	if mutate != nil {
		mutate(&sm)
	}
	return sm
}

func TestCheckShareManagerLabels(t *testing.T) {
	for _, test := range []struct {
		name     string
		mutate   func(*longhorn.ShareManager)
		expected types.LogCollection
	}{
		{
			name:     "all critical labels present",
			mutate:   nil,
			expected: types.LogCollection{},
		},
		{
			name: "missing critical label",
			mutate: func(sm *longhorn.ShareManager) {
				delete(sm.Labels, "longhorn.io/share-manager")
			},
			expected: types.LogCollection{Error: []string{
				`ShareManager sm-1 is missing critical label "longhorn.io/share-manager"`,
			}},
		},
		{
			name: "unexpected label value",
			mutate: func(sm *longhorn.ShareManager) {
				sm.Labels["longhorn.io/share-manager"] = "sm-other"
			},
			expected: types.LogCollection{Error: []string{
				`ShareManager sm-1 has unexpected label value longhorn.io/share-manager="sm-other" (expected "sm-1")`,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			sm := newShareManager("sm-1", "longhornio/longhorn-share-manager:v1.9.0", test.mutate)
			result := &Result{}
			checkShareManagerLabels(&sm, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateShareManagers(t *testing.T) {
	for _, test := range []struct {
		name          string
		shareManagers []longhorn.ShareManager
		expected      types.LogCollection
	}{
		{
			name:          "no share managers",
			shareManagers: nil,
			expected:      types.LogCollection{Info: []string{"No ShareManagers are found"}},
		},
		{
			name: "share managers with valid labels",
			shareManagers: []longhorn.ShareManager{
				newShareManager("sm-1", "longhornio/longhorn-share-manager:v1.9.0", nil),
				newShareManager("sm-2", "longhornio/longhorn-share-manager:v1.9.0", nil),
			},
			expected: types.LogCollection{Info: []string{"2 share managers are found"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateShareManagers(test.shareManagers, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func newBackingImageManager(name, nodeID, diskUUID string, mutate func(*longhorn.BackingImageManager)) longhorn.BackingImageManager {
	bim := longhorn.BackingImageManager{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: lhtypes.GetBackingImageManagerLabels(nodeID, diskUUID),
		},
		Spec: longhorn.BackingImageManagerSpec{NodeID: nodeID, DiskUUID: diskUUID},
	}
	if mutate != nil {
		mutate(&bim)
	}
	return bim
}

func TestCheckBackingImageManagerLabels(t *testing.T) {
	for _, test := range []struct {
		name     string
		mutate   func(*longhorn.BackingImageManager)
		expected types.LogCollection
	}{
		{
			name:     "all critical labels present",
			mutate:   nil,
			expected: types.LogCollection{},
		},
		{
			name: "missing critical label",
			mutate: func(bim *longhorn.BackingImageManager) {
				delete(bim.Labels, "longhorn.io/disk-uuid")
			},
			expected: types.LogCollection{Error: []string{
				`BackingImageManager bim-1 is missing critical label "longhorn.io/disk-uuid"`,
			}},
		},
		{
			name: "unexpected label value",
			mutate: func(bim *longhorn.BackingImageManager) {
				bim.Labels["longhorn.io/node"] = "node-other"
			},
			expected: types.LogCollection{Error: []string{
				`BackingImageManager bim-1 has unexpected label value longhorn.io/node="node-other" (expected "node-1")`,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			bim := newBackingImageManager("bim-1", "node-1", "disk-1", test.mutate)
			result := &Result{}
			checkBackingImageManagerLabels(&bim, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestEvaluateBackingImageManagers(t *testing.T) {
	for _, test := range []struct {
		name                 string
		backingImageManagers []longhorn.BackingImageManager
		expected             types.LogCollection
	}{
		{
			name:                 "no backing image managers",
			backingImageManagers: nil,
			expected:             types.LogCollection{Info: []string{"No BackingImageManagers are found"}},
		},
		{
			name: "backing image managers with valid labels",
			backingImageManagers: []longhorn.BackingImageManager{
				newBackingImageManager("bim-1", "node-1", "disk-1", nil),
			},
			expected: types.LogCollection{Info: []string{"1 backing image managers are found"}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &Result{}
			evaluateBackingImageManagers(test.backingImageManagers, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
	}
}

func TestCheckLonghornManagerPodLabels(t *testing.T) {
	newManagerPod := func(mutate func(*corev1.Pod)) corev1.Pod {
		pod := corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "longhorn-manager-abc",
				Labels: lhtypes.GetManagerLabels(),
			},
		}
		if mutate != nil {
			mutate(&pod)
		}
		return pod
	}

	for _, test := range []struct {
		name     string
		mutate   func(*corev1.Pod)
		expected types.LogCollection
	}{
		{
			name:     "all critical labels present",
			mutate:   nil,
			expected: types.LogCollection{},
		},
		{
			name: "missing critical label",
			mutate: func(pod *corev1.Pod) {
				delete(pod.Labels, "app")
			},
			expected: types.LogCollection{Error: []string{
				`Pod longhorn-manager-abc is missing critical label "app"`,
			}},
		},
		{
			name: "unexpected label value",
			mutate: func(pod *corev1.Pod) {
				pod.Labels["app"] = "other"
			},
			expected: types.LogCollection{Error: []string{
				`Pod longhorn-manager-abc has unexpected label value app="other" (expected "longhorn-manager")`,
			}},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			pod := newManagerPod(test.mutate)
			result := &Result{}
			checkLonghornManagerPodLabels(&pod, result)
			assert.Equal(t, test.expected, result.LogCollection)
		})
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

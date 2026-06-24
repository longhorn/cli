package upgrade

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	lhtypes "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func TestReplicasOnMultipleNodes(t *testing.T) {
	replica := func(name, nodeID string, deleting bool) lhtypes.Replica {
		r := lhtypes.Replica{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Spec:       lhtypes.ReplicaSpec{InstanceSpec: lhtypes.InstanceSpec{NodeID: nodeID}},
		}
		if deleting {
			now := metav1.Now()
			r.DeletionTimestamp = &now
		}
		return r
	}

	tests := []struct {
		name     string
		replicas []lhtypes.Replica
		want     bool
	}{
		{name: "single replica", replicas: []lhtypes.Replica{replica("r1", "node-1", false)}, want: false},
		{name: "co-located replicas", replicas: []lhtypes.Replica{replica("r1", "node-1", false), replica("r2", "node-1", false)}, want: false},
		{name: "replicas on distinct nodes", replicas: []lhtypes.Replica{replica("r1", "node-1", false), replica("r2", "node-2", false)}, want: true},
		{name: "deleting remote replica", replicas: []lhtypes.Replica{replica("r1", "node-1", false), replica("r2", "node-2", true)}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := replicasOnMultipleNodes(tt.replicas); got != tt.want {
				t.Fatalf("replicasOnMultipleNodes() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasSchedulableBlockDisk(t *testing.T) {
	newNode := func(diskType lhtypes.DiskType, status lhtypes.ConditionStatus) *lhtypes.Node {
		return &lhtypes.Node{
			Spec: lhtypes.NodeSpec{Disks: map[string]lhtypes.DiskSpec{"disk": {Type: diskType}}},
			Status: lhtypes.NodeStatus{DiskStatus: map[string]*lhtypes.DiskStatus{"disk": {
				Conditions: []lhtypes.Condition{{Type: lhtypes.DiskConditionTypeSchedulable, Status: status}},
			}}},
		}
	}

	if !hasSchedulableBlockDisk(newNode(lhtypes.DiskTypeBlock, lhtypes.ConditionStatusTrue)) {
		t.Fatal("expected schedulable block disk to be accepted")
	}
	if hasSchedulableBlockDisk(newNode(lhtypes.DiskTypeBlock, lhtypes.ConditionStatusFalse)) {
		t.Fatal("expected unschedulable block disk to be rejected")
	}
	if hasSchedulableBlockDisk(newNode(lhtypes.DiskTypeFilesystem, lhtypes.ConditionStatusTrue)) {
		t.Fatal("expected filesystem disk to be rejected")
	}
}

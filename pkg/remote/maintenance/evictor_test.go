package maintenance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	longhorn "github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func newTestNode() *longhorn.Node {
	return &longhorn.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "worker-1"},
		Spec: longhorn.NodeSpec{
			Name:            "worker-1",
			AllowScheduling: true,
			Disks: map[string]longhorn.DiskSpec{
				"default-disk-abc": {
					Path:            "/var/lib/longhorn",
					AllowScheduling: true,
				},
			},
		},
		Status: longhorn.NodeStatus{
			DiskStatus: map[string]*longhorn.DiskStatus{
				"default-disk-abc": {
					DiskUUID: "11111111-2222-3333-4444-555555555555",
					ScheduledReplica: map[string]int64{
						"replica-a": 1073741824,
						"replica-b": 2147483648,
					},
				},
				"default-disk-def": {
					DiskUUID:         "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
					ScheduledReplica: map[string]int64{},
				},
			},
		},
	}
}

func TestFindDiskNameByUUID(t *testing.T) {
	node := newTestNode()

	diskName, err := findDiskNameByUUID(node, "11111111-2222-3333-4444-555555555555")
	assert.NoError(t, err)
	assert.Equal(t, "default-disk-abc", diskName)

	diskName, err = findDiskNameByUUID(node, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	assert.NoError(t, err)
	assert.Equal(t, "default-disk-def", diskName)

	_, err = findDiskNameByUUID(node, "00000000-0000-0000-0000-000000000000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_, err = findDiskNameByUUID(&longhorn.Node{}, "11111111-2222-3333-4444-555555555555")
	assert.Error(t, err)
}

func TestCountScheduledReplicas(t *testing.T) {
	node := newTestNode()

	assert.Equal(t, 2, countScheduledReplicas(node, ""))
	assert.Equal(t, 2, countScheduledReplicas(node, "default-disk-abc"))
	assert.Equal(t, 0, countScheduledReplicas(node, "default-disk-def"))
	assert.Equal(t, 0, countScheduledReplicas(node, "nonexistent-disk"))
	assert.Equal(t, 0, countScheduledReplicas(&longhorn.Node{}, ""))
}

func TestDrainCommand(t *testing.T) {
	assert.Equal(t,
		"kubectl drain worker-1 --ignore-daemonsets",
		drainCommand("worker-1"))
}

func TestValidate(t *testing.T) {
	evictor := &Evictor{}
	assert.Error(t, evictor.Validate())

	evictor.NodeID = "worker-1"
	assert.NoError(t, evictor.Validate())
}

func TestValidateDiskEviction(t *testing.T) {
	evictor := &Evictor{}
	assert.Error(t, evictor.ValidateDiskEviction())

	evictor.NodeID = "worker-1"
	assert.Error(t, evictor.ValidateDiskEviction())

	evictor.DiskUUID = "11111111-2222-3333-4444-555555555555"
	assert.NoError(t, evictor.ValidateDiskEviction())
}

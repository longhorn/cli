package types

// VolumeCollection represents a collection of volumes.
type VolumeCollection struct {
	Volumes map[string][]*VolumeInfo `json:"volumes" yaml:"volumes"`
}

// ReplicaInfo holds information about a replica.
type VolumeInfo struct {
	Replicas []*ReplicaInfo `json:"replicas,omitempty" yaml:"replicas,omitempty"`
}

package types

import (
	lhmgrutil "github.com/longhorn/longhorn-manager/util"
)

// ReplicaCollection represents a collection of replicas.
type ReplicaCollection struct {
	Replicas map[string][]*ReplicaInfo `json:"replicas" yaml:"replicas"`
}

// ReplicaInfo holds information about a replica.
type ReplicaInfo struct {
	Node              string                `json:"node,omitempty" yaml:"node,omitempty"`
	Directory         string                `json:"directory,omitempty" yaml:"directory,omitempty"`
	IsInUse           *bool                 `json:"isInUse,omitempty" yaml:"isInUse,omitempty"`
	VolumeName        string                `json:"volumeName,omitempty" yaml:"volumeName,omitempty"`
	Metadata          *lhmgrutil.VolumeMeta `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	ExportedDirectory string                `json:"exportedDirectory,omitempty" yaml:"exportedDirectory,omitempty"`

	Error string `json:"error,omitempty" yaml:"error,omitempty"`
	Warn  string `json:"warn,omitempty" yaml:"warn,omitempty"`
}

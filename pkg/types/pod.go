package types

// PodCollection represents a collection of pods.
type PodCollections struct {
	Pods map[string]*PodInfo `json:"pods" yaml:"pods"`
}

// PodInfo holds information about a pod.
type PodInfo struct {
	Node string `json:"node,omitempty" yaml:"node,omitempty"`
	Log  string `json:"log,omitempty" yaml:"log,omitempty"`
}

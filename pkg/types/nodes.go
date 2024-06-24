package types

// NodeCollection represents a collection of nodes.
type NodeCollection struct {
	Log *LogCollection `json:"log,omitempty" yaml:"log,omitempty"`
}

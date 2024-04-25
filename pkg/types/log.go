package types

// LogCollection holds error and warn logs.
type LogCollection struct {
	Error []string `json:"error,omitempty" yaml:"error,omitempty"`
	Info  []string `json:"info,omitempty" yaml:"info,omitempty"`
	Warn  []string `json:"warn,omitempty" yaml:"warn,omitempty"`
}

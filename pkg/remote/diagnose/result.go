package diagnose

import (
	"fmt"

	"github.com/longhorn/cli/pkg/types"
)

// Status is the outcome of a check.
type Status string

const (
	StatusPass Status = "pass" // No issue is found.
	StatusWarn Status = "warn" // Issues are found, but they do not prevent Longhorn from working.
	StatusFail Status = "fail" // Longhorn is not healthy.
)

// Result holds the outcome of a check, along with the messages explaining it.
type Result struct {
	Name   string `json:"name" yaml:"name"`
	Status Status `json:"status" yaml:"status"`

	types.LogCollection `json:",inline" yaml:",inline"`
}

func (r *Result) infof(format string, args ...any) {
	r.Info = append(r.Info, fmt.Sprintf(format, args...))
}

// warnf records an issue that does not prevent Longhorn from working.
func (r *Result) warnf(format string, args ...any) {
	r.Warn = append(r.Warn, fmt.Sprintf(format, args...))
}

// errorf records an issue that makes the check fail.
func (r *Result) errorf(format string, args ...any) {
	r.Error = append(r.Error, fmt.Sprintf(format, args...))
}

// updateStatus sets the status according to the most severe recorded message.
func (r *Result) updateStatus() {
	switch {
	case len(r.Error) > 0:
		r.Status = StatusFail
	case len(r.Warn) > 0:
		r.Status = StatusWarn
	default:
		r.Status = StatusPass
	}
}

// Results holds the results of all checks.
type Results []*Result

// Failed returns the names of the failed checks.
func (results Results) Failed() []string {
	failed := []string{}
	for _, result := range results {
		if result.Status == StatusFail {
			failed = append(failed, result.Name)
		}
	}
	return failed
}

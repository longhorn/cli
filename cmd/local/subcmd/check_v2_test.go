package subcmd

import (
	"errors"
	"testing"
)

type failedPreflightReport struct {
	runErr, outputErr error
	wrote             bool
}

func (report *failedPreflightReport) Run() error { return report.runErr }
func (report *failedPreflightReport) Output() error {
	report.wrote = true
	return report.outputErr
}

func TestV2PreflightFailureStillOutputsReport(t *testing.T) {
	qualificationErr, outputErr := errors.New("host qualification failed"), errors.New("report write failed")
	for _, writeErr := range []error{nil, outputErr} {
		report := &failedPreflightReport{runErr: qualificationErr, outputErr: writeErr}
		err := runAndOutputV2Preflight(report)
		if !report.wrote || !errors.Is(err, qualificationErr) || (writeErr != nil && !errors.Is(err, writeErr)) {
			t.Fatalf("missing report or failure: wrote=%v error=%v", report.wrote, err)
		}
	}
}

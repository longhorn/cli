package subcmd

import "errors"

type preflightReportRunner interface {
	Run() error
	Output() error
}

// Write the complete report before returning the qualification verdict. In the
// remote transport phase the checker itself suppresses only that final verdict.
func runAndOutputV2Preflight(checker preflightReportRunner) error {
	runErr := checker.Run()
	return errors.Join(runErr, checker.Output())
}

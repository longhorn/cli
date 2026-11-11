//go:build !linux

package preflight

import "fmt"

func qualifyV2AttemptFilesystem(string) error {
	return fmt.Errorf("V2 installer replay protection requires a qualified Linux host")
}

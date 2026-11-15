//go:build linux

package preflight

import "testing"

func TestV2AttemptStoreRejectsVirtualFilesystems(t *testing.T) {
	if err := qualifyV2AttemptFilesystem("/proc/longhorn-cli/nonexistent-attempts"); err == nil {
		t.Fatal("virtual attempt store incorrectly promised durable replay protection")
	}
}

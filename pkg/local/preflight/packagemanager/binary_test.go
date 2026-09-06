package packagemanager

import (
	"testing"
)

func TestBinaryPackageManager_PackageToBinaryMapping(t *testing.T) {
	expectedMappings := map[string]string{
		"nfs-client":    "mount.nfs4",
		"open-iscsi":    "iscsiadm",
		"cryptsetup":    "cryptsetup",
		"device-mapper": "dmsetup",
	}

	for pkg, expectedBinary := range expectedMappings {
		binary, ok := packageToBinaryMap[pkg]
		if !ok {
			t.Errorf("expected mapping for package %q", pkg)
		}
		if binary != expectedBinary {
			t.Errorf("package %q: expected binary %q, got %q", pkg, expectedBinary, binary)
		}
	}
}

func TestBinaryPackageManager_UnsupportedOperations(t *testing.T) {
	pm := NewBinaryPackageManager(nil)

	if _, err := pm.InstallPackage("nfs-client"); err == nil {
		t.Error("expected error on InstallPackage, got nil")
	}

	if _, err := pm.UninstallPackage("nfs-client"); err == nil {
		t.Error("expected error on UninstallPackage, got nil")
	}
}

package utils

import (
	"testing"

	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
)

func TestParseOSreleaseFile(t *testing.T) {
	for _, test := range []struct {
		input  []string
		output string
	}{
		{
			input:  []string{"ID=\"my-os\"", "ID_LIKE=\"rhel centos fedora\""},
			output: "rhel",
		},
		{
			input:  []string{"ID=\"my-os\"", "ID_LIKE=\"rhel\""},
			output: "rhel",
		},
		{
			input:  []string{"ID=\"my-os\"", "ID_LIKE=\"  rhel  \""},
			output: "rhel",
		},
		{
			input:  []string{"ID=\"my-os\""},
			output: "my-os",
		},
		{
			input:  []string{""},
			output: "",
		},
	} {
		result, _ := parseOSreleaseFile(test.input)
		if result != test.output {
			t.Errorf("expected: %s, got: %s", test.output, result)
		}
	}
}

func TestGetPackageManagerType(t *testing.T) {
	tests := []struct {
		name       string
		osRelease  string
		wantType   pkgmgr.PackageManagerType
		shouldFail bool
	}{
		{
			name:       "SUSE Linux Enterprise Server",
			osRelease:  "sles",
			wantType:   pkgmgr.PackageManagerZypper,
			shouldFail: false,
		},
		{
			name:       "openSUSE",
			osRelease:  "opensuse",
			wantType:   pkgmgr.PackageManagerZypper,
			shouldFail: false,
		},
		{
			name:       "SUSE Micro",
			osRelease:  "sl-micro",
			wantType:   pkgmgr.PackageManagerTransactionalUpdate,
			shouldFail: false,
		},
		{
			name:       "Ubuntu",
			osRelease:  "ubuntu",
			wantType:   pkgmgr.PackageManagerApt,
			shouldFail: false,
		},
		{
			name:       "Debian",
			osRelease:  "debian",
			wantType:   pkgmgr.PackageManagerApt,
			shouldFail: false,
		},
		{
			name:       "RHEL",
			osRelease:  "rhel",
			wantType:   pkgmgr.PackageManagerYum,
			shouldFail: false,
		},
		{
			name:       "CentOS",
			osRelease:  "centos",
			wantType:   pkgmgr.PackageManagerYum,
			shouldFail: false,
		},
		{
			name:       "Fedora",
			osRelease:  "fedora",
			wantType:   pkgmgr.PackageManagerYum,
			shouldFail: false,
		},
		{
			name:       "Amazon Linux",
			osRelease:  "amzn",
			wantType:   pkgmgr.PackageManagerYum,
			shouldFail: false,
		},
		{
			name:       "Arch Linux",
			osRelease:  "arch",
			wantType:   pkgmgr.PackageManagerPacman,
			shouldFail: false,
		},
		{
			name:       "Oracle Linux",
			osRelease:  "ol",
			wantType:   pkgmgr.PackageManagerYum,
			shouldFail: false,
		},
		{
			name:       "Rocky Linux",
			osRelease:  "rocky",
			wantType:   pkgmgr.PackageManagerYum,
			shouldFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, err := GetPackageManagerType(tt.osRelease)
			if (err != nil) != tt.shouldFail {
				t.Errorf("GetPackageManagerType() error = %v, wantErr %v", err, tt.shouldFail)
				return
			}
			if gotType != tt.wantType {
				t.Errorf("GetPackageManagerType() = %v, want %v", gotType, tt.wantType)
			}
		})
	}
}

func TestIsCommandAvailable(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{
			name:    "Check ls command (should exists on most systems)",
			command: "ls",
		},
		{
			name:    "Check nonexistent command",
			command: "nonexistent_command_12345",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = isCommandAvailable(tt.command)
		})
	}
}

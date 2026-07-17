package utils

import (
	"testing"

	pkgmgr "github.com/longhorn/cli/pkg/local/preflight/packagemanager"
)

func TestParseOSreleaseFile(t *testing.T) {
	for _, test := range []struct {
		name   string
		input  []string
		output string
	}{
		{
			name:   "Non-SUSE system with ID_LIKE",
			input:  []string{"ID=\"my-os\"", "ID_LIKE=\"rhel centos fedora\""},
			output: "rhel",
		},
		{
			name:   "Non-SUSE system with single ID_LIKE",
			input:  []string{"ID=\"my-os\"", "ID_LIKE=\"rhel\""},
			output: "rhel",
		},
		{
			name:   "Non-SUSE system with whitespace",
			input:  []string{"ID=\"my-os\"", "ID_LIKE=\"  rhel  \""},
			output: "rhel",
		},
		{
			name:   "System with only ID",
			input:  []string{"ID=\"my-os\""},
			output: "my-os",
		},
		{
			name:   "SLE Micro 6.1 with sl-micro ID",
			input:  []string{"ID=\"sl-micro\"", "ID_LIKE=\"suse sle-micro opensuse-microos microos\"", "VARIANT_ID=\"SLE-Micro-Rancher\""},
			output: "sl-micro",
		},
		{
			name:   "SLE Micro 6.1 variant 2",
			input:  []string{"ID=\"sl-micro\"", "ID_LIKE=\"suse sl-micro\"", "VARIANT_ID=\"SLE-Micro\""},
			output: "sl-micro",
		},
		{
			name:   "SLE Micro 6.2 (official format from documentation)",
			input:  []string{"ID=\"sles\"", "ID_LIKE=\"suse opensuse\"", "VARIANT_ID=\"transactional\""},
			output: "sl-micro",
		},
		{
			name:   "SLE Micro 6.2 variant with SLE-Micro VARIANT_ID",
			input:  []string{"ID=\"sles\"", "ID_LIKE=\"suse opensuse\"", "VARIANT_ID=\"other variant\""},
			output: "suse",
		},
		{
			name:   "Regular SLES without micro variant",
			input:  []string{"ID=\"sles\"", "ID_LIKE=\"suse opensuse\"", "VARIANT_ID=\"\""},
			output: "suse",
		},
		{
			name:   "Regular SLES without VARIANT_ID field",
			input:  []string{"ID=\"sles\"", "ID_LIKE=\"suse\""},
			output: "suse",
		},
		{
			name:   "openSUSE Leap",
			input:  []string{"ID=\"opensuse-leap\"", "ID_LIKE=\"suse opensuse\""},
			output: "suse",
		},
		{
			name:   "Empty input",
			input:  []string{""},
			output: "",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, _ := parseOSreleaseFile(test.input)
			if result != test.output {
				t.Errorf("expected: %s, got: %s", test.output, result)
			}
		})
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
			name:       "OpenCloudOS",
			osRelease:  "opencloudos",
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

func TestIsCommandAvailableOnHost(t *testing.T) {
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
			_ = isCommandAvailableOnHost(tt.command)
		})
	}
}

package utils

import (
	"testing"
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

func TestGetKernelMajorVersion(t *testing.T) {
	for _, tc := range []struct {
		kernelVersion        string
		expectedMajorVersion int
	}{
		{
			kernelVersion:        "",
			expectedMajorVersion: -1,
		},
		{
			kernelVersion:        "six.zero.zero",
			expectedMajorVersion: -1,
		},
		{
			kernelVersion:        "a.4.5",
			expectedMajorVersion: -1,
		},
		{
			kernelVersion:        "5-beta.4.5",
			expectedMajorVersion: -1,
		},
		{
			kernelVersion:        "5.4.5",
			expectedMajorVersion: 5,
		},
		{
			kernelVersion:        "6.0.0-rc1",
			expectedMajorVersion: 6,
		},
		{
			kernelVersion:        "  2.1.5-generic  ",
			expectedMajorVersion: 2,
		},
	} {
		majorVersion, err := getKernelMajorVersion(tc.kernelVersion)
		if tc.expectedMajorVersion != majorVersion {
			t.Errorf("expected kernel major version: %d, got: %d (err: %v)", tc.expectedMajorVersion, majorVersion, err)
		}
	}
}

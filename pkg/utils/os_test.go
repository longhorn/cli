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

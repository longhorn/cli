package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildImageName(t *testing.T) {
	for testName, testCase := range map[string]struct {
		expected, image, globalRegistry string
	}{
		"empty image string":                         {"", "", ""},
		"no global registry provided":                {"my-registry.com/path/to/image:1.0", "my-registry.com/path/to/image:1.0", ""},
		"replace existing registry":                  {"new-registry.com/path/to/image:1.0", "my-registry.com/path/to/image:1.0", "new-registry.com"},
		"prepend registry to image without registry": {"new-registry.com/path/to/image:1.0", "path/to/image:1.0", "new-registry.com"},
	} {
		t.Run(testName, func(t *testing.T) {
			if got := BuildImageName(testCase.image, testCase.globalRegistry); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

func TestMarshalYAML(t *testing.T) {
	type sample struct {
		Name  string   `yaml:"name"`
		Items []string `yaml:"items"`
	}
	out, err := MarshalYAML([]sample{{Name: "first", Items: []string{"a", "b"}}})
	require.NoError(t, err)
	assert.Equal(t, "- name: first\n  items:\n  - a\n  - b\n", string(out))
}

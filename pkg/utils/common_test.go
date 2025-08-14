package utils

import "testing"

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

package utils

import "testing"

func TestConvertStringToTypeOrDefault(t *testing.T) {
	for testName, testCase := range map[string]struct {
		input        string
		defaultValue any
		expected     any
	}{
		"empty string returns default int":      {"", 2048, 2048},
		"valid int string":                      {"4096", 2048, 4096},
		"invalid int returns default":           {"bad", 2048, 2048},
		"empty string returns default bool":     {"", true, true},
		"valid bool string true":                {"true", false, true},
		"valid bool string false":               {"false", true, false},
		"invalid bool returns default":          {"bad", true, true},
		"invalid bool returns default (false)":  {"bad", false, false},
	} {
		t.Run(testName, func(t *testing.T) {
			var got any
			switch d := testCase.defaultValue.(type) {
			case int:
				got = ConvertStringToTypeOrDefault(testCase.input, d)
			case bool:
				got = ConvertStringToTypeOrDefault(testCase.input, d)
			}
			if got != testCase.expected {
				t.Errorf("expected %v, got %v", testCase.expected, got)
			}
		})
	}
}

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

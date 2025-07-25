package utils

import "testing"

func TestBuildImageName(t *testing.T) {
	for testName, testCase := range map[string]struct {
		expected, image, custReg, defReg string
	}{
		"empty image":              {"", "", "", ""},
		"image with no registry":   {"image:1.2.3", "image:1.2.3", "", ""},
		"use default registry":     {"default-registry.com/image:1.2.3", "image:1.2.3", "", "default-registry.com"},
		"use custom registry":      {"custom-registry.com/image:1.2.3", "image:1.2.3", "custom-registry.com", ""},
		"custom overrides default": {"custom-registry.com/image:1.2.3", "image:1.2.3", "custom-registry.com", "default-registry.com"},
	} {
		t.Run(testName, func(t *testing.T) {
			if got := BuildImageName(testCase.image, testCase.custReg, testCase.defReg); got != testCase.expected {
				t.Errorf("expected %q, got %q", testCase.expected, got)
			}
		})
	}
}

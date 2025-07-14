package utils

import "testing"

func TestImage(t *testing.T) {
	for _, tt := range []struct {
		expected, image, custReg, defReg string
	}{
		{"", "", "", ""},
		{"image:1.2.3", "image:1.2.3", "", ""},
		{"default-registry.com/image:1.2.3", "image:1.2.3", "", "default-registry.com"},
		{"custom-registry.com/image:1.2.3", "image:1.2.3", "custom-registry.com", ""},
		{"custom-registry.com/image:1.2.3", "image:1.2.3", "custom-registry.com", "default-registry.com"},
	} {
		if got := Image(tt.image, tt.custReg, tt.defReg); got != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, got)
		}
	}
}

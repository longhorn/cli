package utils

import (
	"path"
	"strings"
)

// BuildImageName returns an image name with the given global registry prepended.
// If no global registry is provided, the original image string is returned unchanged.
func BuildImageName(image, globalRegistry string) string {
	if globalRegistry == "" {
		return image
	}

	imagePathWithTag := getImagePathWithTag(image)
	return path.Join(globalRegistry, imagePathWithTag)
}

// getImagePathWithTag removes the registry domain from the image reference,
// preserving the path and tag. If the image does not have a registry,
// the full image reference is returned unchanged.
func getImagePathWithTag(image string) string {
	parts := strings.SplitN(image, "/", 2)
	if len(parts) < 2 {
		// no slash means it's just "imageName:tag" → no registry
		return image
	}
	firstSegment := parts[0]
	// Per Docker rule, first segment is a registry if it has '.' (domain), ':' (port), or is "localhost".
	// For simplicity, we only check for '.'
	if strings.Contains(firstSegment, ".") {
		return parts[1]
	}

	// first segment is not a registry → keep full image reference
	return image
}

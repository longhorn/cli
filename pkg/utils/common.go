package utils

import "path"

// BuildImageName returns the full image name, preferring the custom registry if provided.
func BuildImageName(imageName, customRegistry, defaultRegistry string) string {
	registry := defaultRegistry
	if customRegistry != "" {
		registry = customRegistry
	}
	return path.Join(registry, imageName)
}

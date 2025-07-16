package utils

import "path"

// Image returns the full image name, preferring the custom registry if provided.
func Image(imageName, customRegistry, defaultRegistry string) string {
	registry := defaultRegistry
	if customRegistry != "" {
		registry = customRegistry
	}
	return path.Join(registry, imageName)
}

// Package imageregistry points Obot's own images at a private registry that mirrors them.
package imageregistry

import (
	"strings"
)

const (
	// obotImagePrefix is where Obot publishes its images.
	obotImagePrefix = "ghcr.io/obot-platform/"
)

// Rewrite returns image pulled from registry instead of ghcr.io when it's one of Obot's images, keeping its path and tag
// or digest: with registry.example.com/mirror, ghcr.io/obot-platform/x:v1 becomes
// registry.example.com/mirror/obot-platform/x:v1.
// Other images, and every image when registry is empty, are returned as they are.
func Rewrite(image, registry string) string {
	registry = strings.TrimRight(registry, "/")
	if registry == "" || !strings.HasPrefix(image, obotImagePrefix) {
		return image
	}
	return registry + "/" + strings.TrimPrefix(image, "ghcr.io/")
}

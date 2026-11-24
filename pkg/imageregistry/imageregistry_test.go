package imageregistry

import (
	"testing"
)

func TestRewrite(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		registry string
		want     string
	}{
		{
			name:     "no registry",
			image:    "ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.28.0",
			registry: "",
			want:     "ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.28.0",
		},
		{
			name:     "obot image with tag",
			image:    "ghcr.io/obot-platform/mcp-images/stdio-wrapper:v0.28.0",
			registry: "registry.example.com",
			want:     "registry.example.com/obot-platform/mcp-images/stdio-wrapper:v0.28.0",
		},
		{
			name:     "obot image with digest and registry path",
			image:    "ghcr.io/obot-platform/obot-claude-runner@sha256:3fc6fde0409805b8878c364bc4f14da9dc7aff54cb2a07094a61a688f6183b72",
			registry: "registry.example.com:5000/mirror/",
			want:     "registry.example.com:5000/mirror/obot-platform/obot-claude-runner@sha256:3fc6fde0409805b8878c364bc4f14da9dc7aff54cb2a07094a61a688f6183b72",
		},
		{
			name:     "other ghcr.io image",
			image:    "ghcr.io/example/server:v1",
			registry: "registry.example.com",
			want:     "ghcr.io/example/server:v1",
		},
		{
			name:     "docker hub image",
			image:    "postgres:16",
			registry: "registry.example.com",
			want:     "postgres:16",
		},
		{
			name:     "already mirrored",
			image:    "registry.example.com/obot-platform/pii-filter:v0.5.6",
			registry: "registry.example.com",
			want:     "registry.example.com/obot-platform/pii-filter:v0.5.6",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Rewrite(tt.image, tt.registry); got != tt.want {
				t.Errorf("Rewrite() = %q, want %q", got, tt.want)
			}
		})
	}
}

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestListImages(t *testing.T) {
	catalog := t.TempDir()
	for name, contents := range map[string]string{
		"pii-filter.yaml":  "name: PII Filter\nruntime: containerized\ncontainerizedConfig:\n  image: ghcr.io/obot-platform/pii-filter:v0.5.6\n  port: 8080\n  path: /\n",
		"same-filter.yaml": "name: Same Filter\nruntime: containerized\ncontainerizedConfig:\n  image: ghcr.io/obot-platform/pii-filter:v0.5.6\n",
		"remote.yaml":      "name: Remote\nruntime: remote\nremoteConfig:\n  url: https://example.com/mcp\n",
		"README.md":        "# System catalog\n",
	} {
		if err := os.WriteFile(filepath.Join(catalog, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	catalogImages, err := systemCatalogImages(catalog)
	if err != nil {
		t.Fatal(err)
	}

	images := listImages("v0.29.0", catalogImages, []byte(`        - name: atelet
          image: ghcr.io/obot-platform/substrate/atelet:v0.2.1
`))

	for _, want := range []string{
		"ghcr.io/obot-platform/obot:v0.29.0",
		"ghcr.io/obot-platform/substrate/atelet:v0.2.1",
		"ghcr.io/obot-platform/obot-enterprise:v0.29.0",
		"ghcr.io/obot-platform/pii-filter:v0.5.6",
	} {
		if !slices.Contains(images, want) {
			t.Errorf("images = %v, want it to include %s", images, want)
		}
	}
	if !slices.ContainsFunc(images, func(image string) bool {
		return strings.HasPrefix(image, "ghcr.io/obot-platform/mcp-images/stdio-wrapper:")
	}) {
		t.Errorf("images = %v, want it to include the default MCP base image", images)
	}
	if !slices.IsSorted(images) || len(slices.Compact(slices.Clone(images))) != len(images) {
		t.Errorf("images = %v, want them sorted without duplicates", images)
	}
}

func TestListImagesWithoutTagOrCatalog(t *testing.T) {
	images := listImages("", nil, nil)
	for _, image := range images {
		if image == "" {
			t.Fatal("listed an empty image")
		}
	}
	if slices.ContainsFunc(images, func(image string) bool { return image == "ghcr.io/obot-platform/obot:" }) {
		t.Errorf("images = %v, want no server images without a tag", images)
	}
}

func TestSystemCatalogImagesRefusesInvalidCatalog(t *testing.T) {
	catalog := t.TempDir()
	if err := os.WriteFile(filepath.Join(catalog, "broken.yaml"), []byte("name: [unterminated\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := systemCatalogImages(catalog); err == nil {
		t.Fatal("systemCatalogImages() succeeded, want an error")
	}
}

func TestChartImages(t *testing.T) {
	rendered := `apiVersion: v1
kind: ConfigMap
data:
  OBOT_SERVER_SUBSTRATE_WORKER_IMAGE: "ghcr.io/obot-platform/substrate/ateom-gvisor:v0.2.1"
  OBOT_SERVER_MCPBASE_IMAGE: ""
  config.yaml: |
    pauseImage: "registry.k8s.io/pause:3.10.2"
    workerImage:
---
kind: Deployment
spec:
  template:
    spec:
      containers:
        - image: "ghcr.io/obot-platform/obot:v0.29.0"
          imagePullPolicy: IfNotPresent
        - name: envoy
          image: envoyproxy/envoy:v1.39-latest
`
	want := []string{
		"ghcr.io/obot-platform/substrate/ateom-gvisor:v0.2.1",
		"registry.k8s.io/pause:3.10.2",
		"ghcr.io/obot-platform/obot:v0.29.0",
		"envoyproxy/envoy:v1.39-latest",
	}
	if got := chartImages([]byte(rendered)); !slices.Equal(got, want) {
		t.Errorf("chartImages() = %v, want %v", got, want)
	}
}

func TestRenderChartListsSubstrateImages(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm isn't installed")
	}

	rendered, err := renderChart("../../chart", "v0.29.0")
	if err != nil {
		t.Fatal(err)
	}
	images := chartImages(rendered)
	for _, want := range []string{
		"ghcr.io/obot-platform/obot:v0.29.0",
		"ghcr.io/obot-platform/substrate/atelet:",
		"ghcr.io/obot-platform/substrate/ateom-gvisor:",
	} {
		if !slices.ContainsFunc(images, func(image string) bool { return strings.HasPrefix(image, want) }) {
			t.Errorf("chart images = %v, want one starting with %s", images, want)
		}
	}
}

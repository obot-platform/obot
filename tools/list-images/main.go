// Command list-images writes the images an Obot release pulls, one per line, for mirroring into a private registry. It
// lists the Obot server images for -tag, the default of every image setting, the images in the system MCP catalog
// checked out at -system-catalog, and the images the Helm chart in -chart deploys with Substrate on, which it renders
// with helm. The release build runs it and publishes the list as images.txt.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/mcpcatalog"
	"github.com/obot-platform/obot/pkg/services"
)

var (
	// chartImageLine matches a manifest line that sets an image: a key named image, or ending in Image or _IMAGE, such as
	// a container's image, a subchart's pauseImage, or OBOT_SERVER_SUBSTRATE_WORKER_IMAGE in Obot's configuration.
	chartImageLine = regexp.MustCompile(`^\s*(?:-\s+)?[A-Za-z0-9_]*(?:image|Image|_IMAGE):\s+"?([^"\s]+)"?\s*$`)
)

func main() {
	tag := flag.String("tag", "", "the release tag, such as v0.29.0, to list the Obot server images for")
	chart := flag.String("chart", "chart", "the Helm chart to list the images of, rendered with helm; empty skips it")
	systemCatalog := flag.String("system-catalog", "", "a checkout of the system MCP catalog to list the images of; empty skips it")
	out := flag.String("out", "", "the file to write the list to (default stdout)")
	flag.Parse()

	var rendered []byte
	if *chart != "" {
		var err error
		if rendered, err = renderChart(*chart, *tag); err != nil {
			fmt.Fprintf(os.Stderr, "failed to render the Helm chart: %v\n", err)
			os.Exit(1)
		}
	}

	var catalogImages []string
	if *systemCatalog != "" {
		var err error
		if catalogImages, err = systemCatalogImages(*systemCatalog); err != nil {
			fmt.Fprintf(os.Stderr, "failed to read the system MCP catalog: %v\n", err)
			os.Exit(1)
		}
	}

	images := listImages(*tag, catalogImages, rendered)
	list := strings.Join(images, "\n") + "\n"

	if *out == "" {
		fmt.Print(list)
		return
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *out, err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, []byte(list), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *out, err)
		os.Exit(1)
	}
	fmt.Printf("listed %d images in %s\n", len(images), *out)
}

// listImages returns the sorted images a release pulls:
// - The Obot and Obot Enterprise server images for tag, when it's set
// - The default of every image setting; see services.ImageSettings
// - catalogImages, the system MCP catalog's images; see systemCatalogImages
// - The images in chart, the Helm chart's rendered manifests; see chartImages
func listImages(tag string, catalogImages []string, chart []byte) []string {
	images := slices.Clone(catalogImages)
	if tag != "" {
		images = append(images, "ghcr.io/obot-platform/obot:"+tag, "ghcr.io/obot-platform/obot-enterprise:"+tag)
	}

	for _, setting := range services.ImageSettings() {
		if image := setting.Tag.Get("default"); image != "" {
			images = append(images, image)
		}
	}

	images = append(images, chartImages(chart)...)

	slices.Sort(images)
	return slices.Compact(images)
}

// systemCatalogImages returns the containerized images of the entries in the system MCP catalog checked out at dir.
// Unlike a catalog sync, which skips invalid files, it fails on any, so the list never misses part of the catalog.
func systemCatalogImages(dir string) ([]string, error) {
	files, err := mcpcatalog.WalkCatalogFiles(dir)
	if err != nil {
		return nil, err
	}
	var images []string
	for path, err := range files {
		if err != nil {
			return nil, err
		}
		entries, _, err := mcpcatalog.DecodeCatalogFile[types.SystemMCPServerCatalogEntryManifest](path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		for _, entry := range entries {
			if entry.ContainerizedConfig != nil && entry.ContainerizedConfig.Image != "" {
				images = append(images, entry.ContainerizedConfig.Image)
			}
		}
	}
	return images, nil
}

// chartImages returns the images set in rendered manifests. It scans lines rather than decoding them, so it also finds
// images in configuration files that the manifests embed as strings.
func chartImages(rendered []byte) []string {
	var images []string
	for line := range strings.Lines(string(rendered)) {
		if match := chartImageLine.FindStringSubmatch(line); match != nil {
			images = append(images, match[1])
		}
	}
	return images
}

// renderChart renders the Helm chart at chart with Substrate on, so its images are included, and with tag as Obot's
// image tag when it's set. The other values are placeholders the chart requires.
func renderChart(chart, tag string) ([]byte, error) {
	args := []string{
		"template", "obot", chart,
		"--set", "dev.useEmbeddedDb=true",
		"--set", "substrate.enabled=true",
		"--set", "config.OBOT_SERVER_SUBSTRATE_SNAPSHOT_LOCATION=s3://images",
	}
	if tag != "" {
		args = append(args, "--set", "image.tag="+tag)
	}

	var stderr strings.Builder
	cmd := exec.Command("helm", args...)
	cmd.Stderr = &stderr
	rendered, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return rendered, nil
}

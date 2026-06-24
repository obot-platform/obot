package mcpcatalog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sigs.k8s.io/yaml"
)

const (
	obotCatalogMetadataFilename = ".obotcatalog"
	catalogReferenceSeparator   = "|"
)

type mcpCatalogMetadata struct {
	ID string `json:"id,omitempty"`
}

func readGitCatalogMetadata(repoRoot string) (string, error) {
	yamlFilename := obotCatalogMetadataFilename + ".yaml"
	metadataPath := filepath.Join(repoRoot, yamlFilename)
	if _, err := os.Stat(metadataPath); err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to stat %s: %w", yamlFilename, err)
		}

		ymlFilename := obotCatalogMetadataFilename + ".yml"
		ymlPath := filepath.Join(repoRoot, ymlFilename)
		if _, err := os.Stat(ymlPath); err != nil {
			if os.IsNotExist(err) {
				// file is optional for now
				return "", nil
			}
			return "", fmt.Errorf("failed to stat %s: %w", ymlFilename, err)
		}
		metadataPath = ymlPath
	}

	contents, err := os.ReadFile(metadataPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", filepath.Base(metadataPath), err)
	}

	var metadata mcpCatalogMetadata
	if err := yaml.Unmarshal(contents, &metadata); err != nil {
		return "", fmt.Errorf("failed to decode %s: %w", filepath.Base(metadataPath), err)
	}

	id := strings.TrimSpace(metadata.ID)
	if id == "" {
		return "", fmt.Errorf("%s id is required", filepath.Base(metadataPath))
	}
	if strings.Contains(id, catalogReferenceSeparator) {
		return "", fmt.Errorf("%s id cannot contain %s", filepath.Base(metadataPath), catalogReferenceSeparator)
	}

	return id, nil
}

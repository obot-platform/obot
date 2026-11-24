package mcpcatalog

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
)

// OpenAPIImporter imports a source using the instance's network policy. It must
// not receive catalog Git credentials or credentials for the described API.
type OpenAPIImporter interface {
	Import(context.Context, types.OpenAPIRuntimeConfig) (*openapi.Result, error)
}

// PrepareSourceEntry imports generated data needed to validate a catalog source
// entry. It saves nothing and must run before ValidateManifest, both during sync
// and when checking proposed files. Source files cannot supply stored snapshots.
func PrepareSourceEntry(ctx context.Context, entry *types.MCPServerCatalogEntryManifest, importer OpenAPIImporter) error {
	if entry.Runtime != types.RuntimeOpenAPI {
		return nil
	}
	if entry.OpenAPIConfig == nil {
		return fmt.Errorf("openAPIConfig is required")
	}
	config := *entry.OpenAPIConfig
	config.Source.URL = strings.TrimSpace(config.Source.URL)
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	if err := openapi.ValidateInput(config); err != nil {
		return err
	}
	if importer == nil {
		return fmt.Errorf("OpenAPI importer is not configured")
	}
	result, err := importer.Import(ctx, config)
	if err != nil {
		return fmt.Errorf("failed to import OpenAPI schema: %w", err)
	}
	// Destination status is advisory during import. ValidateManifest checks it
	// against the configured network policy before the entry can be accepted.
	config.Schema = &types.OpenAPISchema{Raw: result.Schema}
	entry.OpenAPIConfig = &config
	return nil
}

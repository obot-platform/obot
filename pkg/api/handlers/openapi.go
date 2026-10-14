package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/openapi"
)

// ImportOpenAPI previews a schema for the catalog creation form without
// creating a draft entry. Uploaded files are sent as source.content (JSON/YAML).
func (h *MCPCatalogHandler) ImportOpenAPI(req api.Context) error {
	var config types.OpenAPIRuntimeConfig
	if err := req.Read(&config); err != nil {
		if syntaxErr, ok := errors.AsType[*json.SyntaxError](err); ok {
			return types.NewErrBadRequest("invalid OpenAPI import request JSON at byte %d: %v", syntaxErr.Offset, syntaxErr)
		}
		if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
			return types.NewErrBadRequest("invalid OpenAPI import request field %q: expected %s at byte %d", typeErr.Field, typeErr.Type, typeErr.Offset)
		}
		if errors.Is(err, io.EOF) {
			return types.NewErrBadRequest("OpenAPI import request body is empty")
		}
		return err
	}
	if err := openapi.ValidateInput(config); err != nil {
		return types.NewErrBadRequest("invalid OpenAPI import request: %v", err)
	}
	// The importer uses its own bounded, SSRF-protected client. It never
	// receives request headers, user credentials, or a caller-provided client.
	result, err := h.openAPIImporter.Import(req.Context(), config)
	if err != nil {
		return types.NewErrBadRequest("failed to import OpenAPI schema: %v", err)
	}
	return req.Write(types.OpenAPIImportResponse{
		BaseURL:               result.BaseURL,
		SuggestedHeaders:      result.SuggestedHeaders,
		SuggestedMetadata:     result.SuggestedMetadata,
		DestinationValidation: result.DestinationValidation,
	})
}

// prepareOpenAPIEntry imports the source on every save. Existing deployments
// keep their snapshots until explicitly upgraded.
func (h *MCPCatalogHandler) prepareOpenAPIEntry(ctx context.Context, manifest *types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeOpenAPI || manifest.OpenAPIConfig == nil {
		return nil // The runtime validator reports missing configuration.
	}
	config := *manifest.OpenAPIConfig
	// Ignore a client-supplied snapshot; it is always regenerated from Source.
	config.Schema = nil
	if err := openapi.ValidateInput(config); err != nil {
		return err
	}

	result, err := h.openAPIImporter.Import(ctx, config)
	if err != nil {
		return err
	}
	// Import reports destination failures for previews. CreateEntry and
	// UpdateEntry still validate the manifest before persisting this snapshot.
	config.Schema = &types.OpenAPISchema{Raw: result.Schema}
	manifest.OpenAPIConfig = &config
	return nil
}

// withoutOpenAPISchema omits the stored snapshot from API responses. Clients
// never use it, and it can be up to 1 MiB per entry or server.
func withoutOpenAPISchema(config *types.OpenAPIRuntimeConfig) *types.OpenAPIRuntimeConfig {
	if config == nil || config.Schema == nil {
		return config
	}
	c := *config
	c.Schema = nil
	return &c
}

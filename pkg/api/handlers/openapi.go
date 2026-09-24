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

// ImportOpenAPI returns a snapshot for the catalog creation form without
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
		Schema:            &types.OpenAPISchema{Raw: result.Schema},
		BaseURL:           result.BaseURL,
		SuggestedHeaders:  append([]types.MCPConfig{}, result.SuggestedHeaders...),
		SuggestedMetadata: result.SuggestedMetadata,
	})
}

// prepareOpenAPIEntry imports the source on every save. Existing deployments
// keep their snapshots until explicitly upgraded.
func (h *MCPCatalogHandler) prepareOpenAPIEntry(ctx context.Context, manifest *types.MCPServerCatalogEntryManifest) error {
	if manifest.Runtime != types.RuntimeOpenAPI || manifest.OpenAPIConfig == nil {
		return nil // The runtime validator reports missing configuration.
	}
	config := *manifest.OpenAPIConfig
	if err := openapi.ValidateInput(config); err != nil {
		return err
	}

	result, err := h.openAPIImporter.Import(ctx, config)
	if err != nil {
		return err
	}
	config.Schema = &types.OpenAPISchema{Raw: result.Schema}
	manifest.OpenAPIConfig = &config
	return nil
}

package mcp

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
)

func (OpenAPIValidator) ValidateConfig(_ context.Context, manifest types.MCPServerManifest) error {
	return validateOpenAPIConfig(manifest.OpenAPIConfig, manifest.Config)
}

func (OpenAPIValidator) ValidateCatalogConfig(_ context.Context, manifest types.MCPServerCatalogEntryManifest) error {
	return validateOpenAPIConfig(manifest.OpenAPIConfig, manifest.Config)
}

func (OpenAPIValidator) ValidateSystemConfig(_ context.Context, _ types.SystemMCPServerManifest) error {
	return types.RuntimeValidationError{
		Runtime: types.RuntimeOpenAPI,
		Field:   "runtime",
		Message: "OpenAPI runtime is not supported for system MCP servers",
	}
}

func validateOpenAPIConfig(config *types.OpenAPIRuntimeConfig, headers []types.MCPConfig) error {
	if config == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeOpenAPI,
			Field:   "openAPIConfig",
			Message: "OpenAPI configuration is required",
		}
	}
	if err := validateEgressDomains(types.RuntimeOpenAPI, config.EgressDomains, config.DenyAllEgress); err != nil {
		return err
	}
	if _, err := openapi.SnapshotEnvironment(*config, headers); err != nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeOpenAPI,
			Field:   "openAPIConfig",
			Message: err.Error(),
		}
	}
	return nil
}

func configureOpenAPIRuntime(server *ServerConfig, config *types.OpenAPIRuntimeConfig, headers []types.MCPConfig, credentials map[string]string) ([]string, error) {
	if config == nil {
		return nil, fmt.Errorf("openapi runtime requires OpenAPI config")
	}

	env, err := openapi.SnapshotEnvironment(*config, headers)
	if err != nil {
		return nil, err
	}

	server.ContainerPort = 8080
	server.ContainerPath = "/mcp"
	// The wrapper keeps /healthz healthy after conversion failures; /readyz
	// only succeeds when the OpenAPI tools are ready to serve requests.
	server.HealthzPath = "/readyz"
	server.Env = env
	server.Files = []File{{
		EnvKey:  "OPENAPI_SPEC_FILE",
		Data:    string(config.Schema.Raw),
		Dynamic: false,
	}}

	return configureHeaders(server, headers, credentials), nil
}

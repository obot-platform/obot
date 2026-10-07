package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/openapi"
	"github.com/obot-platform/obot/pkg/safehttp"
)

func (v OpenAPIValidator) ValidateConfig(ctx context.Context, manifest types.MCPServerManifest) error {
	return validateOpenAPIConfig(ctx, manifest.OpenAPIConfig, manifest.Config, v)
}

func (v OpenAPIValidator) ValidateCatalogConfig(ctx context.Context, manifest types.MCPServerCatalogEntryManifest) error {
	return validateOpenAPIConfig(ctx, manifest.OpenAPIConfig, manifest.Config, v)
}

func (OpenAPIValidator) ValidateSystemConfig(_ context.Context, _ types.SystemMCPServerManifest) error {
	return types.RuntimeValidationError{
		Runtime: types.RuntimeOpenAPI,
		Field:   "runtime",
		Message: "OpenAPI runtime is not supported for system MCP servers",
	}
}

func validateOpenAPIConfig(ctx context.Context, config *types.OpenAPIRuntimeConfig, headers []types.MCPConfig, validator OpenAPIValidator) error {
	if config == nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeOpenAPI,
			Field:   "openAPIConfig",
			Message: "OpenAPI configuration is required",
		}
	}
	if err := openapi.ValidateSource(config.Source); err != nil {
		return types.RuntimeValidationError{
			Runtime: types.RuntimeOpenAPI,
			Field:   "openAPIConfig.source",
			Message: err.Error(),
		}
	}
	if err := validateEgressDomains(types.RuntimeOpenAPI, config.EgressDomains, config.DenyAllEgress); err != nil {
		return err
	}
	if _, err := openapi.ValidateSnapshotEnvironment(ctx, *config, headers, openAPINetworkOptions(validator.RemoteMCPURLValidationConfig), validator.DevMode); err != nil {
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

func openAPINetworkOptions(config RemoteMCPURLValidationConfig) safehttp.Options {
	return safehttp.Options{
		BlockLoopback:  !config.AllowLocalhostMCP,
		BlockPrivateIP: !config.AllowPrivateIPMCP,
		BlockLinkLocal: !config.AllowLinkLocalMCP,
	}
}

func (sm *SessionManager) validateOpenAPIDestination(ctx context.Context, server ServerConfig) error {
	if server.Runtime != types.RuntimeOpenAPI {
		return nil
	}
	for _, env := range server.Env {
		if baseURL, ok := strings.CutPrefix(env, "OPENAPI_BASE_URL="); ok {
			return openapi.ValidateDestination(ctx, baseURL, openAPINetworkOptions(sm.remoteURLValidationConfig), sm.devMode)
		}
	}
	return fmt.Errorf("OpenAPI deployment requires an API destination")
}

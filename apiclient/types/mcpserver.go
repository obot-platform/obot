package types

import (
	"fmt"
	"net/url"
)

// Runtime represents the execution runtime type for MCP servers
type Runtime string

// Runtime constants for different MCP server execution environments
const (
	RuntimeUVX           Runtime = "uvx"
	RuntimeNPX           Runtime = "npx"
	RuntimeContainerized Runtime = "containerized"
	RuntimeRemote        Runtime = "remote"
)

// UVXRuntimeConfig represents configuration for UVX runtime (Python packages via uvx)
type UVXRuntimeConfig struct {
	Package string   `json:"package"`        // Required: Python package name
	Command string   `json:"command"`        // Optional: Specific command to run inside of the package. If empty, the package name will be treated as the command.
	Args    []string `json:"args,omitempty"` // Optional: Additional arguments
}

// NPXRuntimeConfig represents configuration for NPX runtime (Node.js packages via npx)
type NPXRuntimeConfig struct {
	Package string   `json:"package"`        // Required: NPM package name
	Args    []string `json:"args,omitempty"` // Optional: Additional arguments
}

// ContainerizedRuntimeConfig represents configuration for containerized runtime (Docker containers)
type ContainerizedRuntimeConfig struct {
	Image   string   `json:"image"`             // Required: Docker image name
	Command string   `json:"command,omitempty"` // Optional: Override container command
	Args    []string `json:"args,omitempty"`    // Optional: Container arguments
	Port    int      `json:"port"`              // Required: Container port
	Path    string   `json:"path"`              // Required: HTTP path for MCP endpoint
}

// RemoteRuntimeConfig represents configuration for remote runtime (External MCP servers)
type RemoteRuntimeConfig struct {
	URL     string      `json:"url"`               // Required: Full URL to remote MCP server
	Headers []MCPHeader `json:"headers,omitempty"` // Optional
}

// RemoteCatalogConfig represents template configuration for remote servers in catalog entries
type RemoteCatalogConfig struct {
	FixedURL string      `json:"fixedURL,omitempty"` // Fixed URL for all instances
	Hostname string      `json:"hostname,omitempty"` // Required hostname for user URLs
	Headers  []MCPHeader `json:"headers,omitempty"`  // Optional
}

type MCPServerCatalogEntry struct {
	Metadata
	Manifest    MCPServerCatalogEntryManifest `json:"manifest"`
	Editable    bool                          `json:"editable,omitempty"`
	CatalogName string                        `json:"catalogName,omitempty"`
	SourceURL   string                        `json:"sourceURL,omitempty"`
	UserCount   int                           `json:"userCount,omitempty"`
}

type MCPServerCatalogEntryManifest struct {
	Metadata    map[string]string `json:"metadata,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	RepoURL     string            `json:"repoURL,omitempty"`
	ToolPreview []MCPServerTool   `json:"toolPreview,omitempty"`

	// Runtime configuration
	Runtime Runtime `json:"runtime"`

	// Runtime-specific configurations (only one should be populated based on runtime)
	UVXConfig           *UVXRuntimeConfig           `json:"uvxConfig,omitempty"`
	NPXConfig           *NPXRuntimeConfig           `json:"npxConfig,omitempty"`
	ContainerizedConfig *ContainerizedRuntimeConfig `json:"containerizedConfig,omitempty"`
	RemoteConfig        *RemoteCatalogConfig        `json:"remoteConfig,omitempty"`

	Env []MCPEnv `json:"env,omitempty"`
}

type MCPHeader struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	Key       string `json:"key"`
	Sensitive bool   `json:"sensitive"`
	Required  bool   `json:"required"`
}

type MCPEnv struct {
	MCPHeader `json:",inline"`
	File      bool `json:"file"`
}

type MCPServerCatalogEntryList List[MCPServerCatalogEntry]

type MCPServerManifest struct {
	Metadata    map[string]string `json:"metadata,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Icon        string            `json:"icon"`
	ToolPreview []MCPServerTool   `json:"toolPreview,omitempty"`

	// Runtime configuration
	Runtime Runtime `json:"runtime"`

	// Runtime-specific configurations (only one should be populated based on runtime)
	UVXConfig           *UVXRuntimeConfig           `json:"uvxConfig,omitempty"`
	NPXConfig           *NPXRuntimeConfig           `json:"npxConfig,omitempty"`
	ContainerizedConfig *ContainerizedRuntimeConfig `json:"containerizedConfig,omitempty"`
	RemoteConfig        *RemoteRuntimeConfig        `json:"remoteConfig,omitempty"`

	Env []MCPEnv `json:"env,omitempty"`

	// Legacy fields that are deprecated, used only for cleaning up old servers
	Command string      `json:"command,omitempty"`
	Args    []string    `json:"args,omitempty"`
	URL     string      `json:"url,omitempty"`
	Headers []MCPHeader `json:"headers,omitempty"`
}

type MCPServer struct {
	Metadata
	MCPServerManifest       MCPServerManifest `json:"manifest"`
	UserID                  string            `json:"userID"`
	Configured              bool              `json:"configured"`
	MissingRequiredEnvVars  []string          `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders  []string          `json:"missingRequiredHeader,omitempty"`
	CatalogEntryID          string            `json:"catalogEntryID"`
	SharedWithinCatalogName string            `json:"sharedWithinCatalogName,omitempty"`
	ConnectURL              string            `json:"connectURL,omitempty"`
	// NeedsUpdate indicates whether the configuration in this server's catalog entry has drift from this server's configuration.
	NeedsUpdate bool `json:"needsUpdate,omitempty"`
	// NeedsURL indicates whether the server's URL needs to be updated to match the catalog entry.
	NeedsURL bool `json:"needsURL,omitempty"`
	// PreviousURL contains the URL of the server before it was updated to match the catalog entry.
	PreviousURL string `json:"previousURL,omitempty"`
	// MCPServerInstanceUserCount contains the number of unique users with server instances pointing to this MCP server.
	// This is only set for multi-user servers.
	MCPServerInstanceUserCount *int `json:"mcpServerInstanceUserCount,omitempty"`
}

type MCPServerList List[MCPServer]

type MCPServerTool struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Params      map[string]string `json:"params,omitempty"`
	Credentials []string          `json:"credentials,omitempty"`
	Enabled     bool              `json:"enabled"`
	Unsupported bool              `json:"unsupported,omitempty"`
}

type ProjectMCPServerManifest struct {
	MCPID string `json:"mcpID"`
}

type ProjectMCPServer struct {
	Metadata
	ProjectMCPServerManifest
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	UserID      string `json:"userID"`
}

type ProjectMCPServerList List[ProjectMCPServer]

// RuntimeValidationError represents a validation error for runtime-specific configuration
type RuntimeValidationError struct {
	Runtime Runtime
	Field   string
	Message string
}

func (e RuntimeValidationError) Error() string {
	return fmt.Sprintf("runtime %s validation error for field %s: %s", e.Runtime, e.Field, e.Message)
}

// MapCatalogEntryToServer converts an MCPServerCatalogEntryManifest to an MCPServerManifest
// For remote runtime, userURL is used when the catalog entry has a hostname constraint
func MapCatalogEntryToServer(catalogEntry MCPServerCatalogEntryManifest, userURL string) (MCPServerManifest, error) {
	serverManifest := MCPServerManifest{
		// Copy common fields
		Metadata:    catalogEntry.Metadata,
		Name:        catalogEntry.Name,
		Description: catalogEntry.Description,
		Icon:        catalogEntry.Icon,
		ToolPreview: catalogEntry.ToolPreview,
		Runtime:     catalogEntry.Runtime,
		Env:         catalogEntry.Env,
	}

	// Handle runtime-specific mapping
	switch catalogEntry.Runtime {
	case RuntimeUVX:
		if catalogEntry.UVXConfig == nil {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeUVX,
				Field:   "uvxConfig",
				Message: "UVX configuration is required for UVX runtime",
			}
		}
		serverManifest.UVXConfig = &UVXRuntimeConfig{
			Package: catalogEntry.UVXConfig.Package,
			Command: catalogEntry.UVXConfig.Command,
			Args:    catalogEntry.UVXConfig.Args,
		}

	case RuntimeNPX:
		if catalogEntry.NPXConfig == nil {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeNPX,
				Field:   "npxConfig",
				Message: "NPX configuration is required for NPX runtime",
			}
		}
		serverManifest.NPXConfig = &NPXRuntimeConfig{
			Package: catalogEntry.NPXConfig.Package,
			Args:    catalogEntry.NPXConfig.Args,
		}

	case RuntimeContainerized:
		if catalogEntry.ContainerizedConfig == nil {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeContainerized,
				Field:   "containerizedConfig",
				Message: "containerized configuration is required for containerized runtime",
			}
		}
		serverManifest.ContainerizedConfig = &ContainerizedRuntimeConfig{
			Image:   catalogEntry.ContainerizedConfig.Image,
			Command: catalogEntry.ContainerizedConfig.Command,
			Args:    catalogEntry.ContainerizedConfig.Args,
			Port:    catalogEntry.ContainerizedConfig.Port,
			Path:    catalogEntry.ContainerizedConfig.Path,
		}

	case RuntimeRemote:
		if catalogEntry.RemoteConfig == nil {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeRemote,
				Field:   "remoteConfig",
				Message: "remote configuration is required for remote runtime",
			}
		}

		remoteConfig := &RemoteRuntimeConfig{}

		if catalogEntry.RemoteConfig.FixedURL != "" {
			// Use the fixed URL from catalog entry
			remoteConfig.URL = catalogEntry.RemoteConfig.FixedURL
		} else if catalogEntry.RemoteConfig.Hostname != "" {
			// Validate that userURL uses the required hostname
			if userURL == "" {
				return serverManifest, RuntimeValidationError{
					Runtime: RuntimeRemote,
					Field:   "URL",
					Message: "user URL is required when catalog entry specifies hostname constraint",
				}
			}
			if err := validateURLHostname(userURL, catalogEntry.RemoteConfig.Hostname); err != nil {
				return serverManifest, err
			}
			remoteConfig.URL = userURL
		} else {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeRemote,
				Field:   "remoteConfig",
				Message: "either fixedURL or hostname must be specified in catalog entry",
			}
		}

		// Copy headers from catalog entry
		remoteConfig.Headers = catalogEntry.RemoteConfig.Headers
		serverManifest.RemoteConfig = remoteConfig

	default:
		return serverManifest, RuntimeValidationError{
			Runtime: catalogEntry.Runtime,
			Field:   "runtime",
			Message: fmt.Sprintf("unsupported runtime type: %s", catalogEntry.Runtime),
		}
	}

	return serverManifest, nil
}

// validateURLHostname checks if the provided URL uses the required hostname
func validateURLHostname(userURL, requiredHostname string) error {
	parsedURL, err := url.Parse(userURL)
	if err != nil {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: fmt.Sprintf("invalid URL format: %v", err),
		}
	}

	if parsedURL.Hostname() != requiredHostname {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: fmt.Sprintf("URL hostname '%s' does not match required hostname '%s'", parsedURL.Hostname(), requiredHostname),
		}
	}

	return nil
}

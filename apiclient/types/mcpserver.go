package types

import (
	"fmt"
	"net/url"
	"strings"
)

// Runtime represents the execution runtime type for MCP servers
type Runtime string

// Runtime constants for different MCP server execution environments
const (
	RuntimeUVX           Runtime = "uvx"
	RuntimeNPX           Runtime = "npx"
	RuntimeContainerized Runtime = "containerized"
	RuntimeRemote        Runtime = "remote"
	RuntimeComposite     Runtime = "composite"
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
	URL        string      `json:"url"`               // Required: Full URL to remote MCP server
	Headers    []MCPHeader `json:"headers,omitempty"` // Optional
	IsTemplate bool        `json:"isTemplate"`        // Optional: Whether the URL is a template
}

// RemoteCatalogConfig represents template configuration for remote servers in catalog entries
type RemoteCatalogConfig struct {
	FixedURL    string      `json:"fixedURL,omitempty"`    // Fixed URL for all instances
	URLTemplate string      `json:"urlTemplate,omitempty"` // URL template for user URLs
	Hostname    string      `json:"hostname,omitempty"`    // Required hostname for user URLs
	Headers     []MCPHeader `json:"headers,omitempty"`     // Optional
}

// CompositeCatalogConfig represents configuration for composite servers in catalog entries.
type CompositeCatalogConfig struct {
	ComponentServers []CatalogComponentServer `json:"componentServers"`
}

// CatalogComponentServer represents a component server in a composite server catalog entry.
type CatalogComponentServer struct {
	CatalogEntryID string                        `json:"catalogEntryID"`
	Manifest       MCPServerCatalogEntryManifest `json:"manifest"`
	ToolOverrides  []ToolOverride                `json:"toolOverrides,omitempty"`
	Disabled       bool                          `json:"disabled,omitempty"`
}

type CompositeRuntimeConfig struct {
	ComponentServers []ComponentServer `json:"componentServers"`
}

type ComponentServer struct {
	CatalogEntryID string            `json:"catalogEntryID"`
	Manifest       MCPServerManifest `json:"manifest"`
	ToolOverrides  []ToolOverride    `json:"toolOverrides,omitempty"`
	Disabled       bool              `json:"disabled,omitempty"`
}

type MCPServerCatalogEntry struct {
	Metadata
	Manifest                  MCPServerCatalogEntryManifest `json:"manifest"`
	Editable                  bool                          `json:"editable,omitempty"`
	CatalogName               string                        `json:"catalogName,omitempty"`
	SourceURL                 string                        `json:"sourceURL,omitempty"`
	UserCount                 int                           `json:"userCount,omitempty"`
	LastUpdated               *Time                         `json:"lastUpdated,omitempty"`
	ToolPreviewsLastGenerated *Time                         `json:"toolPreviewsLastGenerated,omitempty"`
	PowerUserWorkspaceID      string                        `json:"powerUserWorkspaceID,omitempty"`
	PowerUserID               string                        `json:"powerUserID,omitempty"`
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
	CompositeConfig     *CompositeCatalogConfig     `json:"compositeConfig,omitempty"`

	Env []MCPEnv `json:"env,omitempty"`
}

// ToolOverride defines how a single component tool is exposed by the composite server
type ToolOverride struct {
	// Name is the original tool name as returned by the component server
	Name string `json:"name"`
	// OverrideName is the tool name exposed by the composite server
	OverrideName string `json:"overrideName"`
	// Optional overrides for display
	OverrideDescription string `json:"overrideDescription,omitempty"`
	// Whether to include this tool (default true)
	Enabled bool `json:"enabled,omitempty"`
}

type MCPHeader struct {
	Name        string `json:"name"`
	Description string `json:"description"`

	Key string `json:"key"`

	// For static headers
	Value string `json:"value"`

	// For user-supplied headers
	Sensitive bool   `json:"sensitive"`
	Required  bool   `json:"required"`
	Prefix    string `json:"prefix,omitempty"` // Optional prefix to prepend to user-supplied values (e.g., "Bearer ")
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
	CompositeConfig     *CompositeRuntimeConfig     `json:"compositeConfig,omitempty"`

	Env []MCPEnv `json:"env,omitempty"`

	// Legacy fields that are deprecated, used only for cleaning up old servers
	Command string      `json:"command,omitempty"`
	Args    []string    `json:"args,omitempty"`
	URL     string      `json:"url,omitempty"`
	Headers []MCPHeader `json:"headers,omitempty"`
}

type MCPServer struct {
	Metadata
	MCPServerManifest MCPServerManifest `json:"manifest"`

	// Alias is a user-defined alias for the MCP server.
	// This may only be set for single user and remote MCP servers (i.e. where `MCPCatalogID` is "").
	Alias                  string   `json:"alias,omitempty"`
	UserID                 string   `json:"userID"`
	Configured             bool     `json:"configured"`
	MissingRequiredEnvVars []string `json:"missingRequiredEnvVars,omitempty"`
	MissingRequiredHeaders []string `json:"missingRequiredHeader,omitempty"`
	CatalogEntryID         string   `json:"catalogEntryID"`
	PowerUserWorkspaceID   string   `json:"powerUserWorkspaceID"`
	MCPCatalogID           string   `json:"mcpCatalogID,omitempty"`
	ConnectURL             string   `json:"connectURL,omitempty"`

	// NeedsUpdate indicates whether the configuration in this server's catalog entry has drift from this server's configuration.
	NeedsUpdate bool `json:"needsUpdate,omitempty"`

	// NeedsURL indicates whether the server's URL needs to be updated to match the catalog entry.
	NeedsURL bool `json:"needsURL,omitempty"`

	// PreviousURL contains the URL of the server before it was updated to match the catalog entry.
	PreviousURL string `json:"previousURL,omitempty"`

	// MCPServerInstanceUserCount contains the number of unique users with server instances pointing to this MCP server.
	// This is only set for multi-user servers.
	MCPServerInstanceUserCount *int `json:"mcpServerInstanceUserCount,omitempty"`

	// DeploymentStatus indicates the overall status of the MCP server deployment (Ready, Progressing, Failed).
	DeploymentStatus string `json:"deploymentStatus,omitempty"`

	// DeploymentAvailableReplicas is the number of available replicas in the deployment.
	DeploymentAvailableReplicas *int32 `json:"deploymentAvailableReplicas,omitempty"`

	// DeploymentReadyReplicas is the number of ready replicas in the deployment.
	DeploymentReadyReplicas *int32 `json:"deploymentReadyReplicas,omitempty"`

	// DeploymentReplicas is the desired number of replicas in the deployment.
	DeploymentReplicas *int32 `json:"deploymentReplicas,omitempty"`

	// DeploymentConditions contains key deployment conditions that indicate deployment health.
	DeploymentConditions []DeploymentCondition `json:"deploymentConditions,omitempty"`

	// Template indicates whether this MCP server is a template server.
	// Template servers are hidden from user views and are used for creating project instances.
	Template bool `json:"template,omitempty"`
}

type DeploymentCondition struct {
	// Type of deployment condition.
	Type string `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status string `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime Time `json:"lastTransitionTime"`
	// Last time the condition was updated.
	LastUpdateTime Time `json:"lastUpdateTime"`
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
	Alias string `json:"alias,omitempty"`
}

type ProjectMCPServer struct {
	Metadata
	ProjectMCPServerManifest
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	UserID      string `json:"userID"`

	// The following status fields are always copied from the MCPServer that this points to.
	Configured  bool `json:"configured"`
	NeedsURL    bool `json:"needsURL"`
	NeedsUpdate bool `json:"needsUpdate"`
}

type ProjectMCPServerList List[ProjectMCPServer]

// K8sSettingsStatus represents the K8s settings status of a deployed MCP server
type K8sSettingsStatus struct {
	// NeedsK8sUpdate indicates whether the server needs redeployment with new K8s settings
	NeedsK8sUpdate bool `json:"needsK8sUpdate"`

	// CurrentSettings are the current global K8s settings
	CurrentSettings *K8sSettings `json:"currentSettings,omitempty"`

	// DeployedSettingsHash is the hash of the K8s settings the server was deployed with
	DeployedSettingsHash string `json:"deployedSettingsHash,omitempty"`
}

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
			if err := ValidateURLHostname(userURL, catalogEntry.RemoteConfig.Hostname); err != nil {
				return serverManifest, err
			}
			remoteConfig.URL = userURL
		} else if catalogEntry.RemoteConfig.URLTemplate != "" {
			remoteConfig.IsTemplate = true
		} else {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeRemote,
				Field:   "remoteConfig",
				Message: "either fixedURL, hostname, or urlTemplate must be specified in catalog entry",
			}
		}

		// Copy headers from catalog entry
		remoteConfig.Headers = catalogEntry.RemoteConfig.Headers
		serverManifest.RemoteConfig = remoteConfig

	case RuntimeComposite:
		if catalogEntry.CompositeConfig == nil {
			return serverManifest, RuntimeValidationError{
				Runtime: RuntimeComposite,
				Field:   "compositeConfig",
				Message: "composite configuration is required for composite runtime",
			}
		}

		// Convert CatalogComponentServer to ComponentServer
		componentServers := make([]ComponentServer, len(catalogEntry.CompositeConfig.ComponentServers))
		for i, catalogComponent := range catalogEntry.CompositeConfig.ComponentServers {
			// Convert the component's catalog manifest to server manifest
			componentServerManifest, err := MapCatalogEntryToServer(catalogComponent.Manifest, "")
			if err != nil {
				return serverManifest, RuntimeValidationError{
					Runtime: RuntimeComposite,
					Field:   fmt.Sprintf("compositeConfig.componentServers[%d]", i),
					Message: fmt.Sprintf("failed to convert component manifest: %v", err),
				}
			}

			componentServers[i] = ComponentServer{
				CatalogEntryID: catalogComponent.CatalogEntryID,
				Manifest:       componentServerManifest,
				ToolOverrides:  catalogComponent.ToolOverrides,
				Disabled:       false,
			}
		}

		serverManifest.CompositeConfig = &CompositeRuntimeConfig{
			ComponentServers: componentServers,
		}

	default:
		return serverManifest, RuntimeValidationError{
			Runtime: catalogEntry.Runtime,
			Field:   "runtime",
			Message: fmt.Sprintf("unsupported runtime type: %s", catalogEntry.Runtime),
		}
	}

	return serverManifest, nil
}

// ValidateURLHostname checks if the URL matches the hostname.
// If the provided hostname does not contain a wildcard, the hostname in the URL must match the provided hostname.
// A wildcard prefix (*.) can be used to match any number of characters (i.e. "*.example.com" matches both "foo.example.com" and "foo.bar.example.com", but not "example.com").
func ValidateURLHostname(u string, hostname string) error {
	if u == "" || hostname == "" {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: "url and hostname are required",
		}
	}

	parsedURL, err := url.Parse(u)
	if err != nil {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: fmt.Sprintf("invalid url: %v", err),
		}
	}

	if parsedURL.Scheme != "https" && parsedURL.Scheme != "http" {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: "URL scheme must be either https or http",
		}
	}

	urlHostname := parsedURL.Hostname()
	if urlHostname == "" {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: "url hostname is required",
		}
	}

	if !strings.HasPrefix(hostname, "*.") && urlHostname != hostname {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: fmt.Sprintf("URL hostname '%s' does not match required hostname '%s'", urlHostname, hostname),
		}
	}

	suffix := strings.TrimPrefix(hostname, "*")
	if !strings.HasSuffix(urlHostname, suffix) {
		return RuntimeValidationError{
			Runtime: RuntimeRemote,
			Field:   "userURL",
			Message: fmt.Sprintf("URL hostname '%s' does not match required hostname '%s'", urlHostname, hostname),
		}
	}
	return nil
}

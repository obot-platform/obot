package mcp

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

type GlobalTokenStore interface {
	ForUserAndMCP(userID, mcpID string) nmcp.TokenStorage
}

type TokenService interface {
	NewTokenWithClaims(context.Context, jwt.MapClaims) (*jwt.Token, string, error)
}

type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type ServerConfig struct {
	Runtime types.Runtime `json:"runtime"`

	// uvx/npx based configuration.
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Env     []string `json:"env"`
	Files   []File   `json:"files"`

	// Remote configuration.
	URL     string   `json:"url"`
	Headers []string `json:"headers"`

	// Containerized configuration.
	ContainerImage string `json:"containerImage"`
	ContainerPort  int    `json:"containerPort"`
	ContainerPath  string `json:"containerPath"`

	// Composite configuration.
	Components []ComponentServer `json:"components"`

	Scope                string `json:"scope"`
	UserID               string `json:"userID"`
	MCPServerNamespace   string `json:"mcpServerNamespace"`
	MCPServerName        string `json:"mcpServerName"`
	MCPCatalogName       string `json:"mcpCatalogName"`
	MCPCatalogEntryName  string `json:"mcpCatalogEntryName"`
	MCPServerDisplayName string `json:"mcpServerDisplayName"`
	NanobotAgentName     string `json:"nanobotAgentName"`

	ProjectMCPServer   bool `json:"projectMCPServer"`
	ComponentMCPServer bool `json:"componentMCPServer"`

	Issuer    string   `json:"issuer"`
	Audiences []string `json:"audiences"`

	AuthorizeEndpoint         string `json:"authorizeEndpoint"`
	TokenExchangeEndpoint     string `json:"tokenExchangeEndpoint"`
	JWKSEndpoint              string `json:"jwksEndpoint"`
	TokenExchangeClientID     string `json:"tokenExchangeClientID"`
	TokenExchangeClientSecret string `json:"tokenExchangeClientSecret"`

	AuditLogToken    string `json:"auditLogToken"`
	AuditLogEndpoint string `json:"auditLogEndpoint"`
	AuditLogMetadata string `json:"auditLogMetadata"`
}

type File struct {
	Data   string `json:"data"`
	EnvKey string `json:"envKey"`
}

type ComponentServer struct {
	Name  string               `json:"name"`
	URL   string               `json:"url"`
	Tools []types.ToolOverride `json:"tools"`
}

var envVarRegex = regexp.MustCompile(`\${([^}]+)}`)

// expandEnvVars replaces ${VAR} patterns with values from credEnv
func expandEnvVars(text string, credEnv map[string]string, fileEnvVars map[string]struct{}) string {
	if credEnv == nil {
		return text
	}

	return envVarRegex.ReplaceAllStringFunc(text, func(match string) string {
		varName := match[2 : len(match)-1] // Remove ${ and }
		if _, isFileVar := fileEnvVars[varName]; !isFileVar {
			// If it's a file variable, then don't expand here.
			if val, ok := credEnv[varName]; ok {
				return val
			}
		}
		return match // Return original if not found
	})
}

// applyPrefix adds a prefix to a value if the value doesn't already start with it.
// Returns the original value if prefix is empty or if value already starts with the prefix.
func applyPrefix(value, prefix string) string {
	if value == "" || strings.HasPrefix(value, prefix) {
		return value
	}
	return prefix + value
}

func legacyServerToServerConfig(mcpServer v1.MCPServer, userID, scope string, credEnv map[string]string, fileEnvVars map[string]struct{}) (ServerConfig, []string, error) {
	// Expand environment variables in command, args, and URL
	command := expandEnvVars(mcpServer.Spec.Manifest.Command, credEnv, fileEnvVars)
	url := expandEnvVars(mcpServer.Spec.Manifest.URL, credEnv, fileEnvVars)

	args := make([]string, len(mcpServer.Spec.Manifest.Args))
	for i, arg := range mcpServer.Spec.Manifest.Args {
		args[i] = expandEnvVars(arg, credEnv, fileEnvVars)
	}

	serverConfig := ServerConfig{
		Command: command,
		Args:    args,
		Env:     make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
		URL:     url,
		UserID:  userID,
		Headers: make([]string, 0, len(mcpServer.Spec.Manifest.Headers)),
		Scope:   fmt.Sprintf("%s-%s", mcpServer.Name, scope),
	}

	var missingRequiredNames []string
	for _, env := range mcpServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Key]
		if !ok && env.Required {
			missingRequiredNames = append(missingRequiredNames, env.Key)
			continue
		}

		// Apply prefix if specified (e.g., "Bearer ", "sk-")
		val = applyPrefix(val, env.Prefix)

		if !env.File {
			serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
			continue
		}

		serverConfig.Files = append(serverConfig.Files, File{
			Data:   val,
			EnvKey: env.Key,
		})
	}

	for _, header := range mcpServer.Spec.Manifest.Headers {
		var (
			val      string
			hasValue bool
		)

		// Check for static value first
		if header.Value != "" {
			val = header.Value
			hasValue = true
		} else {
			// Fall back to user-configured value from credentials
			val, hasValue = credEnv[header.Key]
		}

		if !hasValue {
			if header.Required {
				missingRequiredNames = append(missingRequiredNames, header.Key)
			}
			continue
		}

		// Apply prefix if specified (e.g., "Bearer ", "Token ")
		// Only apply to user-supplied values, not static values
		if header.Value == "" {
			val = applyPrefix(val, header.Prefix)
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
	}

	return serverConfig, missingRequiredNames, nil
}

func CompositeServerToServerConfig(mcpServer v1.MCPServer, components []v1.MCPServer, instances []v1.MCPServerInstance, audiences []string, issuer, userID, scope, mcpCatalogName string, credEnv, tokenExchangeCredEnv map[string]string) (ServerConfig, []string, error) {
	config, missing, err := ServerToServerConfig(mcpServer, audiences, issuer, userID, scope, mcpCatalogName, credEnv, tokenExchangeCredEnv)
	if err != nil {
		return config, missing, err
	}

	overrides := make(map[string]types.ComponentServer, len(mcpServer.Spec.Manifest.CompositeConfig.ComponentServers))
	for _, component := range mcpServer.Spec.Manifest.CompositeConfig.ComponentServers {
		if component.CatalogEntryID != "" {
			overrides[component.CatalogEntryID] = component
		} else if component.MCPServerID != "" {
			overrides[component.MCPServerID] = component
		}
	}

	config.Components = make([]ComponentServer, 0, len(components)+len(instances))
	for _, component := range components {
		name := component.Spec.Manifest.Name
		if name == "" {
			name = component.Name
		}

		override := overrides[component.Spec.MCPServerCatalogEntryName]
		if override.Disabled {
			continue
		}

		tools := make([]types.ToolOverride, 0, len(override.ToolOverrides))
		for _, tool := range override.ToolOverrides {
			if tool.Enabled {
				tools = append(tools, types.ToolOverride{
					Name:                tool.Name,
					OverrideName:        tool.OverrideName,
					OverrideDescription: tool.OverrideDescription,
					Enabled:             tool.Enabled,
				})
			}
		}

		config.Components = append(config.Components, ComponentServer{
			Name:  name,
			URL:   system.MCPConnectURL(issuer, component.Name),
			Tools: tools,
		})
	}

	for _, instance := range instances {
		override := overrides[instance.Spec.MCPServerName]
		if override.Disabled {
			continue
		}

		tools := make([]types.ToolOverride, 0, len(override.ToolOverrides))
		for _, tool := range override.ToolOverrides {
			if tool.Enabled {
				tools = append(tools, types.ToolOverride{
					Name:                tool.Name,
					OverrideName:        tool.OverrideName,
					OverrideDescription: tool.OverrideDescription,
					Enabled:             tool.Enabled,
				})
			}
		}

		config.Components = append(config.Components, ComponentServer{
			Name:  instance.Name,
			URL:   system.MCPConnectURL(issuer, instance.Name),
			Tools: tools,
		})
	}

	slices.SortFunc(config.Components, func(a, b ComponentServer) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return config, missing, err
}

func ServerToServerConfig(mcpServer v1.MCPServer, audiences []string, issuer, userID, scope, mcpCatalogName string, credEnv, secretsCred map[string]string) (ServerConfig, []string, error) {
	fileEnvVars := make(map[string]struct{})
	for _, file := range mcpServer.Spec.Manifest.Env {
		if file.File {
			fileEnvVars[file.Key] = struct{}{}
		}
	}
	if string(mcpServer.Spec.Manifest.Runtime) == "" {
		return legacyServerToServerConfig(mcpServer, userID, scope, credEnv, fileEnvVars)
	}

	displayName := mcpServer.Spec.Manifest.Name
	if displayName == "" {
		displayName = mcpServer.Name
	}

	var powerUserWorkspaceID string
	if system.IsPowerUserWorkspaceID(mcpCatalogName) {
		powerUserWorkspaceID = mcpCatalogName
	}

	serverConfig := ServerConfig{
		Env:                       make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
		UserID:                    userID,
		Scope:                     fmt.Sprintf("%s-%s", mcpServer.Name, scope),
		MCPServerNamespace:        mcpServer.Namespace,
		MCPServerName:             mcpServer.Name,
		MCPCatalogName:            mcpCatalogName,
		MCPCatalogEntryName:       mcpServer.Spec.MCPServerCatalogEntryName,
		MCPServerDisplayName:      displayName,
		Runtime:                   mcpServer.Spec.Manifest.Runtime,
		Issuer:                    issuer,
		Audiences:                 audiences,
		TokenExchangeClientID:     secretsCred["TOKEN_EXCHANGE_CLIENT_ID"],
		TokenExchangeClientSecret: secretsCred["TOKEN_EXCHANGE_CLIENT_SECRET"],
		TokenExchangeEndpoint:     fmt.Sprintf("%s/oauth/token", issuer),
		JWKSEndpoint:              fmt.Sprintf("%s/oauth/jwks.json", issuer),
		AuthorizeEndpoint:         fmt.Sprintf("%s/oauth/authorize", issuer),
		ComponentMCPServer:        mcpServer.Spec.CompositeName != "",
		NanobotAgentName:          mcpServer.Spec.NanobotAgentID,
	}

	if mcpServer.Spec.CompositeName == "" {
		// Don't set these for component MCP servers. Audit logging is handled at the composite level for these.
		serverConfig.AuditLogEndpoint = fmt.Sprintf("%s/api/mcp-audit-logs", issuer)
		serverConfig.AuditLogToken = secretsCred["AUDIT_LOG_TOKEN"]
		serverConfig.AuditLogMetadata = fmt.Sprintf("mcpID=%s,mcpServerCatalogEntryName=%s,powerUserWorkspaceID=%s,mcpServerDisplayName=%s", mcpServer.Name, mcpServer.Spec.MCPServerCatalogEntryName, powerUserWorkspaceID, displayName)
	}

	var missingRequiredNames []string

	// Handle runtime-specific configuration
	switch mcpServer.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if mcpServer.Spec.Manifest.UVXConfig != nil {
			serverConfig.Command = "uvx"
			if mcpServer.Spec.Manifest.UVXConfig.Command != "" {
				serverConfig.Args = []string{"--from", mcpServer.Spec.Manifest.UVXConfig.Package, expandEnvVars(mcpServer.Spec.Manifest.UVXConfig.Command, credEnv, fileEnvVars)}
			} else {
				serverConfig.Args = []string{mcpServer.Spec.Manifest.UVXConfig.Package}
			}
			for _, arg := range mcpServer.Spec.Manifest.UVXConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires uvx config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeNPX:
		if mcpServer.Spec.Manifest.NPXConfig != nil {
			serverConfig.Command = "npx"
			serverConfig.Args = []string{mcpServer.Spec.Manifest.NPXConfig.Package}
			for _, arg := range mcpServer.Spec.Manifest.NPXConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires npx config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeContainerized:
		if mcpServer.Spec.Manifest.ContainerizedConfig != nil {
			serverConfig.ContainerImage = expandEnvVars(mcpServer.Spec.Manifest.ContainerizedConfig.Image, credEnv, fileEnvVars)
			serverConfig.ContainerPort = mcpServer.Spec.Manifest.ContainerizedConfig.Port
			serverConfig.ContainerPath = mcpServer.Spec.Manifest.ContainerizedConfig.Path
			serverConfig.Command = expandEnvVars(mcpServer.Spec.Manifest.ContainerizedConfig.Command, credEnv, fileEnvVars)
			serverConfig.Args = make([]string, 0, len(mcpServer.Spec.Manifest.ContainerizedConfig.Args))
			for _, arg := range mcpServer.Spec.Manifest.ContainerizedConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires containerized config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeRemote:
		if mcpServer.Spec.Manifest.RemoteConfig != nil {
			serverConfig.URL = mcpServer.Spec.Manifest.RemoteConfig.URL
			// Add headers from remote config
			serverConfig.Headers = make([]string, 0, len(mcpServer.Spec.Manifest.RemoteConfig.Headers))
			for _, header := range mcpServer.Spec.Manifest.RemoteConfig.Headers {
				var (
					val      string
					hasValue bool
				)

				// Check for static value first
				if header.Value != "" {
					val = header.Value
					hasValue = true
				} else {
					// Fall back to user-configured value from credentials
					credVal, ok := credEnv[header.Key]
					if ok && credVal != "" {
						val = credVal
						hasValue = true
					}
				}

				if !hasValue {
					if header.Required {
						missingRequiredNames = append(missingRequiredNames, header.Key)
					}
					continue
				}

				// Apply prefix if specified (e.g., "Bearer ", "Token ")
				// Only apply to user-supplied values, not static values
				if header.Value == "" {
					val = applyPrefix(val, header.Prefix)
				}

				serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires remote config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeComposite:
		return serverConfig, nil, nil
	default:
		return serverConfig, missingRequiredNames, fmt.Errorf("unknown runtime %s", mcpServer.Spec.Manifest.Runtime)
	}

	for _, env := range mcpServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Key]
		if !ok || val == "" {
			if env.Required {
				missingRequiredNames = append(missingRequiredNames, env.Key)
			}
			continue
		}

		// Apply prefix if specified (e.g., "Bearer ", "sk-")
		val = applyPrefix(val, env.Prefix)

		if !env.File {
			serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
			continue
		}

		serverConfig.Files = append(serverConfig.Files, File{
			Data:   val,
			EnvKey: env.Key,
		})
	}

	return serverConfig, missingRequiredNames, nil
}

func ProjectServerToConfig(projectMCPServer v1.ProjectMCPServer, publicBaseURL, internalBaseURL, userID string) (ServerConfig, error) {
	return ServerConfig{
		URL:                projectMCPServer.ConnectURL(internalBaseURL),
		UserID:             userID,
		MCPServerNamespace: projectMCPServer.Namespace,
		MCPServerName:      projectMCPServer.Spec.Manifest.MCPID,
		Scope:              fmt.Sprintf("%s-%s", projectMCPServer.Name, userID),
		Runtime:            types.RuntimeRemote,
		Audiences:          []string{projectMCPServer.Audience(publicBaseURL)},
		ProjectMCPServer:   true,
	}, nil
}

// SystemServerToServerConfig converts a v1.SystemMCPServer to a ServerConfig for deployment
func SystemServerToServerConfig(systemServer v1.SystemMCPServer, credEnv map[string]string) (ServerConfig, []string, error) {
	fileEnvVars := make(map[string]struct{})
	for _, env := range systemServer.Spec.Manifest.Env {
		if env.File {
			fileEnvVars[env.Key] = struct{}{}
		}
	}

	displayName := systemServer.Spec.Manifest.Name
	if displayName == "" {
		displayName = systemServer.Name
	}

	serverConfig := ServerConfig{
		Env:                  make([]string, 0, len(systemServer.Spec.Manifest.Env)),
		MCPServerNamespace:   systemServer.Namespace,
		MCPServerName:        systemServer.Name,
		MCPServerDisplayName: displayName,
		Runtime:              systemServer.Spec.Manifest.Runtime,
		Scope:                fmt.Sprintf("%s-system", systemServer.Name),
	}

	var missingRequiredNames []string

	// Handle runtime-specific configuration
	switch systemServer.Spec.Manifest.Runtime {
	case types.RuntimeContainerized:
		if systemServer.Spec.Manifest.ContainerizedConfig != nil {
			serverConfig.ContainerImage = systemServer.Spec.Manifest.ContainerizedConfig.Image
			serverConfig.ContainerPort = systemServer.Spec.Manifest.ContainerizedConfig.Port
			serverConfig.ContainerPath = systemServer.Spec.Manifest.ContainerizedConfig.Path

			if systemServer.Spec.Manifest.ContainerizedConfig.Command != "" {
				serverConfig.Command = expandEnvVars(systemServer.Spec.Manifest.ContainerizedConfig.Command, credEnv, fileEnvVars)
			}

			for _, arg := range systemServer.Spec.Manifest.ContainerizedConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
			}
		}

	case types.RuntimeRemote:
		if systemServer.Spec.Manifest.RemoteConfig != nil {
			serverConfig.URL = expandEnvVars(systemServer.Spec.Manifest.RemoteConfig.URL, credEnv, fileEnvVars)

			// Add headers from remote config
			serverConfig.Headers = make([]string, 0, len(systemServer.Spec.Manifest.RemoteConfig.Headers))
			for _, header := range systemServer.Spec.Manifest.RemoteConfig.Headers {
				var (
					val      string
					hasValue bool
				)

				// Check for static value first
				if header.Value != "" {
					val = header.Value
					hasValue = true
				} else {
					// Fall back to user-configured value from credentials
					credVal, ok := credEnv[header.Key]
					if ok && credVal != "" {
						val = credVal
						hasValue = true
					}
				}

				if !hasValue {
					if header.Required {
						missingRequiredNames = append(missingRequiredNames, header.Key)
					}
					continue
				}

				// Apply prefix if specified (e.g., "Bearer ", "Token ")
				// Only apply to user-supplied values, not static values
				if header.Value == "" {
					val = applyPrefix(val, header.Prefix)
				}

				serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
			}
		}
	}

	// Process environment variables
	for _, env := range systemServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Key]
		if !ok || val == "" {
			if env.Required {
				missingRequiredNames = append(missingRequiredNames, env.Key)
			}
			continue
		}

		// Apply prefix if specified (e.g., "Bearer ", "sk-")
		val = applyPrefix(val, env.Prefix)

		if !env.File {
			serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
			continue
		}

		serverConfig.Files = append(serverConfig.Files, File{
			Data:   val,
			EnvKey: env.Key,
		})
	}

	return serverConfig, missingRequiredNames, nil
}

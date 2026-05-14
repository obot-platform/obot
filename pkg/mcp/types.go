package mcp

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// MaxMCPServerStartupTimeout is the maximum value allowed to be used in ServerConfig.StartupTimeout
const MaxMCPServerStartupTimeout = 10 * time.Minute

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
	URL                     string   `json:"url"`
	Headers                 []string `json:"headers"`
	PassthroughHeaderNames  []string `json:"passthroughHeaderNames"`
	PassthroughHeaderValues []string `json:"passthroughHeaderValues"`

	// Containerized configuration.
	ContainerImage string `json:"containerImage"`
	ContainerPort  int    `json:"containerPort"`
	ContainerPath  string `json:"containerPath"`

	// Composite configuration.
	Components []ComponentServer `json:"components"`

	Scope                string `json:"scope"`
	UserID               string `json:"userID"`
	OwnerUserID          string `json:"ownerUserID"`
	MCPServerNamespace   string `json:"mcpServerNamespace"`
	MCPServerName        string `json:"mcpServerName"`
	MCPCatalogName       string `json:"mcpCatalogName"`
	MCPCatalogEntryName  string `json:"mcpCatalogEntryName"`
	MCPServerDisplayName string `json:"mcpServerDisplayName"`
	NanobotAgentName     string `json:"nanobotAgentName"`
	ProjectMCPServer     bool   `json:"projectMCPServer"`
	ComponentMCPServer   bool   `json:"componentMCPServer"`
	SystemMCPServer      bool   `json:"systemMCPServer"`

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

	StartupTimeout time.Duration `json:"startupTimeout,omitempty"`
}

type File struct {
	Data    string `json:"data"`
	EnvKey  string `json:"envKey"`
	Dynamic bool   `json:"dynamic"`
}

type ComponentServer struct {
	Name       string               `json:"name"`
	URL        string               `json:"url"`
	Tools      []types.ToolOverride `json:"tools"`
	ToolPrefix string               `json:"toolPrefix"`
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

func configureUVXRuntime(serverConfig *ServerConfig, uvxConfig *types.UVXRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}) error {
	if uvxConfig == nil {
		return fmt.Errorf("uvx runtime requires uvx config")
	}

	serverConfig.Command = "uvx"
	if uvxConfig.Command != "" {
		serverConfig.Args = []string{"--from", uvxConfig.Package, expandEnvVars(uvxConfig.Command, credEnv, fileEnvVars)}
	} else {
		serverConfig.Args = []string{uvxConfig.Package}
	}

	for _, arg := range uvxConfig.Args {
		serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
	}

	return nil
}

func configureNPXRuntime(serverConfig *ServerConfig, npxConfig *types.NPXRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}) error {
	if npxConfig == nil {
		return fmt.Errorf("npx runtime requires npx config")
	}

	serverConfig.Command = "npx"
	serverConfig.Args = []string{npxConfig.Package}
	for _, arg := range npxConfig.Args {
		serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
	}

	return nil
}

func configureContainerizedRuntime(serverConfig *ServerConfig, containerizedConfig *types.ContainerizedRuntimeConfig, credEnv map[string]string, fileEnvVars map[string]struct{}, expandImage bool) error {
	if containerizedConfig == nil {
		return fmt.Errorf("containerized runtime requires containerized config")
	}

	serverConfig.ContainerImage = containerizedConfig.Image
	if expandImage {
		serverConfig.ContainerImage = expandEnvVars(containerizedConfig.Image, credEnv, fileEnvVars)
	}
	serverConfig.ContainerPort = containerizedConfig.Port
	serverConfig.ContainerPath = containerizedConfig.Path
	serverConfig.Command = expandEnvVars(containerizedConfig.Command, credEnv, fileEnvVars)
	for _, arg := range containerizedConfig.Args {
		serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv, fileEnvVars))
	}

	return nil
}

func configureRemoteRuntime(serverConfig *ServerConfig, remoteConfig *types.RemoteRuntimeConfig, credEnv map[string]string) ([]string, error) {
	if remoteConfig == nil {
		return nil, fmt.Errorf("remote runtime requires remote config")
	}

	var missingRequiredNames []string
	serverConfig.URL = remoteConfig.URL
	serverConfig.Headers = make([]string, 0, len(remoteConfig.Headers))
	for _, header := range remoteConfig.Headers {
		val := header.Value
		if val == "" {
			val = credEnv[header.Key]
		}

		if val == "" {
			if header.Required {
				missingRequiredNames = append(missingRequiredNames, header.Key)
			}
			continue
		}

		// Only apply the prefix if the value is not static.
		if header.Value == "" {
			val = applyPrefix(val, header.Prefix)
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
	}

	return missingRequiredNames, nil
}

func configureCompositeRuntime(serverConfig ServerConfig) (ServerConfig, []string, error) {
	return serverConfig, nil, nil
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
			Name:       name,
			URL:        system.MCPConnectURL(issuer, component.Name),
			Tools:      tools,
			ToolPrefix: override.ToolPrefix,
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
			Name:       instance.Name,
			URL:        system.MCPConnectURL(issuer, instance.Name),
			Tools:      tools,
			ToolPrefix: override.ToolPrefix,
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

	displayName := mcpServer.Spec.Manifest.Name
	if displayName == "" {
		displayName = mcpServer.Name
	}

	var powerUserWorkspaceID string
	if system.IsPowerUserWorkspaceID(mcpCatalogName) {
		powerUserWorkspaceID = mcpCatalogName
	}

	startupTimeoutSeconds := mcpServer.Spec.Manifest.RuntimeStartupTimeoutSeconds()
	startupTimeout := time.Duration(startupTimeoutSeconds) * time.Second
	if startupTimeout > MaxMCPServerStartupTimeout {
		return ServerConfig{}, nil, fmt.Errorf("input %d exceeds the max of %s", startupTimeoutSeconds, MaxMCPServerStartupTimeout)
	}

	var passthroughHeaderNames []string
	if mcpServer.Spec.Manifest.MultiUserConfig != nil && len(mcpServer.Spec.Manifest.MultiUserConfig.UserDefinedHeaders) > 0 {
		passthroughHeaderNames = make([]string, len(mcpServer.Spec.Manifest.MultiUserConfig.UserDefinedHeaders))
		for i, header := range mcpServer.Spec.Manifest.MultiUserConfig.UserDefinedHeaders {
			passthroughHeaderNames[i] = header.Key
		}
	}

	serverConfig := ServerConfig{
		Env:                       make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
		UserID:                    userID,
		OwnerUserID:               mcpServer.Spec.UserID,
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
		PassthroughHeaderNames:    passthroughHeaderNames,
		ComponentMCPServer:        mcpServer.Spec.CompositeName != "",
		NanobotAgentName:          mcpServer.Spec.NanobotAgentID,
		StartupTimeout:            startupTimeout,
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
		if err := configureUVXRuntime(&serverConfig, mcpServer.Spec.Manifest.UVXConfig, credEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeNPX:
		if err := configureNPXRuntime(&serverConfig, mcpServer.Spec.Manifest.NPXConfig, credEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeContainerized:
		serverConfig.Args = make([]string, 0, len(mcpServer.Spec.Manifest.ContainerizedConfig.Args))
		if err := configureContainerizedRuntime(&serverConfig, mcpServer.Spec.Manifest.ContainerizedConfig, credEnv, fileEnvVars, true); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeRemote:
		var err error
		missingRequiredNames, err = configureRemoteRuntime(&serverConfig, mcpServer.Spec.Manifest.RemoteConfig, credEnv)
		if err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeComposite:
		return configureCompositeRuntime(serverConfig)
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
			Data:    val,
			EnvKey:  env.Key,
			Dynamic: env.DynamicFile,
		})
	}

	return serverConfig, missingRequiredNames, nil
}

func ProjectServerToConfig(projectMCPServer v1.ProjectMCPServer, publicBaseURL, internalBaseURL, userID string) (ServerConfig, error) {
	return ServerConfig{
		URL:                projectMCPServer.ConnectURL(internalBaseURL),
		UserID:             userID,
		OwnerUserID:        projectMCPServer.Spec.UserID,
		MCPServerNamespace: projectMCPServer.Namespace,
		MCPServerName:      projectMCPServer.Spec.Manifest.MCPID,
		Scope:              fmt.Sprintf("%s-%s", projectMCPServer.Name, userID),
		Runtime:            types.RuntimeRemote,
		Audiences:          []string{projectMCPServer.Audience(publicBaseURL)},
		ProjectMCPServer:   true,
	}, nil
}

// SystemServerToServerConfig converts a v1.SystemMCPServer to a ServerConfig for deployment
func SystemServerToServerConfig(systemServer v1.SystemMCPServer, audiences []string, issuer string, credEnv, secretsCred map[string]string) (ServerConfig, []string, error) {
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

	startupTimeoutSeconds := systemServer.Spec.Manifest.RuntimeStartupTimeoutSeconds()
	startupTimeout := time.Duration(startupTimeoutSeconds) * time.Second
	if startupTimeout > MaxMCPServerStartupTimeout {
		return ServerConfig{}, nil, fmt.Errorf("input %d exceeds the max of %s", startupTimeoutSeconds, MaxMCPServerStartupTimeout)
	}

	serverConfig := ServerConfig{
		Env:                       make([]string, 0, len(systemServer.Spec.Manifest.Env)),
		MCPServerNamespace:        systemServer.Namespace,
		MCPServerName:             systemServer.Name,
		MCPServerDisplayName:      displayName,
		Runtime:                   systemServer.Spec.Manifest.Runtime,
		Scope:                     fmt.Sprintf("%s-system", systemServer.Name),
		Issuer:                    issuer,
		Audiences:                 audiences,
		TokenExchangeClientID:     secretsCred["TOKEN_EXCHANGE_CLIENT_ID"],
		TokenExchangeClientSecret: secretsCred["TOKEN_EXCHANGE_CLIENT_SECRET"],
		TokenExchangeEndpoint:     fmt.Sprintf("%s/oauth/token", issuer),
		JWKSEndpoint:              fmt.Sprintf("%s/oauth/jwks.json", issuer),
		AuthorizeEndpoint:         fmt.Sprintf("%s/oauth/authorize", issuer),
		AuditLogEndpoint:          fmt.Sprintf("%s/api/mcp-audit-logs", issuer),
		AuditLogToken:             secretsCred["AUDIT_LOG_TOKEN"],
		AuditLogMetadata:          fmt.Sprintf("mcpID=%s,mcpServerDisplayName=%s", systemServer.Name, displayName),
		SystemMCPServer:           true,
		StartupTimeout:            startupTimeout,
	}

	var missingRequiredNames []string

	// Handle runtime-specific configuration
	switch systemServer.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if err := configureUVXRuntime(&serverConfig, systemServer.Spec.Manifest.UVXConfig, credEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeNPX:
		if err := configureNPXRuntime(&serverConfig, systemServer.Spec.Manifest.NPXConfig, credEnv, fileEnvVars); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeContainerized:
		if err := configureContainerizedRuntime(&serverConfig, systemServer.Spec.Manifest.ContainerizedConfig, credEnv, fileEnvVars, false); err != nil {
			return serverConfig, missingRequiredNames, err
		}
	case types.RuntimeRemote:
		var err error
		missingRequiredNames, err = configureRemoteRuntime(&serverConfig, systemServer.Spec.Manifest.RemoteConfig, credEnv)
		if err != nil {
			return serverConfig, missingRequiredNames, err
		}
	default:
		return ServerConfig{}, nil, fmt.Errorf("unsupported runtime type: %s", systemServer.Spec.Manifest.Runtime)
	}

	// Process environment variables
	for _, env := range systemServer.Spec.Manifest.Env {
		var (
			val      string
			hasValue bool
		)

		// Check for static value first
		if env.Value != "" {
			val = env.Value
			hasValue = true
		} else {
			// Fall back to user-configured value from credentials
			credVal, ok := credEnv[env.Key]
			if ok && credVal != "" {
				val = credVal
				hasValue = true
			}
		}

		if !hasValue {
			if env.Required {
				missingRequiredNames = append(missingRequiredNames, env.Key)
			}
			continue
		}

		// Apply prefix if specified (e.g., "Bearer ", "sk-")
		// Only apply to user-supplied values, not static values
		if env.Value == "" {
			val = applyPrefix(val, env.Prefix)
		}

		if !env.File {
			serverConfig.Env = append(serverConfig.Env, fmt.Sprintf("%s=%s", env.Key, val))
			continue
		}

		serverConfig.Files = append(serverConfig.Files, File{
			Data:    val,
			EnvKey:  env.Key,
			Dynamic: env.DynamicFile,
		})
	}

	return serverConfig, missingRequiredNames, nil
}

func copyHeaders[T header](headers T, keys, values []string) {
	for i, key := range keys {
		if i < len(values) {
			headers.Set(key, values[i])
		}
	}
}

type header interface {
	Set(key, value string)
}

type headerMap map[string]string

func (h headerMap) Set(key, value string) {
	h[key] = value
}

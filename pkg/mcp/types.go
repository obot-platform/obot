package mcp

import (
	"fmt"
	"regexp"

	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/jwt"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type GlobalTokenStore interface {
	ForUserAndMCP(userID, mcpID string) nmcp.TokenStorage
}

type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type ServerConfig struct {
	gmcp.ServerConfig `json:",inline"`
	Files             []File        `json:"files"`
	ContainerImage    string        `json:"containerImage"`
	ContainerPort     int           `json:"containerPort"`
	ContainerPath     string        `json:"containerPath"`
	Runtime           types.Runtime `json:"runtime"`
}

type File struct {
	Data   string `json:"data"`
	EnvKey string `json:"envKey"`
}

var envVarRegex = regexp.MustCompile(`\${([^}]+)}`)

// expandEnvVars replaces ${VAR} patterns with values from credEnv
func expandEnvVars(text string, credEnv map[string]string) string {
	if credEnv == nil {
		return text
	}

	return envVarRegex.ReplaceAllStringFunc(text, func(match string) string {
		varName := match[2 : len(match)-1] // Remove ${ and }
		if val, ok := credEnv[varName]; ok {
			return val
		}
		return match // Return original if not found
	})
}

func legacyServerToServerConfig(mcpServer v1.MCPServer, scope string, credEnv map[string]string, allowedTools ...string) (ServerConfig, []string, error) {
	// Expand environment variables in command, args, and URL
	command := expandEnvVars(mcpServer.Spec.Manifest.Command, credEnv)
	url := expandEnvVars(mcpServer.Spec.Manifest.URL, credEnv)

	args := make([]string, len(mcpServer.Spec.Manifest.Args))
	for i, arg := range mcpServer.Spec.Manifest.Args {
		args[i] = expandEnvVars(arg, credEnv)
	}

	serverConfig := ServerConfig{
		ServerConfig: gmcp.ServerConfig{
			Command:      command,
			Args:         args,
			Env:          make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
			URL:          url,
			Headers:      make([]string, 0, len(mcpServer.Spec.Manifest.Headers)),
			Scope:        fmt.Sprintf("%s-%s", mcpServer.Name, scope),
			AllowedTools: allowedTools,
		},
	}

	var missingRequiredNames []string
	for _, env := range mcpServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Key]
		if !ok && env.Required {
			missingRequiredNames = append(missingRequiredNames, env.Key)
			continue
		}

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
		val, ok := credEnv[header.Key]
		if !ok && header.Required {
			missingRequiredNames = append(missingRequiredNames, header.Key)
			continue
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
	}

	return serverConfig, missingRequiredNames, nil
}

func ServerToServerConfig(mcpServer v1.MCPServer, scope string, credEnv map[string]string, allowedTools ...string) (ServerConfig, []string, error) {
	if string(mcpServer.Spec.Manifest.Runtime) == "" {
		return legacyServerToServerConfig(mcpServer, scope, credEnv, allowedTools...)
	}

	serverConfig := ServerConfig{
		ServerConfig: gmcp.ServerConfig{
			Env:          make([]string, 0, len(mcpServer.Spec.Manifest.Env)),
			Scope:        fmt.Sprintf("%s-%s", mcpServer.Name, scope),
			AllowedTools: allowedTools,
		},
		Runtime: mcpServer.Spec.Manifest.Runtime,
	}

	var missingRequiredNames []string

	// Handle runtime-specific configuration
	switch mcpServer.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if mcpServer.Spec.Manifest.UVXConfig != nil {
			serverConfig.Command = "uvx"
			if mcpServer.Spec.Manifest.UVXConfig.Command != "" {
				serverConfig.Args = []string{"--from", mcpServer.Spec.Manifest.UVXConfig.Package, expandEnvVars(mcpServer.Spec.Manifest.UVXConfig.Command, credEnv)}
			} else {
				serverConfig.Args = []string{mcpServer.Spec.Manifest.UVXConfig.Package}
			}
			for _, arg := range mcpServer.Spec.Manifest.UVXConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires uvx config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeNPX:
		if mcpServer.Spec.Manifest.NPXConfig != nil {
			serverConfig.Command = "npx"
			serverConfig.Args = []string{mcpServer.Spec.Manifest.NPXConfig.Package}
			for _, arg := range mcpServer.Spec.Manifest.NPXConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires npx config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeContainerized:
		if mcpServer.Spec.Manifest.ContainerizedConfig != nil {
			serverConfig.ContainerImage = expandEnvVars(mcpServer.Spec.Manifest.ContainerizedConfig.Image, credEnv)
			serverConfig.ContainerPort = mcpServer.Spec.Manifest.ContainerizedConfig.Port
			serverConfig.ContainerPath = mcpServer.Spec.Manifest.ContainerizedConfig.Path
			serverConfig.Command = expandEnvVars(mcpServer.Spec.Manifest.ContainerizedConfig.Command, credEnv)
			serverConfig.Args = make([]string, 0, len(mcpServer.Spec.Manifest.ContainerizedConfig.Args))
			for _, arg := range mcpServer.Spec.Manifest.ContainerizedConfig.Args {
				serverConfig.Args = append(serverConfig.Args, expandEnvVars(arg, credEnv))
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires containerized config", mcpServer.Spec.Manifest.Runtime)
		}
	case types.RuntimeRemote:
		if mcpServer.Spec.Manifest.RemoteConfig != nil {
			serverConfig.URL = expandEnvVars(mcpServer.Spec.Manifest.RemoteConfig.URL, credEnv)
			// Add headers from remote config
			serverConfig.Headers = make([]string, 0, len(mcpServer.Spec.Manifest.RemoteConfig.Headers))
			for _, header := range mcpServer.Spec.Manifest.RemoteConfig.Headers {
				val, ok := credEnv[header.Key]
				if !ok && header.Required {
					missingRequiredNames = append(missingRequiredNames, header.Key)
					continue
				}
				if ok {
					serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
				}
			}
		} else {
			return serverConfig, missingRequiredNames, fmt.Errorf("runtime %s requires remote config", mcpServer.Spec.Manifest.Runtime)
		}
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

func ProjectServerToConfig(tokenService *jwt.TokenService, projectMCPServer v1.ProjectMCPServer, baseURL, userID string, userIsAdmin bool, allowedTools ...string) (ServerConfig, error) {
	tokenContext := jwt.TokenContext{
		UserID: userID,
	}
	if userIsAdmin {
		tokenContext.UserGroups = []string{authz.AdminGroup}
	}
	token, err := tokenService.NewToken(tokenContext)
	if err != nil {
		return ServerConfig{}, fmt.Errorf("failed to create token: %w", err)
	}

	return ServerConfig{
		ServerConfig: gmcp.ServerConfig{
			URL:          projectMCPServer.ConnectURL(baseURL),
			Headers:      []string{fmt.Sprintf("Authorization=Bearer %s", token)},
			Scope:        fmt.Sprintf("%s-%s", projectMCPServer.Name, userID),
			AllowedTools: allowedTools,
		},
		Runtime: types.RuntimeRemote,
	}, nil
}

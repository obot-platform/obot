package mcp

import (
	"fmt"
	"regexp"
	"strings"

	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/jwt"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type GlobalTokenStore interface {
	ForMCPID(mcpID string) nmcp.TokenStorage
}

type Config struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type ServerConfig struct {
	gmcp.ServerConfig `json:",inline"`
	Files             []File `json:"files"`
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

func ToServerConfig(tokenService *jwt.TokenService, mcpServer v1.MCPServer, baseURL, scope string, credEnv map[string]string, allowedTools ...string) (ServerConfig, []string, error) {
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

	if strings.HasPrefix(serverConfig.URL, baseURL+"/mcp-connect/") {
		token, err := tokenService.NewToken(jwt.TokenContext{
			UserID: mcpServer.Spec.UserID,
		})
		if err != nil {
			return ServerConfig{}, nil, fmt.Errorf("failed to create token: %w", err)
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("Authorization=Bearer %s", token))
	}

	return serverConfig, missingRequiredNames, nil
}

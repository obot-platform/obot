package mcp

import (
	"fmt"
	"regexp"

	gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

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

func ToServerConfig(mcpServer v1.MCPServer, projectThreadName string, credEnv map[string]string, allowedTools []string) (ServerConfig, []string) {
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
			Scope:        fmt.Sprintf("%s-%s", mcpServer.Name, projectThreadName),
			AllowedTools: allowedTools,
		},
	}

	var missingRequiredNames []string
	for _, env := range mcpServer.Spec.Manifest.Env {
		val, ok := credEnv[env.Key]
		if !ok && env.Required {
			name := env.Name
			if name == "" {
				name = env.Key
			}
			missingRequiredNames = append(missingRequiredNames, name)
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
			name := header.Name
			if name == "" {
				name = header.Key
			}
			missingRequiredNames = append(missingRequiredNames, name)
			continue
		}

		serverConfig.Headers = append(serverConfig.Headers, fmt.Sprintf("%s=%s", header.Key, val))
	}

	return serverConfig, missingRequiredNames
}

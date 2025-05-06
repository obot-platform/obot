package mcp

import gmcp "github.com/gptscript-ai/gptscript/pkg/mcp"

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

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gptscript-ai/gptscript/pkg/engine"
	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
)

// Run is responsible for calling MCP tools when the LLM requests their execution. This method is called by GPTScript.
func (sm *SessionManager) Run(ctx engine.Context, _ chan<- gtypes.CompletionStatus, tool gtypes.Tool, input string) (string, error) {
	fields := strings.Fields(tool.Instructions)
	if len(fields) < 2 {
		return "", fmt.Errorf("invalid mcp call, invalid number of fields in %s", tool.Instructions)
	}

	id := fields[1]
	clientScope := fields[2]
	toolName, ok := strings.CutPrefix(fields[0], gtypes.MCPInvokePrefix)
	if !ok {
		return "", fmt.Errorf("invalid mcp call, invalid tool name in %s", tool.Instructions)
	}

	arguments := map[string]any{}

	if input != "" {
		if err := json.Unmarshal([]byte(input), &arguments); err != nil {
			return "", fmt.Errorf("failed to unmarshal input: %w", err)
		}
	}

	session := sm.getClient(id, clientScope)
	if session == nil {
		// The session being nil here means that we don't have a client for this MCP server.
		// This likely means that Obot was restarted between starting the run and making the tool call.
		// Luckily, we have the metadata on the tool and can create a new client.
		var serverConfig ServerConfig
		err := json.Unmarshal([]byte(tool.MetaData["obot-server-config"]), &serverConfig)
		if err != nil {
			log.Errorf("failed to unmarshal server config: %v", err)
			return "", fmt.Errorf("session not found for MCP server %s, %s", id, clientScope)
		}

		now := time.Now().Add(-time.Second)
		// TODO(thedadams): This needs to be fixed before user information headers can be passed to the MCP server.
		_, token, err := sm.tokenService.NewTokenWithClaims(ctx.Ctx, jwt.MapClaims{
			"aud": gtypes.FirstSet(serverConfig.Audiences...),
			"exp": float64(now.Add(time.Hour + 15*time.Minute).Unix()),
			"iat": float64(now.Unix()),
			"sub": serverConfig.UserID,
			// "name":       server.UserName,
			// "email":      server.UserEmail,
			// "picture":    server.Picture,
			// "UserGroups": strings.Join(server.UserGroups, ","),
			"MCPID": serverConfig.MCPServerName,
		})
		if err != nil {
			log.Errorf("failed to create token: %v", err)
			return "", fmt.Errorf("session not found for MCP server %s, %s", id, clientScope)
		}

		serverConfig.Headers = []string{fmt.Sprintf("Authorization=Bearer %s", token)}

		session, err = sm.clientForServer(ctx.Ctx, serverConfig)
		if err != nil {
			log.Errorf("failed to create session for MCP server %s, %s: %v", id, clientScope, err)
			return "", fmt.Errorf("session not found for MCP server %s, %s", id, clientScope)
		}
	}

	result, err := session.Call(ctx.Ctx, toolName, arguments)
	if err != nil {
		if ctx.ToolCategory == engine.NoCategory && ctx.Parent != nil {
			var output []byte
			if result != nil {
				output, _ = json.Marshal(result)
			}
			// If this is a sub-call, then don't return the error; return the error as a message so that the LLM can retry.
			return fmt.Sprintf("ERROR: got (%v) while running tool, OUTPUT: %s", err, string(output)), nil
		}
		return "", fmt.Errorf("failed to call tool %s: %w", toolName, err)
	}

	str, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("failed to marshal result: %w", err)
	}

	return string(str), nil
}

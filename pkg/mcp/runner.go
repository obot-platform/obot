package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/engine"
	gtypes "github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/jwt/ephemeral"
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

		userID := tool.MetaData["obot-user-id"]

		// Ephemeral tokens "expire" every time Obot restarts. Create a new one.
		token, err := sm.ephemeralTokenService.NewToken(ephemeral.TokenContext{
			UserID:     userID,
			UserGroups: []string{types.GroupBasic},
		})
		if err != nil {
			log.Errorf("failed to create token: %v", err)
			return "", fmt.Errorf("session not found for MCP server %s, %s", id, clientScope)
		}

		serverConfig.Headers = []string{fmt.Sprintf("Authorization=Bearer %s", token)}

		session, err = sm.ClientForServer(ctx.Ctx, userID, tool.MetaData["obot-server-display-name"], tool.MetaData["obot-project-mcp-server-name"], serverConfig)
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

package mcp

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gptscript-ai/gptscript/pkg/engine"
	"github.com/gptscript-ai/gptscript/pkg/types"
)

// Run is responsible for calling MCP tools when the LLM requests their execution. This method is called by GPTScript.
func (sm *SessionManager) Run(ctx engine.Context, _ chan<- types.CompletionStatus, tool types.Tool, input string) (string, error) {
	fields := strings.Fields(tool.Instructions)
	if len(fields) < 2 {
		return "", fmt.Errorf("invalid mcp call, invalid number of fields in %s", tool.Instructions)
	}

	id := fields[1]
	clientScope := fields[2]
	toolName, ok := strings.CutPrefix(fields[0], types.MCPInvokePrefix)
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
		return "", fmt.Errorf("session not found for MCP server %s, %s", id, clientScope)
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

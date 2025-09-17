package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/types"
	"github.com/modelcontextprotocol/go-sdk/jsonschema"
	nmcp "github.com/nanobot-ai/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/jwt/ephemeral"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (sm *SessionManager) GPTScriptTools(ctx context.Context, tokenService *ephemeral.TokenService, projectMCPServer v1.ProjectMCPServer, userID, mcpServerDisplayName, serverURL string, allowedTools []string) ([]gptscript.ToolDef, error) {
	if mcpServerDisplayName == "" {
		mcpServerDisplayName = projectMCPServer.Name
	}

	serverConfig, err := ProjectServerToConfig(tokenService, projectMCPServer, serverURL, userID, allowedTools...)
	if err != nil {
		return nil, fmt.Errorf("failed to convert MCP server %s to server config: %w", mcpServerDisplayName, err)
	}

	client, err := sm.ClientForServer(ctx, userID, mcpServerDisplayName, projectMCPServer.Name, serverConfig)
	if err != nil {
		return nil, determineError(err, mcpServerDisplayName)
	}

	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, determineError(err, mcpServerDisplayName)
	}

	allToolsAllowed := allowedTools == nil || slices.Contains(allowedTools, "*")

	toolDefs := []gptscript.ToolDef{{ /* this is a placeholder for main tool */ }}
	var toolNames []string

	for _, tool := range tools.Tools {
		if tool.Name == "" {
			// I dunno, bad tool?
			continue
		}
		if !allToolsAllowed && !slices.Contains(allowedTools, tool.Name) {
			continue
		}

		toolName := mcpServerDisplayName + " -> " + tool.Name

		var schema jsonschema.Schema
		if err = json.Unmarshal(tool.InputSchema, &schema); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool input schema: %w", err)
		}

		annotations, err := json.Marshal(tool.Annotations)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal tool annotations: %w", err)
		}

		toolDef := gptscript.ToolDef{
			Name:         toolName,
			Description:  tool.Description,
			Arguments:    &schema,
			Instructions: fmt.Sprintf("%s%s %s default", types.MCPInvokePrefix, tool.Name, client.ID),
		}

		if string(annotations) != "{}" && string(annotations) != "null" {
			toolDef.MetaData = map[string]string{
				"mcp-tool-annotations": string(annotations),
			}
		}

		if tool.Annotations != nil && tool.Annotations.Title != "" && !slices.Contains(strings.Fields(tool.Annotations.Title), "as") {
			toolNames = append(toolNames, toolName+" as "+tool.Annotations.Title)
		} else {
			toolNames = append(toolNames, toolName)
		}

		toolDefs = append(toolDefs, toolDef)
	}

	main := gptscript.ToolDef{
		Name:        mcpServerDisplayName + "-bundle",
		Description: client.Session.InitializeResult.ServerInfo.Name,
		MetaData: map[string]string{
			"bundle": "true",
		},
	}

	if client.Session.InitializeResult.Instructions != "" {
		data, _ := json.Marshal(map[string]any{
			"tools":        toolNames,
			"instructions": client.Session.InitializeResult.Instructions,
		})
		toolDefs = append(toolDefs, gptscript.ToolDef{
			Name: client.ID,
			Type: "context",
			Instructions: types.EchoPrefix + "\n" + `# START MCP SERVER INFO: ` + client.Session.InitializeResult.ServerInfo.Name + "\n" +
				`You have available the following tools from an MCP Server that has provided the following additional instructions` + "\n" +
				string(data) + "\n" +
				`# END MCP SERVER INFO` + "\n",
		})

		main.ExportContext = append(main.ExportContext, client.ID)
	}

	toolDefs[0] = main
	return toolDefs, nil
}

// determineError determines the error message to return based on the error type.
// This is a slightly more performant version of calling errors.Is or errors.As a bunch of times
// because this version will go through the unwrap tree once.
func determineError(err error, mcpServerDisplayName string) error {
	_, err = findSpecialError(err, mcpServerDisplayName)
	return err
}

func findSpecialError(err error, mcpServerDisplayName string) (bool, error) {
	for unwrappedErr := err; unwrappedErr != nil; unwrappedErr = errors.Unwrap(unwrappedErr) {
		switch {
		case unwrappedErr == nmcp.ErrNoResult || unwrappedErr == nmcp.ErrNoResponse || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()):
			return true, fmt.Errorf("no response from MCP server %s, this is likely due to a configuration error", mcpServerDisplayName)
		case unwrappedErr == ErrHealthCheckFailed || unwrappedErr == ErrHealthCheckTimeout:
			return true, fmt.Errorf("MCP server %s is unhealthy", mcpServerDisplayName)
		default:
			switch e := unwrappedErr.(type) {
			case nmcp.AuthRequiredErr:
				return true, fmt.Errorf("MCP server %s requires OAuth", mcpServerDisplayName)
			case interface{ Unwrap() []error }:
				for _, err := range e.Unwrap() {
					if found, err := findSpecialError(err, mcpServerDisplayName); found {
						return true, err
					}
				}
			}
		}
	}

	// If no specific error was found, return the original error
	return false, err
}

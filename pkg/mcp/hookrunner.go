package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	hookClientScope = "Obot MCP Hook"
)

// HookServerConfigs maps the server portion of a hook target (server/tool) to
// the MCP server configuration used to call that hook.
type HookServerConfigs map[string]ServerConfig

type hookMCPClient interface {
	CallTool(context.Context, *gomcp.CallToolParams) (*gomcp.CallToolResult, error)
}

// SessionManagerHookRunner invokes hook tools through Obot's MCP session manager.
type SessionManagerHookRunner struct {
	clientForServer func(context.Context, ServerConfig) (hookMCPClient, error)
}

type FilterExecutionError struct {
	Class   string
	message string
}

func NewHookRunner(sessionManager *SessionManager) *SessionManagerHookRunner {
	return &SessionManagerHookRunner{
		clientForServer: func(ctx context.Context, server ServerConfig) (hookMCPClient, error) {
			return sessionManager.clientForServerWithScope(ctx, hookClientScope, server)
		},
	}
}

func (r *SessionManagerHookRunner) ExecuteFilter(ctx context.Context, candidate FilterCandidate, payload json.RawMessage) (FilterExecutionResult, error) {
	toolName := candidate.ToolName
	if toolName == "" {
		serverName, parsedToolName, ok := strings.Cut(candidate.Target, "/")
		if !ok || serverName == "" || parsedToolName == "" {
			return FilterExecutionResult{}, newFilterExecutionError(FilterErrorExecution, "invalid Filter target")
		}
		toolName = parsedToolName
	}

	server := candidate.Server
	if server.URL == "" && server.MCPServerName == "" {
		return FilterExecutionResult{}, newFilterExecutionError(FilterErrorExecution, "Filter server is not configured")
	}
	if r == nil || r.clientForServer == nil {
		return FilterExecutionResult{}, newFilterExecutionError(FilterErrorExecution, "Filter runner is not configured")
	}

	client, err := r.clientForServer(ctx, server)
	if err != nil {
		return FilterExecutionResult{}, classifyFilterCallError(ctx, err, "failed to connect to Filter")
	}
	input := struct {
		Accept  bool            `json:"accept"`
		Mutated bool            `json:"mutated"`
		Message json.RawMessage `json:"message"`
		Reason  string          `json:"reason"`
	}{
		Accept:  true,
		Message: payload,
	}
	result, err := client.CallTool(ctx, &gomcp.CallToolParams{Name: toolName, Arguments: input})
	if err != nil {
		return FilterExecutionResult{}, classifyFilterCallError(ctx, err, "Filter tool call failed")
	}
	if result == nil {
		return FilterExecutionResult{}, newFilterExecutionError(FilterErrorMalformedResponse, "Filter returned no result")
	}
	rawResult, _ := json.Marshal(result)
	if result.IsError {
		return FilterExecutionResult{RawResponse: rawResult}, newFilterExecutionError(FilterErrorExecution, "Filter tool returned an error")
	}

	structuredContent := result.StructuredContent
	if structuredContent == nil && len(result.Content) == 1 {
		if text, ok := result.Content[0].(*gomcp.TextContent); ok && text.Text != "" {
			var object map[string]any
			if json.Unmarshal([]byte(text.Text), &object) == nil {
				structuredContent = object
			}
		}
	}
	if structuredContent == nil {
		return FilterExecutionResult{RawResponse: rawResult}, newFilterExecutionError(FilterErrorMalformedResponse, "Filter returned no structured decision")
	}

	data, err := json.Marshal(structuredContent)
	if err != nil {
		return FilterExecutionResult{}, newFilterExecutionError(FilterErrorMalformedResponse, "Filter response could not be encoded")
	}
	var output struct {
		Accept  *bool           `json:"accept"`
		Mutated bool            `json:"mutated"`
		Message json.RawMessage `json:"message"`
		Reason  string          `json:"reason"`
	}
	if err := json.Unmarshal(data, &output); err != nil {
		return FilterExecutionResult{RawResponse: data}, newFilterExecutionError(FilterErrorMalformedResponse, "Filter response was malformed")
	}
	if output.Accept == nil || (!*output.Accept && output.Mutated) {
		return FilterExecutionResult{RawResponse: data}, newFilterExecutionError(FilterErrorMalformedResponse, "Filter response was contradictory or incomplete")
	}
	if !*output.Accept {
		return FilterExecutionResult{
			Decision:    FilterDecisionReject,
			Reason:      output.Reason,
			RawResponse: data,
		}, nil
	}
	if !output.Mutated {
		return FilterExecutionResult{
			Decision:    FilterDecisionAccept,
			Reason:      output.Reason,
			RawResponse: data,
		}, nil
	}
	if len(output.Message) == 0 || string(output.Message) == "null" || !json.Valid(output.Message) {
		return FilterExecutionResult{RawResponse: data}, newFilterExecutionError(FilterErrorMalformedResponse, "Filter mutation did not contain a complete JSON message")
	}
	return FilterExecutionResult{
		Decision:    FilterDecisionMutate,
		Message:     output.Message,
		Reason:      output.Reason,
		RawResponse: data,
	}, nil
}

func classifyFilterCallError(ctx context.Context, err error, executionMessage string) *FilterExecutionError {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return newFilterExecutionError(FilterErrorTimeout, "Filter timed out")
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return newFilterExecutionError(FilterErrorCanceled, "Filter call was canceled")
	}
	return newFilterExecutionError(FilterErrorExecution, executionMessage)
}

func newFilterExecutionError(class, message string) *FilterExecutionError {
	return &FilterExecutionError{Class: class, message: message}
}

func (e *FilterExecutionError) Error() string {
	if e == nil {
		return "Filter evaluation failed"
	}
	return e.message
}

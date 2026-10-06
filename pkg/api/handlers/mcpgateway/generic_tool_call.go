package mcpgateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/mmmcp/toolsearch"
	"github.com/obot-platform/obot/pkg/mcp"
)

// resolvedToolCall is the currently granted operation behind a generic call.
// MMMCP still resolves the forwarded call independently before invocation.
type resolvedToolCall struct {
	name      string
	revision  string
	arguments json.RawMessage
}

type completedGenericToolCall struct {
	id     any
	result *gomcp.CallToolResult
}

func (e *completedGenericToolCall) Error() string {
	return "generic tool call completed during resolution"
}

func (h *Handler) resolveGenericToolCall(req *http.Request, cfg *mmmcpconfig.Config, token string, inspect bool) (*resolvedToolCall, error) {
	if !inspect || !cfg.ToolSearch || req.Method != http.MethodPost || req.Body == nil {
		return nil, nil
	}

	// Inspect a copy so audit and hook processing still receive the original body.
	body, err := io.ReadAll(io.LimitReader(req.Body, maxMCPProxyHookBodySize+1))
	if err != nil {
		return nil, fmt.Errorf("read generic tool call: %w", err)
	}
	if len(body) > maxMCPProxyHookBodySize {
		return nil, fmt.Errorf("generic tool call body too large")
	}
	_ = req.Body.Close()
	req.Body = io.NopCloser(bytes.NewReader(body))
	decoded, _, err := decodeMCPHookBody(io.NopCloser(bytes.NewReader(body)), req.Header.Get("Content-Encoding"))
	if err != nil {
		return nil, fmt.Errorf("decode generic tool call: %w", err)
	}
	defer decoded.Close()
	body, err = readMCPHookBody(decoded)
	if err != nil {
		return nil, fmt.Errorf("read decoded generic tool call: %w", err)
	}

	var message mcp.Message
	if decodeMCPHookMessage(body, &message) != nil || message.Method != "tools/call" {
		return nil, nil
	}
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal(message.Params, &params) != nil || params.Name != toolsearch.CallToolName {
		return nil, nil
	}

	headers := req.Header.Clone()
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	ctx := component.ContextWithRequestHeaders(req.Context(), headers)
	call, ok, err := h.composite.ResolveToolCall(ctx, cfg, params.Name, params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("resolve generic tool call: %w", err)
	}
	if !ok || call.Route == nil {
		if call.Result != nil {
			return nil, &completedGenericToolCall{id: message.ID, result: call.Result}
		}
		// The composite owner may have a newer catalog. Forwarding an unresolved
		// call could let it invoke a tool without its tool-specific hooks.
		return nil, fmt.Errorf("generic tool call cannot be resolved against the current catalog")
	}
	args, err := toolsearch.ParseCallArguments(params.Arguments)
	if err != nil {
		return nil, fmt.Errorf("parse resolved generic tool call: %w", err)
	}
	return &resolvedToolCall{name: args.Name, revision: args.Revision, arguments: call.Arguments}, nil
}

// hookMessage presents a generic call to policies as the underlying tool call.
func (r *resolvedToolCall) hookMessage(wire mcp.Message) (mcp.Message, error) {
	var params map[string]json.RawMessage
	if err := json.Unmarshal(wire.Params, &params); err != nil {
		return mcp.Message{}, fmt.Errorf("decode generic call for hook: %w", err)
	}
	name, _ := json.Marshal(r.name)
	params["name"] = name
	params["arguments"] = r.arguments
	encoded, err := json.Marshal(params)
	if err != nil {
		return mcp.Message{}, fmt.Errorf("encode resolved call for hook: %w", err)
	}
	result := wire
	result.Params = encoded
	return result, nil
}

// restoreHookMutation retains the validated target and revision while applying
// a policy's changes to the underlying arguments and shared call metadata.
func (r *resolvedToolCall) restoreHookMutation(wire, mutated mcp.Message) (mcp.Message, error) {
	if mutated.Method != wire.Method || mcp.MessageIDString(mutated.ID) != mcp.MessageIDString(wire.ID) || mutated.JSONRPC != wire.JSONRPC {
		return mcp.Message{}, fmt.Errorf("generic call hook cannot change the request identity")
	}
	var direct, outer, wrapper map[string]json.RawMessage
	if err := json.Unmarshal(mutated.Params, &direct); err != nil || direct == nil {
		return mcp.Message{}, fmt.Errorf("generic call hook returned invalid parameters")
	}
	var name string
	if json.Unmarshal(direct["name"], &name) != nil || name != r.name {
		return mcp.Message{}, fmt.Errorf("generic call hook cannot change the target tool")
	}
	arguments := direct["arguments"]
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(arguments, &object) != nil || object == nil {
		return mcp.Message{}, fmt.Errorf("generic call hook arguments must be an object")
	}
	if err := json.Unmarshal(wire.Params, &outer); err != nil {
		return mcp.Message{}, fmt.Errorf("decode original generic call: %w", err)
	}
	if outer == nil {
		return mcp.Message{}, fmt.Errorf("original generic call parameters must be an object")
	}
	if err := json.Unmarshal(outer["arguments"], &wrapper); err != nil {
		return mcp.Message{}, fmt.Errorf("decode original generic call arguments: %w", err)
	}
	if wrapper == nil {
		return mcp.Message{}, fmt.Errorf("original generic call arguments must be an object")
	}
	var originalName, originalRevision string
	if json.Unmarshal(wrapper["name"], &originalName) != nil || originalName != r.name ||
		json.Unmarshal(wrapper["revision"], &originalRevision) != nil || originalRevision != r.revision {
		return mcp.Message{}, fmt.Errorf("generic call reference changed before hook mutation")
	}
	for key := range outer {
		if key != "name" && key != "arguments" {
			delete(outer, key)
		}
	}
	for key, value := range direct {
		if key != "name" && key != "arguments" {
			outer[key] = value
		}
	}
	wrapper["arguments"] = arguments
	var err error
	outer["arguments"], err = json.Marshal(wrapper)
	if err != nil {
		return mcp.Message{}, fmt.Errorf("encode generic call arguments: %w", err)
	}
	encoded, err := json.Marshal(outer)
	if err != nil {
		return mcp.Message{}, fmt.Errorf("encode generic call after hook: %w", err)
	}
	result := wire
	result.Params = encoded
	result.HookMutations = mutated.HookMutations
	return result, nil
}

func matchesGenericCallHook(hook mcp.HookMapping, method string, params map[string]string, genericCall bool) bool {
	if !genericCall || method != "tools/call" || hook.Params["name"] != toolsearch.CallToolName ||
		params["name"] == toolsearch.CallToolName {
		return false
	}
	aliased := make(map[string]string, len(params))
	maps.Copy(aliased, params)
	aliased["name"] = toolsearch.CallToolName
	return hook.Matches(method, aliased)
}

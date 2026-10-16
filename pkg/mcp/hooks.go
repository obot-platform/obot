package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	// HookMutationsMetaKey identifies hook mutation metadata in MCP results.
	HookMutationsMetaKey = "ai.obot.hooks/mutations"
)

var (
	ErrRPCUnknown = NewRPCError(-32001, "JSON RPC unknown error")
)

// HookRunner executes one configured hook target.
type HookRunner interface {
	RunHook(ctx context.Context, servers HookServerConfigs, input SessionMessageHook, target string) (*SessionMessageHook, error)
}

type Hooks []HookMapping

type HookMapping struct {
	Name    string
	Params  map[string]string
	Targets []HookTarget
}

type HookTarget struct {
	Target           string
	MutateDisallowed bool
	// Name is the filter's display name, recorded in audit logs.
	Name string
	// Guardrail is set for a guardrail filter, which Obot runs in-process instead of
	// calling Target on a filter MCP server.
	Guardrail *GuardrailTarget
}

// GuardrailTarget is a guardrail filter in a hook chain.
type GuardrailTarget struct {
	// FilterName is the filter's resource name (its ID).
	FilterName string
	Config     types.MCPGuardrailFilterConfig
}

// Message is the JSON-RPC envelope sent to and returned by MCP hooks.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`

	HookMutations map[string]HookMutation `json:"-"`
}

type HookMutation struct {
	Mutated bool     `json:"mutated"`
	Reasons []string `json:"reasons,omitempty"`
}

type SessionMessageHook struct {
	Accept  bool     `json:"accept"`
	Mutated bool     `json:"mutated"`
	Message *Message `json:"message"`
	Reason  string   `json:"reason"`
}

type RPCError struct {
	Code    int             `json:"code,omitempty"`
	Message string          `json:"message,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// HasGuardrails reports whether any hook target is a guardrail filter.
func (h Hooks) HasGuardrails() bool {
	for _, hook := range h {
		for _, target := range hook.Targets {
			if target.Guardrail != nil {
				return true
			}
		}
	}
	return false
}

// GuardrailsOnly returns the hooks with only their guardrail filter targets, which need no
// hook runner.
func (h Hooks) GuardrailsOnly() Hooks {
	var result Hooks
	for _, hook := range h {
		var targets []HookTarget
		for _, target := range hook.Targets {
			if target.Guardrail != nil {
				targets = append(targets, target)
			}
		}
		if len(targets) > 0 {
			hook.Targets = targets
			result = append(result, hook)
		}
	}
	return result
}

func (h HookMapping) Matches(name string, params map[string]string) bool {
	if h.Name != name && h.Name != "*" {
		return false
	}
	for key, value := range h.Params {
		if params[key] != value {
			return false
		}
	}
	return true
}

func NewRPCError(code int, message string) *RPCError {
	return &RPCError{Code: code, Message: message}
}

func (e *RPCError) WithMessage(format string, args ...any) *RPCError {
	result := *e
	result.Message += ": " + fmt.Sprintf(format, args...)
	return &result
}

func MessageIDString(id any) string {
	switch value := id.(type) {
	case nil:
		return ""
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	default:
		return fmt.Sprint(id)
	}
}

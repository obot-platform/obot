package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

const (
	// HookMutationsMetaKey is retained for compatibility with clients that already
	// consume hook mutation metadata from MCP results.
	HookMutationsMetaKey = "ai.nanobot.hooks/mutations"
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

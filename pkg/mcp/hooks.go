package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// HookMutationsMetaKey identifies hook mutation metadata in MCP results.
	HookMutationsMetaKey = "ai.obot.hooks/mutations"
)

var (
	ErrRPCUnknown = NewRPCError(-32001, "JSON RPC unknown error")
)

// FilterExecutor executes one configured Filter against a raw JSON payload.
// Source-specific payload parsing and mutation validation belong outside the
// executor so this interface can be shared by MCP and local-agent callers.
type FilterExecutor interface {
	ExecuteFilter(ctx context.Context, candidate FilterCandidate, payload json.RawMessage) (FilterExecutionResult, error)
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
	Candidate        FilterCandidate
}

// FilterCandidate is a fully resolved Filter invocation target. ResourceName
// is the stable internal resource name used for ordering and de-duplication;
// DisplayName is a historical/user-facing snapshot.
type FilterCandidate struct {
	ResourceID      string
	ResourceName    string
	DisplayName     string
	Target          string
	ToolName        string
	Server          ServerConfig
	AllowedToMutate bool
	AuditName       string
}

type FilterDecision string

const (
	FilterDecisionAccept FilterDecision = "accept"
	FilterDecisionReject FilterDecision = "reject"
	FilterDecisionMutate FilterDecision = "mutate"
)

type FilterDecisionKind string

const (
	FilterDecisionKindPolicy         FilterDecisionKind = "policy"
	FilterDecisionKindInfrastructure FilterDecisionKind = "infrastructure"
)

// FilterExecutionResult is the normalized output of one Filter MCP tool call.
// RawResponse is retained only for the protected decision recorder introduced
// by the shared pipeline; callers must never log it or include it in errors.
type FilterExecutionResult struct {
	Decision    FilterDecision
	Message     json.RawMessage
	Reason      string
	RawResponse json.RawMessage
}

// FilterDecisionRecord is source-neutral decision detail produced after a
// Filter result has been normalized and any mutation has been validated.
// Payload fields are sensitive and must be encrypted by production recorders.
type FilterDecisionRecord struct {
	EvaluationID   string
	Sequence       int
	FilterID       string
	FilterName     string
	Source         string
	Event          string
	Phase          string
	Decision       FilterDecision
	DecisionKind   FilterDecisionKind
	DurationMs     int64
	ErrorClass     string
	SourceContext  json.RawMessage
	Input          json.RawMessage
	RawResponse    json.RawMessage
	MutatedPayload json.RawMessage
	Reason         string
	Diagnostic     string
}

type FilterDecisionRecorder interface {
	RecordFilterDecision(context.Context, FilterDecisionRecord) error
}

type FilterMutationValidator func(original, current, replacement json.RawMessage) error

type FilterPipelineRequest struct {
	Candidates       []FilterCandidate
	Payload          json.RawMessage
	Source           string
	Event            string
	Phase            string
	SourceContext    json.RawMessage
	MutationAllowed  bool
	ValidateMutation FilterMutationValidator
}

type FilterOutcome struct {
	Candidate    FilterCandidate
	Decision     FilterDecision
	DecisionKind FilterDecisionKind
	Reason       string
	ErrorClass   string
	Duration     time.Duration
}

type FilterPipelineResult struct {
	EvaluationID string
	Decision     FilterDecision
	Payload      json.RawMessage
	Mutated      bool
	Reason       string
	ErrorClass   string
	Outcomes     []FilterOutcome
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

// FilterCandidatesForMCP resolves every Filter matching one MCP surface. The
// pipeline performs the final resource-name ordering and de-duplication.
func FilterCandidatesForMCP(hooks Hooks, servers HookServerConfigs, method string, params map[string]string) []FilterCandidate {
	var candidates []FilterCandidate
	for _, hook := range hooks {
		if !hook.Matches(method, params) {
			continue
		}
		for _, target := range hook.Targets {
			candidate := target.Candidate
			if candidate.Target == "" {
				candidate.Target = target.Target
			}
			if candidate.ResourceName == "" {
				candidate.ResourceName = candidate.Target
			}
			if candidate.ResourceID == "" {
				candidate.ResourceID = candidate.ResourceName
			}
			if candidate.DisplayName == "" {
				candidate.DisplayName = candidate.ResourceName
			}
			if candidate.ToolName == "" {
				_, candidate.ToolName, _ = strings.Cut(candidate.Target, "/")
			}
			candidate.AllowedToMutate = !target.MutateDisallowed
			candidate.AuditName = hook.Name
			if candidate.Server.URL == "" && candidate.Server.MCPServerName == "" {
				serverName, _, _ := strings.Cut(candidate.Target, "/")
				candidate.Server = servers[serverName]
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates
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

package provenance

import (
	"time"
)

const (
	SurfaceLLM    Surface = "llm"    // the LLM gateway: model turns and the context that fed them
	SurfaceMCP    Surface = "mcp"    // the MCP gateway: tool calls and their results
	SurfaceDevice Surface = "device" // Obot Sentry hooks on a user's device
	SurfaceEgress Surface = "egress" // the Claude Environments egress gateway

	KindSessionStart   EventKind = "session.start"
	KindSessionEnd     EventKind = "session.end"
	KindLLMTurn        EventKind = "llm.turn"
	KindToolCall       EventKind = "tool.call"
	KindToolResult     EventKind = "tool.result"
	KindFileRead       EventKind = "file.read"
	KindFileWrite      EventKind = "file.write"
	KindShellExec      EventKind = "shell.exec"
	KindNetEgress      EventKind = "net.egress"
	KindDelegation     EventKind = "delegation"
	KindApproval       EventKind = "approval"
	KindPolicyDecision EventKind = "policy.decision"
	KindKill           EventKind = "kill"

	EffectAllow           Effect = "allow"
	EffectDeny            Effect = "deny"
	EffectRequireApproval Effect = "require_approval"
	EffectRedact          Effect = "redact"
	EffectQuarantine      Effect = "quarantine"
	EffectDownshift       Effect = "downshift"
	EffectLog             Effect = "log"
)

var (
	// AllSurfaces lists every surface, in a stable order.
	AllSurfaces = []Surface{SurfaceLLM, SurfaceMCP, SurfaceDevice, SurfaceEgress}
)

// Surface is one of the four places Obot observes an agent.
type Surface string

// EventKind is the normalized kind of thing that happened. The same kind can
// be reported by more than one surface: a tool.call is reported by the MCP
// gateway for a hosted server and by a device hook for a local one.
type EventKind string

// Actor identifies who or what performed an event beyond the user: the agent
// program, the device it ran on, the hosted environment, the API key or MCP
// client it used. Every field is optional; a surface fills what it knows.
type Actor struct {
	// Agent is the agent program (claude_code, codex, cursor, kiro, a vMCP
	// client's name, ...).
	Agent string `json:"agent,omitempty"`
	// AgentVersion is the agent's reported version, when known.
	AgentVersion string `json:"agentVersion,omitempty"`
	// DeviceID is the enrolled Obot Sentry device.
	DeviceID string `json:"deviceID,omitempty"`
	// EnvironmentID is the Claude Environment session or workspace.
	EnvironmentID string `json:"environmentID,omitempty"`
	// APIKeyID is the Obot API key used at the LLM gateway.
	APIKeyID string `json:"apiKeyID,omitempty"`
	// ClientSessionID is the MCP client session (the Mcp-Session-Id) or the
	// agent's own session identifier reported by a hook.
	ClientSessionID string `json:"clientSessionID,omitempty"`
	// Delegation is the chain of principals the authority passed through,
	// outermost first: the user, then the agent, then any sub-agents.
	Delegation []string `json:"delegation,omitempty"`
}

// Sink describes where an action sends information. It is derived from the
// action's capabilities, destination and surface.
type Sink struct {
	// External reports that the destination is outside the organization's
	// approved set: an arbitrary host, a public paste site, a non-corporate
	// email address.
	External bool `json:"external"`
	// Destination is the host, address, channel or path the action writes to.
	Destination string `json:"destination,omitempty"`
	// Writes reports that the action persists or transmits data (as opposed to
	// reading it).
	Writes bool `json:"writes"`
	// Exec reports that the action executes code or a command.
	Exec bool `json:"exec"`
}

// Source describes what an action reads. It is derived from the capability
// labels of the tool or resource.
type Source struct {
	// Private reports that the action reads organization-private data.
	Private bool `json:"private"`
	// Untrusted reports that the action reads content an outsider could have
	// authored (web pages, inbound email, public issues, tickets).
	Untrusted bool `json:"untrusted"`
}

// Action is the normalized description of one thing an agent is about to do,
// across every surface. Policies are evaluated against it. Arguments are the
// action's content and are used only for lineage matching and classification;
// they are never persisted by the engine.
type Action struct {
	Surface Surface   `json:"surface"`
	Kind    EventKind `json:"kind"`

	// Tool is the tool name for tool calls (the runtime tool name on a device,
	// the MCP tool name at the gateway).
	Tool string `json:"tool,omitempty"`
	// Server is the display name of the MCP server for MCP tool calls.
	Server string `json:"server,omitempty"`
	// ServerID is the Obot MCPServer or catalog entry ID when the server is
	// known to Obot.
	ServerID string `json:"serverID,omitempty"`
	// Model and Provider are set for LLM turns.
	Model    string `json:"model,omitempty"`
	Provider string `json:"provider,omitempty"`
	// Host, Method and Path are set for network egress.
	Host   string `json:"host,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
	// Command is the shell command for shell.exec.
	Command string `json:"command,omitempty"`
	// File is the path for file.read and file.write.
	File string `json:"file,omitempty"`

	// Capabilities are the capability labels of the tool or resource.
	Capabilities Labels `json:"capabilities,omitempty"`
	Sink         Sink   `json:"sink"`
	Source       Source `json:"source"`

	// Arguments is the action's content: tool arguments, the request body, the
	// command line. It is scanned for lineage and classified, never stored.
	Arguments map[string]any `json:"-"`
	// Text is a flattened rendering of Arguments for scanning. Adapters may set
	// it directly when the content is not a map.
	Text string `json:"-"`
}

// Event is one recorded thing that happened in a session. It is the unit of
// the session graph. Payloads are content-free by default: tool names,
// argument hashes, hosts, byte counts and labels, not the arguments themselves.
type Event struct {
	ID        string    `json:"id"`
	SessionID string    `json:"sessionID"`
	Surface   Surface   `json:"surface"`
	Kind      EventKind `json:"kind"`
	At        time.Time `json:"at"`
	UserID    string    `json:"userID"`
	Actor     Actor     `json:"actor"`

	// ParentID is the causal parent: a tool.call's parent is the llm.turn that
	// emitted it, a tool.result's parent is its tool.call, a policy.decision's
	// parent is the action it decided.
	ParentID string `json:"parentID,omitempty"`

	// Action is the normalized action for events that are actions. Content
	// fields are not serialized.
	Action *Action `json:"action,omitempty"`

	// Labels are the labels this event introduced into the session (for
	// ingress: tool results, file reads, model input) or carried out of it
	// (for egress: matched lineage).
	Labels Labels `json:"labels,omitempty"`
	// Lineage lists the sensitive spans from earlier events that were found in
	// this event's content.
	Lineage []LineageHit `json:"lineage,omitempty"`

	// Decision is the policy outcome for actions.
	Decision *Decision `json:"decision,omitempty"`

	// Ref links the event to the surface's own audit record.
	Ref SurfaceRef `json:"ref"`

	// Summary is a short, content-free, human-readable description used by the
	// session replay ("tools/call github.search_issues", "POST paste.example").
	Summary string `json:"summary,omitempty"`
	// Bytes is the size of the content the event carried, when known.
	Bytes int `json:"bytes,omitempty"`
}

// SurfaceRef points at the surface-native audit record for an event.
type SurfaceRef struct {
	MCPAuditLogID         uint   `json:"mcpAuditLogID,omitempty"`
	LLMAuditLogID         string `json:"llmAuditLogID,omitempty"`
	EnforcementDecisionID uint   `json:"enforcementDecisionID,omitempty"`
	EgressAuditLogID      uint   `json:"egressAuditLogID,omitempty"`
	// EgressAuditLogUUID is the Claude Environments egress audit record, whose
	// primary key is a UUID string.
	EgressAuditLogUUID string `json:"egressAuditLogUUID,omitempty"`
	LocalToolCallID    uint   `json:"localToolCallID,omitempty"`
}

// LineageHit records that a sensitive span first seen in an earlier event was
// found again in this one.
type LineageHit struct {
	// OriginEventID is the event that introduced the span.
	OriginEventID string `json:"originEventID"`
	// OriginSummary is the origin event's summary, repeated so a hit explains
	// itself without a lookup.
	OriginSummary string `json:"originSummary,omitempty"`
	// Label is the sensitivity label the span carries.
	Label Label `json:"label"`
	// Count is how many distinct spans from the origin were matched.
	Count int `json:"count"`
	// Exact reports that at least one match was an exact structured identifier
	// (an email address, an account number) rather than a fuzzy text match.
	Exact bool `json:"exact"`
}

// Effect is what a policy does when its condition holds.
type Effect string

// Decision is the outcome of deciding an action.
type Decision struct {
	// Allow is the final answer the surface must act on.
	Allow bool `json:"allow"`
	// Effect is the strongest effect of the policies that matched, or allow.
	Effect Effect `json:"effect"`
	// PolicyID and PolicyName identify the policy that produced the effect.
	PolicyID   string `json:"policyID,omitempty"`
	PolicyName string `json:"policyName,omitempty"`
	// Reason explains the decision to a person, including the causal chain
	// ("session read github issue #4411 (untrusted) and 14 PII records from
	// salesforce.query, and now wants to POST to paste.example").
	Reason string `json:"reason"`
	// Shadow reports that the deciding policy was in shadow mode: the effect
	// was recorded but not enforced, and Allow is true regardless.
	Shadow bool `json:"shadow,omitempty"`
	// ApprovalID is set when the effect is require_approval and an approval
	// request was created; the surface blocks (or holds) until it resolves.
	ApprovalID string `json:"approvalID,omitempty"`
	// Matched lists every policy whose condition held, strongest first, so a
	// shadow policy's would-have-been outcome is visible next to the enforced
	// one.
	Matched []PolicyMatch `json:"matched,omitempty"`
	// Lineage is the lineage evidence the decision was based on.
	Lineage []LineageHit `json:"lineage,omitempty"`
	// Labels is the session's label set at decision time.
	Labels Labels `json:"labels,omitempty"`
	// Risk is the session's risk score at decision time, 0 to 100.
	Risk int `json:"risk"`
	// Killed reports that the session, user or organization was killed and the
	// action was denied for that reason alone.
	Killed bool `json:"killed,omitempty"`
	// Honeytoken reports that a planted canary was found in the action's
	// content. This is definitive exfiltration evidence.
	Honeytoken bool `json:"honeytoken,omitempty"`
	// Redactions are the spans to remove when the effect is redact, as offsets
	// into Action.Text.
	Redactions []Span `json:"-"`
}

// PolicyMatch is one policy whose condition held for an action.
type PolicyMatch struct {
	PolicyID   string `json:"policyID"`
	PolicyName string `json:"policyName"`
	Effect     Effect `json:"effect"`
	Shadow     bool   `json:"shadow"`
}

// Span is a half-open byte range in a text.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Severity orders effects from least to most restrictive.
func (e Effect) Severity() int {
	switch e {
	case EffectDeny:
		return 6
	case EffectRequireApproval:
		return 5
	case EffectQuarantine:
		return 4
	case EffectDownshift:
		return 3
	case EffectRedact:
		return 2
	case EffectLog:
		return 1
	case EffectAllow:
		return 0
	}
	return -1
}

// Blocks reports whether the effect stops the action from proceeding as
// issued.
func (e Effect) Blocks() bool {
	return e == EffectDeny || e == EffectRequireApproval
}

// ValidEffect reports whether e is a known effect.
func ValidEffect(e Effect) bool {
	return e.Severity() >= 0
}

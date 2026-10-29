package types

import "encoding/json"

const (
	EnforcementDecisionAllow = "allow"
	EnforcementDecisionDeny  = "deny"
)

// EnforcementDecisionServer is the resolved target MCP server of a normalized tool call.
type EnforcementDecisionServer struct {
	URL      string                  `json:"url,omitempty"`
	Package  *AllowlistServerPackage `json:"package,omitempty"`
	Command  string                  `json:"command,omitempty"`
	Hostname string                  `json:"hostname,omitempty"`
	// Connector is the display name of a hosted agent-account connector, for
	// servers that expose no local URL and no local command. It is the only
	// local evidence of what such a server is.
	Connector string `json:"connector,omitempty"`
}

// EnforcementDecisionRequest is the parameter-free normalized tool call a device
// submits to the decision endpoint. The fleet configuration is resolved from the
// authenticated device identity, never from this body.
type EnforcementDecisionRequest struct {
	Agent      string                    `json:"agent,omitempty"`
	Tool       string                    `json:"tool,omitempty"`
	Kind       string                    `json:"kind,omitempty"`
	ServerName string                    `json:"serverName,omitempty"`
	Server     EnforcementDecisionServer `json:"server,omitzero"`

	// Unresolved is set by the device when it could not establish what the call
	// targets (unsupported stdio runner, disallowed runner flag, MCP server
	// absent from every config file). The device has already blocked the call;
	// this exists so the decision log records why.
	Unresolved       bool   `json:"unresolved,omitempty"`
	UnresolvedReason string `json:"unresolvedReason,omitempty"`

	// Session context for the provenance session graph. Every field is
	// optional: a hook that reports them gets a session-aware Obot Policy
	// decision on top of the fleet allowlist verdict, stitched into the same
	// session as the agent's LLM and MCP gateway traffic.

	// SessionID is the agent's own session identifier (Claude Code's
	// session_id).
	SessionID string `json:"sessionID,omitempty"`
	// ToolUseID is the agent's identifier for this tool call.
	ToolUseID string `json:"toolUseID,omitempty"`
	// Input is the tool's input as the agent reported it (Claude Code's
	// tool_input). It is scanned for lineage and classified; it is never
	// stored.
	Input json.RawMessage `json:"input,omitempty"`
	// CWD is the agent's working directory.
	CWD string `json:"cwd,omitempty"`
	// Hostname and LocalUsername describe the device as it sees itself.
	Hostname      string `json:"hostname,omitempty"`
	LocalUsername string `json:"localUsername,omitempty"`
	// UserEmail is the user's email as the device reports it (git config, the
	// agent's login). It is unverified and used only to label the session.
	UserEmail string `json:"userEmail,omitempty"`
}

type EnforcementDecisionResponse struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason,omitempty"`

	// The fields below are set when Obot Policy decided the call (the
	// allowlist allowed it, or there was no allowlist to apply).

	// SessionID is the provenance session the call was recorded in.
	SessionID string `json:"sessionID,omitempty"`
	// Effect is the policy effect that produced the decision: allow, log,
	// redact, downshift, quarantine, require_approval or deny.
	Effect string `json:"effect,omitempty"`
	// ApprovalID is set when the call is held for approval. The device blocks
	// the call; the approval can be resolved by an administrator.
	ApprovalID string `json:"approvalID,omitempty"`
	// PolicyName names the policy that produced the effect.
	PolicyName string `json:"policyName,omitempty"`
	// Labels is the session's label set at decision time.
	Labels []string `json:"labels,omitempty"`
	// Risk is the session's risk score at decision time, 0 to 100.
	Risk int `json:"risk,omitempty"`
}

// EnforcementDecisionEvent is the public, read-side shape of a recorded decision.
// It is the decision log's own event type.
type EnforcementDecisionEvent struct {
	ID                 string                     `json:"id"`
	CreatedAt          Time                       `json:"createdAt"`
	MDMConfigurationID uint                       `json:"mdmConfigurationID"`
	DeviceID           string                     `json:"deviceID,omitempty"`
	ClientIP           string                     `json:"clientIP,omitempty"`
	Agent              string                     `json:"agent,omitempty"`
	Tool               string                     `json:"tool,omitempty"`
	Kind               string                     `json:"kind,omitempty"`
	ServerName         string                     `json:"serverName,omitempty"`
	ObotHosted         bool                       `json:"obotHosted,omitempty"`
	Decision           string                     `json:"decision"`
	Reason             string                     `json:"reason,omitempty"`
	Server             *EnforcementDecisionServer `json:"server,omitempty"`

	// Unresolved reports that the device could not establish what the call
	// targeted, and UnresolvedReason is the specific cause it reported. Such a
	// row is always a deny, so these exist to let the UI label it as "could not
	// be identified" rather than "not allowlisted".
	Unresolved       bool   `json:"unresolved,omitempty"`
	UnresolvedReason string `json:"unresolvedReason,omitempty"`
}

type EnforcementDecisionEventList List[EnforcementDecisionEvent]

type EnforcementDecisionEventResponse struct {
	EnforcementDecisionEventList `json:",inline"`
	Total                        int64 `json:"total"`
	Limit                        int   `json:"limit"`
	Offset                       int   `json:"offset"`
}

// EnforcementDecisionAllowlistCheck is the result of replaying a recorded
// decision against its fleet's current allowlist: would this call be allowed if
// it were made now? The decision log is append-only evidence of what devices
// were told, so asking this question records nothing.
type EnforcementDecisionAllowlistCheck struct {
	ID                 string `json:"id"`
	AllowlistDecision  string `json:"allowlistDecision"`
	AllowlistReason    string `json:"allowlistReason,omitempty"`
	EnforcementEnabled bool   `json:"enforcementEnabled"`
}

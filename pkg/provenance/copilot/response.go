package copilot

import (
	"encoding/json"

	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// Reason codes returned with a block, so a SIEM can group them without
	// parsing the reason text.
	ReasonCodeDenied           = 100
	ReasonCodeApprovalRequired = 101
	ReasonCodeKilled           = 102
	ReasonCodeHoneytoken       = 103

	// Error codes returned with an ErrorResponse.
	ErrorCodeInvalidRequest = 4000
	ErrorCodeMissingField   = 4001
	ErrorCodeUnavailable    = 5031

	// Decisions.
	DecisionApprove = "approve"
	DecisionBlock   = "block"
)

// Response is the documented analyze-tool-execution answer.
type Response struct {
	// BlockAction is true when the tool invocation must not proceed.
	BlockAction bool `json:"blockAction"`
	// ReasonCode is one of the ReasonCode constants when blocked.
	ReasonCode int `json:"reasonCode,omitempty"`
	// Reason is the human-readable explanation, with the causal chain.
	Reason string `json:"reason,omitempty"`
	// Diagnostics is a pre-serialized JSON Result, as the contract requires
	// a string.
	Diagnostics string `json:"diagnostics,omitempty"`
}

// ErrorResponse is the documented answer for an invalid request.
type ErrorResponse struct {
	ErrorCode   int    `json:"errorCode"`
	Message     string `json:"message"`
	HTTPStatus  int    `json:"httpStatus"`
	Diagnostics string `json:"diagnostics,omitempty"`
}

// ValidateResponse is the documented answer to POST /validate.
type ValidateResponse struct {
	IsSuccessful bool   `json:"isSuccessful"`
	Status       string `json:"status"`
}

// ActionDecision is the outcome for one planned action.
type ActionDecision struct {
	Name       string            `json:"name"`
	Server     string            `json:"server,omitempty"`
	Decision   string            `json:"decision"`
	Effect     provenance.Effect `json:"effect,omitempty"`
	PolicyID   string            `json:"policyId,omitempty"`
	PolicyName string            `json:"policyName,omitempty"`
	Reason     string            `json:"reason,omitempty"`
	Shadow     bool              `json:"shadow,omitempty"`
	ApprovalID string            `json:"approvalId,omitempty"`
	EventID    string            `json:"eventId,omitempty"`
	Risk       int               `json:"risk,omitempty"`
	Killed     bool              `json:"killed,omitempty"`
	Honeytoken bool              `json:"honeytoken,omitempty"`
}

// Result is everything Obot decided for one call. It is the response's
// diagnostics.
type Result struct {
	SessionID string           `json:"sessionId,omitempty"`
	Actions   []ActionDecision `json:"actions"`
	// TimedOut reports that Obot answered approve because the decision did
	// not fit the response window.
	TimedOut bool `json:"timedOut,omitempty"`
	// Error is set when the decision could not be made and the call was
	// approved for that reason.
	Error string `json:"error,omitempty"`
}

// DecisionFor converts an engine decision into an action decision.
func DecisionFor(action provenance.Action, ev *provenance.Event, d provenance.Decision) ActionDecision {
	out := ActionDecision{
		Name:       action.Tool,
		Server:     action.Server,
		Decision:   DecisionApprove,
		Effect:     d.Effect,
		PolicyID:   d.PolicyID,
		PolicyName: d.PolicyName,
		Reason:     d.Reason,
		Shadow:     d.Shadow,
		ApprovalID: d.ApprovalID,
		Risk:       d.Risk,
		Killed:     d.Killed,
		Honeytoken: d.Honeytoken,
	}
	if ev != nil {
		out.EventID = ev.ID
	}
	if !d.Allow {
		out.Decision = DecisionBlock
	}
	return out
}

// Blocked reports whether any action was blocked, with the first blocked one.
func (r Result) Blocked() (ActionDecision, bool) {
	for _, a := range r.Actions {
		if a.Decision == DecisionBlock {
			return a, true
		}
	}
	return ActionDecision{}, false
}

// Response renders the result in the documented shape: block when any action
// was blocked, with the first blocked action's reason.
func (r Result) Response() Response {
	diagnostics, _ := json.Marshal(r)
	resp := Response{Diagnostics: string(diagnostics)}
	if r.TimedOut {
		resp.Reason = "decision timed out"
		return resp
	}
	blocked, ok := r.Blocked()
	if !ok {
		return resp
	}
	resp.BlockAction = true
	resp.Reason = blocked.Reason
	switch {
	case blocked.Killed:
		resp.ReasonCode = ReasonCodeKilled
	case blocked.Honeytoken:
		resp.ReasonCode = ReasonCodeHoneytoken
	case blocked.Effect == provenance.EffectRequireApproval:
		resp.ReasonCode = ReasonCodeApprovalRequired
	default:
		resp.ReasonCode = ReasonCodeDenied
	}
	return resp
}

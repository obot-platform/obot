package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

const FilterAPIVersionV1 = "obot.obot.ai/filter/v1"

// FilterContractVersion selects the shape used to invoke a Filter. Legacy MCP is an explicit
// persisted value even though nanobot's legacy hook target has no marker on the wire.
type FilterContractVersion string

const (
	FilterContractVersionLegacyMCP FilterContractVersion = "legacy-mcp"
	FilterContractVersionV1        FilterContractVersion = FilterAPIVersionV1
)

type FilterSource string

const (
	FilterSourceMCP        FilterSource = "mcp"
	FilterSourceLocalAgent FilterSource = "local_agent"
)

type FilterEventType string

const (
	FilterEventTypeMCPMessage FilterEventType = "mcp_message"
	FilterEventTypeUserPrompt FilterEventType = "user_prompt"
	FilterEventTypeToolCall   FilterEventType = "tool_call"
)

type FilterPhase string

const (
	FilterPhaseRequest  FilterPhase = "request"
	FilterPhaseResponse FilterPhase = "response"
	FilterPhaseFailure  FilterPhase = "failure"
)

type FilterSurface string

const (
	FilterSurfaceUserPrompt    FilterSurface = "user_prompt"
	FilterSurfaceToolArguments FilterSurface = "tool_arguments"
	FilterSurfaceToolResponse  FilterSurface = "tool_response"
)

var allFilterSurfaces = [...]FilterSurface{
	FilterSurfaceUserPrompt,
	FilterSurfaceToolArguments,
	FilterSurfaceToolResponse,
}

func KnownFilterSurfaces() []FilterSurface {
	return allFilterSurfaces[:]
}

type FilterDecision string

const (
	FilterDecisionAccept FilterDecision = "accept"
	FilterDecisionReject FilterDecision = "reject"
	FilterDecisionMutate FilterDecision = "mutate"
)

// FilterToolRequest is the MCP tool input for a v1 Filter.
type FilterToolRequest struct {
	Request FilterRequest `json:"request"`
}

// FilterRequest is the common v1 Filter envelope. Payload is the complete value under review.
type FilterRequest struct {
	APIVersion   string             `json:"apiVersion"`
	Source       FilterSource       `json:"source"`
	Event        FilterEvent        `json:"event"`
	Context      FilterContext      `json:"context"`
	Capabilities FilterCapabilities `json:"capabilities"`
	Payload      json.RawMessage    `json:"payload"`
}

type FilterEvent struct {
	Type       FilterEventType `json:"type"`
	Phase      FilterPhase     `json:"phase"`
	Surface    FilterSurface   `json:"surface,omitempty"`
	Method     string          `json:"method,omitempty"`
	Identifier string          `json:"identifier,omitempty"`
}

type FilterContext struct {
	Trace       *FilterTraceContext       `json:"trace,omitempty"`
	MCP         *FilterMCPContext         `json:"mcp,omitempty"`
	LocalAgent  *FilterLocalAgentContext  `json:"localAgent,omitempty"`
	Device      *FilterDeviceContext      `json:"device,omitempty"`
	Environment *FilterEnvironmentContext `json:"environment,omitempty"`
}

type FilterTraceContext struct {
	EventID   string `json:"eventId,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
	TurnID    string `json:"turnId,omitempty"`
	ToolUseID string `json:"toolUseId,omitempty"`
}

type FilterMCPContext struct {
	ServerName      string `json:"serverName,omitempty"`
	ServerShortName string `json:"serverShortName,omitempty"`
}

type FilterLocalAgentContext struct {
	Provider     LocalAgentProvider `json:"provider"`
	AgentVersion string             `json:"agentVersion,omitempty"`
	Model        string             `json:"model,omitempty"`
	ModelID      string             `json:"modelId,omitempty"`
	ToolName     string             `json:"toolName,omitempty"`
	ToolKind     string             `json:"toolKind,omitempty"`
}

type FilterDeviceContext struct {
	ID           string `json:"id"`
	DeploymentID string `json:"deploymentId"`
}

type FilterEnvironmentContext struct {
	OperatingSystem  string `json:"operatingSystem,omitempty"`
	Architecture     string `json:"architecture,omitempty"`
	WorkingDirectory string `json:"workingDirectory,omitempty"`
}

type FilterCapabilities struct {
	CanReject bool `json:"canReject"`
	CanMutate bool `json:"canMutate"`
}

type FilterResponse struct {
	Decision FilterDecision  `json:"decision"`
	Reason   string          `json:"reason,omitempty"`
	Payload  json.RawMessage `json:"payload,omitempty"`
}

func (r FilterRequest) Validate() error {
	if r.APIVersion != FilterAPIVersionV1 {
		return fmt.Errorf("unsupported filter API version %q", r.APIVersion)
	}
	if len(r.Payload) == 0 || !json.Valid(r.Payload) {
		return errors.New("filter payload must be valid JSON")
	}

	switch r.Source {
	case FilterSourceMCP:
		if r.Event.Type != FilterEventTypeMCPMessage {
			return errors.New("MCP source requires an mcp_message event")
		}
		if r.Event.Phase != FilterPhaseRequest && r.Event.Phase != FilterPhaseResponse && r.Event.Phase != FilterPhaseFailure {
			return errors.New("MCP event requires a request, response, or failure phase")
		}
		if r.Event.Surface != "" {
			return errors.New("MCP events must not declare a local-agent surface")
		}
	case FilterSourceLocalAgent:
		if err := r.Event.validateLocalAgent(); err != nil {
			return err
		}
		if r.Context.LocalAgent == nil || r.Context.LocalAgent.Provider == "" {
			return errors.New("local-agent events require an agent provider")
		}
		if r.Context.Device == nil || r.Context.Device.ID == "" || r.Context.Device.DeploymentID == "" {
			return errors.New("local-agent events require authenticated device and deployment identity")
		}
		if r.Event.Surface == FilterSurfaceUserPrompt && r.Capabilities.CanMutate {
			return errors.New("user prompts cannot advertise mutation capability")
		}
	default:
		return fmt.Errorf("unknown filter source %q", r.Source)
	}

	return nil
}

func (e FilterEvent) validateLocalAgent() error {
	switch e.Surface {
	case FilterSurfaceUserPrompt:
		if e.Type != FilterEventTypeUserPrompt || e.Phase != FilterPhaseRequest {
			return errors.New("user_prompt surface requires a user_prompt request event")
		}
	case FilterSurfaceToolArguments:
		if e.Type != FilterEventTypeToolCall || e.Phase != FilterPhaseRequest {
			return errors.New("tool_arguments surface requires a tool_call request event")
		}
	case FilterSurfaceToolResponse:
		if e.Type != FilterEventTypeToolCall || (e.Phase != FilterPhaseResponse && e.Phase != FilterPhaseFailure) {
			return errors.New("tool_response surface requires a tool_call response or failure event")
		}
	default:
		return fmt.Errorf("unknown local-agent surface %q", e.Surface)
	}
	return nil
}

func (r FilterResponse) Validate(capabilities FilterCapabilities) error {
	hasPayload := len(bytes.TrimSpace(r.Payload)) != 0
	switch r.Decision {
	case FilterDecisionAccept:
		if hasPayload {
			return fmt.Errorf("%s response must not contain a payload", r.Decision)
		}
	case FilterDecisionReject:
		if !capabilities.CanReject {
			return errors.New("rejection not allowed by filter capabilities")
		}
		if hasPayload {
			return fmt.Errorf("%s response must not contain a payload", r.Decision)
		}
	case FilterDecisionMutate:
		if !capabilities.CanMutate {
			return errors.New("mutation not allowed by filter capabilities")
		}
		if !hasPayload || !json.Valid(r.Payload) {
			return errors.New("mutate response requires a valid JSON payload")
		}
	default:
		return fmt.Errorf("unknown filter decision %q", r.Decision)
	}
	return nil
}

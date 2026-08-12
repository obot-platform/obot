package types

import (
	"encoding/json"
	"errors"
	"fmt"
)

const LocalAgentFilterEventAPIVersionV1 = "obot.obot.ai/local-agent-filter-event/v1"

// LocalAgentFilterEvent is the provider-neutral event submitted by an enrolled device. Device and
// deployment identity are intentionally absent and must be supplied by the authenticated server.
type LocalAgentFilterEvent struct {
	APIVersion   string                       `json:"apiVersion"`
	OccurredAt   Time                         `json:"occurredAt"`
	Event        FilterEvent                  `json:"event"`
	Context      LocalAgentFilterEventContext `json:"context"`
	Capabilities FilterCapabilities           `json:"capabilities"`
	Payload      json.RawMessage              `json:"payload"`
}

func (e LocalAgentFilterEvent) Validate() error {
	if e.APIVersion != LocalAgentFilterEventAPIVersionV1 {
		return fmt.Errorf("unsupported local-agent Filter event API version %q", e.APIVersion)
	}
	if e.OccurredAt.IsZero() {
		return errors.New("local-agent Filter event occurrence time is required")
	}
	if err := e.Event.validateLocalAgent(); err != nil {
		return err
	}
	if e.Context.LocalAgent.Provider == "" {
		return errors.New("local-agent Filter events require an agent provider")
	}
	if e.Event.Surface == FilterSurfaceUserPrompt && e.Capabilities.CanMutate {
		return errors.New("user prompts cannot advertise mutation capability")
	}
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return errors.New("local-agent Filter event payload must be valid JSON")
	}
	return nil
}

type LocalAgentFilterEventContext struct {
	Trace       FilterTraceContext       `json:"trace"`
	LocalAgent  FilterLocalAgentContext  `json:"localAgent"`
	Environment FilterEnvironmentContext `json:"environment,omitzero"`
}

// LocalAgentFilterResult is agent-neutral. Obot Sentry maps it to the active agent's hook protocol.
type LocalAgentFilterResult struct {
	Decision       FilterDecision           `json:"decision"`
	Reason         string                   `json:"reason,omitempty"`
	Payload        json.RawMessage          `json:"payload,omitempty"`
	FilterStatuses []LocalAgentFilterStatus `json:"filterStatuses,omitempty"`
	Unenforceable  bool                     `json:"unenforceable,omitempty"`
}

type LocalAgentFilterStatus struct {
	FilterID      string         `json:"filterId"`
	FilterName    string         `json:"filterName,omitempty"`
	Decision      FilterDecision `json:"decision"`
	Reason        string         `json:"reason,omitempty"`
	ErrorCategory string         `json:"errorCategory,omitempty"`
	LatencyMs     int64          `json:"latencyMs,omitempty"`
	Unenforceable bool           `json:"unenforceable,omitempty"`
}

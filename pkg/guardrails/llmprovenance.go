package guardrails

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/provenance"
	"github.com/obot-platform/obot/pkg/provenance/llm"
)

const (
	// provenanceDecisionBudget bounds how long a proposed tool call may be decided, which
	// bounds how long a response is held.
	provenanceDecisionBudget = 3 * time.Second
	// provenanceTitleLimit is how much of the first user message a session's title keeps.
	provenanceTitleLimit = 80
	// provenanceDefaultAgent names the tools' "server" when the agent is unknown.
	provenanceDefaultAgent = "client"
)

// ProvenanceContext is a request's place in the provenance session graph, which Obot's LLM
// proxy binds along with the request. When bound, the gateway plugins feed the session graph
// from the request (the model turn and the tool results it feeds back) and decide every tool
// call the model proposes before the client sees it, blocking the response when Obot Policy
// does not allow one of them.
type ProvenanceContext struct {
	Engine  *provenance.Engine
	Subject provenance.Subject
	// Session is the request's session key. The proxy resolves the session before binding
	// and sets Explicit, so every event of the request lands in the same session.
	Session provenance.SessionKey
	// AuditID is the request's LLM audit log request ID, which events carry as their ref.
	AuditID string

	lock sync.Mutex
	// turnID is the llm.turn event of the request, the parent of what it ingests and decides.
	turnID string
	turn   llm.Turn
}

func (pc *ProvenanceContext) agent() string {
	if a := pc.Subject.Actor.Agent; a != "" {
		return a
	}
	return provenanceDefaultAgent
}

func (pc *ProvenanceContext) ref() provenance.SurfaceRef {
	return provenance.SurfaceRef{LLMAuditLogID: pc.AuditID}
}

// capabilities returns the capability labels of one of the request's tools, labelled under
// the agent's name since the gateway doesn't know which server serves the tool.
func (pc *ProvenanceContext) capabilities(ctx context.Context, tool string, turn llm.Turn) provenance.Labels {
	return pc.Engine.ToolCapabilities(ctx, pc.agent(), tool, turn.ToolDescription(tool))
}

func (pc *ProvenanceContext) current() (string, llm.Turn) {
	pc.lock.Lock()
	defer pc.lock.Unlock()
	return pc.turnID, pc.turn
}

// recordTurn records the model turn a request makes and ingests the tool results the request
// feeds back to the model, as the turn's children. Only the results after the last assistant
// message are new; earlier ones were fed back by earlier requests. It runs once per request.
func (pc *ProvenanceContext) recordTurn(ctx context.Context, req *schemas.BifrostRequest) {
	turn := llm.ExtractTurn(req)
	summary := "model turn " + turn.Model
	if turn.UserMessage != "" && pc.untitled(ctx) {
		// The first turn's user message titles the session.
		summary = truncateRunes(turn.UserMessage, provenanceTitleLimit)
	}
	turnID := ""
	ev, err := pc.Engine.Record(ctx, pc.Subject, pc.Session, provenance.Event{
		Surface: provenance.SurfaceLLM, Kind: provenance.KindLLMTurn, Actor: pc.Subject.Actor,
		Action:  &provenance.Action{Surface: provenance.SurfaceLLM, Kind: provenance.KindLLMTurn, Model: turn.Model, Provider: turn.Provider},
		Ref:     pc.ref(),
		Summary: summary,
	})
	if err != nil {
		slog.Warn("failed to record an LLM turn in the session graph", "error", err)
	} else {
		turnID = ev.ID
	}
	for _, r := range turn.NewResults {
		if _, err := pc.Engine.Ingest(ctx, provenance.IngestRequest{
			Subject: pc.Subject, Session: pc.Session, Surface: provenance.SurfaceLLM, Kind: provenance.KindToolResult,
			ParentID: turnID, Tool: r.Name, Server: pc.agent(), Capabilities: pc.capabilities(ctx, r.Name, turn),
			Content: r.Text, Ref: pc.ref(),
		}); err != nil {
			slog.Warn("failed to ingest a tool result into the session graph", "tool", r.Name, "error", err)
		}
	}
	pc.lock.Lock()
	pc.turnID, pc.turn = turnID, turn
	pc.lock.Unlock()
}

// untitled reports whether the session has no title yet.
func (pc *ProvenanceContext) untitled(ctx context.Context) bool {
	s, err := pc.Engine.Resolve(ctx, pc.Subject, pc.Session, provenance.SurfaceLLM)
	return err == nil && s.Title == ""
}

// quarantined reports whether the session routes untrusted content through quarantine.
func (pc *ProvenanceContext) quarantined(ctx context.Context) bool {
	s, err := pc.Engine.Resolve(ctx, pc.Subject, pc.Session, provenance.SurfaceLLM)
	return err == nil && s.State.Quarantined
}

// quarantine rewrites the untrusted tool results of a request to schema-constrained summaries
// when the session is quarantined, and reports whether it rewrote anything. This is the
// CaMeL-lite hook: once a quarantine policy fires, the model only ever sees untrusted content
// through a summary that drops injection vocabulary.
func (pc *ProvenanceContext) quarantine(ctx context.Context, req *schemas.BifrostRequest) bool {
	if !pc.quarantined(ctx) {
		return false
	}
	_, turn := pc.current()
	n := llm.Quarantine(req, func(tool string) bool {
		return pc.capabilities(ctx, tool, turn).Has(provenance.CapReadsUntrusted)
	})
	return n > 0
}

// decide decides every tool call the model proposes and returns the error that blocks the
// response when one of them is not allowed. Every call is decided, so each is recorded, and
// the first denial is reported. An engine failure blocks, like a guardrail that can't be
// evaluated.
func (pc *ProvenanceContext) decide(ctx context.Context, calls []llm.ProposedCall) *schemas.BifrostError {
	turnID, turn := pc.current()
	var denied *schemas.BifrostError
	for _, c := range calls {
		if c.Name == "" {
			continue
		}
		caps := pc.capabilities(ctx, c.Name, turn)
		action := provenance.Action{
			Surface: provenance.SurfaceLLM, Kind: provenance.KindToolCall, Tool: c.Name, Server: pc.agent(),
			Capabilities: caps, Sink: provenance.SinkFromCapabilities(caps), Source: provenance.SourceFromCapabilities(caps),
			Arguments: llm.ParseArguments(c.ArgumentsJSON),
		}
		if dest := llm.Destination(action.Arguments); dest != "" {
			action.Sink.Destination = dest
			if action.Sink.Writes {
				action.Sink.External = pc.Engine.IsExternal(ctx, dest)
			}
		}
		_, d, err := pc.Engine.Decide(ctx, provenance.DecideRequest{
			Subject: pc.Subject, Session: pc.Session, Action: action, ParentID: turnID, Ref: pc.ref(),
			Budget: provenanceDecisionBudget,
		})
		if err != nil {
			slog.Error("failed to decide a proposed tool call; blocking the response", "tool", c.Name, "error", err)
			if denied == nil {
				denied = guardrailError(fmt.Sprintf("Response blocked: Obot Policy could not decide the tool call %s.", c.Name))
			}
			continue
		}
		if d.Allow || denied != nil {
			continue
		}
		message := fmt.Sprintf("Obot Policy blocked the tool call %s: %s", c.Name, d.Reason)
		if d.ApprovalID != "" {
			message += fmt.Sprintf(" (approval %s pending)", d.ApprovalID)
		}
		denied = guardrailError(message)
	}
	return denied
}

func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit])
}

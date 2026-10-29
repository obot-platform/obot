package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/provenance"
	"github.com/obot-platform/obot/pkg/provenance/copilot"
)

const (
	// copilotResponseDeadline is when the handler answers on its own. Copilot
	// Studio treats no answer within 1,000 ms as approve, so Obot answers
	// approve explicitly and records that it did.
	copilotResponseDeadline = 900 * time.Millisecond
	// copilotDecisionBudget is the decision time shared by the planned
	// actions, leaving room for ingestion and the network.
	copilotDecisionBudget = 800 * time.Millisecond
)

// CopilotStudioHandler is the external threat detection provider Microsoft
// Copilot Studio calls before an agent invokes a tool. It feeds the planner
// context into the conversation's session and decides every planned action
// with Obot Policy.
type CopilotStudioHandler struct {
	engine *provenance.Engine
	// Deadline is when the handler answers approve on its own.
	Deadline time.Duration
	// Budget is the decision time shared by the planned actions.
	Budget time.Duration
	// ResolveUser maps the Copilot Studio user to an Obot user. When nil,
	// an email or UPN in the payload is looked up in the gateway.
	ResolveUser copilot.UserResolver
	seen        *copilot.Dedupe
}

// NewCopilotStudioHandler creates the handler.
func NewCopilotStudioHandler(engine *provenance.Engine) *CopilotStudioHandler {
	return &CopilotStudioHandler{
		engine:   engine,
		Deadline: copilotResponseDeadline,
		Budget:   copilotDecisionBudget,
		seen:     copilot.NewDedupe(8192),
	}
}

// Validate answers Copilot Studio's readiness check (POST /validate).
func (h *CopilotStudioHandler) Validate(req api.Context) error {
	if h.engine == nil {
		return req.WriteCode(copilot.ErrorResponse{ErrorCode: copilot.ErrorCodeUnavailable, Message: "Obot Policy is not available", HTTPStatus: http.StatusServiceUnavailable}, http.StatusServiceUnavailable)
	}
	return req.Write(copilot.ValidateResponse{IsSuccessful: true, Status: "OK"})
}

// RuntimeMonitor answers one analyze-tool-execution call. It always answers
// within the deadline: a decision that does not fit is approved, logged and
// marked as timed out in the diagnostics.
func (h *CopilotStudioHandler) RuntimeMonitor(req api.Context) error {
	if h.engine == nil {
		return req.WriteCode(copilot.ErrorResponse{ErrorCode: copilot.ErrorCodeUnavailable, Message: "Obot Policy is not available", HTTPStatus: http.StatusServiceUnavailable}, http.StatusServiceUnavailable)
	}
	var cr copilot.Request
	if err := req.Read(&cr); err != nil {
		return req.WriteCode(copilot.ErrorResponse{ErrorCode: copilot.ErrorCodeInvalidRequest, Message: "Invalid request body: " + err.Error(), HTTPStatus: http.StatusBadRequest}, http.StatusBadRequest)
	}
	if len(cr.Actions) == 0 {
		return req.WriteCode(copilot.ErrorResponse{ErrorCode: copilot.ErrorCodeMissingField, Message: "Missing required field: toolDefinition", HTTPStatus: http.StatusBadRequest}, http.StatusBadRequest)
	}
	if cr.Correlation == "" {
		cr.Correlation = req.Request.Header.Get("x-ms-correlation-id")
	}
	resolver := h.ResolveUser
	if resolver == nil && req.GatewayClient != nil {
		resolver = gatewayUserResolver(req.GatewayClient)
	}

	deadline := h.Deadline
	if deadline <= 0 {
		deadline = copilotResponseDeadline
	}
	ctx, cancel := context.WithTimeout(req.Context(), deadline)
	defer cancel()
	results := make(chan copilot.Result, 1)
	go func() { results <- h.evaluate(ctx, &cr, resolver) }()
	select {
	case result := <-results:
		return req.Write(result.Response())
	case <-ctx.Done():
		slog.WarnContext(req.Context(), "Copilot Studio runtime monitor could not decide in time; approving",
			"conversation", cr.ConversationID, "agent", cr.AgentID, "actions", len(cr.Actions), "correlation", cr.Correlation)
		return req.Write(copilot.Result{TimedOut: true}.Response())
	}
}

// evaluate feeds the planner context into the session and decides the
// planned actions.
func (h *CopilotStudioHandler) evaluate(ctx context.Context, cr *copilot.Request, resolver copilot.UserResolver) copilot.Result {
	userID := copilot.ResolveUserID(ctx, resolver, cr)
	subject, session := copilot.Subject(cr, userID), copilot.Session(cr)
	scope := cr.ConversationID
	var result copilot.Result
	parentID := ""

	// The user's message is a model turn: it is scanned (a pasted secret is
	// lineage too) and titles the session.
	if cr.Prompt != "" && !h.seen.Seen(scope, "prompt", cr.Prompt) {
		summary := copilot.Summary(cr.Prompt)
		ev, err := h.engine.Ingest(ctx, provenance.IngestRequest{
			Subject: subject, Session: session, Surface: provenance.SurfaceCopilot, Kind: provenance.KindLLMTurn,
			Content: cr.Prompt, Summary: summary,
		})
		if err != nil {
			slog.WarnContext(ctx, "Copilot Studio prompt could not be recorded", "conversation", scope, "error", err)
		} else {
			parentID = ev.ID
			if s, err := h.engine.Resolve(ctx, subject, session, provenance.SurfaceCopilot); err == nil && s.Title == "" {
				if err := h.engine.SetTitle(ctx, s.ID, summary); err != nil {
					slog.DebugContext(ctx, "Copilot Studio session title", "error", err)
				}
			}
		}
	}

	// Previous tool outputs and tool messages in the history are content that
	// entered the agent's context; each enters the session once.
	ingestResult := func(tool, content string) {
		if content == "" || h.seen.Seen(scope, "tool", tool+"\x00"+content) {
			return
		}
		server := copilot.ServerName(cr, copilot.PlanAction{})
		_, err := h.engine.Ingest(ctx, provenance.IngestRequest{
			Subject: subject, Session: session, Surface: provenance.SurfaceCopilot, Kind: provenance.KindToolResult,
			ParentID: parentID, Tool: tool, Server: server,
			Capabilities: copilot.ToolResultCapabilities(ctx, h.engine, server, tool),
			Content:      content,
		})
		if err != nil {
			slog.WarnContext(ctx, "Copilot Studio tool output could not be ingested", "conversation", scope, "tool", tool, "error", err)
		}
	}
	for _, out := range cr.ToolOutputs {
		ingestResult(out.ToolName, out.Content)
	}
	for _, msg := range cr.History {
		switch msg.Role {
		case "tool", "function", "tool_result":
			ingestResult(msg.Name, msg.Content)
		}
	}

	actions := copilot.BuildActions(ctx, h.engine, cr)
	budget := h.Budget
	if budget <= 0 {
		budget = copilotDecisionBudget
	}
	perAction := budget / time.Duration(len(actions))
	for _, action := range actions {
		ev, decision, err := h.engine.Decide(ctx, provenance.DecideRequest{
			Subject: subject, Session: session, Action: action, ParentID: parentID, Budget: perAction,
		})
		if err != nil {
			slog.WarnContext(ctx, "Obot Policy could not decide a Copilot Studio action; approving", "conversation", scope, "tool", action.Tool, "error", err)
			result.Error = err.Error()
			result.Actions = append(result.Actions, copilot.ActionDecision{Name: action.Tool, Server: action.Server, Decision: copilot.DecisionApprove, Reason: "Obot Policy could not decide: " + err.Error()})
			continue
		}
		result.SessionID = ev.SessionID
		result.Actions = append(result.Actions, copilot.DecisionFor(action, ev, decision))
	}
	return result
}

// gatewayUserResolver looks the user up by email or UPN. The documented
// payload carries only the Entra object ID; mapping that to an Obot user
// needs the Entra auth provider and is left to ResolveUser.
func gatewayUserResolver(client *gclient.Client) copilot.UserResolver {
	return func(ctx context.Context, _, email string) (string, error) {
		if email == "" {
			return "", nil
		}
		users, err := client.Users(ctx, gtypes.UserQuery{Email: email})
		if err != nil || len(users) == 0 {
			return "", err
		}
		return strconv.FormatUint(uint64(users[0].ID), 10), nil
	}
}

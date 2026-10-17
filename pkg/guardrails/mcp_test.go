package guardrails

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func secretsFilter(action, direction string) MCPFilter {
	return MCPFilter{ID: "mwv1secrets", Name: "No secrets", Config: apitypes.MCPGuardrailFilterConfig{
		GuardrailIDs: []string{"secrets"}, Action: action, Direction: direction, SamplingRate: 100,
	}}
}

func secretsEngine(t *testing.T) (*Engine, *fakeStore) {
	t.Helper()
	store := &fakeStore{guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)}}
	return New(store, nil), store
}

func TestMCPFilterRedactsToolCallArguments(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter(apitypes.MCPGuardrailActionRedact, "")
	f.Config.Condition = `mcp_tool == "send" && mcp_method == "tools/call"`
	msg := MCPMessage{
		Identity: Identity{UserID: "u"}, Server: "mail", Method: MCPMethodToolsCall, Name: "send", Stage: StageInput,
		Params: json.RawMessage(`{"name":"send","arguments":{"to":"bob","body":{"lines":["key ` + leakedKey + `"]},"n":1.50}}`),
	}

	d := e.CheckMCPFilter(t.Context(), f, msg)
	if !d.Applied || d.Blocked || d.Params == nil {
		t.Fatalf("expected redacted params, got %+v", d)
	}
	params := string(d.Params)
	if strings.Contains(params, leakedKey) || !strings.Contains(params, "[REDACTED:AWS_ACCESS_KEY_ID]") || !strings.Contains(params, `"n":1.50`) || !strings.Contains(params, `"name":"send"`) {
		t.Fatalf("unexpected params %s", params)
	}

	// The condition limits the filter to one tool.
	msg.Name = "read"
	if d := e.CheckMCPFilter(t.Context(), f, msg); d.Applied || d.Params != nil {
		t.Fatalf("filter should not apply to other tools: %+v", d)
	}
}

func TestMCPFilterToolResult(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter(apitypes.MCPGuardrailActionRedact, apitypes.MCPGuardrailDirectionResponse)
	msg := MCPMessage{
		Identity: Identity{UserID: "u"}, Server: "files", Method: MCPMethodToolsCall, Name: "read", Stage: StageOutput,
		Params: json.RawMessage(`{"name":"read"}`),
		Result: json.RawMessage(`{"content":[{"type":"text","text":"found ` + leakedKey + `"},{"type":"image","data":"` + leakedKey + `"}],"structuredContent":{"key":"` + leakedKey + `"}}`),
	}

	d := e.CheckMCPFilter(t.Context(), f, msg)
	if d.Result == nil {
		t.Fatalf("expected a redacted result, got %+v", d)
	}
	var out struct {
		Content []struct {
			Text string `json:"text"`
			Data string `json:"data"`
		} `json:"content"`
		StructuredContent map[string]string `json:"structuredContent"`
	}
	if err := json.Unmarshal(d.Result, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.Content[0].Text, leakedKey) || strings.Contains(out.StructuredContent["key"], leakedKey) {
		t.Fatalf("result not redacted: %s", d.Result)
	}
	// Binary content isn't text and is left alone.
	if out.Content[1].Data != leakedKey {
		t.Fatalf("image data should be untouched: %s", d.Result)
	}

	// A response-only filter doesn't check requests.
	msg.Stage = StageInput
	msg.Params = json.RawMessage(`{"name":"read","arguments":{"text":"` + leakedKey + `"}}`)
	if d := e.CheckMCPFilter(t.Context(), f, msg); d.Applied {
		t.Fatalf("response filter applied to a request: %+v", d)
	}

	f = secretsFilter(apitypes.MCPGuardrailActionBlock, "")
	f.Config.BlockMessage = "no secrets please"
	if d := e.CheckMCPFilter(t.Context(), f, msg); !d.Blocked || d.Message != "no secrets please" {
		t.Fatalf("expected the tool call to be blocked, got %+v", d)
	}
}

func TestMCPFilterResourcesAndPrompts(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter(apitypes.MCPGuardrailActionRedact, "")

	d := e.CheckMCPFilter(t.Context(), f, MCPMessage{
		Method: MCPMethodResourcesRead, Name: "file:///keys", Stage: StageOutput,
		Params: json.RawMessage(`{"uri":"file:///keys"}`),
		Result: json.RawMessage(`{"contents":[{"uri":"file:///keys","text":"key ` + leakedKey + `"},{"uri":"file:///bin","blob":"` + leakedKey + `"}]}`),
	})
	if d.Result == nil || strings.Count(string(d.Result), leakedKey) != 1 || !strings.Contains(string(d.Result), "REDACTED") {
		t.Fatalf("resource text not redacted (blob kept): %+v %s", d, d.Result)
	}

	d = e.CheckMCPFilter(t.Context(), f, MCPMessage{
		Method: MCPMethodPromptsGet, Name: "review", Stage: StageInput,
		Params: json.RawMessage(`{"name":"review","arguments":{"code":"` + leakedKey + `"}}`),
	})
	if d.Params == nil || strings.Contains(string(d.Params), leakedKey) {
		t.Fatalf("prompt arguments not redacted: %+v %s", d, d.Params)
	}

	d = e.CheckMCPFilter(t.Context(), f, MCPMessage{
		Method: MCPMethodPromptsGet, Name: "review", Stage: StageOutput,
		Params: json.RawMessage(`{"name":"review"}`),
		Result: json.RawMessage(`{"messages":[{"role":"user","content":{"type":"text","text":"use ` + leakedKey + `"}},{"role":"user","content":{"type":"resource","resource":{"uri":"x","text":"` + leakedKey + `"}}}]}`),
	})
	if d.Result == nil || strings.Contains(string(d.Result), leakedKey) {
		t.Fatalf("prompt messages not redacted: %+v %s", d, d.Result)
	}
}

func TestMCPFilterMissingGuardrail(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter(apitypes.MCPGuardrailActionBlock, "")
	f.Config.GuardrailIDs = []string{"gone"}
	msg := MCPMessage{Method: MCPMethodToolsCall, Name: "send", Stage: StageInput, Params: json.RawMessage(`{"name":"send","arguments":{"a":"b"}}`)}

	// Fail-open: the message passes, and the decision says which guardrail failed and why.
	d := e.CheckMCPFilter(t.Context(), f, msg)
	if d.Blocked {
		t.Fatalf("a fail-open filter should not block: %+v", d)
	}
	want := `guardrail "gone" failed: guardrail gone does not exist; allowed because the filter fails open`
	if len(d.Errors) != 1 || d.Errors[0] != want {
		t.Fatalf("errors = %q, want [%q]", d.Errors, want)
	}

	// Fail-closed: the block message explains it, so there's nothing to add.
	f.Config.FailClosed = true
	if d := e.CheckMCPFilter(t.Context(), f, msg); !d.Blocked || len(d.Errors) != 0 {
		t.Fatalf("a fail-closed filter should block: %+v", d)
	}
}

// Filters only block or redact. A log-only filter saved before that changed blocks.
func TestMCPFilterLogActionBlocks(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter("log", "")
	msg := MCPMessage{Method: MCPMethodToolsCall, Name: "send", Stage: StageInput, Params: json.RawMessage(`{"name":"send","arguments":{"a":"` + leakedKey + `"}}`)}
	if d := e.CheckMCPFilter(t.Context(), f, msg); !d.Blocked {
		t.Fatalf("a log-only filter should block: %+v", d)
	}
}

func TestMCPFilterBlocksWhenGuardrailsCannotBeLoaded(t *testing.T) {
	e, store := secretsEngine(t)
	store.loadErr = errors.New("database unavailable")
	e.Invalidate()
	f := secretsFilter(apitypes.MCPGuardrailActionBlock, "")
	msg := MCPMessage{Method: MCPMethodToolsCall, Name: "send", Stage: StageInput, Params: json.RawMessage(`{"name":"send","arguments":{"a":"b"}}`)}
	if d := e.CheckMCPFilter(t.Context(), f, msg); !d.Blocked {
		t.Fatalf("a filter that can't load its guardrails must block, even when fail-open: %+v", d)
	}
}

func TestMCPFilterSubjects(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter(apitypes.MCPGuardrailActionBlock, "")
	f.Config.Subjects = []apitypes.MCPGuardrailFilterSubject{{Type: apitypes.MCPGuardrailSubjectGroup, ID: "eng"}}
	msg := MCPMessage{Method: MCPMethodToolsCall, Name: "send", Stage: StageInput, Params: json.RawMessage(`{"name":"send","arguments":{"a":"` + leakedKey + `"}}`)}

	msg.Identity = Identity{UserID: "u", Groups: []string{"sales"}}
	if d := e.CheckMCPFilter(t.Context(), f, msg); d.Applied {
		t.Fatalf("filter applied to a user outside its subjects: %+v", d)
	}
	msg.Identity.Groups = []string{"eng"}
	if d := e.CheckMCPFilter(t.Context(), f, msg); !d.Blocked {
		t.Fatalf("filter should block its subjects: %+v", d)
	}
}

func TestMCPFilterIgnoresMessagesWithoutContent(t *testing.T) {
	e, _ := secretsEngine(t)
	f := secretsFilter(apitypes.MCPGuardrailActionBlock, `mcp_arguments.x == "y"`)
	f.Config.FailClosed = true
	for _, msg := range []MCPMessage{
		{Method: "initialize", Stage: StageInput, Params: json.RawMessage(`{"protocolVersion":"2025-06-18"}`)},
		{Method: "tools/list", Stage: StageOutput, Result: json.RawMessage(`{"tools":[]}`)},
		{Method: MCPMethodResourcesRead, Stage: StageInput, Params: json.RawMessage(`{"uri":"file:///a"}`)},
		{Method: "tools/list", Stage: StageInput, Params: json.RawMessage(`["not","an","object"]`)},
	} {
		if d := e.CheckMCPFilter(t.Context(), f, msg); d.Applied || d.Blocked {
			t.Fatalf("%s %s must pass a guardrail filter untouched: %+v", msg.Method, msg.Stage, d)
		}
	}
}

func TestResolveIdentityAdmin(t *testing.T) {
	tests := []struct {
		name  string
		user  kuser.Info
		admin bool
	}{
		{
			name:  "admin session",
			user:  &kuser.DefaultInfo{UID: "1", Groups: []string{apitypes.GroupAdmin}},
			admin: true,
		},
		{
			name: "LLM-only API key of an admin",
			user: &kuser.DefaultInfo{UID: "1", Groups: []string{apitypes.GroupLLM}, Extra: map[string][]string{
				"obot_groups": {apitypes.GroupAdmin, apitypes.GroupBasic},
			}},
			admin: true,
		},
		{
			name: "LLM-only API key of the owner",
			user: &kuser.DefaultInfo{UID: "1", Groups: []string{apitypes.GroupLLM}, Extra: map[string][]string{
				"obot_groups": {apitypes.GroupOwner},
			}},
			admin: true,
		},
		{
			name: "LLM-only API key of a basic user",
			user: &kuser.DefaultInfo{UID: "4", Groups: []string{apitypes.GroupLLM}, Extra: map[string][]string{
				"obot_groups": {apitypes.GroupBasic},
			}},
			admin: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, admin, err := ResolveIdentity(t.Context(), nil, tt.user)
			if err != nil {
				t.Fatal(err)
			}
			if admin != tt.admin {
				t.Fatalf("admin = %v, want %v", admin, tt.admin)
			}
		})
	}
}

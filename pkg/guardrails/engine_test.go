package guardrails

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	// testProfile is the gateway profile test policies belong to, and that test callers are in
	// unless a test says otherwise.
	testProfile = "everyone"
)

var (
	countingLock  sync.Mutex
	countingCalls = map[string]*[][]string{}
)

type fakeStore struct {
	policies   []types.GuardrailPolicy
	guardrails []types.Guardrail
	secrets    map[string]map[string]string
	// loadErr makes listing policies and guardrails fail, like an unreachable database.
	loadErr error
}

// countingChecker records every call, so tests can tell how often a guardrail ran and on what.
// In "span" mode it finds the word "secret" and says where; in "flag" mode it flags
// everything without saying where; in "fail" mode it errors.
type countingChecker struct {
	mode  string
	calls *[][]string
	lock  *sync.Mutex
}

// misplacedChecker reports a finding whose span doesn't fit the text, like a service whose
// offsets were misread.
type misplacedChecker struct{}

// failingChecker always errors, to exercise fail-open and fail-closed behavior.
type failingChecker struct{}

// flaggingChecker flags everything without saying where, like a content classifier.
type flaggingChecker struct{}

func init() {
	Register(ProviderInfo{Type: "test-failing"}, func(map[string]any, map[string]string, Deps) (Checker, error) {
		return failingChecker{}, nil
	})
	Register(ProviderInfo{Type: "test-misplaced", Redacts: true}, func(map[string]any, map[string]string, Deps) (Checker, error) {
		return misplacedChecker{}, nil
	})
	Register(ProviderInfo{Type: "test-flagging"}, func(map[string]any, map[string]string, Deps) (Checker, error) {
		return flaggingChecker{}, nil
	})
	Register(ProviderInfo{Type: "test-counting", Redacts: true}, func(config map[string]any, _ map[string]string, _ Deps) (Checker, error) {
		key, _ := config["key"].(string)
		mode, _ := config["mode"].(string)
		countingLock.Lock()
		defer countingLock.Unlock()
		calls, ok := countingCalls[key]
		if !ok {
			calls = &[][]string{}
			countingCalls[key] = calls
		}
		return countingChecker{mode: mode, calls: calls, lock: &countingLock}, nil
	})
}

func (f *fakeStore) ListGuardrailPolicies(context.Context) ([]types.GuardrailPolicy, error) {
	return f.policies, f.loadErr
}

func (f *fakeStore) ListGuardrails(context.Context) ([]types.Guardrail, error) {
	return f.guardrails, f.loadErr
}

func (f *fakeStore) GuardrailSecrets(_ context.Context, id string) (map[string]string, error) {
	return f.secrets[id], nil
}

// outcomes returns the outcome of each of a decision's detections.
func outcomes(d Decision) []string {
	var out []string
	for _, det := range d.Detections {
		out = append(out, det.Outcome)
	}
	return out
}

func guardrail(id, typ string, config map[string]any) types.Guardrail {
	raw, _ := json.Marshal(config)
	return types.Guardrail{ID: id, Name: id, Type: typ, Config: raw}
}

func policy(name, action string, guardrails ...string) types.GuardrailPolicy {
	return types.GuardrailPolicy{
		ID: name, Name: name, Enabled: true, Target: types.GuardrailTargetLLM,
		ApplyTo: types.GuardrailApplyBoth, Action: action, SamplingRate: 100, GuardrailIDs: guardrails,
		ProfileID: testProfile,
	}
}

func (failingChecker) Check(context.Context, CheckInput) (Result, error) {
	return Result{}, errors.New("service unavailable")
}

func (misplacedChecker) Check(_ context.Context, in CheckInput) (Result, error) {
	end := len(in.Segments[0].Text) + 10
	return Result{Findings: []Finding{{Category: "pii", Spans: []Span{{Segment: 0, Start: 2, End: end, Label: "PII"}}}}}, nil
}

func (flaggingChecker) Check(context.Context, CheckInput) (Result, error) {
	return Result{Findings: []Finding{{Category: "harmful", Message: "unsafe content"}}}, nil
}

func evaluate(t *testing.T, store *fakeStore, identity Identity, vars map[string]any, segments ...Segment) Decision {
	t.Helper()
	if identity.ProfileIDs == nil {
		identity.ProfileIDs = []string{testProfile}
	}
	e := New(store, nil)
	plan, err := e.Plan(t.Context(), types.GuardrailTargetLLM, StageInput, identity, vars)
	if err != nil {
		t.Fatal(err)
	}
	return e.Evaluate(t.Context(), plan, Request{Target: types.GuardrailTargetLLM, Stage: StageInput, Identity: identity, Vars: vars, Segments: segments})
}

func TestRedactsSecretsAndPII(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{
			guardrail("secrets", "secrets", nil),
			guardrail("pii", "regex", map[string]any{"piiTemplate": []any{"EMAIL", "CREDIT_CARD"}}),
		},
		policies: []types.GuardrailPolicy{policy("redact", types.GuardrailActionRedact, "secrets", "pii")},
	}
	d := evaluate(t, store, Identity{UserID: "u1"}, nil,
		Segment{Role: RoleUser, Text: "my key is AKIAIOSFODNN7EXAMPLE and email bob@example.com"},
		Segment{Role: RoleAssistant, Text: "card 4111 1111 1111 1111, not 1234 5678 9012 3456"},
	)
	if d.Blocked || !d.Redacted {
		t.Fatalf("expected redaction, got %+v", d)
	}
	if got := d.Segments[0].Text; got != "my key is [REDACTED:AWS_ACCESS_KEY_ID] and email [REDACTED:EMAIL]" {
		t.Fatalf("unexpected redaction %q", got)
	}
	if got := d.Segments[1].Text; got != "card [REDACTED:CREDIT_CARD], not 1234 5678 9012 3456" {
		t.Fatalf("a Luhn-invalid number must not be redacted: %q", got)
	}
	if o := outcomes(d); len(o) != 2 || o[0] != "redacted" {
		t.Fatalf("unexpected detections %v", o)
	}
}

func TestBlockAndConditions(t *testing.T) {
	block := policy("no-secrets-on-gpt", types.GuardrailActionBlock, "secrets")
	block.Condition = `model.startsWith("openai/") && !("security" in user.groups)`
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{block},
	}
	secret := Segment{Role: RoleUser, Text: "token ghp_abcdefghijklmnopqrstuvwxyz0123456789"}

	d := evaluate(t, store, Identity{UserID: "u1"}, map[string]any{"model": "openai/gpt-4.1"}, secret)
	if !d.Blocked || !strings.Contains(d.Message, "GITHUB_TOKEN") {
		t.Fatalf("expected a block naming the finding, got %+v", d)
	}
	if d := evaluate(t, store, Identity{UserID: "u1"}, map[string]any{"model": "anthropic/claude"}, secret); d.Blocked {
		t.Fatal("the condition should exclude other providers")
	}
	if d := evaluate(t, store, Identity{UserID: "u1", Groups: []string{"security"}}, map[string]any{"model": "openai/gpt-4.1"}, secret); d.Blocked {
		t.Fatal("the condition should exclude the security group")
	}
}

func TestSubjectsScopeAndFailureModes(t *testing.T) {
	scoped := policy("sales-only", types.GuardrailActionBlock, "flag")
	scoped.ProfileID = "sales"
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("flag", "test-flagging", nil)},
		policies:   []types.GuardrailPolicy{scoped},
	}
	msg := Segment{Role: RoleUser, Text: "hello"}
	if d := evaluate(t, store, Identity{UserID: "bob", ProfileIDs: []string{"everyone"}}, nil, msg); d.Blocked {
		t.Fatal("policy must only apply to its profile")
	}
	if d := evaluate(t, store, Identity{UserID: "alice", ProfileIDs: []string{"everyone", "sales"}}, nil, msg); !d.Blocked {
		t.Fatal("policy must apply to callers in its profile")
	}

	// A policy without a profile applies to no one, rather than to everyone.
	orphan := policy("orphan", types.GuardrailActionBlock, "flag")
	orphan.ProfileID = ""
	store.policies = []types.GuardrailPolicy{orphan}
	if d := evaluate(t, store, Identity{UserID: "alice", ProfileIDs: []string{"everyone", ""}}, nil, msg); d.Blocked {
		t.Fatal("a policy without a profile must not apply")
	}

	// A redacting policy whose checker can't locate findings blocks instead.
	store.policies = []types.GuardrailPolicy{policy("redact-unlocatable", types.GuardrailActionRedact, "flag")}
	if d := evaluate(t, store, Identity{UserID: "bob"}, nil, msg); !d.Blocked {
		t.Fatal("unlocatable findings must block a redacting policy")
	}

	// Checker errors fail open unless the policy fails closed.
	store.guardrails = []types.Guardrail{guardrail("down", "test-failing", nil)}
	open := policy("open", types.GuardrailActionBlock, "down")
	store.policies = []types.GuardrailPolicy{open}
	if d := evaluate(t, store, Identity{UserID: "bob"}, nil, msg); d.Blocked {
		t.Fatal("a failing guardrail must not block a fail-open policy")
	}
	closed := open
	closed.FailClosed = true
	store.policies = []types.GuardrailPolicy{closed}
	if d := evaluate(t, store, Identity{UserID: "bob"}, nil, msg); !d.Blocked {
		t.Fatal("a failing guardrail must block a fail-closed policy")
	}
}

func TestLatestScope(t *testing.T) {
	r := policy("latest", types.GuardrailActionBlock, "secrets")
	r.InputScope = types.GuardrailScopeLatest
	store := &fakeStore{guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)}, policies: []types.GuardrailPolicy{r}}
	old := Segment{Role: RoleUser, Text: "AKIAIOSFODNN7EXAMPLE"}
	if d := evaluate(t, store, Identity{}, nil, old, Segment{Role: RoleAssistant, Text: "ok"}, Segment{Role: RoleUser, Text: "thanks"}); d.Blocked {
		t.Fatal("latest scope must ignore messages before the model's last reply")
	}
}

func TestCompileCondition(t *testing.T) {
	if _, err := CompileCondition(types.GuardrailTargetLLM, `mcp_tool == "x"`); err == nil {
		t.Fatal("LLM policies must not see MCP variables")
	}
	if _, err := CompileCondition(types.GuardrailTargetMCP, `mcp_tool == "delete_repo" && user.id != ""`); err != nil {
		t.Fatal(err)
	}
	if _, err := CompileCondition(types.GuardrailTargetLLM, `model`); err == nil {
		t.Fatal("non-boolean conditions must be rejected")
	}
}

func (f *fakeStore) GetGuardrailPolicy(_ context.Context, id string) (*types.GuardrailPolicy, error) {
	for _, r := range f.policies {
		if r.ID == id {
			return &r, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeStore) GetGuardrail(_ context.Context, id string) (*types.Guardrail, error) {
	for _, p := range f.guardrails {
		if p.ID == id {
			return &p, nil
		}
	}
	return nil, errors.New("not found")
}

func (f *fakeStore) CreateGuardrail(_ context.Context, p *types.Guardrail, _ map[string]string) error {
	f.guardrails = append(f.guardrails, *p)
	return nil
}

func (f *fakeStore) CreateGuardrailPolicy(_ context.Context, r *types.GuardrailPolicy) error {
	f.policies = append(f.policies, *r)
	return nil
}

func TestRedactionThatCannotBeAppliedBlocks(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("p", "test-misplaced", nil)},
		policies:   []types.GuardrailPolicy{policy("redact", types.GuardrailActionRedact, "p")},
	}
	d := evaluate(t, store, Identity{UserID: "u1"}, nil, Segment{Role: RoleUser, Text: "hello"})
	if !d.Blocked || d.Redacted {
		t.Fatalf("a span outside the text must block, not claim a redaction: %+v", d)
	}
	if got := outcomes(d); len(got) != 1 || got[0] != "blocked" {
		t.Fatalf("unexpected detections %v", got)
	}
}

func TestConditionRuntimeErrors(t *testing.T) {
	broken := policy("needs-header", types.GuardrailActionBlock, "secrets")
	broken.Condition = `headers["x-team"] == "a"`
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{broken},
	}
	vars := map[string]any{"headers": map[string]string{}}

	// Fail-open: the policy is skipped.
	d := evaluate(t, store, Identity{UserID: "u1"}, vars, Segment{Role: RoleUser, Text: "hello"})
	if d.Blocked || len(d.Detections) != 0 {
		t.Fatalf("a fail-open policy whose condition fails must be skipped: %+v", d)
	}

	// Fail-closed: the request is blocked, and the detection says why.
	store.policies[0].FailClosed = true
	d = evaluate(t, store, Identity{UserID: "u1"}, vars, Segment{Role: RoleUser, Text: "hello"})
	if !d.Blocked || !strings.HasPrefix(d.Message, "Request blocked") || !strings.Contains(d.Message, "needs-header") {
		t.Fatalf("a fail-closed policy whose condition fails must block: %+v", d)
	}
	if len(d.Detections) != 1 || d.Detections[0].Outcome != "error" || !strings.Contains(d.Detections[0].Error, "x-team") {
		t.Fatalf("the detection must say why the condition failed: %+v", d.Detections)
	}

	// On output, it is the response that is blocked.
	e := New(store, nil)
	identity := Identity{UserID: "u1", ProfileIDs: []string{testProfile}}
	plan, err := e.Plan(t.Context(), types.GuardrailTargetLLM, StageOutput, identity, vars)
	if err != nil {
		t.Fatal(err)
	}
	d = e.Evaluate(t.Context(), plan, Request{Target: types.GuardrailTargetLLM, Stage: StageOutput, Identity: identity, Vars: vars, Segments: []Segment{{Role: RoleAssistant, Text: "hi"}}})
	if !d.Blocked || !strings.HasPrefix(d.Message, "Response blocked") {
		t.Fatalf("a blocked response must say so: %+v", d)
	}
}

func TestFindingsOnDeliveredContentAreErrors(t *testing.T) {
	store := &fakeStore{
		guardrails: []types.Guardrail{guardrail("secrets", "secrets", nil)},
		policies:   []types.GuardrailPolicy{policy("redact", types.GuardrailActionRedact, "secrets")},
	}
	e := New(store, nil)
	plan, err := e.Plan(t.Context(), types.GuardrailTargetLLM, StageOutput, Identity{UserID: "u1", ProfileIDs: []string{testProfile}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	d := e.Evaluate(t.Context(), plan, Request{
		Target: types.GuardrailTargetLLM, Stage: StageOutput, Identity: Identity{UserID: "u1"}, Delivered: true,
		Segments: []Segment{{Role: RoleAssistant, Text: "key AKIAIOSFODNN7EXAMPLE"}},
	})
	if !d.Blocked || d.Redacted {
		t.Fatalf("content already delivered can't be redacted; the response must end: %+v", d)
	}
	if got := outcomes(d); len(got) != 1 || got[0] != "error" {
		t.Fatalf("expected an error detection rather than a redaction, got %v", got)
	}
}

func TestJudgeExplanationIsShownButNotStored(t *testing.T) {
	findings := []Finding{{Category: "policy", Message: "Violates the policy", Explanation: "Your message mentions AKIA..."}}
	r := &compiledPolicy{policy: types.GuardrailPolicy{Name: "judge"}}
	if msg := blockMessage(r, findings); !strings.Contains(msg, "Your message mentions") {
		t.Fatalf("the caller should see the explanation: %s", msg)
	}
	raw, _ := json.Marshal(findings)
	if strings.Contains(string(raw), "AKIA") {
		t.Fatalf("the explanation must not be stored: %s", raw)
	}
}

func TestBlockMessagePunctuation(t *testing.T) {
	r := &compiledPolicy{policy: types.GuardrailPolicy{Name: "No cooking"}}
	if got := blockMessage(r, []Finding{{Explanation: "Recipes aren't allowed here."}}); strings.HasSuffix(got, "..") {
		t.Fatalf("double period: %q", got)
	}
	if got := blockMessage(r, []Finding{{Category: "policy"}}); !strings.HasSuffix(got, "policy.") {
		t.Fatalf("missing period: %q", got)
	}
}

// A policy whose guardrail was deleted still applies: it fails like a guardrail error, so a
// fail-closed policy blocks and a fail-open one lets the request through.
func TestPolicyWithMissingGuardrailFollowsItsFailureMode(t *testing.T) {
	msg := Segment{Role: RoleUser, Text: "hello"}
	closed := policy("closed", types.GuardrailActionBlock, "deleted")
	closed.FailClosed = true
	store := &fakeStore{policies: []types.GuardrailPolicy{closed}}
	if d := evaluate(t, store, Identity{UserID: "u1"}, nil, msg); !d.Blocked {
		t.Fatal("a fail-closed policy with a deleted guardrail let the request through")
	}

	open := policy("open", types.GuardrailActionBlock, "deleted")
	store = &fakeStore{policies: []types.GuardrailPolicy{open}}
	if d := evaluate(t, store, Identity{UserID: "u1"}, nil, msg); d.Blocked {
		t.Fatal("a fail-open policy with a deleted guardrail blocked the request")
	}
}

func (c countingChecker) Check(_ context.Context, in CheckInput) (Result, error) {
	var texts []string
	for _, s := range in.Segments {
		texts = append(texts, s.Text)
	}
	c.lock.Lock()
	*c.calls = append(*c.calls, texts)
	c.lock.Unlock()

	switch c.mode {
	case "fail":
		return Result{}, errors.New("service unavailable")
	case "flag":
		return Result{Findings: []Finding{{Category: "harmful"}}}, nil
	}
	var result Result
	for i, s := range in.Segments {
		if start := strings.Index(s.Text, "secret"); start >= 0 {
			result.Findings = append(result.Findings, Finding{
				Category: "secret",
				Spans:    []Span{{Segment: i, Start: start, End: start + len("secret"), Label: "secret"}},
			})
		}
	}
	return result, nil
}

func TestSharedGuardrailRunsOncePerInput(t *testing.T) {
	scoped := func(p types.GuardrailPolicy, scope string) types.GuardrailPolicy {
		p.InputScope = scope
		return p
	}
	failClosed := func(p types.GuardrailPolicy) types.GuardrailPolicy {
		p.FailClosed = true
		return p
	}
	conversation := []Segment{
		{Role: RoleUser, Text: "first"},
		{Role: RoleAssistant, Text: "reply"},
		{Role: RoleUser, Text: "second"},
	}

	tests := []struct {
		name           string
		mode           string
		policies       []types.GuardrailPolicy
		segments       []Segment
		wantCalls      [][]string
		wantDetections []string
		wantBlocked    bool
		wantMessage    string
	}{
		{
			name: "two policies share a guardrail",
			mode: "flag",
			policies: []types.GuardrailPolicy{
				policy("a", types.GuardrailActionLog, "g"),
				policy("b", types.GuardrailActionLog, "g"),
			},
			segments:       []Segment{{Role: RoleUser, Text: "hello"}},
			wantCalls:      [][]string{{"hello"}},
			wantDetections: []string{"a:logged", "b:logged"},
		},
		{
			name: "a redaction between them changes the input",
			mode: "span",
			policies: []types.GuardrailPolicy{
				policy("a", types.GuardrailActionRedact, "g"),
				policy("b", types.GuardrailActionBlock, "g"),
			},
			segments:       []Segment{{Role: RoleUser, Text: "my secret"}},
			wantCalls:      [][]string{{"my secret"}, {"my [REDACTED:SECRET]"}},
			wantDetections: []string{"a:redacted"},
		},
		{
			name: "different input scopes see different content",
			mode: "flag",
			policies: []types.GuardrailPolicy{
				scoped(policy("a", types.GuardrailActionLog, "g"), types.GuardrailScopeAll),
				scoped(policy("b", types.GuardrailActionLog, "g"), types.GuardrailScopeLatest),
			},
			segments:       conversation,
			wantCalls:      [][]string{{"first", "reply", "second"}, {"second"}},
			wantDetections: []string{"a:logged", "b:logged"},
		},
		{
			name: "the same input scope shares a call",
			mode: "flag",
			policies: []types.GuardrailPolicy{
				scoped(policy("a", types.GuardrailActionLog, "g"), types.GuardrailScopeLatest),
				scoped(policy("b", types.GuardrailActionLog, "g"), types.GuardrailScopeLatest),
			},
			segments:       conversation,
			wantCalls:      [][]string{{"second"}},
			wantDetections: []string{"a:logged", "b:logged"},
		},
		{
			name: "an error applies each policy's failure mode",
			mode: "fail",
			policies: []types.GuardrailPolicy{
				policy("open", types.GuardrailActionBlock, "g"),
				failClosed(policy("closed", types.GuardrailActionBlock, "g")),
			},
			segments:       []Segment{{Role: RoleUser, Text: "hello"}},
			wantCalls:      [][]string{{"hello"}},
			wantDetections: []string{"open:error", "closed:error"},
			wantBlocked:    true,
			wantMessage:    `"closed"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := t.Name()
			store := &fakeStore{
				guardrails: []types.Guardrail{guardrail("g", "test-counting", map[string]any{"key": key, "mode": tt.mode})},
				policies:   tt.policies,
			}
			d := evaluate(t, store, Identity{UserID: "u1"}, nil, tt.segments...)

			countingLock.Lock()
			calls := *countingCalls[key]
			countingLock.Unlock()
			if !equalCalls(calls, tt.wantCalls) {
				t.Fatalf("calls = %q, want %q", calls, tt.wantCalls)
			}

			var detections []string
			for _, det := range d.Detections {
				detections = append(detections, det.PolicyID+":"+det.Outcome)
			}
			if strings.Join(detections, ",") != strings.Join(tt.wantDetections, ",") {
				t.Fatalf("detections = %v, want %v", detections, tt.wantDetections)
			}
			if d.Blocked != tt.wantBlocked || !strings.Contains(d.Message, tt.wantMessage) {
				t.Fatalf("blocked = %v (%q), want %v (%q)", d.Blocked, d.Message, tt.wantBlocked, tt.wantMessage)
			}
		})
	}
}

func equalCalls(a, b [][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.Join(a[i], "\x00") != strings.Join(b[i], "\x00") {
			return false
		}
	}
	return true
}

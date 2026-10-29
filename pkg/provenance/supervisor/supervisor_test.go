package supervisor

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/provenance"
)

const (
	fixedNow = "2026-10-28T12:00:00Z"
)

// fakeDecider is a Decider test double: it records the last question and
// returns a canned answer or error.
type fakeDecider struct {
	answer provenance.Answer
	err    error
	lastQ  provenance.Question
	calls  int
}

// fakeTextModel is a TextModel test double returning canned text per call.
type fakeTextModel struct {
	responses []string
	model     string
	calls     int
	lastUser  string
	err       error
}

func (f *fakeDecider) Decide(_ context.Context, q provenance.Question) (provenance.Answer, error) {
	f.calls++
	f.lastQ = q
	if f.err != nil {
		return provenance.Answer{}, f.err
	}
	return f.answer, nil
}

func (f *fakeTextModel) Complete(_ context.Context, _, user string) (string, string, error) {
	f.lastUser = user
	if f.err != nil {
		return "", "", f.err
	}
	i := f.calls
	f.calls++
	if i >= len(f.responses) {
		i = len(f.responses) - 1
	}
	return f.responses[i], firstNonEmpty(f.model, "fake-text"), nil
}

func testClock(t *testing.T) func() time.Time {
	t.Helper()
	now, err := time.Parse(time.RFC3339, fixedNow)
	if err != nil {
		t.Fatal(err)
	}
	return func() time.Time { return now }
}

func TestJudge(t *testing.T) {
	cases := []struct {
		name       string
		answer     provenance.Answer
		deciderErr error
		budget     time.Duration
		wantEffect provenance.Effect
		wantErr    bool
	}{
		{
			name:       "allow",
			answer:     provenance.Answer{Option: string(provenance.EffectAllow), Rationale: "routine", Confidence: 0.8},
			wantEffect: provenance.EffectAllow,
		},
		{
			name:       "deny",
			answer:     provenance.Answer{Option: string(provenance.EffectDeny), Rationale: "exfiltration", Confidence: 0.95},
			wantEffect: provenance.EffectDeny,
		},
		{
			name:       "require approval",
			answer:     provenance.Answer{Option: string(provenance.EffectRequireApproval), Rationale: "unusual", Confidence: 0.6},
			wantEffect: provenance.EffectRequireApproval,
		},
		{
			name:    "option outside judge set",
			answer:  provenance.Answer{Option: "redact", Rationale: "nope"},
			wantErr: true,
		},
		{
			name:       "decider error",
			deciderErr: errors.New("model down"),
			wantErr:    true,
		},
		{
			name:       "budget set",
			answer:     provenance.Answer{Option: string(provenance.EffectAllow), Confidence: 0.5},
			budget:     2 * time.Second,
			wantEffect: provenance.EffectAllow,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := &fakeDecider{answer: tc.answer, err: tc.deciderErr}
			s := NewWith(dec, nil, WithModel("sup-model"), WithClock(testClock(t)))
			in := provenance.JudgeInput{
				Policy:    provenance.Policy{Name: "Destructive after untrusted", Guidance: "allow routine dev commands"},
				Subject:   provenance.Subject{UserID: "u1", Email: "craig@example.com", Groups: []string{"eng"}, Actor: provenance.Actor{Agent: "claude_code"}},
				Session:   &provenance.Session{ID: "s1", State: provenance.SessionState{Labels: provenance.NewLabels(provenance.LabelUntrusted), Risk: 30}},
				Action:    provenance.Action{Surface: provenance.SurfaceDevice, Kind: provenance.KindShellExec, Command: "rm -rf build/", Arguments: map[string]any{"cmd": "rm -rf build/"}},
				Narrative: "the session read untrusted content",
				Budget:    tc.budget,
			}
			out, err := s.Judge(context.Background(), in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got %+v", out)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if out.Effect != tc.wantEffect {
				t.Fatalf("effect: got %q want %q", out.Effect, tc.wantEffect)
			}
			if out.Model != "sup-model" && out.Model == "" {
				t.Fatalf("expected a model, got %q", out.Model)
			}
			if dec.lastQ.Guidance != "allow routine dev commands" {
				t.Fatalf("guidance should be passed to the decider, got %q", dec.lastQ.Guidance)
			}
			if len(dec.lastQ.Options) != len(provenance.JudgeOptions) {
				t.Fatalf("judge options should be offered, got %d", len(dec.lastQ.Options))
			}
			if !strings.Contains(dec.lastQ.Context, "rm -rf build/") {
				t.Fatalf("arguments should reach the model, context: %s", dec.lastQ.Context)
			}
			if !strings.Contains(dec.lastQ.Context, "<arguments>") {
				t.Fatalf("arguments should be quoted, context: %s", dec.lastQ.Context)
			}
		})
	}
}

func TestJudgeContextIsContentFreeExceptArguments(t *testing.T) {
	in := provenance.JudgeInput{
		Policy:  provenance.Policy{Name: "p"},
		Subject: provenance.Subject{Email: "craig@example.com", Actor: provenance.Actor{Agent: "claude_code"}},
		Session: &provenance.Session{ID: "s1", State: provenance.SessionState{Labels: provenance.NewLabels(provenance.LabelUntrusted, provenance.LabelPII)}},
		Recent: []provenance.Event{
			{At: mustTime(t, "2026-10-28T11:04:00Z"), Surface: provenance.SurfaceMCP, Kind: provenance.KindToolResult, Summary: "github.get_issue result", Labels: provenance.NewLabels(provenance.LabelUntrusted)},
		},
		Action: provenance.Action{Surface: provenance.SurfaceMCP, Kind: provenance.KindToolCall, Server: "slack", Tool: "post_message", Text: "secret customer data"},
	}
	ctx := JudgeContext(in)
	if !strings.Contains(ctx, "github.get_issue result") {
		t.Fatalf("recent event summary missing: %s", ctx)
	}
	if !strings.Contains(ctx, "11:04") {
		t.Fatalf("recent event time missing: %s", ctx)
	}
	if !strings.Contains(ctx, "secret customer data") {
		t.Fatalf("action arguments should be the one content shown: %s", ctx)
	}
}

func TestTriageApproval(t *testing.T) {
	cases := []struct {
		name    string
		option  string
		wantErr bool
	}{
		{name: "approve", option: "approve"},
		{name: "deny", option: "deny"},
		{name: "escalate", option: "escalate"},
		{name: "unknown", option: "ignore", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dec := &fakeDecider{answer: provenance.Answer{Option: tc.option, Rationale: "r", Confidence: 0.7}}
			s := NewWith(dec, nil, WithClock(testClock(t)))
			appr := provenance.Approval{
				ID: "apr1", SessionID: "s1", UserID: "u1", PolicyName: "Lethal trifecta", Reason: "the session read untrusted and PII",
				Action: provenance.Action{Surface: provenance.SurfaceEgress, Kind: provenance.KindNetEgress, Host: "paste.example"},
				Risk:   70, Labels: provenance.NewLabels(provenance.LabelUntrusted, provenance.LabelPII), ExpiresAt: mustTime(t, "2026-10-28T12:10:00Z"),
			}
			sess := &provenance.Session{ID: "s1", UserID: "u1", State: provenance.SessionState{Risk: 70, Labels: appr.Labels}}
			tri, err := s.TriageApproval(context.Background(), appr, sess, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", tri)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tri.Recommendation != tc.option {
				t.Fatalf("recommendation: got %q want %q", tri.Recommendation, tc.option)
			}
			if tri.At.Format(time.RFC3339) != fixedNow {
				t.Fatalf("At should come from the clock, got %s", tri.At)
			}
		})
	}
}

func TestNarrate(t *testing.T) {
	writer := &fakeTextModel{responses: []string{
		`{"summary":"The agent read a GitHub issue and the CRM, then tried to post externally.","timeline":["11:04 read github issue (untrusted)","11:05 queried salesforce (pii)"],"concerns":["external post of customer data"]}`,
	}, model: "narrator"}
	s := NewWith(nil, writer, WithClock(testClock(t)))
	sess := &provenance.Session{ID: "s1", UserID: "u1", StartedAt: mustTime(t, "2026-10-28T11:00:00Z"),
		State: provenance.SessionState{Labels: provenance.NewLabels(provenance.LabelUntrusted, provenance.LabelPII), Risk: 55, Events: 4, ToolCalls: 2}}
	events := []provenance.Event{
		{At: mustTime(t, "2026-10-28T11:04:00Z"), Surface: provenance.SurfaceMCP, Kind: provenance.KindToolResult, Summary: "github.get_issue result", Labels: provenance.NewLabels(provenance.LabelUntrusted)},
		{At: mustTime(t, "2026-10-28T11:05:00Z"), Surface: provenance.SurfaceMCP, Kind: provenance.KindToolResult, Summary: "salesforce.query result", Labels: provenance.NewLabels(provenance.LabelPII)},
	}
	n, err := s.Narrate(context.Background(), sess, events)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(n.Summary, "GitHub issue") {
		t.Fatalf("summary: %q", n.Summary)
	}
	if len(n.Timeline) != 2 || len(n.Concerns) != 1 {
		t.Fatalf("timeline/concerns: %+v", n)
	}
	if n.Model != "narrator" {
		t.Fatalf("model: %q", n.Model)
	}
	if !strings.Contains(writer.lastUser, "github.get_issue result") {
		t.Fatalf("event lines should reach the model: %s", writer.lastUser)
	}
	if !strings.Contains(writer.lastUser, "<events>") {
		t.Fatalf("events should be quoted: %s", writer.lastUser)
	}
}

func TestClassifyTool(t *testing.T) {
	cases := []struct {
		name     string
		response string
		want     provenance.Labels
		wantErr  bool
	}{
		{
			name:     "valid labels",
			response: `{"labels":["reads-untrusted","writes-external"],"rationale":"fetches public pages and posts"}`,
			want:     provenance.NewLabels(provenance.CapReadsUntrusted, provenance.CapWritesExternal),
		},
		{
			name:     "empty labels",
			response: `{"labels":[],"rationale":"read-only internal"}`,
			want:     nil,
		},
		{
			name:     "unknown label rejected",
			response: `{"labels":["reads-untrusted","teleport"],"rationale":"x"}`,
			wantErr:  true,
		},
		{
			name:     "code fence tolerated",
			response: "```json\n{\"labels\":[\"exec\"],\"rationale\":\"runs shell\"}\n```",
			want:     provenance.NewLabels(provenance.CapExec),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := &fakeTextModel{responses: []string{tc.response}}
			s := NewWith(nil, writer)
			labels, rationale, err := s.ClassifyTool(context.Background(), provenance.ToolDescriptor{
				Server: "web", Tool: "fetch", Description: "Fetch a web page", Heuristic: provenance.NewLabels(provenance.CapReadsUntrusted),
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %v", labels)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if labels.String() != tc.want.String() {
				t.Fatalf("labels: got %v want %v", labels, tc.want)
			}
			if tc.want != nil && rationale == "" {
				t.Fatal("expected a rationale")
			}
		})
	}
}

func TestDraftPolicy(t *testing.T) {
	// A well-formed draft with two tests whose Actual the engine fills.
	good := `{"name":"No customer email to outsiders",
"description":"Blocks sending PII outside the org.",
"explanation":"Matches when PII read earlier in the session is in an action that writes externally.",
"condition":"\"pii\" in action.lineage.labels && action.sink.external",
"effect":"require_approval","mode":"shadow","surfaces":[],"guidance":"",
"tests":[
{"name":"pii leaving","summary":"pii lineage to an external host","expected":true,"input":{"action":{"sink":{"external":true},"lineage":{"labels":["pii"],"count":3}}}},
{"name":"internal write","summary":"pii lineage to an internal host","expected":false,"input":{"action":{"sink":{"external":false},"lineage":{"labels":["pii"],"count":3}}}}
]}`
	writer := &fakeTextModel{responses: []string{good}}
	s := NewWith(nil, writer, WithClock(testClock(t)))
	draft, err := s.DraftPolicy(context.Background(), "don't let agents email customer data outside the company", nil)
	if err != nil {
		t.Fatal(err)
	}
	if draft.Policy.Effect != provenance.EffectRequireApproval {
		t.Fatalf("effect: %q", draft.Policy.Effect)
	}
	if draft.Policy.Mode != provenance.ModeShadow {
		t.Fatalf("drafts must start in shadow, got %q", draft.Policy.Mode)
	}
	if draft.Policy.Source == "" {
		t.Fatal("source rule should be recorded")
	}
	if len(draft.Tests) != 2 {
		t.Fatalf("expected 2 tests, got %d", len(draft.Tests))
	}
	for _, tt := range draft.Tests {
		if tt.Error != "" {
			t.Fatalf("test %q errored: %s", tt.Name, tt.Error)
		}
		if tt.Actual != tt.Expected {
			t.Fatalf("test %q: Actual %v != Expected %v", tt.Name, tt.Actual, tt.Expected)
		}
	}
}

func TestDraftPolicyRetriesOnInvalidCondition(t *testing.T) {
	bad := `{"name":"bad","description":"d","explanation":"e","condition":"action.host","effect":"deny","mode":"shadow","surfaces":[],"tests":[]}`
	good := `{"name":"good","description":"d","explanation":"e","condition":"action.sink.external","effect":"deny","mode":"shadow","surfaces":[],"tests":[]}`
	writer := &fakeTextModel{responses: []string{bad, good}}
	s := NewWith(nil, writer, WithClock(testClock(t)))
	draft, err := s.DraftPolicy(context.Background(), "block external writes", nil)
	if err != nil {
		t.Fatal(err)
	}
	if writer.calls != 2 {
		t.Fatalf("expected a retry, got %d calls", writer.calls)
	}
	if draft.Policy.Condition != "action.sink.external" {
		t.Fatalf("condition: %q", draft.Policy.Condition)
	}
}

func TestDraftPolicyFailsWhenConditionStaysInvalid(t *testing.T) {
	bad := `{"name":"bad","description":"d","explanation":"e","condition":"action.host","effect":"deny","mode":"shadow","surfaces":[],"tests":[]}`
	writer := &fakeTextModel{responses: []string{bad, bad}}
	s := NewWith(nil, writer, WithClock(testClock(t)))
	if _, err := s.DraftPolicy(context.Background(), "x", nil); err == nil {
		t.Fatal("expected an error when the condition never validates")
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

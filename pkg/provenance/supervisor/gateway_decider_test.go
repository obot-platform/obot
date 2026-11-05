package supervisor

import (
	"context"
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/provenance"
)

// stubCompleter is a Completer test double.
type stubCompleter struct {
	text       string
	model      string
	err        error
	lastModel  string
	lastSchema map[string]any
	calls      int
}

func (s *stubCompleter) Complete(_ context.Context, model, _, _ string, schema map[string]any) (string, string, error) {
	s.calls++
	s.lastModel = model
	s.lastSchema = schema
	if s.err != nil {
		return "", "", s.err
	}
	return s.text, firstNonEmpty(s.model, model), nil
}

func TestGatewayDeciderParsesAnswer(t *testing.T) {
	cases := []struct {
		name       string
		text       string
		wantOption string
		wantErr    bool
	}{
		{
			name:       "plain json",
			text:       `{"option":"deny","rationale":"leaks data","confidence":0.9}`,
			wantOption: string(provenance.EffectDeny),
		},
		{
			name:       "code fence",
			text:       "```json\n{\"option\":\"allow\",\"rationale\":\"routine\",\"confidence\":0.5}\n```",
			wantOption: string(provenance.EffectAllow),
		},
		{
			name:       "prose around json",
			text:       `Here is my decision: {"option":"require_approval","rationale":"unusual","confidence":0.4} — done.`,
			wantOption: string(provenance.EffectRequireApproval),
		},
		{
			name:       "fallback to key in prose",
			text:       "I would deny this because it exfiltrates customer data.",
			wantOption: string(provenance.EffectDeny),
		},
		{
			name:    "option not offered",
			text:    `{"option":"redact","rationale":"x","confidence":0.5}`,
			wantErr: true,
		},
		{
			name:    "ambiguous prose",
			text:    "You could allow or deny this.",
			wantErr: true,
		},
		{
			name:    "empty",
			text:    "",
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &stubCompleter{text: tc.text, model: "mini"}
			d := NewGatewayDecider(c, "")
			ans, err := d.Decide(context.Background(), provenance.Question{
				Question: "Should this proceed?", Context: "ctx", Options: provenance.JudgeOptions,
			})
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %+v", ans)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ans.Option != tc.wantOption {
				t.Fatalf("option: got %q want %q", ans.Option, tc.wantOption)
			}
		})
	}
}

func TestGatewayDeciderPassesSchemaAndModel(t *testing.T) {
	c := &stubCompleter{text: `{"option":"approve","rationale":"ok","confidence":0.6}`}
	d := NewGatewayDecider(c, "custom-model")
	if _, err := d.Decide(context.Background(), provenance.Question{Question: "q", Options: provenance.TriageOptions}); err != nil {
		t.Fatal(err)
	}
	if c.lastModel != "custom-model" {
		t.Fatalf("model: got %q", c.lastModel)
	}
	if c.lastSchema == nil {
		t.Fatal("a JSON schema should be passed for structured output")
	}
	props, _ := c.lastSchema["properties"].(map[string]any)
	opt, _ := props["option"].(map[string]any)
	enum, _ := opt["enum"].([]string)
	if len(enum) != len(provenance.TriageOptions) {
		t.Fatalf("schema enum should list the option keys, got %v", enum)
	}
	confidence := props["confidence"].(map[string]any)
	if confidence["minimum"] != nil || confidence["maximum"] != nil || c.lastSchema["additionalProperties"] != false {
		t.Fatalf("schema must use the supported Anthropic structured-output subset: %+v", c.lastSchema)
	}
}

func TestGatewayDeciderClampsConfidenceWithoutProviderBounds(t *testing.T) {
	for _, tt := range []struct {
		response string
		want     float64
	}{
		{
			response: `{"option":"allow","rationale":"routine","confidence":2}`,
			want:     1,
		},
		{
			response: `{"option":"allow","rationale":"routine","confidence":-1}`,
			want:     0,
		},
	} {
		c := &stubCompleter{text: tt.response}
		answer, err := NewGatewayDecider(c, "").Decide(t.Context(), provenance.Question{Question: "Proceed?", Options: provenance.JudgeOptions})
		if err != nil || answer.Confidence != tt.want {
			t.Fatalf("confidence=%v want=%v error=%v", answer.Confidence, tt.want, err)
		}
	}
}

func TestGatewayDeciderErrors(t *testing.T) {
	d := NewGatewayDecider(&stubCompleter{err: errors.New("down")}, "")
	if _, err := d.Decide(context.Background(), provenance.Question{Question: "q", Options: provenance.JudgeOptions}); err == nil {
		t.Fatal("expected the completer error to propagate")
	}
	empty := NewGatewayDecider(&stubCompleter{text: `{"option":"allow"}`}, "")
	if _, err := empty.Decide(context.Background(), provenance.Question{Question: "q"}); err == nil {
		t.Fatal("a question with no options should error")
	}
}

func TestClampAndWordMatch(t *testing.T) {
	if clamp01(1.5) != 1 || clamp01(-0.2) != 0 || clamp01(0.5) != 0.5 {
		t.Fatal("clamp01 bounds are wrong")
	}
	if containsWord("require_approval is best", "approve") {
		t.Fatal("approve should not match inside require_approval")
	}
	if !containsWord("I would approve this", "approve") {
		t.Fatal("approve should match as a word")
	}
}

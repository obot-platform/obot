package guardrails

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
)

func TestCrowdStrikeChecker(t *testing.T) {
	segments := []Segment{
		{Role: RoleUser, Text: "My SSN is 123-45-6789"},
		{Role: RoleAssistant, Text: " "},
		{Role: RoleToolResult, Text: "weather: sunny", Name: "weather"},
	}

	tests := []struct {
		name             string
		target           string
		stage            string
		wantEventType    string
		responses        []string
		status           int
		wantErr          string
		wantFindings     []string
		wantReplacements map[int]string
	}{
		{
			name:          "nothing detected",
			target:        gatewaytypes.GuardrailTargetLLM,
			stage:         StageInput,
			wantEventType: "input",
			responses: []string{`{"status":"Success","request_id":"prq_1","result":{"blocked":false,"transformed":false,
				"detectors":{"malicious_prompt":{"detected":false},"confidential_and_pii_entity":{"detected":false}}}}`},
		},
		{
			name:          "redacted PII replaces segments",
			target:        gatewaytypes.GuardrailTargetLLM,
			stage:         StageOutput,
			wantEventType: "output",
			responses: []string{`{"status":"Success","request_id":"prq_2","result":{"blocked":false,"transformed":true,
				"guard_output":{"messages":[{"role":"user","content":"My SSN is <US_SSN>"},{"role":"tool","content":[{"type":"text","text":"weather: sunny"}]}]},
				"detectors":{"confidential_and_pii_entity":{"detected":true,"data":{"entities":[{"type":"US_SSN","value":"123-45-6789","action":"redacted:replaced"}]}},
					"malicious_prompt":{"detected":false}}}}`},
			wantFindings:     []string{"confidential_and_pii_entity: confidential and pii entity detected"},
			wantReplacements: map[int]string{0: "My SSN is <US_SSN>"},
		},
		{
			name:          "blocked request has no replacements",
			target:        gatewaytypes.GuardrailTargetMCP,
			stage:         StageOutput,
			wantEventType: "tool_output",
			responses: []string{`{"status":"Success","request_id":"prq_3","result":{"blocked":true,"transformed":true,
				"guard_output":{"messages":[{"role":"user","content":"My SSN is <US_SSN>"},{"role":"tool","content":"weather: sunny"}]},
				"detectors":{"malicious_prompt":{"detected":true,"data":{"action":"blocked"}},
					"confidential_and_pii_entity":{"detected":true,"data":{"entities":[{"type":"US_SSN"},{"type":"US_SSN"}]}}}}}`},
			wantFindings: []string{"confidential_and_pii_entity: confidential and pii entity detected", "malicious_prompt: malicious prompt detected"},
		},
		{
			name:          "accepted requests are polled",
			target:        gatewaytypes.GuardrailTargetMCP,
			stage:         StageInput,
			wantEventType: "tool_input",
			responses: []string{
				`{"status":"Accepted","request_id":"prq_4","result":{"ttl_mins":5,"retry_counter":0,"location":"/request/prq_4"}}`,
				`{"status":"Success","request_id":"prq_4","result":{"blocked":true,"detectors":{}}}`,
			},
			wantFindings: []string{"blocked: Blocked by CrowdStrike AIDR policy"},
		},
		{
			name:          "error status",
			target:        gatewaytypes.GuardrailTargetLLM,
			stage:         StageInput,
			wantEventType: "input",
			status:        http.StatusBadRequest,
			responses:     []string{`{"status":"ValidationError","summary":"guard_input is invalid"}`},
			wantErr:       "400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasPrefix(r.URL.Path, "/aidr/aiguard/") {
					// Ignore anything else probing the listener.
					http.NotFound(w, r)
					return
				}
				n := int(calls.Add(1)) - 1
				if got := r.Header.Get("Authorization"); got != "Bearer cs-token" {
					t.Errorf("Authorization = %q", got)
				}
				if n > 0 {
					if r.Method != http.MethodGet || r.URL.Path != "/aidr/aiguard/request/prq_4" {
						t.Errorf("unexpected poll %s %s", r.Method, r.URL.Path)
					}
				} else {
					if r.Method != http.MethodPost || r.URL.Path != "/aidr/aiguard/v1/guard_chat_completions" {
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					}
					var body crowdStrikeRequest
					raw, _ := io.ReadAll(r.Body)
					if err := json.Unmarshal(raw, &body); err != nil {
						t.Errorf("invalid body: %v", err)
					}
					msgs := body.GuardInput.Messages
					if body.EventType != tt.wantEventType || body.AppID != "obot" || body.CollectorInstanceID != "" ||
						len(msgs) != 2 || msgs[0].Role != "user" || msgs[1].Role != "tool" || msgs[1].Content != "weather: sunny" {
						t.Errorf("unexpected body %s", raw)
					}
				}
				if n < len(tt.responses)-1 {
					w.WriteHeader(http.StatusAccepted)
				} else if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = w.Write([]byte(tt.responses[min(n, len(tt.responses)-1)]))
			}))
			defer srv.Close()

			c, err := newChecker("crowdstrike-aidr", map[string]any{"baseURL": srv.URL + "/aidr/aiguard", "appID": "obot"}, map[string]string{"apiToken": "cs-token"}, Deps{HTTPClient: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Check(context.Background(), CheckInput{Target: tt.target, Stage: tt.stage, Segments: segments})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "guard_input is invalid") {
					t.Fatalf("error = %v, want it to mention %q and the body", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if int(calls.Load()) != len(tt.responses) {
				t.Errorf("calls = %d, want %d", calls.Load(), len(tt.responses))
			}
			assertProviderFindings(t, result.Findings, tt.wantFindings)
			assertProviderReplacements(t, result.Replacements, tt.wantReplacements)
			for _, f := range result.Findings {
				if strings.Contains(f.Message, "123-45-6789") {
					t.Errorf("finding message leaks matched text: %q", f.Message)
				}
			}
		})
	}
}

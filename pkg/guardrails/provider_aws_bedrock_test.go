package guardrails

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBedrockChecker(t *testing.T) {
	tests := []struct {
		name             string
		stage            string
		response         string
		status           int
		wantSource       string
		wantErr          string
		wantFindings     []string
		wantReplacements map[int]string
	}{
		{
			name:         "no intervention",
			stage:        StageInput,
			wantSource:   "INPUT",
			response:     `{"action":"NONE","outputs":[],"assessments":[{}]}`,
			wantFindings: nil,
		},
		{
			name:       "anonymized content is replaced per segment",
			stage:      StageOutput,
			wantSource: "OUTPUT",
			response: `{"action":"GUARDRAIL_INTERVENED",
				"outputs":[{"text":"Hi, my name is {NAME}."},{"text":"call me"}],
				"assessments":[{"sensitiveInformationPolicy":{"piiEntities":[
					{"type":"NAME","match":"Zaïd","action":"ANONYMIZED","detected":true},
					{"type":"EMAIL","match":"z@example.com","action":"ANONYMIZED","detected":false}]}}]}`,
			wantFindings:     []string{"pii: NAME detected"},
			wantReplacements: map[int]string{0: "Hi, my name is {NAME}."},
		},
		{
			name:       "blocked content has no replacements",
			stage:      StageInput,
			wantSource: "INPUT",
			response: `{"action":"GUARDRAIL_INTERVENED",
				"outputs":[{"text":"Sorry, I can't help."},{"text":"canned"}],
				"assessments":[{
					"topicPolicy":{"topics":[{"name":"Cars","type":"DENY","action":"BLOCKED"}]},
					"contentPolicy":{"filters":[{"type":"INSULTS","confidence":"HIGH","action":"BLOCKED"}]},
					"wordPolicy":{"customWords":[{"match":"secretword","action":"BLOCKED"}]},
					"sensitiveInformationPolicy":{"piiEntities":[{"type":"NAME","match":"Zaïd","action":"ANONYMIZED"}]}}]}`,
			wantFindings: []string{"topic: Denied topic: Cars", "content_filter: INSULTS content (high confidence)", "word_filter: Blocked word", "pii: NAME detected"},
		},
		{
			name:       "error status",
			stage:      StageInput,
			wantSource: "INPUT",
			status:     http.StatusForbidden,
			response:   `{"message":"not authorized"}`,
			wantErr:    "403",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.EscapedPath() != "/guardrail/arn:aws:bedrock:us-east-1:123456789012:guardrail%2Fabc/version/3/apply" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.EscapedPath())
				}
				if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") || !strings.Contains(auth, "/us-east-1/bedrock/aws4_request") {
					t.Errorf("request not SigV4-signed for bedrock: %q", auth)
				}
				if r.Header.Get("X-Amz-Security-Token") != "session" {
					t.Errorf("session token not sent")
				}
				var body bedrockApplyRequest
				raw, _ := io.ReadAll(r.Body)
				if err := json.Unmarshal(raw, &body); err != nil {
					t.Errorf("invalid body: %v", err)
				}
				if body.Source != tt.wantSource {
					t.Errorf("source = %q, want %q", body.Source, tt.wantSource)
				}
				// The empty segment is skipped; one content block per checked segment.
				if len(body.Content) != 2 || body.Content[0].Text.Text != "Hi, my name is Zaïd." || body.Content[1].Text.Text != "call me" {
					t.Errorf("unexpected content blocks: %s", raw)
				}
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
				_, _ = w.Write([]byte(tt.response))
			}))
			defer srv.Close()

			c, err := newChecker("aws-bedrock", map[string]any{
				"region":           "us-east-1",
				"guardrailId":      "arn:aws:bedrock:us-east-1:123456789012:guardrail/abc",
				"guardrailVersion": "3",
				"endpoint":         srv.URL,
			}, map[string]string{"accessKeyId": "AKIDEXAMPLE", "secretAccessKey": "secret", "sessionToken": "session"}, Deps{HTTPClient: srv.Client()})
			if err != nil {
				t.Fatal(err)
			}
			segments := []Segment{
				{Role: RoleUser, Text: "Hi, my name is Zaïd."},
				{Role: RoleAssistant, Text: "  "},
				{Role: RoleToolResult, Text: "call me", Name: "lookup"},
			}
			result, err := c.Check(context.Background(), CheckInput{Stage: tt.stage, Segments: segments})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "not authorized") {
					t.Fatalf("error = %v, want it to mention %q and the body", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			assertProviderFindings(t, result.Findings, tt.wantFindings)
			assertProviderReplacements(t, result.Replacements, tt.wantReplacements)
			for _, f := range result.Findings {
				if strings.Contains(f.Message, "Zaïd") || strings.Contains(f.Message, "secretword") {
					t.Errorf("finding message leaks matched text: %q", f.Message)
				}
			}
		})
	}
}

func TestBedrockCheckerConfig(t *testing.T) {
	if _, err := newChecker("aws-bedrock", map[string]any{"guardrailId": "abc"}, nil, Deps{}); err == nil {
		t.Error("expected an error without a region")
	}
	if _, err := newChecker("aws-bedrock", map[string]any{"region": "us-east-1", "guardrailId": "abc"}, map[string]string{"accessKeyId": "AKID"}, Deps{}); err == nil {
		t.Error("expected an error with an access key but no secret")
	}
}

func assertProviderFindings(t *testing.T, findings []Finding, want []string) {
	t.Helper()
	got := make([]string, 0, len(findings))
	for _, f := range findings {
		got = append(got, f.Category+": "+f.Message)
	}
	if len(got) != len(want) {
		t.Fatalf("findings = %q, want %q", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("findings = %q, want %q", got, want)
		}
	}
}

func assertProviderReplacements(t *testing.T, got, want map[int]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("replacements = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("replacements = %v, want %v", got, want)
		}
	}
}

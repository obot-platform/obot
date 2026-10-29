package supervisor

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/provenance"
)

func TestOpenAIDecisionsDecider(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantOption string
		wantErr    bool
	}{
		{
			name:       "option field",
			status:     http.StatusOK,
			body:       `{"option":"deny","rationale":"exfiltration","confidence":0.92,"model":"gpt-5-mini"}`,
			wantOption: string(provenance.EffectDeny),
		},
		{
			name:       "choice alias",
			status:     http.StatusOK,
			body:       `{"choice":"allow","rationale":"routine","confidence":0.5}`,
			wantOption: string(provenance.EffectAllow),
		},
		{
			name:    "option not offered",
			status:  http.StatusOK,
			body:    `{"option":"redact","rationale":"x","confidence":0.5}`,
			wantErr: true,
		},
		{
			name:    "server error",
			status:  http.StatusInternalServerError,
			body:    `{"error":{"message":"boom"}}`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath, gotAuth, gotCT string
			var gotReq decisionsRequest
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				gotCT = r.Header.Get("Content-Type")
				body, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(body, &gotReq)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			d := NewOpenAIDecisionsDecider(srv.URL, "sk-test", "gpt-5-mini", WithHTTPClient(srv.Client()))
			ans, err := d.Decide(context.Background(), provenance.Question{
				Question: "Should this proceed?", Context: "the session read untrusted content", Guidance: "be careful",
				Options: provenance.JudgeOptions,
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
			if gotPath != "/v1/decisions" {
				t.Fatalf("path: got %q", gotPath)
			}
			if gotAuth != "Bearer sk-test" {
				t.Fatalf("auth: got %q", gotAuth)
			}
			if gotCT != "application/json" {
				t.Fatalf("content-type: got %q", gotCT)
			}
			if gotReq.Model != "gpt-5-mini" || gotReq.Question == "" || len(gotReq.Options) != len(provenance.JudgeOptions) {
				t.Fatalf("request shape: %+v", gotReq)
			}
			if gotReq.Guidance != "be careful" {
				t.Fatalf("guidance not sent: %q", gotReq.Guidance)
			}
		})
	}
}

func TestOpenAIDecisionsDefaults(t *testing.T) {
	d := NewOpenAIDecisionsDecider("", "", "")
	if d.baseURL != DefaultOpenAIDecisionsBaseURL {
		t.Fatalf("base URL default: %q", d.baseURL)
	}
	if d.model != DefaultOpenAIDecisionsModel {
		t.Fatalf("model default: %q", d.model)
	}
	if _, err := d.Decide(context.Background(), provenance.Question{Question: "q"}); err == nil {
		t.Fatal("a question with no options should error before any call")
	}
}

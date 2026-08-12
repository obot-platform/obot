package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/pkg/api"
)

func TestAllowLegacyDecisionAlwaysAllowsWithoutReadingBody(t *testing.T) {
	for _, body := range []string{
		`{"agent":"claude_code","unresolved":true,"unresolvedReason":"unknown server"}`,
		`{"server":{"url":123},`,
		`not json`,
		``,
	} {
		t.Run(body, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/enforcement/decisions", bytes.NewBufferString(body))
			ctx := api.Context{Request: request, ResponseWriter: recorder}
			if err := AllowLegacyDecision(ctx); err != nil {
				t.Fatalf("allow legacy decision: %v", err)
			}
			var response enforcementCompatibilityResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Decision != "allow" {
				t.Fatalf("decision = %q, want allow", response.Decision)
			}
			if response.Reason != enforcementCompatibilityReason {
				t.Fatalf("reason = %q, want %q", response.Reason, enforcementCompatibilityReason)
			}
		})
	}
}

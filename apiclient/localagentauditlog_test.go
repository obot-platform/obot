package apiclient

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

// TestSubmitLocalAgentAuditLogsSendsPayloadBytesUnescaped pins that the bodies and raw event of
// an audit log reach the server as the agent wrote them. The server rebuilds a raw event from the
// bodies, so an escape added on the way (\u003c for <, \u0026 for &) shows up in the raw event it
// presents.
func TestSubmitLocalAgentAuditLogsSendsPayloadBytesUnescaped(t *testing.T) {
	const (
		body = "{\"stdout\":\"<x> && a->b\"}"
		raw  = "{\"tool_output\":\"<html>&amp;\"}"
	)

	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	c := &Client{BaseURL: srv.URL, Token: "device-token"}

	err := c.SubmitLocalAgentAuditLogs(t.Context(), []types.LocalAgentToolCallAuditLogInput{{
		Details: types.LocalAgentToolCallAuditLogReportedDetails{
			Request:  types.LocalAgentToolCallAuditLogPayload{Body: json.RawMessage(body)},
			Response: types.LocalAgentToolCallAuditLogPayload{Body: json.RawMessage(body)},
			RawEvent: json.RawMessage(raw),
		},
	}})
	if err != nil {
		t.Fatalf("SubmitLocalAgentAuditLogs: %v", err)
	}

	if bytes.Count(got, []byte(body)) != 2 || !bytes.Contains(got, []byte(raw)) {
		t.Fatalf("request body = %s, want the bodies %s and the raw event %s unchanged", got, body, raw)
	}
	if bytes.HasSuffix(got, []byte("\n")) {
		t.Fatalf("request body ends in a newline: %q", got)
	}
}

package apiclient

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
)

func decideTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: "device-token"}
}

func TestDecideReturnsTheVerdict(t *testing.T) {
	for _, tt := range []types.EnforcementDecisionResponse{
		{Decision: types.EnforcementDecisionAllow, Reason: "matched an allowlisted server entry"},
		{Decision: types.EnforcementDecisionDeny, Reason: "no matching allowlist entry"},
		{Decision: types.EnforcementDecisionAllow},
	} {
		t.Run(tt.Decision+"/"+tt.Reason, func(t *testing.T) {
			c := decideTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if r.URL.Path != "/enforcement/decisions" {
					t.Errorf("path = %s, want /enforcement/decisions", r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer device-token" {
					t.Errorf("Authorization = %q, want the device token", got)
				}
				_ = json.NewEncoder(w).Encode(tt)
			})

			got, err := c.Decide(t.Context(), types.EnforcementDecisionRequest{Agent: "claude_code", Tool: "search", Kind: "mcp"})
			if err != nil {
				t.Fatalf("Decide: %v", err)
			}
			if got != tt {
				t.Fatalf("Decide() = %+v, want %+v", got, tt)
			}
		})
	}
}

// TestDecideMarshalsTheFullRequest pins the device wire protocol: every field
// obot-sentry populates has to reach the server, the unresolved pair above all —
// without it the decision log records a misleading reason for a call the device
// could not identify.
func TestDecideMarshalsTheFullRequest(t *testing.T) {
	var body []byte
	c := decideTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		body, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(types.EnforcementDecisionResponse{Decision: types.EnforcementDecisionDeny})
	})

	req := types.EnforcementDecisionRequest{
		Agent:      "claude_code",
		Tool:       "search_issues",
		Kind:       "mcp",
		ServerName: "linear",
		Server: types.EnforcementDecisionServer{
			URL:       "https://mcp.linear.app/sse",
			Package:   &types.AllowlistServerPackage{Source: types.AllowlistServerPackageSourceNPM, Name: "linear-mcp", Version: "1.2.3"},
			Command:   "npx",
			Hostname:  "gitmcp.io",
			Connector: "claude.ai Linear",
		},
		Unresolved:       true,
		UnresolvedReason: "npx flag --registry is not allowed",
	}
	if _, err := c.Decide(t.Context(), req); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	var got types.EnforcementDecisionRequest
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("unmarshal request body: %v (body %s)", err, body)
	}
	if got.Server.Package == nil {
		t.Fatalf("package identity did not survive the round trip: %s", body)
	}
	if *got.Server.Package != *req.Server.Package {
		t.Fatalf("package = %+v, want %+v", *got.Server.Package, *req.Server.Package)
	}
	got.Server.Package, req.Server.Package = nil, nil
	if got != req {
		t.Fatalf("request round-tripped as %+v, want %+v (body %s)", got, req, body)
	}

	// Assert the wire names directly: the device and the server agree on these
	// strings, not on the Go field names.
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal raw body: %v", err)
	}
	if raw["unresolved"] != true {
		t.Errorf(`body["unresolved"] = %v, want true (body %s)`, raw["unresolved"], body)
	}
	if raw["unresolvedReason"] != req.UnresolvedReason {
		t.Errorf(`body["unresolvedReason"] = %v, want %q`, raw["unresolvedReason"], req.UnresolvedReason)
	}
}

// TestDecideOmitsUnresolvedWhenFalse keeps a resolved call's body identical to
// what it was before the unresolved pair existed.
func TestDecideOmitsUnresolvedWhenFalse(t *testing.T) {
	var body []byte
	c := decideTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ = io.ReadAll(r.Body)
		_ = json.NewEncoder(w).Encode(types.EnforcementDecisionResponse{Decision: types.EnforcementDecisionAllow})
	})

	if _, err := c.Decide(t.Context(), types.EnforcementDecisionRequest{Agent: "codex", Tool: "bash", Kind: "shell"}); err != nil {
		t.Fatalf("Decide: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("unmarshal raw body: %v", err)
	}
	for _, key := range []string{"unresolved", "unresolvedReason"} {
		if _, ok := raw[key]; ok {
			t.Errorf("body carries %q for a resolved call: %s", key, body)
		}
	}
}

// TestDecideFailsWithAnErrorNeverAZeroVerdict is the property the device relies
// on. A zero EnforcementDecisionResponse has Decision == "", which a naive
// caller would read as "not a deny"; every failure has to be distinguishable
// from a verdict by the error alone.
func TestDecideFailsWithAnErrorNeverAZeroVerdict(t *testing.T) {
	for _, tt := range []struct {
		name    string
		handler http.HandlerFunc
		code    int
	}{
		{"401", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}, http.StatusUnauthorized},
		{"403", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "forbidden", http.StatusForbidden)
		}, http.StatusForbidden},
		{"404", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "not found", http.StatusNotFound)
		}, http.StatusNotFound},
		{"500", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}, http.StatusInternalServerError},
		// A 2xx whose body is not a decision is just as unusable as a 500.
		{"200 with a non-JSON body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("<html>proxy error</html>"))
		}, 0},
		{"200 with a truncated body", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"decision":`))
		}, 0},
		{"200 with an empty body", func(w http.ResponseWriter, _ *http.Request) {}, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := decideTestClient(t, tt.handler)

			got, err := c.Decide(t.Context(), types.EnforcementDecisionRequest{Agent: "codex", Tool: "bash", Kind: "shell"})
			if err == nil {
				t.Fatalf("Decide() returned %+v with a nil error, want an error", got)
			}
			if got != (types.EnforcementDecisionResponse{}) {
				t.Fatalf("Decide() returned %+v alongside an error, want the zero response", got)
			}
			if tt.code != 0 {
				var httpErr *types.ErrHTTP
				if !errors.As(err, &httpErr) {
					t.Fatalf("error is %T, want *types.ErrHTTP: %v", err, err)
				}
				if httpErr.Code != tt.code {
					t.Fatalf("error code = %d, want %d", httpErr.Code, tt.code)
				}
			}
		})
	}
}

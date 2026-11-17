//go:build integration

package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

// Response is a response read in full, for tests that assert on error statuses, headers and bodies.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// RequestAs issues method path with body as JSON, authenticated with bearer when it is not empty, and returns the
// response whatever its status. The integration server runs with authentication off, so a request without a bearer
// is the server's single owner, and one with an agent's key is that agent.
func (h *Harness) RequestAs(t *testing.T, method, path, bearer string, body any) Response {
	t.Helper()

	var reqBody io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		reqBody = bytes.NewReader(buf)
	}

	ctx, cancel := context.WithTimeout(t.Context(), requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, h.BaseURL+path, reqBody)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return Response{StatusCode: resp.StatusCode, Header: resp.Header, Body: respBody}
}

// CreateAgent creates an agent identity and registers its deletion as cleanup.
func (h *Harness) CreateAgent(t *testing.T, displayName string) types.AgentIdentity {
	t.Helper()
	var created types.AgentIdentity
	h.Post(t, "/api/agents", types.AgentIdentityManifest{DisplayName: displayName}, &created)
	if created.ID == "" {
		t.Fatalf("agent create returned empty ID: %+v", created)
	}
	h.AddCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = h.status(ctx, http.MethodDelete, "/api/agents/"+created.ID)
	})
	return created
}

// CreateAgentAPIKey creates an API key for agentID and returns the response, which holds the full key.
func (h *Harness) CreateAgentAPIKey(t *testing.T, agentID string, req types.AgentAPIKeyCreateRequest) types.AgentAPIKeyCreateResponse {
	t.Helper()
	var created types.AgentAPIKeyCreateResponse
	h.Post(t, "/api/agents/"+agentID+"/api-keys", req, &created)
	if created.Key == "" {
		t.Fatalf("agent API key create returned no key: %+v", created)
	}
	return created
}

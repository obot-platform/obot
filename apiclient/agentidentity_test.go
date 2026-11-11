package apiclient

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

func TestAgentIdentityRequests(t *testing.T) {
	tests := []struct {
		name     string
		call     func(c *Client) error
		method   string
		path     string
		query    string
		wantBody string
		status   int
		response string
	}{
		{
			name: "list with deleted",
			call: func(c *Client) error {
				_, err := c.ListAgentIdentities(t.Context(), true)
				return err
			},
			method:   http.MethodGet,
			path:     "/agents",
			query:    "includeDeleted=true",
			status:   http.StatusOK,
			response: `{"items":[]}`,
		},
		{
			name: "create",
			call: func(c *Client) error {
				_, err := c.CreateAgentIdentity(t.Context(), types.AgentIdentityManifest{DisplayName: "bot", OwnerUserIDs: []string{"1"}})
				return err
			},
			method:   http.MethodPost,
			path:     "/agents",
			wantBody: `{"displayName":"bot","ownerUserIDs":["1"]}`,
			status:   http.StatusOK,
			response: `{"id":"7"}`,
		},
		{
			name: "update",
			call: func(c *Client) error {
				_, err := c.UpdateAgentIdentity(t.Context(), "7", types.AgentIdentityManifest{DisplayName: "bot"})
				return err
			},
			method:   http.MethodPut,
			path:     "/agents/7",
			wantBody: `{"displayName":"bot"}`,
			status:   http.StatusOK,
			response: `{"id":"7"}`,
		},
		{
			name: "disable",
			call: func(c *Client) error {
				_, err := c.DisableAgentIdentity(t.Context(), "7", "why")
				return err
			},
			method:   http.MethodPost,
			path:     "/agents/7/disable",
			wantBody: `{"reason":"why"}`,
			status:   http.StatusOK,
			response: `{"id":"7"}`,
		},
		{
			name: "enable",
			call: func(c *Client) error {
				_, err := c.EnableAgentIdentity(t.Context(), "7")
				return err
			},
			method:   http.MethodPost,
			path:     "/agents/7/enable",
			status:   http.StatusOK,
			response: `{"id":"7"}`,
		},
		{
			name: "delete 204",
			call: func(c *Client) error {
				return c.DeleteAgentIdentity(t.Context(), "7")
			},
			method: http.MethodDelete,
			path:   "/agents/7",
			status: http.StatusNoContent,
		},
		{
			name: "list keys showing revoked",
			call: func(c *Client) error {
				_, err := c.ListAgentAPIKeys(t.Context(), "7", true)
				return err
			},
			method:   http.MethodGet,
			path:     "/agents/7/api-keys",
			query:    "show_revoked=true",
			status:   http.StatusOK,
			response: `{"items":[]}`,
		},
		{
			name: "create key",
			call: func(c *Client) error {
				resp, err := c.CreateAgentAPIKey(t.Context(), "7", types.AgentAPIKeyCreateRequest{Name: "k", MCPServerIDs: []string{"*"}})
				if err == nil {
					require.Equal(t, "ok1-secret", resp.Key)
					require.Equal(t, uint(3), resp.ID)
				}
				return err
			},
			method:   http.MethodPost,
			path:     "/agents/7/api-keys",
			wantBody: `{"name":"k","mcpServerIds":["*"]}`,
			status:   http.StatusOK,
			response: `{"id":3,"key":"ok1-secret"}`,
		},
		{
			name: "get key",
			call: func(c *Client) error {
				_, err := c.GetAgentAPIKey(t.Context(), "7", "3")
				return err
			},
			method:   http.MethodGet,
			path:     "/agents/7/api-keys/3",
			status:   http.StatusOK,
			response: `{"id":3}`,
		},
		{
			name: "revoke key 204",
			call: func(c *Client) error {
				return c.RevokeAgentAPIKey(t.Context(), "7", "3")
			},
			method: http.MethodDelete,
			path:   "/agents/7/api-keys/3",
			status: http.StatusNoContent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, tt.method, r.Method)
				require.Equal(t, tt.path, r.URL.Path)
				require.Equal(t, tt.query, r.URL.RawQuery)
				body, _ := io.ReadAll(r.Body)
				if tt.wantBody == "" {
					require.Empty(t, body)
				} else {
					require.JSONEq(t, tt.wantBody, string(body))
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()
			require.NoError(t, tt.call(&Client{BaseURL: server.URL}))
		})
	}
}

func TestAgentIdentityErrorKeepsStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "at least one scope is required", http.StatusBadRequest)
	}))
	defer server.Close()

	_, err := (&Client{BaseURL: server.URL}).CreateAgentAPIKey(t.Context(), "7", types.AgentAPIKeyCreateRequest{})
	var httpErr *types.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusBadRequest, httpErr.Code)
	require.Contains(t, httpErr.Message, "at least one scope")
}

func TestAgentIdentityPathEscapesIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/agents/1%2F2", r.URL.EscapedPath())
		_ = json.NewEncoder(w).Encode(types.AgentIdentity{})
	}))
	defer server.Close()

	_, err := (&Client{BaseURL: server.URL}).GetAgentIdentity(t.Context(), "1/2")
	require.NoError(t, err)
}

package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
)

type agentsRequest struct {
	Method string
	Path   string
	Query  string
	Body   string
}

// agentsTestServer answers the token scope check, records every other request, and passes the rest to handle.
func agentsTestServer(t *testing.T, handle func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, func() []agentsRequest) {
	t.Helper()
	var (
		mu   sync.Mutex
		reqs []agentsRequest
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/api-keys/auth" {
			_, _ = w.Write([]byte(`{"allowed": true, "scopes": {"canAccessAPI": true}}`))
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		reqs = append(reqs, agentsRequest{Method: r.Method, Path: r.URL.Path, Query: r.URL.RawQuery, Body: string(body)})
		mu.Unlock()
		handle(w, r)
	}))
	t.Cleanup(server.Close)
	return server, func() []agentsRequest {
		mu.Lock()
		defer mu.Unlock()
		return append([]agentsRequest(nil), reqs...)
	}
}

func runAgents(t *testing.T, server *httptest.Server, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := cmd.Command(&Agents{root: mcpTestRoot(server.URL)})
	command.SetContext(t.Context())
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetIn(strings.NewReader(""))
	command.SetArgs(args)
	err := command.Execute()
	return stdout.String(), stderr.String(), err
}

func testAgent(id string) types.AgentIdentity {
	var agent types.AgentIdentity
	agent.ID = id
	return agent
}

func writeTestJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func TestAgentsUpdateMergesOnlyChangedFields(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantBody string
	}{
		{
			name:     "name only keeps description and owners",
			args:     []string{"update", "7", "--name", "new"},
			wantBody: `{"displayName":"new","description":"old desc","ownerUserIDs":["1","2"]}`,
		},
		{
			name:     "empty description clears it",
			args:     []string{"update", "7", "--description", ""},
			wantBody: `{"displayName":"old","ownerUserIDs":["1","2"]}`,
		},
		{
			name:     "empty owner clears owners",
			args:     []string{"update", "7", "--owner", ""},
			wantBody: `{"displayName":"old","description":"old desc"}`,
		},
		{
			name:     "repeated owner replaces owners",
			args:     []string{"update", "7", "--owner", "3", "--owner", "4"},
			wantBody: `{"displayName":"old","description":"old desc","ownerUserIDs":["3","4"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := agentsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
				agent := testAgent("7")
				agent.DisplayName = "old"
				agent.Description = "old desc"
				agent.OwnerUserIDs = []string{"1", "2"}
				writeTestJSON(w, agent)
			})
			_, _, err := runAgents(t, server, tt.args...)
			require.NoError(t, err)
			reqs := requests()
			require.Len(t, reqs, 2)
			require.Equal(t, http.MethodGet, reqs[0].Method)
			require.Equal(t, http.MethodPut, reqs[1].Method)
			require.Equal(t, "/api/agents/7", reqs[1].Path)
			require.JSONEq(t, tt.wantBody, reqs[1].Body)
		})
	}
}

func TestAgentsUpdateRequiresAFlag(t *testing.T) {
	server, requests := agentsTestServer(t, func(http.ResponseWriter, *http.Request) {})
	_, _, err := runAgents(t, server, "update", "7")
	require.ErrorContains(t, err, "nothing to update")
	require.Empty(t, requests())
}

func TestAgentsCreateAndLifecycleRequests(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantMethod string
		wantPath   string
		wantQuery  string
		wantBody   string
	}{
		{
			name:       "create",
			args:       []string{"create", "--name", "bot", "--description", "d", "--owner", "1", "--owner", "2"},
			wantMethod: http.MethodPost,
			wantPath:   "/api/agents",
			wantBody:   `{"displayName":"bot","description":"d","ownerUserIDs":["1","2"]}`,
		},
		{
			name:       "list include deleted",
			args:       []string{"list", "--include-deleted"},
			wantMethod: http.MethodGet,
			wantPath:   "/api/agents",
			wantQuery:  "includeDeleted=true",
		},
		{
			name:       "get",
			args:       []string{"get", "7"},
			wantMethod: http.MethodGet,
			wantPath:   "/api/agents/7",
		},
		{
			name:       "disable",
			args:       []string{"disable", "7", "--reason", "leaked"},
			wantMethod: http.MethodPost,
			wantPath:   "/api/agents/7/disable",
			wantBody:   `{"reason":"leaked"}`,
		},
		{
			name:       "enable",
			args:       []string{"enable", "7"},
			wantMethod: http.MethodPost,
			wantPath:   "/api/agents/7/enable",
		},
		{
			name:       "delete with yes",
			args:       []string{"delete", "7", "--yes"},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/agents/7",
		},
		{
			name:       "keys list",
			args:       []string{"keys", "list", "7", "--show-revoked"},
			wantMethod: http.MethodGet,
			wantPath:   "/api/agents/7/api-keys",
			wantQuery:  "show_revoked=true",
		},
		{
			name:       "keys get",
			args:       []string{"keys", "get", "7", "3"},
			wantMethod: http.MethodGet,
			wantPath:   "/api/agents/7/api-keys/3",
		},
		{
			name:       "keys revoke",
			args:       []string{"keys", "revoke", "7", "3"},
			wantMethod: http.MethodDelete,
			wantPath:   "/api/agents/7/api-keys/3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, requests := agentsTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodDelete:
					w.WriteHeader(http.StatusNoContent)
				case strings.Contains(r.URL.Path, "/api-keys/"):
					writeTestJSON(w, types.AgentAPIKey{ID: 3, AgentID: "7"})
				case strings.HasSuffix(r.URL.Path, "/api-keys") || r.URL.Path == "/api/agents":
					if r.Method == http.MethodGet {
						_, _ = w.Write([]byte(`{"items":[]}`))
						return
					}
					writeTestJSON(w, testAgent("7"))
				default:
					writeTestJSON(w, testAgent("7"))
				}
			})
			_, _, err := runAgents(t, server, tt.args...)
			require.NoError(t, err)
			reqs := requests()
			require.Len(t, reqs, 1)
			require.Equal(t, tt.wantMethod, reqs[0].Method)
			require.Equal(t, tt.wantPath, reqs[0].Path)
			require.Equal(t, tt.wantQuery, reqs[0].Query)
			if tt.wantBody == "" {
				require.Empty(t, reqs[0].Body)
			} else {
				require.JSONEq(t, tt.wantBody, reqs[0].Body)
			}
		})
	}
}

func TestAgentsKeysCreate(t *testing.T) {
	created := func(w http.ResponseWriter, _ *http.Request) {
		var resp types.AgentAPIKeyCreateResponse
		resp.ID, resp.AgentID, resp.Name = 3, "7", "ci"
		resp.CanAccessLLMProxy = true
		resp.MCPServerIDs = []string{"*", "v2"}
		resp.Key = "ok1-7-3-secret"
		writeTestJSON(w, resp)
	}

	t.Run("repeated mcp and star pass through", func(t *testing.T) {
		server, requests := agentsTestServer(t, created)
		_, _, err := runAgents(t, server, "keys", "create", "7", "--name", "ci", "--mcp", "*", "--mcp", "v2", "--llm", "--skills")
		require.NoError(t, err)
		reqs := requests()
		require.Len(t, reqs, 1)
		require.Equal(t, "/api/agents/7/api-keys", reqs[0].Path)
		require.JSONEq(t, `{"name":"ci","mcpServerIds":["*","v2"],"canAccessLLMProxy":true,"canAccessSkills":true}`, reqs[0].Body)
	})

	t.Run("human output puts only the key on stdout", func(t *testing.T) {
		server, _ := agentsTestServer(t, created)
		stdout, stderr, err := runAgents(t, server, "keys", "create", "7", "--llm")
		require.NoError(t, err)
		require.Equal(t, "ok1-7-3-secret\n", stdout)
		require.Contains(t, stderr, "will not be shown again")
		require.Contains(t, stderr, "ci")
		require.NotContains(t, stderr, "ok1-7-3-secret")
	})

	t.Run("json output is the full response", func(t *testing.T) {
		server, _ := agentsTestServer(t, created)
		stdout, stderr, err := runAgents(t, server, "keys", "create", "7", "--llm", "--json")
		require.NoError(t, err)
		var got types.AgentAPIKeyCreateResponse
		require.NoError(t, json.Unmarshal([]byte(stdout), &got))
		require.Equal(t, "ok1-7-3-secret", got.Key)
		require.Equal(t, uint(3), got.ID)
		require.Contains(t, stderr, "will not be shown again")
	})

	t.Run("no scope flags surfaces the 400 with a hint", func(t *testing.T) {
		server, _ := agentsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "an API key needs at least one scope", http.StatusBadRequest)
		})
		stdout, _, err := runAgents(t, server, "keys", "create", "7")
		require.ErrorContains(t, err, "at least one scope")
		require.ErrorContains(t, err, "--mcp '*', --llm or --skills")
		require.Empty(t, stdout)
	})

	t.Run("expiry flags are exclusive", func(t *testing.T) {
		server, requests := agentsTestServer(t, created)
		_, _, err := runAgents(t, server, "keys", "create", "7", "--llm", "--expires-in", "1h", "--expires-at", "2030-01-02T03:04:05Z")
		require.ErrorContains(t, err, "mutually exclusive")
		require.Empty(t, requests())
	})

	t.Run("expires-at is sent", func(t *testing.T) {
		server, requests := agentsTestServer(t, created)
		_, _, err := runAgents(t, server, "keys", "create", "7", "--llm", "--expires-at", "2030-01-02T03:04:05Z")
		require.NoError(t, err)
		require.Contains(t, requests()[0].Body, `"expiresAt":"2030-01-02T03:04:05Z"`)
	})
}

func TestAgentsDeleteRefusesWithoutYesOffTerminal(t *testing.T) {
	server, requests := agentsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	_, _, err := runAgents(t, server, "delete", "7")
	require.ErrorContains(t, err, "--yes")
	require.Empty(t, requests())
}

func TestAgentsErrorMapping(t *testing.T) {
	tests := []struct {
		name    string
		handle  func(w http.ResponseWriter, r *http.Request)
		args    []string
		wantErr string
	}{
		{
			name: "403 requires an administrator",
			handle: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "forbidden", http.StatusForbidden)
			},
			args:    []string{"list"},
			wantErr: "requires an administrator",
		},
		{
			name: "404 on every route means the feature is off",
			handle: func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, "not found", http.StatusNotFound)
			},
			args:    []string{"get", "7"},
			wantErr: "agent identities are not enabled on this server",
		},
		{
			name: "404 with the feature on is a missing agent",
			handle: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/agents" {
					_, _ = w.Write([]byte(`{"items":[]}`))
					return
				}
				http.Error(w, "agent not found", http.StatusNotFound)
			},
			args:    []string{"get", "7"},
			wantErr: "agent not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := agentsTestServer(t, tt.handle)
			_, _, err := runAgents(t, server, tt.args...)
			require.ErrorContains(t, err, tt.wantErr)
			if tt.name != "404 with the feature on is a missing agent" {
				require.NotContains(t, err.Error(), "error code")
			}
		})
	}
}

func TestAgentsListOutput(t *testing.T) {
	server, _ := agentsTestServer(t, func(w http.ResponseWriter, _ *http.Request) {
		agent := testAgent("7")
		agent.DisplayName = "bot"
		agent.OwnerUserIDs = []string{"1"}
		agent.Status = types.UserStatusActive
		agent.ActiveAPIKeyCount = 2
		writeTestJSON(w, types.AgentIdentityList{Items: []types.AgentIdentity{agent}})
	})
	stdout, _, err := runAgents(t, server, "list")
	require.NoError(t, err)
	require.Contains(t, stdout, "ID")
	require.Regexp(t, `7\s+bot\s+active\s+2\s+1`, stdout)

	stdout, _, err = runAgents(t, server, "list", "--json")
	require.NoError(t, err)
	var list types.AgentIdentityList
	require.NoError(t, json.Unmarshal([]byte(stdout), &list))
	require.Len(t, list.Items, 1)
}

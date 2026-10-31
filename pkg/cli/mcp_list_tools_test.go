package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/cmd"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeToolListServer answers a tool list request after polls polls with final. Polls before
// failFirst fail with failStatus.
type fakeToolListServer struct {
	t          *testing.T
	polls      int
	final      types.ToolListRequest
	failFirst  int
	failStatus int

	lock    sync.Mutex
	polled  int
	created types.ToolListRequestManifest
}

func (f *fakeToolListServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.lock.Lock()
	defer f.lock.Unlock()
	if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
		f.t.Errorf("authorization = %q, want bearer token", got)
	}
	pending := types.ToolListRequest{
		ID:        "tlr1",
		EntryID:   "default-everything",
		EntryName: "Everything",
		Status:    types.ToolListRequestPending,
		ExpiresAt: types.NewTime(time.Now().Add(time.Minute)),
		URL:       "https://obot.example.com/mcp-servers/list-tools/tlr1",
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/tool-list-requests":
		require.NoError(f.t, json.NewDecoder(r.Body).Decode(&f.created))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(pending)
	case r.Method == http.MethodGet && r.URL.Path == "/api/tool-list-requests/tlr1":
		f.polled++
		if f.polled <= f.failFirst {
			http.Error(w, "unavailable", f.failStatus)
			return
		}
		if f.polled < f.polls {
			_ = json.NewEncoder(w).Encode(pending)
			return
		}
		_ = json.NewEncoder(w).Encode(f.final)
	default:
		f.t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func runListTools(t *testing.T, fake *fakeToolListServer, args ...string) (string, string, []string, error) {
	t.Helper()
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	return runListToolsWithRoot(t, mcpTestRoot(server.URL), args...)
}

func runListToolsWithRoot(t *testing.T, root *Obot, args ...string) (string, string, []string, error) {
	t.Helper()

	interval := toolListPollInterval
	toolListPollInterval = 5 * time.Millisecond
	var opened []string
	open := openToolListURL
	openToolListURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() {
		toolListPollInterval = interval
		openToolListURL = open
	})

	var stdout, stderr bytes.Buffer
	command := cmd.Command(&MCP{root: root})
	command.SetContext(t.Context())
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs(append([]string{"list-tools"}, args...))
	err := command.Execute()
	return stdout.String(), stderr.String(), opened, err
}

func TestMCPListToolsWaitsForConfirmation(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 3, final: types.ToolListRequest{
		ID:        "tlr1",
		EntryID:   "default-everything",
		EntryName: "Everything",
		Status:    types.ToolListRequestCompleted,
		Tools: []types.MCPServerTool{
			{Name: "echo", Description: "Echoes its input"},
			{Name: "get-sum", Description: "Adds two\n  numbers"},
			{Name: "ping"},
		},
	}}
	stdout, stderr, opened, err := runListTools(t, fake, "default-everything")
	require.NoError(t, err)

	assert.Equal(t, "default-everything", fake.created.EntryID)
	assert.Equal(t, []string{"https://obot.example.com/mcp-servers/list-tools/tlr1"}, opened)
	assert.Equal(t, 3, fake.polled, "the command waits until the request is answered")
	assert.Contains(t, stderr, "Confirm it in the Obot page opening in your browser")

	// The tools print as text an agent can use as is: exact names, one per line.
	assert.Equal(t, `Everything (default-everything) has 3 tools. Use these exact names in allowedTools and toolOverrides:

- echo: Echoes its input
- get-sum: Adds two numbers
- ping
`, stdout)
}

func TestMCPListToolsPrintsJSON(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 1, final: types.ToolListRequest{
		ID:        "tlr1",
		EntryID:   "default-everything",
		EntryName: "Everything",
		Status:    types.ToolListRequestCompleted,
		Tools:     []types.MCPServerTool{{Name: "echo", Description: "Echoes its input"}},
	}}
	stdout, _, _, err := runListTools(t, fake, "--json", "--non-interactive", "default-everything")
	require.NoError(t, err)

	var out mcpListToolsOutput
	require.NoError(t, json.Unmarshal([]byte(stdout), &out), stdout)
	assert.Equal(t, mcpListToolsOutput{
		EntryID:   "default-everything",
		EntryName: "Everything",
		Tools:     []mcpListToolsTool{{Name: "echo", Description: "Echoes its input"}},
	}, out)
}

func TestMCPListToolsReportsADeclinedRequest(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 1, final: types.ToolListRequest{
		ID:      "tlr1",
		EntryID: "default-everything",
		Status:  types.ToolListRequestFailed,
		Error:   "the user declined to launch the server",
	}}
	stdout, _, _, err := runListTools(t, fake, "default-everything")
	require.ErrorContains(t, err, "the user declined to launch the server")
	assert.Empty(t, stdout)
}

func TestMCPListToolsPrintsTheURLWithoutABrowser(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 1, final: types.ToolListRequest{
		ID:      "tlr1",
		EntryID: "default-everything",
		Status:  types.ToolListRequestCompleted,
	}}
	_, stderr, opened, err := runListTools(t, fake, "--no-browser", "default-everything")
	require.NoError(t, err)
	assert.Empty(t, opened)
	assert.Contains(t, stderr, "https://obot.example.com/mcp-servers/list-tools/tlr1")
}

// Without a login, the command logs in first, with the CLI's default scopes, rather than failing.
func TestMCPListToolsLogsInWhenNeeded(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 1, final: types.ToolListRequest{
		ID:      "tlr1",
		EntryID: "default-everything",
		Status:  types.ToolListRequestCompleted,
	}}
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)

	var fetchedScopes []string
	root := &Obot{Client: (&apiclient.Client{BaseURL: server.URL + "/api"}).WithTokenFetcher(
		func(_ context.Context, _ string, opts apiclient.TokenFetchOptions) (string, error) {
			fetchedScopes = opts.Scopes
			return "test-token", nil
		})}
	_, _, _, err := runListToolsWithRoot(t, root, "default-everything")
	require.NoError(t, err)
	assert.Equal(t, types.DefaultCLIAPIKeyScopes(), fetchedScopes)
	assert.Equal(t, "default-everything", fake.created.EntryID)
}

// A server error while waiting doesn't end the wait: the user may still confirm.
func TestMCPListToolsKeepsWaitingThroughServerErrors(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 4, failFirst: 2, failStatus: http.StatusServiceUnavailable, final: types.ToolListRequest{
		ID:      "tlr1",
		EntryID: "default-everything",
		Status:  types.ToolListRequestCompleted,
		Tools:   []types.MCPServerTool{{Name: "echo"}},
	}}
	stdout, stderr, _, err := runListTools(t, fake, "default-everything")
	require.NoError(t, err)
	assert.Contains(t, stderr, "still waiting")
	assert.Equal(t, 1, strings.Count(stderr, "still waiting"), "the retry note prints once")
	assert.Contains(t, stdout, "- echo")
}

// An error Obot returns for the request itself ends the wait.
func TestMCPListToolsStopsWhenTheRequestIsGone(t *testing.T) {
	fake := &fakeToolListServer{t: t, polls: 3, failFirst: 1, failStatus: http.StatusNotFound}
	_, _, _, err := runListTools(t, fake, "default-everything")
	require.ErrorContains(t, err, "check the request to list the tools")
	assert.Equal(t, 1, fake.polled)
}

//go:build integration

package harness

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

// CreateMCPServer creates an MCP server at POST /api/mcp-servers and
// registers a cleanup that deletes it when the test ends. The returned server
// has its server-assigned fields populated (ID, etc.).
//
// Requires the caller to be admin or to supply a CatalogEntryID — see the
// CreateServer handler in pkg/api/handlers/mcp.go for the rules. In dev mode
// with EnableAuthentication=false, the no-auth user is admin.
func (h *Harness) CreateMCPServer(ctx context.Context, server types.MCPServer) types.MCPServer {
	h.T.Helper()
	var created types.MCPServer
	h.Post(ctx, "/api/mcp-servers", server, &created)
	h.AddCleanup(func() {
		// Best-effort: ignore failures on shutdown, but never let cleanup hang.
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = h.Status(ctx, http.MethodDelete, "/api/mcp-servers/"+created.ID)
	})
	return created
}

// GetMCPServer fetches the current state of an MCP server by ID.
func (h *Harness) GetMCPServer(ctx context.Context, id string) types.MCPServer {
	h.T.Helper()
	var s types.MCPServer
	h.Get(ctx, "/api/mcp-servers/"+id, &s)
	return s
}

// ConfigureMCPServer applies environment-variable configuration to an MCP
// server. Empty env is allowed — the act of POSTing flips the Configured flag
// to true when no required env vars are missing.
func (h *Harness) ConfigureMCPServer(ctx context.Context, id string, env map[string]string) {
	h.T.Helper()
	if env == nil {
		env = map[string]string{}
	}
	h.Post(ctx, "/api/mcp-servers/"+id+"/configure", env, nil)
}

// LaunchMCPServer asks obot to deploy the MCP server (Docker container,
// remote connection, etc.).
func (h *Harness) LaunchMCPServer(ctx context.Context, id string) {
	h.T.Helper()
	h.Post(ctx, "/api/mcp-servers/"+id+"/launch", map[string]any{}, nil)
}

// WaitForMCPServerAvailable polls backend-neutral deployment details until the
// server is available or the timeout elapses.
func (h *Harness) WaitForMCPServerAvailable(ctx context.Context, id string, timeout time.Duration) types.MCPServerDetails {
	h.T.Helper()
	deadline := time.Now().Add(timeout)
	var last types.MCPServerDetails
	for time.Now().Before(deadline) {
		h.Get(ctx, "/api/mcp-servers/"+id+"/details", &last)
		if last.IsAvailable {
			return last
		}
		time.Sleep(1 * time.Second)
	}
	h.T.Fatalf("MCP server %s did not become available within %s (last details=%+v)", id, timeout, last)
	return last
}

// WaitForMCPServerDeleted waits for the server's finalizers to finish and the
// API object to disappear.
func (h *Harness) WaitForMCPServerDeleted(ctx context.Context, id string, timeout time.Duration) {
	h.T.Helper()
	path := "/api/mcp-servers/" + id
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.Status(ctx, http.MethodGet, path) == http.StatusNotFound {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	h.T.Fatalf("MCP server %s was not deleted within %s", id, timeout)
}

// MCPServerName returns a name unique to this run, suitable for use in
// MCPServerManifest.Name. Collisions across parallel runs are avoided by the
// run ID embedded by the harness.
func (h *Harness) MCPServerName(prefix string) string {
	return fmt.Sprintf("test-%s-%s", h.RunID, prefix)
}

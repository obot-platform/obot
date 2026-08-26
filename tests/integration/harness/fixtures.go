//go:build integration

package harness

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

// CreateMCPCatalogEntry creates a catalog entry and registers cleanup after
// all subsequently registered resources have been removed.
func (h *Harness) CreateMCPCatalogEntry(t *testing.T, catalogID string, manifest types.MCPServerCatalogEntryManifest) types.MCPServerCatalogEntry {
	t.Helper()
	var created types.MCPServerCatalogEntry
	h.Post(t, "/api/mcp-catalogs/"+catalogID+"/entries", manifest, &created)
	h.AddCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = h.status(ctx, http.MethodDelete, "/api/mcp-catalogs/"+catalogID+"/entries/"+created.ID)
	})
	return created
}

// CreateMCPServerFromCatalogEntry creates a single-user server from entryID.
func (h *Harness) CreateMCPServerFromCatalogEntry(t *testing.T, entryID string) types.MCPServer {
	t.Helper()
	var created types.MCPServer
	h.Post(t, "/api/mcp-servers", types.MCPServer{CatalogEntryID: entryID}, &created)
	h.AddCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = h.status(ctx, http.MethodDelete, "/api/mcp-servers/"+created.ID)
	})
	return created
}

// GetMCPServer fetches the current state of an MCP server by ID.
func (h *Harness) GetMCPServer(t *testing.T, id string) types.MCPServer {
	t.Helper()
	var s types.MCPServer
	h.Get(t, "/api/mcp-servers/"+id, &s)
	return s
}

// ConfigureMCPServer applies environment-variable configuration to an MCP
// server. Empty env is allowed — the act of POSTing flips the Configured flag
// to true when no required env vars are missing.
func (h *Harness) ConfigureMCPServer(t *testing.T, id string, env map[string]string) {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	h.Post(t, "/api/mcp-servers/"+id+"/configure", env, nil)
}

// LaunchMCPServer asks obot to deploy the MCP server (Docker container,
// remote connection, etc.).
func (h *Harness) LaunchMCPServer(t *testing.T, id string) {
	t.Helper()
	h.Post(t, "/api/mcp-servers/"+id+"/launch", map[string]any{}, nil)
}

// RestartMCPServer replaces the running MCP server deployment and waits for
// the backend to report that the replacement is ready.
func (h *Harness) RestartMCPServer(t *testing.T, id string) {
	t.Helper()
	h.Post(t, "/api/mcp-servers/"+id+"/restart", map[string]any{}, nil)
}

// WaitForMCPServerAvailable polls backend-neutral deployment details until the
// server is available or the timeout elapses.
func (h *Harness) WaitForMCPServerAvailable(t *testing.T, id string, timeout time.Duration) types.MCPServerDetails {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last types.MCPServerDetails
	for time.Now().Before(deadline) {
		h.Get(t, "/api/mcp-servers/"+id+"/details", &last)
		if last.IsAvailable {
			return last
		}
		time.Sleep(1 * time.Second)
	}
	t.Fatalf("MCP server %s did not become available within %s (last details=%+v)", id, timeout, last)
	return last
}

// WaitForMCPServerDeleted waits for the server's finalizers to finish and the
// API object to disappear.
func (h *Harness) WaitForMCPServerDeleted(t *testing.T, id string, timeout time.Duration) {
	t.Helper()
	path := "/api/mcp-servers/" + id
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.Status(t, http.MethodGet, path) == http.StatusNotFound {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("MCP server %s was not deleted within %s", id, timeout)
}

// MCPServerName returns a name unique to this run, suitable for use in
// MCPServerManifest.Name. Collisions across parallel runs are avoided by the
// run ID embedded by the harness.
func (h *Harness) MCPServerName(prefix string) string {
	return fmt.Sprintf("test-%s-%s", h.RunID, prefix)
}

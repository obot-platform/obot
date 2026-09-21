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

// CreateAccessControlRule creates a catalog-scoped access rule and registers
// cleanup before the catalog entries referenced by the rule are removed.
func (h *Harness) CreateAccessControlRule(t *testing.T, catalogID string, manifest types.AccessControlRuleManifest) types.AccessControlRule {
	t.Helper()
	var created types.AccessControlRule
	h.Post(t, "/api/mcp-catalogs/"+catalogID+"/access-control-rules", manifest, &created)
	h.AddCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = h.status(ctx, http.MethodDelete, "/api/mcp-catalogs/"+catalogID+"/access-control-rules/"+created.ID)
	})
	return created
}

// WaitForMCPCatalogEntryAccess waits for the access-control informer to
// observe a newly-created rule and expose the entry to the test principal.
func (h *Harness) WaitForMCPCatalogEntryAccess(t *testing.T, catalogID, entryID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var entries types.MCPServerCatalogEntryList
		h.Get(t, "/api/mcp-catalogs/"+catalogID+"/entries", &entries)
		for _, entry := range entries.Items {
			if entry.ID == entryID {
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("MCP catalog entry %s did not become accessible within %s", entryID, timeout)
}

// CreatePersonalVMCP creates a vMCP owned by the test principal and registers
// cleanup. Personal vMCPs are the single-user equivalent of catalog servers.
func (h *Harness) CreatePersonalVMCP(t *testing.T, manifest types.VMCPManifest) types.VMCP {
	t.Helper()
	var created types.VMCP
	h.Post(t, "/api/vmcps?scope=personal", manifest, &created)
	h.AddCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = h.status(ctx, http.MethodDelete, "/api/vmcps/"+created.ID)
	})
	return created
}

// CreateVMCPInstance creates the test principal's connection to vmcpID.
func (h *Harness) CreateVMCPInstance(t *testing.T, vmcpID string) types.VMCPInstance {
	t.Helper()
	var created types.VMCPInstance
	h.Post(t, "/api/vmcp-instances", types.VMCPInstanceManifest{VMCPID: vmcpID}, &created)
	h.AddCleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = h.status(ctx, http.MethodDelete, "/api/vmcp-instances/"+created.ID)
	})
	return created
}

// ConfigureVMCPInstance stores user-allowed configuration values for one vMCP
// component on the instance.
func (h *Harness) ConfigureVMCPInstance(t *testing.T, instanceID, componentID string, values map[string]string) {
	t.Helper()
	h.Post(t, "/api/vmcp-instances/"+instanceID+"/configure", types.VMCPConfiguration{
		Components: map[string]map[string]string{componentID: values},
	}, nil)
}

// WaitForVMCPInstanceConfigured waits for the controller to report that the
// instance has every required configuration value and that its configuration
// hash differs from previousHash. Pass an empty previousHash on first configure.
func (h *Harness) WaitForVMCPInstanceConfigured(t *testing.T, instanceID, previousHash string, timeout time.Duration) types.VMCPInstance {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last types.VMCPInstance
	for time.Now().Before(deadline) {
		h.Get(t, "/api/vmcp-instances/"+instanceID, &last)
		if last.Status.Configured && last.Status.UserConfigurationHash != previousHash {
			return last
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("vMCP instance %s was not configured within %s (last=%+v)", instanceID, timeout, last)
	return last
}

// LaunchVMCP asks obot to deploy every component server of the vMCP for the
// test principal.
func (h *Harness) LaunchVMCP(t *testing.T, vmcpID string) {
	t.Helper()
	h.Post(t, "/api/vmcps/"+vmcpID+"/launch", map[string]any{}, nil)
}

// WaitForVMCPInstanceComponentServer waits for the single-user component
// server that backs componentID on instanceID and returns its ID.
func (h *Harness) WaitForVMCPInstanceComponentServer(t *testing.T, instanceID, componentID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var servers types.MCPServerList
		h.Get(t, "/api/mcp-servers", &servers)
		for _, server := range servers.Items {
			if server.VMCPInstanceID == instanceID && server.VMCPComponentID == componentID {
				return server.ID
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("component server for vMCP instance %s component %s was not created within %s", instanceID, componentID, timeout)
	return ""
}

// RestartMCPServer replaces the running MCP server deployment.
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

// WaitForDeleted waits for the object at path to finish its finalizers and
// disappear from the API.
func (h *Harness) WaitForDeleted(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.Status(t, http.MethodGet, path) == http.StatusNotFound {
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s was not deleted within %s", path, timeout)
}

// MCPServerName returns a name unique to this run, suitable for use in
// catalog entry and vMCP names. Collisions across parallel runs are avoided by the
// run ID embedded by the harness.
func (h *Harness) MCPServerName(prefix string) string {
	return fmt.Sprintf("test-%s-%s", h.RunID, prefix)
}

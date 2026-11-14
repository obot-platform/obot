//go:build integration

package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authn"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/tests/integration/harness"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// mcpUser is a user and the API key their MCP client connects with.
type mcpUser struct {
	id  string
	key string
}

// bearerTransport sends every request as the bearer of key, the way an MCP client configured with an API key does.
type bearerTransport struct {
	key string
}

func (b bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+b.key)
	return http.DefaultTransport.RoundTrip(req)
}

// TestMCPConnections connects MCP clients to a shared vMCP whose component runs a server for each connection, while
// billing changes what fits. Connecting is what creates a user's connection, so it's where a plan without room
// refuses them. Nothing is deployed: without Docker a connection that's admitted fails later, when its server
// starts.
func TestMCPConnections(t *testing.T) {
	owner := signInOwner(t)
	// The members need seats, whatever plan the last test left.
	setPlan(t, owner, types.Entitlements{})
	hosted := createCatalogEntry(t, owner, "hosted", types.MCPServerCatalogEntryManifest{
		Runtime: types.RuntimeNPX,
		NPXConfig: &types.NPXRuntimeConfig{
			Package: "@modelcontextprotocol/server-everything",
		},
		Resources: &types.MCPResourceRequirements{
			Requests: types.MCPResourceRequests{
				Memory: fmt.Sprintf("%dMi", serverMemoryMiB),
				CPU:    fmt.Sprintf("%dm", serverCPUMillicores),
			},
		},
	})
	manifest := vmcp(owner, "shared", types.VMCPComponent{
		Name:                    "server",
		MCPCatalogID:            system.DefaultCatalog,
		MCPServerCatalogEntryID: hosted,
		ForceSingleUser:         true,
	})
	manifest.Profiles = []types.VMCPProfile{{
		Name: "everyone",
		Subjects: []types.Subject{{
			Type: types.SubjectTypeSelector,
			ID:   "*",
		}},
		Permissions: types.VMCPProfilePermissions{
			AllowAllComponents: true,
		},
	}}
	shared := owner.CreateSharedVMCP(t, manifest)
	path := "/mcp-connect/" + shared.ID
	url := owner.BaseURL + path

	admin := newMCPUser(t, owner, shared.ID)
	first := newMCPUser(t, signInMember(t, owner, "first"), shared.ID)
	second := newMCPUser(t, signInMember(t, owner, "second"), shared.ID)
	waitForUsage(t, owner, 0, 0)

	refusals := []struct {
		name    string
		granted types.Entitlements
		refusal string
	}{
		{
			name: "memory",
			granted: types.Entitlements{
				MemoryMiB: new(int64(serverMemoryMiB - 1)),
			},
			refusal: memoryRefusal,
		},
		{
			name: "CPU",
			granted: types.Entitlements{
				CPUMillicores: new(int64(serverCPUMillicores - 1)),
			},
			refusal: cpuRefusal,
		},
	}
	for _, tt := range refusals {
		t.Run("a connection that doesn't fit the plan's "+tt.name+" is refused", func(t *testing.T) {
			setPlan(t, owner, tt.granted)
			requireConnectionRefused(t, url, first, tt.refusal)

			// A client that doesn't speak the SDK sees the same thing: an answer to its call, not an HTTP error.
			status, body := initialize(t, path, first)
			var answer struct {
				ID    int `json:"id"`
				Error struct {
					Code    int64  `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(body), &answer); status != http.StatusOK || err != nil {
				t.Fatalf("initialize = %d %s, want a JSON-RPC answer (%v)", status, body, err)
			}
			if answer.ID != 1 || answer.Error.Code != jsonrpc.CodeInternalError || answer.Error.Message != tt.refusal {
				t.Fatalf("initialize = %s, want error %d %q for call 1", body, jsonrpc.CodeInternalError, tt.refusal)
			}

			// Nothing is left of the refused connection.
			if got := connections(t, shared.ID); len(got) != 0 {
				t.Fatalf("connections = %v, want none", got)
			}
			waitForUsage(t, owner, 0, 0)
		})
	}

	t.Run("the same connection is admitted once the plan has room", func(t *testing.T) {
		setPlan(t, owner, types.Entitlements{
			MemoryMiB:     new(int64(serverMemoryMiB)),
			CPUMillicores: new(int64(serverCPUMillicores)),
		})
		requireConnectionAdmitted(t, url, first)
		if got := connections(t, shared.ID); len(got) != 1 || got[0] != first.id {
			t.Fatalf("connections = %v, want one, for user %s", got, first.id)
		}
		waitForUsage(t, owner, serverMemoryMiB, serverCPUMillicores)
		status, body := initialize(t, path, first)
		t.Logf("an admitted connection's initialize, without Docker, answers: %d %s", status, body)
	})

	t.Run("the next user's connection doesn't fit", func(t *testing.T) {
		requireConnectionRefused(t, url, second, memoryRefusal)
		if got := connections(t, shared.ID); len(got) != 1 {
			t.Fatalf("connections = %v, want only the first user's", got)
		}
		waitForUsage(t, owner, serverMemoryMiB, serverCPUMillicores)
	})

	// The plan shrinks under the connection that exists.
	setPlan(t, owner, types.Entitlements{
		MemoryMiB: new(int64(0)),
	})
	eventually(t, "the over-limit clock to start", func() error {
		if getSubscription(t).Status.OverLimitSince == nil {
			return fmt.Errorf("the clock hasn't started")
		}
		return nil
	})

	t.Run("over the limit, a connection that exists is still admitted", func(t *testing.T) {
		requireConnectionAdmitted(t, url, first)
		requireConnectionRefused(t, url, second, memoryRefusal)
		if got := connections(t, shared.ID); len(got) != 1 || got[0] != first.id {
			t.Fatalf("connections = %v, want one, for user %s", got, first.id)
		}
		waitForUsage(t, owner, serverMemoryMiB, serverCPUMillicores)
	})

	// The grace period is 3 days, so the clock is set back instead of waited out.
	setOverLimitSince(t, time.Now().Add(-3*24*time.Hour-time.Minute))

	t.Run("restricted, a member's client is refused before it reaches the vMCP", func(t *testing.T) {
		// The restriction is read from a cache, so it takes a moment to apply.
		var (
			status int
			body   string
		)
		eventually(t, "the member's client to be refused", func() error {
			if status, body = initialize(t, path, first); status != http.StatusPaymentRequired {
				return fmt.Errorf("initialize = %d %s, want 402", status, body)
			}
			return nil
		})
		requireRefused(t, http.StatusPaymentRequired, authn.ErrRestricted.Message)(status, body)

		// It's refused as an HTTP error, which isn't an answer to the client's call. The SDK's client reports the
		// status and drops the body, so its user sees "Payment Required" without the reason.
		_, err := connect(t, url, first)
		if _, ok := errors.AsType[*jsonrpc.Error](err); err == nil || ok || !strings.Contains(err.Error(), http.StatusText(http.StatusPaymentRequired)) {
			t.Fatalf("connect = %v, want an HTTP 402", err)
		}
		t.Logf("a restricted member's MCP client reports: %v", err)
	})

	t.Run("restricted, an admin's client gets through to the plan's limits", func(t *testing.T) {
		requireConnectionRefused(t, url, admin, memoryRefusal)
	})

	t.Run("a plan with room lets the member connect again", func(t *testing.T) {
		setPlan(t, owner, types.Entitlements{})
		eventually(t, "the member's client to be let in", func() error {
			if status, body := initialize(t, path, first); status == http.StatusPaymentRequired {
				return fmt.Errorf("initialize = %d %s", status, body)
			}
			return nil
		})
		requireConnectionAdmitted(t, url, first)
		requireConnectionAdmitted(t, url, second)
		waitForUsage(t, owner, 2*serverMemoryMiB, 2*serverCPUMillicores)
	})
}

// signInMember adds a local account, signs in with it, and returns its session.
func signInMember(t *testing.T, owner *harness.Harness, name string) *harness.Harness {
	t.Helper()
	member := harness.NewSession(t)
	if status, body := member.SignIn(t, createLocalUser(t, owner, name), memberPassword); status != http.StatusOK {
		t.Fatalf("%s sign-in = %d %s, want 200", name, status, body)
	}
	return member
}

// newMCPUser makes the signed-in user an API key for the vMCP, which is what their MCP client connects with.
func newMCPUser(t *testing.T, session *harness.Harness, vmcpID string) mcpUser {
	t.Helper()
	var me struct {
		ID string `json:"id"`
	}
	session.Get(t, "/api/me", &me)
	var created struct {
		Key string `json:"key"`
	}
	session.Post(t, "/api/api-keys", map[string]any{
		"name":         "billing integration test " + session.RunID,
		"mcpServerIds": []string{vmcpID},
	}, &created)
	if me.ID == "" || created.Key == "" {
		t.Fatalf("user %q got API key %t, want both", me.ID, created.Key != "")
	}
	return mcpUser{
		id:  me.ID,
		key: created.Key,
	}
}

// connect connects an MCP client to the vMCP at url as user, which initializes a session.
func connect(t *testing.T, url string, user mcpUser) (*gomcp.ClientSession, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	client := gomcp.NewClient(&gomcp.Implementation{
		Name:    "billing-integration-test",
		Version: "1.0.0",
	}, nil)
	session, err := client.Connect(ctx, &gomcp.StreamableClientTransport{
		Endpoint:             url,
		HTTPClient:           &http.Client{Transport: bearerTransport{key: user.key}},
		MaxRetries:           -1,
		DisableStandaloneSSE: true,
	}, nil)
	if err == nil {
		t.Cleanup(func() { _ = session.Close() })
	}
	return session, err
}

// initialize sends the first call of an MCP session as plain HTTP, and returns the response's status and body.
func initialize(t *testing.T, path string, user mcpUser) (int, string) {
	t.Helper()
	client := harness.New(t)
	client.BearerToken = user.key
	return client.Request(t, http.MethodPost, path, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "billing-integration-test",
				"version": "1.0.0",
			},
		},
	})
}

// requireConnectionRefused checks that the user's MCP client is told the plan has no room, as the answer to its
// first call.
func requireConnectionRefused(t *testing.T, url string, user mcpUser, refusal string) {
	t.Helper()
	_, err := connect(t, url, user)
	rpcErr, ok := errors.AsType[*jsonrpc.Error](err)
	if !ok || rpcErr.Code != jsonrpc.CodeInternalError || rpcErr.Message != refusal {
		t.Fatalf("connect = %v, want JSON-RPC error %d %q", err, jsonrpc.CodeInternalError, refusal)
	}
	if want := `calling "initialize": ` + refusal; err.Error() != want {
		t.Fatalf("connect = %q, want %q", err, want)
	}
}

// requireConnectionAdmitted checks that the plan doesn't refuse the user's MCP client. Its server can't start here,
// so the client still fails, later and for that reason.
func requireConnectionAdmitted(t *testing.T, url string, user mcpUser) {
	t.Helper()
	_, err := connect(t, url, user)
	if err == nil {
		return
	}
	for _, refusal := range []string{memoryRefusal, cpuRefusal, http.StatusText(http.StatusPaymentRequired)} {
		if strings.Contains(err.Error(), refusal) {
			t.Fatalf("connect = %v, want it past the plan's limits", err)
		}
	}
	t.Logf("an admitted MCP client, whose server can't start without Docker, reports: %v", err)
}

// connections returns the IDs of the users who have a connection to the vMCP.
func connections(t *testing.T, vmcpID string) []string {
	t.Helper()
	var instances v1.VMCPInstanceList
	if err := storage.List(t.Context(), &instances, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		t.Fatalf("list the vMCP's connections: %v", err)
	}
	var users []string
	for _, instance := range instances.Items {
		if instance.Spec.Manifest.VMCPID == vmcpID && instance.DeletionTimestamp.IsZero() {
			users = append(users, instance.Spec.UserID)
		}
	}
	return users
}

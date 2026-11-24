package mcpgateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/adminmcp"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Block the tool's direct storage call, which receives the MCP handler context,
// rather than the initial catalog lookup, which uses api.Context.
type blockingAdminStorage struct {
	storage.Client
	started  chan struct{}
	canceled chan struct{}
	release  chan struct{}
}

func (s *blockingAdminStorage) List(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) error {
	if _, ok := list.(*v1.MCPServerCatalogEntryList); ok {
		close(s.started)
		select {
		case <-ctx.Done():
			close(s.canceled)
			return ctx.Err()
		case <-s.release:
			return context.Canceled
		}
	}
	return s.Client.List(ctx, list, opts...)
}

func TestBuiltinAdminToolStopsWhenRequestIsCanceled(t *testing.T) {
	for _, protocol := range []string{"2025-06-18", "2025-11-25", "2026-07-28"} {
		t.Run(protocol, func(t *testing.T) {
			const sourceURL = "https://github.com/example/catalog"
			store := &blockingAdminStorage{
				Client: fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.MCPCatalog{
					Name:      system.DefaultCatalog,
					Namespace: system.DefaultNamespace,
					Spec:      v1.MCPCatalogSpec{SourceURLs: []string{sourceURL}},
				}).Build(),
				started:  make(chan struct{}),
				canceled: make(chan struct{}),
				release:  make(chan struct{}),
			}
			// Release blocked work even if the regression assertion fails.
			t.Cleanup(func() { close(store.release) })
			admin := adminmcp.NewHandler("https://obot.example.com", "kubernetes", "", nil, nil)
			h := &Handler{}
			handlerDone := make(chan struct{})
			h.RegisterBuiltin(adminmcp.BuiltinName, func(req api.Context) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer close(handlerDone)
					admin.Builtin(req).ServeHTTP(w, r)
				})
			})
			client, endpoint, err := h.builtinClient(api.Context{
				Storage: store,
				User: &user.DefaultInfo{
					UID:    "42",
					Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
					Extra:  map[string][]string{"obot_groups": types.RoleAdmin.Groups()},
				},
			}, mcp.ServerConfig{BuiltinName: adminmcp.BuiltinName})
			require.NoError(t, err)

			params := map[string]any{
				"name":      "search_catalog_entries",
				"arguments": map[string]any{"sourceURL": sourceURL},
			}
			if protocol == "2026-07-28" {
				params["_meta"] = map[string]any{
					gomcp.MetaKeyProtocolVersion:    protocol,
					gomcp.MetaKeyClientInfo:         map[string]any{"name": "test", "version": "1"},
					gomcp.MetaKeyClientCapabilities: map[string]any{},
				}
			}
			body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params})
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("MCP-Protocol-Version", protocol)
			req.Header.Set("Mcp-Method", "tools/call")
			req.Header.Set("Mcp-Name", "search_catalog_entries")
			requestDone := make(chan error, 1)
			go func() {
				resp, err := client.Do(req)
				if resp != nil {
					_ = resp.Body.Close()
				}
				requestDone <- err
			}()
			select {
			case <-store.started:
			case err := <-requestDone:
				t.Fatalf("request ended before storage was called: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("tool did not reach storage")
			}
			cancel()
			select {
			case err := <-requestDone:
				require.True(t, errors.Is(err, context.Canceled), "request error: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("in-process request did not stop")
			}
			select {
			case <-store.canceled:
			case <-time.After(time.Second):
				t.Fatal("tool storage context survived request cancellation")
			}
			select {
			case <-handlerDone:
			case <-time.After(5 * time.Second):
				t.Fatal("built-in handler did not finish closing its MCP session")
			}
		})
	}
}

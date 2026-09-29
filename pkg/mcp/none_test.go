package mcp

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

func TestNoneBackendAllowsRemoteAndVMCP(t *testing.T) {
	for _, runtime := range []types.Runtime{types.RuntimeRemote, types.RuntimeVMCP} {
		t.Run(string(runtime), func(t *testing.T) {
			backend := newNoneBackend(8080)
			server := ServerConfig{
				Runtime: runtime,
				URL:     "https://mcp.example.com/mcp",
			}
			if err := backend.deployServer(t.Context(), server); err != nil {
				t.Fatalf("deployServer: %v", err)
			}
			got, err := backend.ensureServerDeployment(t.Context(), server)
			if err != nil {
				t.Fatalf("ensureServerDeployment: %v", err)
			}
			if got.URL != server.URL {
				t.Fatalf("ensureServerDeployment changed the URL: got %q, want %q", got.URL, server.URL)
			}
		})
	}
}

func TestNoneBackendRefusesHostedServers(t *testing.T) {
	for _, runtime := range []types.Runtime{types.RuntimeUVX, types.RuntimeNPX, types.RuntimeContainerized} {
		t.Run(string(runtime), func(t *testing.T) {
			backend := newNoneBackend(8080)
			server := ServerConfig{
				Runtime: runtime,
			}
			if err := backend.deployServer(t.Context(), server); !isNotSupportedByNone(err) {
				t.Fatalf("deployServer: got %v, want ErrNotSupportedByBackend", err)
			}
			if _, err := backend.ensureServerDeployment(t.Context(), server); !isNotSupportedByNone(err) {
				t.Fatalf("ensureServerDeployment: got %v, want ErrNotSupportedByBackend", err)
			}
		})
	}
}

func TestNoneBackendShutdownIsNoop(t *testing.T) {
	// Disabled system MCP servers are shut down at startup; with nothing ever
	// deployed this must succeed rather than retry forever.
	if err := newNoneBackend(8080).shutdownServer(t.Context(), "any-server", true); err != nil {
		t.Fatalf("shutdownServer: %v", err)
	}
}

func TestNoneBackendUnsupportedOperations(t *testing.T) {
	backend := newNoneBackend(8080)
	if _, err := backend.streamServerLogs(t.Context(), "id"); !isNotSupportedByNone(err) {
		t.Fatalf("streamServerLogs: got %v, want ErrNotSupportedByBackend", err)
	}
	if _, err := backend.getServerDetails(t.Context(), "id"); !isNotSupportedByNone(err) {
		t.Fatalf("getServerDetails: got %v, want ErrNotSupportedByBackend", err)
	}
	if err := backend.restartServer(t.Context(), ServerConfig{}); !isNotSupportedByNone(err) {
		t.Fatalf("restartServer: got %v, want ErrNotSupportedByBackend", err)
	}
}

func TestNoneBackendKeepsRemoteURLValidation(t *testing.T) {
	global := RemoteMCPURLValidationConfig{}
	got, extra := newNoneBackend(8080).remoteConfig(global)
	if got != global {
		t.Fatalf("remoteConfig changed the validation settings: got %+v, want %+v", got, global)
	}
	// only Obot's own listener: the target of transformObotHostname
	if len(extra) != 1 || extra[0] != "localhost:8080" {
		t.Fatalf("remoteConfig allowed hosts = %v, want [localhost:8080]", extra)
	}
}

// With the default DisallowLocalhostMCP=true, Obot's calls to itself on the local
// listener (e.g. a filter via /mcp-connect) must still pass the loopback block of
// the MCP HTTP client, while any other localhost port stays blocked.
func TestNoneBackendSelfCallsPassDefaultLoopbackBlock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	obot := httptest.NewServer(handler)
	defer obot.Close()
	other := httptest.NewServer(handler)
	defer other.Close()

	port, err := strconv.Atoi(strings.TrimPrefix(obot.URL, "http://127.0.0.1:"))
	if err != nil {
		t.Fatalf("test server port: %v", err)
	}
	backend := newNoneBackend(port)
	manager := &SessionManager{backend: backend, remoteURLValidationConfig: RemoteMCPURLValidationConfig{}}
	client, err := manager.HTTPClientForServer(ServerConfig{}, HTTPClientOptions{})
	if err != nil {
		t.Fatalf("HTTPClientForServer: %v", err)
	}

	resp, err := client.Get(backend.transformObotHostname("https://obot.example.com/mcp-connect/sms1filter"))
	if err != nil {
		t.Fatalf("self-call on the local listener was blocked: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("self-call status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}

	otherURL := strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
	if resp, err := client.Get(otherURL); err == nil {
		_ = resp.Body.Close()
		t.Fatalf("another localhost port (%s) was allowed", otherURL)
	}
}

func TestNoneBackendTransformObotHostnameUsesLocalListener(t *testing.T) {
	backend := newNoneBackend(8080)
	tests := map[string]string{
		"https://mcp.internal.example.com/mcp-connect/sms1abc":    "http://localhost:8080/mcp-connect/sms1abc",
		"https://obot.example.com/oauth/token?audience=mcp":       "http://localhost:8080/oauth/token?audience=mcp",
		"http://user:pw@obot.example.com:8443/mcp-connect/ms1abc": "http://localhost:8080/mcp-connect/ms1abc",
		"https://[2001:db8::1]:8443/mcp-connect/ms1abc":           "http://localhost:8080/mcp-connect/ms1abc",
		"https://obot.example.com/mcp-connect/ms1abc#fragment":    "http://localhost:8080/mcp-connect/ms1abc#fragment",
		"https://obot.example.com":                                "http://localhost:8080",
		"/mcp-connect/ms1abc":                                     "/mcp-connect/ms1abc",
		"":                                                        "",
		"not-a-url":                                               "not-a-url",
	}
	for input, expected := range tests {
		if got := backend.transformObotHostname(input); got != expected {
			t.Fatalf("transformObotHostname(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestNoneBackendTransformObotHostnameUsesListenPort(t *testing.T) {
	got := newNoneBackend(9999).transformObotHostname("https://obot.example.com/mcp-connect/ms1abc")
	if want := "http://localhost:9999/mcp-connect/ms1abc"; got != want {
		t.Fatalf("transformObotHostname = %q, want %q", got, want)
	}
}

// Filters (MCP webhook validations) are system MCP servers that Obot calls through its
// own /mcp-connect endpoint. With the none backend that call must go to the local
// listener, while the audience keeps the public URL the filter's token is issued for.
func TestNoneBackendWebhooksCallObotOnLocalListener(t *testing.T) {
	indexers := cache.Indexers{}
	for _, index := range []string{"server-names", "catalog-entry-names", "catalog-names", "selectors"} {
		indexers[index] = func(obj any) ([]string, error) {
			if index != "selectors" {
				return nil, nil
			}
			return []string{"*"}, nil
		}
	}
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, indexers)
	webhook := &v1.MCPWebhookValidation{}
	webhook.Name = "filter"
	webhook.Namespace = system.DefaultNamespace
	webhook.Spec.Manifest = types.MCPWebhookValidationManifest{
		Resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
		Selectors: types.MCPSelectors{{Method: "tools/call"}},
	}
	webhook.Status.Configured = true
	if err := indexer.Add(webhook); err != nil {
		t.Fatal(err)
	}

	manager := &SessionManager{
		webhookHelper: NewWebhookHelper(indexer, "https://obot.example.com"),
		backend:       newNoneBackend(8080),
	}
	webhooks, err := manager.webhooksForServerConfig(ServerConfig{
		MCPServerName:      "ms1remote",
		MCPServerNamespace: system.DefaultNamespace,
		Runtime:            types.RuntimeRemote,
		URL:                "https://mcp.example.com/mcp",
	})
	if err != nil {
		t.Fatalf("webhooksForServerConfig: %v", err)
	}
	if len(webhooks) != 1 {
		t.Fatalf("got %d webhooks, want 1", len(webhooks))
	}
	id := system.SystemMCPServerPrefix + webhook.Name
	if want := system.MCPConnectURL("http://localhost:8080", id); webhooks[0].URL != want {
		t.Fatalf("webhook URL = %q, want %q", webhooks[0].URL, want)
	}
	if want := system.MCPConnectURL("https://obot.example.com", id); webhooks[0].Audience != want {
		t.Fatalf("webhook audience = %q, want %q", webhooks[0].Audience, want)
	}
}

func isNotSupportedByNone(err error) bool {
	var nse *ErrNotSupportedByBackend
	return errors.As(err, &nse) && nse.Backend == RuntimeBackendNone
}

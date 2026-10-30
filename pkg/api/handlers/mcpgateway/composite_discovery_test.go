package mcpgateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp/catalog"
	"github.com/obot-platform/mmmcp/component"
	mmmcpconfig "github.com/obot-platform/mmmcp/config"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	obotmcp "github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

type attributionDiscoverer struct {
	calls atomic.Int32
}

func (d *attributionDiscoverer) Discover(context.Context, mmmcpconfig.Server) (*component.Features, error) {
	d.calls.Add(1)
	return &component.Features{Tools: []*gomcp.Tool{{
		Name:        "echo",
		InputSchema: map[string]any{"type": "object"},
	}}}, nil
}

func TestCompositeDiscoveryCacheIgnoresCallerAttribution(t *testing.T) {
	gatewayClient := newLocalAgentAuditLogTestGatewayClient(t)
	discoverer := &attributionDiscoverer{}
	registry := catalog.NewRegistry(discoverer)
	defer registry.Close()
	config := obotmcp.ServerConfig{
		Runtime:       types.RuntimeVMCP,
		MCPServerName: "vmcp",
		Components: []obotmcp.ComponentServer{{
			DisplayName: "component",
			URL:         "https://obot.example/mcp-connect/component",
		}},
	}
	profileChanged := headerTestCaller("42", "first-writer")
	profileChanged.Name = "alice-renamed"
	profileChanged.Extra["email"] = []string{"alice-new@example.com"}
	for _, caller := range []*user.DefaultInfo{
		headerTestCaller("42", "first-writer"),
		headerTestCaller("43", "second-writer"),
		headerTestCaller("42", "renamed-writer"),
		profileChanged,
		headerTestCaller("", ""),
		headerTestCaller("42", "first-writer"),
	} {
		cfg, err := compositeConfig(api.Context{
			Request:       httptest.NewRequest(http.MethodPost, "/mcp", nil),
			GatewayClient: gatewayClient,
			User:          caller,
		}, config)
		require.NoError(t, err)
		_, _, err = registry.Get(t.Context(), cfg)
		require.NoError(t, err)
	}
	require.Equal(t, int32(1), discoverer.calls.Load(), "credentials and account metadata must not create discovery catalogs")

	config.ConfigHash = "updated-component-configuration"
	cfg, err := compositeConfig(api.Context{
		Request:       httptest.NewRequest(http.MethodPost, "/mcp", nil),
		GatewayClient: gatewayClient,
		User:          profileChanged,
	}, config)
	require.NoError(t, err)
	_, _, err = registry.Get(t.Context(), cfg)
	require.NoError(t, err)
	require.Equal(t, int32(2), discoverer.calls.Load(), "actual component changes must still invalidate discovery")
}

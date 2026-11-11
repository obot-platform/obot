package handlers

import (
	"testing"

	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/stretchr/testify/require"
)

func TestMarketOpenDefaultsFalse(t *testing.T) {
	if marketOpenEnabled() {
		t.Fatal("marketOpenEnabled() = true, want false by default")
	}

	t.Setenv("OBOT_MARKET_OPEN", "true")
	if !marketOpenEnabled() {
		t.Fatal("marketOpenEnabled() = false, want true when enabled")
	}
}

func TestHostingCapabilities(t *testing.T) {
	handler := NewVersionHandler(VersionHandlerOptions{})

	worker := handler.hostingCapabilities(mcp.RuntimeBackendOpenAPIWorker)
	require.False(t, worker["dynamicHosting"].(bool))
	require.True(t, worker["openAPIWorker"].(bool))

	docker := handler.hostingCapabilities("docker")
	require.True(t, docker["dynamicHosting"].(bool))
	require.False(t, docker["openAPIWorker"].(bool))
}

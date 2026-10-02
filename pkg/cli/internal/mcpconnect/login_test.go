package mcpconnect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/adrg/xdg"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

func TestLoginReturnsWithoutStartingStdio(t *testing.T) {
	previous := xdg.DataHome
	xdg.DataHome = t.TempDir()
	t.Cleanup(func() { xdg.DataHome = previous })
	mcpServer := gomcp.NewServer(&gomcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	server := httptest.NewServer(gomcp.NewStreamableHTTPHandler(func(*http.Request) *gomcp.Server {
		return mcpServer
	}, nil))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	require.NoError(t, Login(ctx, server.URL+"/mcp-connect/test"))
}

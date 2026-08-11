package mcp

import (
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/pkg/safehttp"
)

// HTTPClientForServer returns an HTTP client that follows the server's
// configured network path, including its tunnel when present.
func (sm *SessionManager) HTTPClientForServer(server ServerConfig, allowHosts []string, headers http.Header, timeout time.Duration) (*http.Client, error) {
	if server.TunnelName == "" {
		remoteURLValidationConfig, backendAllowHosts := sm.RemoteConfigForBackend()
		allowHosts = append(allowHosts, backendAllowHosts...)

		return safehttp.NewClient(safehttp.ClientOptions{
			BlockLoopback:  !remoteURLValidationConfig.AllowLocalhostMCP,
			BlockPrivateIP: !remoteURLValidationConfig.AllowPrivateIPMCP,
			BlockLinkLocal: !remoteURLValidationConfig.AllowLinkLocalMCP,
			AllowList:      allowHosts,
			Timeout:        timeout,
			Headers:        headers,
		}), nil
	}

	if sm.tunnelManager == nil {
		return nil, fmt.Errorf("tunnel manager is not configured")
	}
	return sm.tunnelManager.HTTPClient(server.TunnelName, headers, timeout)
}

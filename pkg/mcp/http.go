package mcp

import (
	"fmt"
	"net/http"
	"time"

	"github.com/obot-platform/obot/pkg/safehttp"
	"github.com/obot-platform/obot/pkg/tunnel"
	"golang.org/x/oauth2"
)

type HTTPClientOptions struct {
	TokenSource   oauth2.TokenSource
	Timeout       time.Duration
	DirectConnect bool
}

// HTTPClientForServer returns an HTTP client that follows the server's
// configured network path, including its tunnel when present.
func (sm *SessionManager) HTTPClientForServer(server ServerConfig, opts HTTPClientOptions) (*http.Client, error) {
	headers := serverConfigHeaders(server)
	if (opts.DirectConnect || server.UserID == "system") && server.TunnelName != "" {
		if sm.tunnelManager == nil {
			return nil, fmt.Errorf("tunnel manager is not configured")
		}

		client, err := sm.tunnelManager.HTTPClient(server.TunnelName, tunnel.HTTPClientOptions{
			Headers:     headers,
			Timeout:     opts.Timeout,
			TokenSource: opts.TokenSource,
		})
		return client, err
	}

	remoteValidationConfig, allowedHosts := sm.RemoteConfigForBackend()

	return safehttp.NewClient(safehttp.Options{
		BlockLoopback:  !remoteValidationConfig.AllowLocalhostMCP,
		BlockPrivateIP: !remoteValidationConfig.AllowPrivateIPMCP,
		BlockLinkLocal: !remoteValidationConfig.AllowLinkLocalMCP,
		AllowList:      allowedHosts,
		Timeout:        opts.Timeout,
		Headers:        headers,
		TokenSource:    opts.TokenSource,
	}), nil
}

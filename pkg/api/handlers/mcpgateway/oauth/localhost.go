package oauth

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (f *MCPOAuthHandlerFactory) upstreamRedirectURL(req api.Context, config mcp.ServerConfig, authRequestID string) (string, error) {
	defaultURL := system.MCPOAuthCallbackURL(f.baseURL)
	if !config.LocalhostCallbackEnabled || authRequestID == "" {
		return defaultURL, nil
	}
	var request v1.OAuthAuthRequest
	if err := req.Get(&request, authRequestID); err != nil {
		return "", fmt.Errorf("load originating OAuth request: %w", err)
	}
	if request.Spec.UserID != req.UserID() {
		return "", fmt.Errorf("localhost callback requires an OAuth request belonging to the user")
	}
	return localhostRedirectURL(defaultURL, request.Spec.RedirectURI, config.LocalhostCallbackPath), nil
}

func localhostRedirectURL(defaultURL, clientRedirect, callbackPath string) string {
	u, err := url.Parse(clientRedirect)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Port() == "" {
		return defaultURL
	}
	ip := net.ParseIP(u.Hostname())
	if !strings.EqualFold(u.Hostname(), "localhost") && (ip == nil || !ip.IsLoopback()) {
		return defaultURL
	}
	if err := mcp.ValidateLocalhostCallbackPath(callbackPath); err != nil {
		return defaultURL
	}
	if callbackPath == "" {
		callbackPath = mcp.DefaultLocalhostCallbackPath
	}
	callback, _ := url.Parse(callbackPath) // validated above
	u.Path, u.RawPath = callback.Path, callback.RawPath
	u.RawQuery, u.Fragment = "", ""
	u.ForceQuery = false
	return u.String()
}

func (f *MCPOAuthHandlerFactory) clientMetadataForRedirect(redirectURL string) string {
	// The hosted metadata document advertises only the hosted callback. A local
	// callback must use static credentials or dynamic client registration instead.
	if redirectURL != system.MCPOAuthCallbackURL(f.baseURL) {
		return ""
	}
	return f.cimdDocumentURL
}

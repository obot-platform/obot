package oauth

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"golang.org/x/oauth2"
)

const (
	localLoginTTL = 10 * time.Minute
)

// localLoginURL refers to the existing UI OAuth state, not an MCP connection.
// In particular, tool previews retain their temporary server and system user.
func (f *MCPOAuthHandlerFactory) localLoginURL(authURL string, config mcp.ServerConfig, authRequestID string) (string, error) {
	if authURL == "" || !config.LocalhostCallbackEnabled || authRequestID != "" {
		return authURL, nil
	}
	u, err := url.Parse(authURL)
	if err != nil || u.Query().Get("state") == "" {
		return "", fmt.Errorf("local OAuth authorization URL has no state")
	}
	return strings.TrimRight(f.baseURL, "/") + "/oauth/mcp/login/" + url.PathEscape(u.Query().Get("state")), nil
}

func uiLocalLoginState(ps *gatewaytypes.MCPOAuthPendingState) bool {
	if ps == nil || ps.OAuthAuthRequestID != "" {
		return false
	}
	u, err := url.Parse(ps.RedirectURL)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(u.Port())
	return err == nil && port >= 1024 && port <= 65535 && u.Path != "" && u.Scheme == "http" && u.Hostname() == "localhost" && u.Port() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && mcp.ValidateLocalhostCallbackPath(u.EscapedPath()) == nil
}

// The random pending state is a short-lived capability. Never return its PKCE
// verifier, client secret, configuration, or upstream tokens to the CLI.
func (h *handler) localLogin(req api.Context) error {
	req.ResponseWriter.Header().Set("Cache-Control", "no-store")
	ps, err := h.oauthChecker.stateMgr.gatewayClient.GetMCPOAuthPendingState(req.Context(), req.PathValue("state"))
	if err != nil || !uiLocalLoginState(ps) || time.Since(ps.CreatedAt) > localLoginTTL {
		return types.NewErrBadRequest("authentication attempt expired; retry authentication in Obot")
	}
	conf := &oauth2.Config{
		ClientID:    ps.ClientID,
		RedirectURL: ps.RedirectURL,
		Scopes:      strings.Fields(ps.Scopes),
		Endpoint:    oauth2.Endpoint{AuthURL: ps.AuthURL},
	}
	authURL, err := mcp.AuthCodeURL(conf, ps.AuthURL, ps.ResourceURL, ps.State, ps.Verifier)
	if err != nil {
		return err
	}
	return req.Write(types.MCPLocalLogin{AuthorizationURL: authURL, RedirectURL: ps.RedirectURL, State: ps.State})
}

func completeLocalLogin(req api.Context, ps *gatewaytypes.MCPOAuthPendingState, loginErr error) {
	u, _ := url.Parse(ps.RedirectURL)
	u.Path, u.RawPath = "/oauth/obot/callback", ""
	query := url.Values{"state": {ps.State}}
	if loginErr != nil {
		query.Set("error", "authentication_failed")
	} else {
		query.Set("status", "complete")
	}
	u.RawQuery = query.Encode()
	http.Redirect(req.ResponseWriter, req.Request, u.String(), http.StatusFound)
}

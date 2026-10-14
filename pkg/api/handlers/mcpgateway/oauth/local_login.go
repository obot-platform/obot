package oauth

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

const (
	localLoginTTL         = 10 * time.Minute
	localLoginStatePrefix = "ui-local-"
)

// localLoginURL refers to the existing OAuth attempt, not an MCP connection.
// In particular, tool previews retain their temporary server and system user.
func (f *MCPOAuthHandlerFactory) localLoginURL(authURL string, config mcp.ServerConfig) (string, error) {
	if authURL == "" || !config.LocalhostCallbackEnabled {
		return authURL, nil
	}
	u, err := url.Parse(authURL)
	if err != nil || u.Query().Get("state") == "" {
		return "", fmt.Errorf("local OAuth authorization URL has no state")
	}
	loginURL := strings.TrimRight(f.baseURL, "/") + "/oauth/mcp/login/" + url.PathEscape(u.Query().Get("state"))
	// Display-only context identifies the component requesting authentication,
	// including when the caller is connecting to an aggregate vMCP.
	if config.MCPServerDisplayName != "" {
		loginURL += "?" + url.Values{"serverName": {config.MCPServerDisplayName}}.Encode()
	}
	return loginURL, nil
}

func localLoginState(ps *gatewaytypes.MCPOAuthPendingState) bool {
	if ps == nil || !strings.HasPrefix(ps.State, localLoginStatePrefix) {
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
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to get pending state: %w", err)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || !localLoginState(ps) {
		return types.NewErrBadRequest("authentication attempt expired; retry authentication in Obot")
	}
	remaining := time.Until(ps.CreatedAt.Add(localLoginTTL))
	if remaining <= 0 {
		return types.NewErrHTTP(http.StatusGone, "authentication attempt expired; request a new command in Obot")
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
	return req.Write(types.MCPLocalLogin{AuthorizationURL: authURL, RedirectURL: ps.RedirectURL, State: ps.State, ExpiresIn: int(math.Ceil(remaining.Seconds()))})
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

func validateLocalLoginRequest(req api.Context, ps *gatewaytypes.MCPOAuthPendingState) error {
	if ps.OAuthAuthRequestID == "" {
		return nil
	}
	var request v1.OAuthAuthRequest
	if err := req.Get(&request, ps.OAuthAuthRequestID); err != nil {
		return err
	}
	if !req.UserIsAuthenticated() || req.User.GetUID() != ps.UserID || request.Spec.UserID != req.UserID() || !request.Spec.ConsentApproved {
		return fmt.Errorf("local login requires the user who approved the OAuth request")
	}
	return nil
}

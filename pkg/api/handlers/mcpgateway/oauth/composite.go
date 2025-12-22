package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type pendingComponentAuth struct {
	CatalogEntryID string `json:"catalogEntryID"`
	MCPServerID    string `json:"mcpServerID"`
	AuthURL        string `json:"authURL"`
}

// checkCompositeAuth checks if the composite OAuth flow is complete.
// If it is not complete, it returns the list of component OAuth URLs still needed (respecting session-scoped skips).
func (h *handler) checkCompositeAuth(req api.Context) error {
	var (
		compositeMCPID     = req.PathValue("mcp_id")
		oauthAuthRequestID = req.URL.Query().Get("oauth_auth_request")
	)
	var compositeServer v1.MCPServer
	if err := req.Get(&compositeServer, compositeMCPID); err != nil {
		return fmt.Errorf("failed to get composite server: %w", err)
	}

	var authRequest v1.OAuthAuthRequest
	if oauthAuthRequestID != "" {
		if err := req.Get(&authRequest, oauthAuthRequestID); err != nil {
			return fmt.Errorf("failed to get OAuth auth request: %w", err)
		}
	}

	var componentServers v1.MCPServerList
	if err := req.Storage.List(req.Context(), &componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	var (
		userID  = req.User.GetUID()
		pending = make([]pendingComponentAuth, 0, len(componentServers.Items))
	)
	// Build disabled set by catalog entry ID for O(1) checks
	var compositeConfig types.CompositeRuntimeConfig
	if compositeServer.Spec.Manifest.CompositeConfig != nil {
		compositeConfig = *compositeServer.Spec.Manifest.CompositeConfig
	}

	disabledComponents := make(map[string]bool, len(compositeConfig.ComponentServers))
	for _, comp := range compositeConfig.ComponentServers {
		disabledComponents[comp.CatalogEntryID] = comp.Disabled
	}

	for _, componentServer := range componentServers.Items {
		if disabledComponents[componentServer.Spec.MCPServerCatalogEntryName] ||
			componentServer.Spec.Manifest.Runtime != types.RuntimeRemote {
			continue
		}

		_, serverConfig, err := handlers.ServerForAction(req, componentServer.Name)
		if err != nil {
			return fmt.Errorf("failed to get server config: %w", err)
		}

		authURL, err := h.oauthChecker.CheckForMCPAuth(req, componentServer, serverConfig, userID, componentServer.Name, oauthAuthRequestID)
		if err != nil || authURL == "" {
			continue
		}

		pending = append(pending, pendingComponentAuth{
			CatalogEntryID: componentServer.Spec.MCPServerCatalogEntryName,
			MCPServerID:    componentServer.Name,
			AuthURL:        authURL,
		})
	}

	if len(pending) > 0 {
		// There are still pending second level OAuth requests
		return req.Write(pending)
	}

	if oauthAuthRequestID != "" {
		// All pending second level OAuth requests are complete, so produce a new authorization code and return redirect URL as JSON for client-side redirect.
		code := strings.ToLower(rand.Text() + rand.Text())
		authRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
		if err := req.Update(&authRequest); err != nil {
			redirectErr := Error{
				Code:        ErrServerError,
				Description: err.Error(),
			}
			return req.Write(map[string]string{
				"redirect_uri": authRequest.Spec.RedirectURI + "?" + redirectErr.toQuery().Encode(),
			})
		}

		// Return redirect URL as JSON instead of performing server-side redirect
		// This avoids CORS issues when called from JavaScript fetch
		q := url.Values{
			"code":  {code},
			"state": {authRequest.Spec.State},
		}
		redirectURL := authRequest.Spec.RedirectURI + "?" + q.Encode()
		return req.Write(map[string]string{
			"redirect_uri": redirectURL,
		})
	}

	return req.Write(pending)
}

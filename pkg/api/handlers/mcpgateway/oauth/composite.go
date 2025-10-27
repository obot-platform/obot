package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"
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
	_, compositeServer, _, err := handlers.ServerForActionWithConnectID(req, compositeMCPID)
	if err != nil {
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
	for _, componentServer := range componentServers.Items {
		// Check if this component is enabled in the composite config
		isEnabled := true // Default to enabled if not found in config
		if compositeServer.Spec.Manifest.CompositeConfig != nil {
			for _, comp := range compositeServer.Spec.Manifest.CompositeConfig.ComponentServers {
				if comp.CatalogEntryID == componentServer.Spec.MCPServerCatalogEntryName {
					isEnabled = comp.Enabled
					break
				}
			}
		}

		if !isEnabled {
			continue
		}

		if componentServer.Spec.Manifest.Runtime != types.RuntimeRemote {
			continue
		}

		serverConfig, err := handlers.ServerConfigForAction(req, componentServer)
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
		// All pending second level OAuth requests are complete, so produce a new authorization code and redirect back to the first-level client redirect. Complete first level OAuth by redirecting to the first level client URL.
		code := strings.ToLower(rand.Text() + rand.Text())
		authRequest.Spec.HashedAuthCode = fmt.Sprintf("%x", sha256.Sum256([]byte(code)))
		if err := req.Update(&authRequest); err != nil {
			redirectWithAuthorizeError(req, authRequest.Spec.RedirectURI, Error{
				Code:        ErrServerError,
				Description: err.Error(),
			})
			return nil
		}
		redirectWithAuthorizeResponse(req, authRequest, code)
	}

	return nil
}

package oauth

import (
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type pendingComponentAuth struct {
	CatalogEntryID string `json:"catalogEntryID"`
	MCPServerID    string `json:"mcpServerID"`
	Name           string `json:"name,omitempty"`
	Icon           string `json:"icon,omitempty"`
	AuthURL        string `json:"authURL"`
}

type componentAuthStatus struct {
	AuthURL string `json:"authURL,omitempty"`
}

func pendingComponentDisplayName(componentServer v1.MCPServer) string {
	if componentServer.Spec.Manifest.Name != "" {
		return componentServer.Spec.Manifest.Name
	}
	return componentServer.Name
}

func (h *handler) checkVMCPComponentAuth(req api.Context) error {
	var component v1.MCPServer
	if err := req.Get(&component, req.PathValue("component_mcp_id")); err != nil {
		return err
	}
	authRequestID := req.URL.Query().Get("oauth_auth_request")
	if authRequestID != "" {
		var authRequest v1.OAuthAuthRequest
		if err := req.Get(&authRequest, authRequestID); err != nil {
			return err
		}
		if authRequest.Spec.UserID != req.UserID() || authRequest.Spec.MCPID != req.PathValue("mcp_id") || !authRequest.Spec.ConsentApproved {
			return types.NewErrForbidden("OAuth request does not belong to this approved vMCP connection")
		}
	}
	connectID := component.Name
	if component.Spec.VMCPID != "" && component.Spec.Manifest.Runtime == types.RuntimeRemote {
		config, err := h.oauthChecker.mcpSessionManager.ServerConfigForVMCP(req.Context(), req.PathValue("mcp_id"), req.User.GetUID())
		if err != nil {
			return err
		}
		connectID = ""
		for _, configured := range config.Components {
			if configured.Name == component.Name {
				connectID = configured.ConnectID()
				break
			}
		}
		if connectID == "" {
			return types.NewErrForbidden("component does not belong to this vMCP connection")
		}
	}
	authURL, err := h.vmcpComponentAuthURL(req, component, connectID, authRequestID)
	if err != nil {
		return err
	}
	return req.Write(componentAuthStatus{AuthURL: authURL})
}

func (h *handler) vmcpComponentAuthURL(req api.Context, component v1.MCPServer, connectID, authRequestID string) (string, error) {
	if component.Spec.Manifest.Runtime != types.RuntimeRemote {
		return "", nil
	}
	server, config, err := h.oauthChecker.mcpSessionManager.ServerForAction(req.Context(), connectID, req.User.GetUID())
	if err != nil {
		return "", fmt.Errorf("failed to get component server config: %w", err)
	}
	return h.oauthChecker.CheckForMCPAuth(req, server, config, req.User.GetUID(), connectID, authRequestID)
}

// checkVMCPAuth checks if the vMCP OAuth flow is complete.
// If it is not complete, it returns the list of component OAuth URLs still needed (respecting session-scoped skips).
func (h *handler) checkVMCPAuth(req api.Context) error {
	var (
		vMCPID             = req.PathValue("mcp_id")
		oauthAuthRequestID = req.URL.Query().Get("oauth_auth_request")
	)
	_, vMCPServer, compositeConfig, err := h.oauthChecker.mcpSessionManager.ServerForActionWithConnectID(req.Context(), vMCPID, req.User.GetUID())
	if err != nil {
		return fmt.Errorf("failed to get vMCP server: %w", err)
	}

	var authRequest v1.OAuthAuthRequest
	if oauthAuthRequestID != "" {
		if err := req.Get(&authRequest, oauthAuthRequestID); err != nil {
			return fmt.Errorf("failed to get OAuth auth request: %w", err)
		}
		if authRequest.Spec.UserID != req.UserID() || authRequest.Spec.MCPID != vMCPID || !authRequest.Spec.ConsentApproved {
			return types.NewErrForbidden("OAuth request does not belong to this approved vMCP connection")
		}
	}

	componentServers, err := h.oauthChecker.componentServersForAuth(req, vMCPServer, compositeConfig)
	if err != nil {
		return err
	}

	var (
		lock    sync.Mutex
		errs    []error
		limit   = make(chan struct{}, 5) // Limit concurrent requests
		pending = make([]pendingComponentAuth, 0, len(componentServers))
	)

	// Fill the channel so we can drain when we're done.
	for range cap(limit) {
		limit <- struct{}{}
	}

	defer close(limit)

	for i, componentServer := range componentServers {
		if componentServer.Spec.Manifest.Runtime != types.RuntimeRemote {
			continue
		}

		// Acquire semaphore
		<-limit

		go func() {
			defer func() {
				limit <- struct{}{}
			}()

			authURL, err := h.vmcpComponentAuthURL(req, componentServer, compositeConfig.Components[i].ConnectID(), oauthAuthRequestID)
			if err != nil {
				lock.Lock()
				defer lock.Unlock()
				errs = append(errs, fmt.Errorf("failed to check component %s authentication: %w", componentServer.Name, err))
				return
			}

			if authURL == "" {
				return
			}

			lock.Lock()
			defer lock.Unlock()

			pending = append(pending, pendingComponentAuth{
				CatalogEntryID: componentServer.Spec.MCPServerCatalogEntryName,
				MCPServerID:    componentServer.Name,
				Name:           pendingComponentDisplayName(componentServer),
				Icon:           componentServer.Spec.Manifest.Icon,
				AuthURL:        authURL,
			})
		}()
	}

	// Wait for all to finish
	for range cap(limit) {
		<-limit
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	if len(pending) > 0 {
		// There are still pending second level OAuth requests
		slog.Debug("vMCP OAuth still pending component authentication", "vMCPID", vMCPID, "pendingComponents", len(pending))
		return req.Write(pending)
	}

	if oauthAuthRequestID != "" {
		// Return completion page URL as JSON instead of performing server-side redirect.
		// This avoids CORS issues when called from JavaScript fetch.
		redirectURL := oauthCompletionURL(authRequest.Name)
		slog.Info("vMCP OAuth completed; returning completion URI to finish authorization", "vMCPID", vMCPID, "authRequest", authRequest.Name)
		return req.Write(map[string]string{
			"redirect_uri": redirectURL,
		})
	}

	slog.Info("vMCP OAuth check completed with no pending component authentication", "vMCPID", vMCPID)
	return req.Write(pending)
}

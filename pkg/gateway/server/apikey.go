package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type createAPIKeyRequest struct {
	Name            string     `json:"name"`
	Description     string     `json:"description,omitempty"`
	ExpiresAt       *time.Time `json:"expiresAt,omitempty"`
	MCPServerIDs    []string   `json:"mcpServerIds,omitempty"`
	CanAccessSkills bool       `json:"canAccessSkills"`
}

// createAPIKey creates an API key for the authenticated user.
func (s *Server) createAPIKey(apiContext api.Context) error {
	var req createAPIKeyRequest
	if err := apiContext.Read(&req); err != nil {
		return types2.NewErrBadRequest("invalid request body: %v", err)
	}

	if req.Name == "" {
		return types2.NewErrBadRequest("name is required")
	}

	if len(req.MCPServerIDs) == 0 && !req.CanAccessSkills {
		return types2.NewErrBadRequest("at least one MCP server must be specified or skills access must be enabled")
	}

	userID := apiContext.UserID()
	if userID == 0 {
		pkgLog.Infof("Rejecting API key creation for unauthenticated request")
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	// Validate that the user has access to all specified MCPServers
	// "*" is a special wildcard that grants access to all servers the user can access
	var errs []error
	for _, serverID := range req.MCPServerIDs {
		if serverID == "*" {
			// Wildcard - no validation needed at creation time
			// Access is checked at authentication time
			continue
		}

		var server v1.MCPServer
		if err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: serverID}, &server); err != nil {
			return types2.NewErrBadRequest("MCP server %q not found", serverID)
		}

		// Check if user has access to this server
		hasAccess, err := s.userHasAccessToMCPServer(apiContext, &server)
		if err != nil {
			return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to check access to MCP server: %v", err))
		}
		if !hasAccess {
			errs = append(errs, fmt.Errorf("MCP server %q not found", serverID))
		}
	}

	if len(errs) > 0 {
		pkgLog.Infof("Rejecting API key creation due to unauthorized MCP server selections: userID=%d requestedServers=%d deniedServers=%d", userID, len(req.MCPServerIDs), len(errs))
		return types2.NewErrHTTP(http.StatusBadRequest, errors.Join(errs...).Error())
	}

	response, err := apiContext.GatewayClient.CreateAPIKey(apiContext.Context(), userID, req.Name, req.Description, req.ExpiresAt, req.MCPServerIDs, req.CanAccessSkills)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to create API key: %v", err))
	}
	pkgLog.Infof("Created API key for user: userID=%d serverScopes=%d", userID, len(req.MCPServerIDs))

	return apiContext.WriteCreated(response)
}

// userHasAccessToMCPServer checks if the current user has access to the given MCPServer.
func (s *Server) userHasAccessToMCPServer(apiContext api.Context, server *v1.MCPServer) (bool, error) {
	userID := apiContext.User.GetUID()

	// Owner always has access
	if server.Spec.UserID == userID {
		return true, nil
	}

	// Check ACR for catalog-scoped servers
	if server.Spec.MCPCatalogID != "" {
		return s.acrHelper.UserHasAccessToMCPServerInCatalog(apiContext.User, server.Name, server.Spec.MCPCatalogID)
	}

	// Check ACR for workspace-scoped servers
	if server.Spec.PowerUserWorkspaceID != "" {
		return s.acrHelper.UserHasAccessToMCPServerInWorkspace(apiContext.User, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
	}

	// If not owner and not in catalog/workspace, no access
	return false, nil
}

// listAPIKeys lists all API keys for the authenticated user.
func (s *Server) listAPIKeys(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keys, err := apiContext.GatewayClient.ListAPIKeys(apiContext.Context(), userID)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to list API keys: %v", err))
	}

	return apiContext.Write(map[string]any{"items": keys})
}

// getAPIKey gets a single API key for the authenticated user.
func (s *Server) getAPIKey(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	key, err := apiContext.GatewayClient.GetAPIKey(apiContext.Context(), userID, uint(keyID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get API key: %v", err))
	}

	return apiContext.Write(key)
}

// deleteAPIKey deletes an API key for the authenticated user.
func (s *Server) deleteAPIKey(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	if err := apiContext.GatewayClient.DeleteAPIKey(apiContext.Context(), userID, uint(keyID)); err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to delete API key: %v", err))
	}
	pkgLog.Infof("Deleted API key for user: userID=%d keyID=%d", userID, keyID)

	return apiContext.Write(map[string]any{"deleted": true})
}

// Admin endpoints for managing any user's API keys

// listAllAPIKeys lists all API keys in the system (admin/owner only).
func (s *Server) listAllAPIKeys(apiContext api.Context) error {
	keys, err := apiContext.GatewayClient.ListAllAPIKeys(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to list API keys: %v", err))
	}

	return apiContext.Write(map[string]any{"items": keys})
}

// getAnyAPIKey gets any API key by ID (admin/owner only).
func (s *Server) getAnyAPIKey(apiContext api.Context) error {
	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	key, err := apiContext.GatewayClient.GetAPIKeyByID(apiContext.Context(), uint(keyID))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get API key: %v", err))
	}

	return apiContext.Write(key)
}

// deleteAnyAPIKey deletes any API key by ID (admin/owner only).
func (s *Server) deleteAnyAPIKey(apiContext api.Context) error {
	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	if err := apiContext.GatewayClient.DeleteAPIKeyByID(apiContext.Context(), uint(keyID)); err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to delete API key: %v", err))
	}

	return apiContext.Write(map[string]any{"deleted": true})
}

// Authentication webhook endpoint

type apiKeyAuthRequest struct {
	MCPID string `json:"mcpId,omitempty"`
}

type apiKeyAuthResponse struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`

	Subject           string `json:"sub,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Email             string `json:"email,omitempty"`
}

func (s *Server) authenticateAPIKey(apiContext api.Context) error {
	// Extract API key from header
	authHeader := apiContext.Request.Header.Get("Authorization")
	if authHeader == "" {
		pkgLog.Infof("Denied API key auth request: reason=missing_authorization_header")
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "missing Authorization header",
		})
	}

	bearer, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok || !strings.HasPrefix(bearer, "ok1-") {
		pkgLog.Infof("Denied API key auth request: reason=invalid_api_key_format")
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid API key format",
		})
	}

	// Parse request body for MCP server info
	var req apiKeyAuthRequest
	if err := apiContext.Read(&req); err != nil {
		pkgLog.Infof("Denied API key auth request: reason=invalid_request_body")
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid request body",
		})
	}

	// Validate the API key
	apiKey, err := apiContext.GatewayClient.ValidateAPIKey(apiContext.Context(), bearer)
	if err != nil {
		pkgLog.Infof("Denied API key auth request: reason=invalid_or_expired_api_key mcpID=%s", req.MCPID)
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid or expired API key",
		})
	}

	// Get user info
	user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), strconv.FormatUint(uint64(apiKey.UserID), 10))
	if err != nil {
		pkgLog.Infof("Denied API key auth request: reason=user_not_found keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "user not found",
		})
	}

	hasWildcard := slices.Contains(apiKey.MCPServerIDs, "*")
	// Tokens have access to all webhook system MCP servers, so we only need to check if the request is not for such an MCP server.
	if !system.IsWebhookSystemMCPServerID(req.MCPID) {
		// Check if this server is in the key's allowed list
		// "*" is a special wildcard that grants access to all servers the user can access
		if !hasWildcard && !slices.Contains(apiKey.MCPServerIDs, req.MCPID) {
			// Check if this is a component server - if so, check the composite server ID
			var mcpServer v1.MCPServer
			if err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: req.MCPID}, &mcpServer); err != nil || mcpServer.Spec.CompositeName == "" || !slices.Contains(apiKey.MCPServerIDs, mcpServer.Spec.CompositeName) {
				pkgLog.Infof("Denied API key auth request: reason=api_key_scope_mismatch keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
				return apiContext.Write(apiKeyAuthResponse{
					Allowed: false,
					Reason:  "API key does not have access to this MCP server",
				})
			}
		}
	}

	pkgLog.Debugf("Authorized API key request: keyUserID=%d mcpID=%s wildcardScope=%v", apiKey.UserID, req.MCPID, hasWildcard)

	err = apiContext.Write(apiKeyAuthResponse{
		Allowed:           true,
		Subject:           fmt.Sprintf("%d", apiKey.UserID),
		Name:              user.DisplayName,
		PreferredUsername: user.Username,
		Email:             user.Email,
	})

	// Update key's last used time
	if keyErr := s.updateKeyLastUsedTime(apiContext, apiKey); keyErr != nil {
		logger.Errorf("failed to update API key last used time: %v", keyErr)
	}

	return err
}

// updateKeyLastUsedTime updates the last used timestamp for an API key.
func (s *Server) updateKeyLastUsedTime(apiContext api.Context, apiKey *types.APIKey) error {
	return apiContext.GatewayClient.UpdateAPIKeyLastUsed(apiContext.Context(), apiKey)
}

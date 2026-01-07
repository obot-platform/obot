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
	Name         string     `json:"name"`
	Description  string     `json:"description,omitempty"`
	ExpiresAt    *time.Time `json:"expiresAt,omitempty"`
	MCPServerIDs []string   `json:"mcpServerIds,omitempty"`
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

	if len(req.MCPServerIDs) == 0 {
		return types2.NewErrBadRequest("at least one MCP server must be specified")
	}

	userID := apiContext.UserID()
	if userID == 0 {
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
		return types2.NewErrHTTP(http.StatusBadRequest, errors.Join(errs...).Error())
	}

	response, err := apiContext.GatewayClient.CreateAPIKey(apiContext.Context(), userID, req.Name, req.Description, req.ExpiresAt, req.MCPServerIDs)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to create API key: %v", err))
	}

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to delete API key: %v", err))
	}

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("API key not found")
		}
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
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "missing Authorization header",
		})
	}

	bearer, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok || !strings.HasPrefix(bearer, "ok1-") {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid API key format",
		})
	}

	// Parse request body for MCP server info
	var req apiKeyAuthRequest
	if err := apiContext.Read(&req); err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid request body",
		})
	}

	// Validate the API key
	apiKey, err := apiContext.GatewayClient.ValidateAPIKey(apiContext.Context(), bearer)
	if err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid or expired API key",
		})
	}

	// Get user info
	user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), strconv.FormatUint(uint64(apiKey.UserID), 10))
	if err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "user not found",
		})
	}

	// Check if this server is in the key's allowed list
	// "*" is a special wildcard that grants access to all servers the user can access
	hasWildcard := slices.Contains(apiKey.MCPServerIDs, "*")
	if !hasWildcard && !slices.Contains(apiKey.MCPServerIDs, req.MCPID) {
		// Check if this is a component server - if so, check the composite server ID
		var mcpServer v1.MCPServer
		if err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: req.MCPID}, &mcpServer); err != nil || mcpServer.Spec.CompositeName == "" || !slices.Contains(apiKey.MCPServerIDs, mcpServer.Spec.CompositeName) {
			return apiContext.Write(apiKeyAuthResponse{
				Allowed: false,
				Reason:  "API key does not have access to this MCP server",
			})
		}
	}

	// Verify user still has access to the server
	var server v1.MCPServer
	if err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: req.MCPID}, &server); err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "MCP server not found",
		})
	}

	hasAccess, err := s.userHasAccessToMCPServerByUserID(apiContext, &server, apiKey.UserID)
	if err != nil {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  fmt.Sprintf("failed to verify access: %v", err),
		})
	}
	if !hasAccess {
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "user does not have access to this MCP server",
		})
	}

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

// userHasAccessToMCPServerByUserID checks if a specific user has access to the given MCPServer.
// This is used in the auth webhook where we don't have an authenticated api.Context.
func (s *Server) userHasAccessToMCPServerByUserID(apiContext api.Context, server *v1.MCPServer, userID uint) (bool, error) {
	userIDStr := strconv.FormatUint(uint64(userID), 10)

	// Owner always has access
	if server.Spec.UserID == userIDStr {
		return true, nil
	}

	// Get user info with groups for ACR checks
	userInfo, err := apiContext.GatewayClient.UserInfoByID(apiContext.Context(), userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user info: %w", err)
	}

	// Check ACR for catalog-scoped servers
	if server.Spec.MCPCatalogID != "" {
		return s.acrHelper.UserHasAccessToMCPServerInCatalog(userInfo, server.Name, server.Spec.MCPCatalogID)
	}

	// Check ACR for workspace-scoped servers
	if server.Spec.PowerUserWorkspaceID != "" {
		return s.acrHelper.UserHasAccessToMCPServerInWorkspace(userInfo, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
	}

	// If not owner and not in catalog/workspace, no access
	return false, nil
}

// updateKeyLastUsedTime updates the last used timestamp for an API key.
func (s *Server) updateKeyLastUsedTime(apiContext api.Context, apiKey *types.APIKey) error {
	return apiContext.GatewayClient.UpdateAPIKeyLastUsed(apiContext.Context(), apiKey)
}

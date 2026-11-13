package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type createAPIKeyRequest struct {
	Name               string     `json:"name"`
	Description        string     `json:"description,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"`
	types.APIKeyScopes `json:",inline"`
}

// Authentication webhook endpoint

type apiKeyAuthRequest struct {
	MCPID        string `json:"mcpId,omitempty"`
	ValidateOnly bool   `json:"validateOnly,omitempty"`
}

type apiKeyAuthResponse struct {
	Allowed bool               `json:"allowed"`
	Reason  string             `json:"reason,omitempty"`
	Scopes  types.APIKeyScopes `json:"scopes"`

	Subject           string `json:"sub,omitempty"`
	Name              string `json:"name,omitempty"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	Email             string `json:"email,omitempty"`
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

	if !req.HasSomeScope() {
		return types2.NewErrBadRequest("at least one MCP server must be specified or a capability must be enabled")
	}

	userID := apiContext.UserID()
	if userID == 0 {
		slog.Info("Rejecting API key creation for unauthenticated request")
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
		if system.IsVMCPID(serverID) {
			var vmcp v1.VMCP
			if err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: serverID}, &vmcp); err != nil {
				return types2.NewErrBadRequest("MCP server %q not found", serverID)
			}
			if !authz.UserCanReadVMCP(apiContext.User, &vmcp) {
				errs = append(errs, fmt.Errorf("MCP server %q not found", serverID))
			}
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
		slog.Info("Rejecting API key creation due to unauthorized MCP server selections", "userID", userID, "requestedServers", len(req.MCPServerIDs), "deniedServers", len(errs))
		return types2.NewErrHTTP(http.StatusBadRequest, errors.Join(errs...).Error())
	}

	response, err := apiContext.GatewayClient.CreateAPIKey(apiContext.Context(), userID, req.Name, req.Description, req.ExpiresAt, req.APIKeyScopes)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to create API key: %v", err))
	}
	slog.Info("Created API key for user", "userID", userID, "serverScopes", len(req.MCPServerIDs))

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
	if server.Spec.IsCatalogServer() {
		return s.acrHelper.UserHasAccessToMCPServerInCatalog(apiContext.User, server.Name, server.Spec.MCPCatalogID)
	}

	// Check ACR for workspace-scoped servers
	if server.Spec.IsPowerUserWorkspaceServer() {
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

	showRevoked, _ := strconv.ParseBool(apiContext.Request.URL.Query().Get("show_revoked"))
	keys, err := apiContext.GatewayClient.ListAPIKeysWithOptions(apiContext.Context(), userID, client.APIKeyListOptions{
		ShowRevoked: showRevoked,
	})
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

// revokeAPIKey revokes an API key for the authenticated user.
func (s *Server) revokeAPIKey(apiContext api.Context) error {
	userID := apiContext.UserID()
	if userID == 0 {
		return types2.NewErrHTTP(http.StatusUnauthorized, "user not authenticated")
	}

	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	if err := apiContext.GatewayClient.RevokeAPIKey(apiContext.Context(), userID, uint(keyID)); err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to revoke API key: %v", err))
	}
	slog.Info("Revoked API key for user", "userID", userID, "keyID", keyID)

	return apiContext.Write(map[string]any{"deleted": true})
}

// Admin endpoints for managing any user's API keys

// listAllAPIKeys lists all API keys in the system (admin/owner only). The optional kind query parameter, user or
// agent, limits the list to the keys of that kind of owner.
func (s *Server) listAllAPIKeys(apiContext api.Context) error {
	query := apiContext.URL.Query()
	showRevoked, _ := strconv.ParseBool(query.Get("show_revoked"))

	var ownerKind principal.Kind
	if query.Has("kind") {
		switch kind := principal.Kind(query.Get("kind")); kind {
		case principal.KindUser, principal.KindAgent:
			ownerKind = kind
		default:
			return types2.NewErrBadRequest("invalid kind %q: must be %q or %q", kind, principal.KindUser, principal.KindAgent)
		}
	}

	keys, err := apiContext.GatewayClient.ListAllAPIKeys(apiContext.Context(), client.APIKeyListOptions{
		ShowRevoked: showRevoked,
		OwnerKind:   ownerKind,
	})
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

// deleteAnyAPIKey revokes any API key by ID while preserving the DELETE route
// for compatibility (admin/owner only).
func (s *Server) deleteAnyAPIKey(apiContext api.Context) error {
	keyID, err := strconv.ParseUint(apiContext.PathValue("id"), 10, 64)
	if err != nil {
		return types2.NewErrBadRequest("invalid key ID")
	}

	// The administrator is recorded as the actor if the key is an agent's (the admin change audit).
	if err := apiContext.GatewayClient.RevokeAPIKeyByID(client.WithAdminUser(apiContext.Context(), apiContext.User), uint(keyID)); err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to revoke API key: %v", err))
	}

	return apiContext.Write(map[string]any{"deleted": true})
}

// authenticateAPIKey answers the API key authentication webhook, which the nanobot shim and the CLI call with a key
// to validate. Its subject is the key owner's storage encoding: a user's decimal ID, or "agent:<id>" for an agent's
// key, which agent identities must be enabled for. An agent has no username or email, so neither is reported for
// one, and its display name is only a name: display names need not be unique.
func (s *Server) authenticateAPIKey(apiContext api.Context) error {
	// Extract API key from header
	authHeader := apiContext.Request.Header.Get("Authorization")
	if authHeader == "" {
		slog.Info("Denied API key auth request", "reason", "missing_authorization_header")
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "missing Authorization header",
		})
	}

	// The prefix names the kind of the key's owner, and ValidateAPIKey looks the key up as a key of that kind of
	// owner only, so the key it returns is one of that kind of owner: a key is never answered for as an owner of
	// another kind, such as the user with its agent's numeric ID.
	bearer, ok := strings.CutPrefix(authHeader, "Bearer ")
	var ownerKind principal.Kind
	switch {
	case !ok:
	case strings.HasPrefix(bearer, system.APIKeyPrefix+"-"):
		ownerKind = principal.KindUser
	case strings.HasPrefix(bearer, system.AgentAPIKeyPrefix+"-") && s.enableAgentIdentities:
		ownerKind = principal.KindAgent
	}
	if ownerKind == "" {
		slog.Info("Denied API key auth request", "reason", "invalid_api_key_format")
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid API key format",
		})
	}

	// Parse request body for MCP server info
	var req apiKeyAuthRequest
	if err := apiContext.Read(&req); err != nil {
		slog.Info("Denied API key auth request", "reason", "invalid_request_body")
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid request body",
		})
	}

	// Validate the API key
	apiKey, err := apiContext.GatewayClient.ValidateAPIKey(apiContext.Context(), bearer)
	if err != nil {
		slog.Info("Denied API key auth request", "reason", "invalid_or_expired_api_key", "mcpID", req.MCPID)
		return apiContext.Write(apiKeyAuthResponse{
			Allowed: false,
			Reason:  "invalid or expired API key",
		})
	}
	owner := apiKey.Owner()

	var response apiKeyAuthResponse
	switch ownerKind {
	case principal.KindUser:
		userID, _ := owner.UserID()
		user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), strconv.FormatUint(uint64(userID), 10))
		if err != nil {
			slog.Info("Denied API key auth request", "reason", "user_not_found", "keyOwner", owner.String(), "mcpID", req.MCPID)
			return apiContext.Write(apiKeyAuthResponse{
				Allowed: false,
				Reason:  "user not found",
			})
		}
		if status := user.Status(); status != types2.UserStatusActive {
			slog.Info("Denied API key auth request", "reason", "user_not_active", "keyOwner", owner.String(), "status", status, "mcpID", req.MCPID)
			return apiContext.Write(apiKeyAuthResponse{
				Allowed: false,
				Reason:  "user is not active",
			})
		}
		response = apiKeyAuthResponse{
			Name:              user.DisplayName,
			PreferredUsername: user.Username,
			Email:             user.Email,
			Scopes:            apiKey.APIKeyScopes,
		}
	case principal.KindAgent:
		agentID, _ := owner.AgentID()
		// ValidateAPIKey does not refuse the key of an owner that is not active, so the agent's access is checked here.
		agent, err := apiContext.GatewayClient.ActiveAgent(apiContext.Context(), agentID)
		if denied, isDenied := errors.AsType[*client.AgentAccessDeniedError](err); isDenied {
			slog.Info("Denied API key auth request", "reason", "agent_not_active", "keyOwner", owner.String(), "status", denied.Status, "mcpID", req.MCPID)
			return apiContext.Write(apiKeyAuthResponse{
				Allowed: false,
				Reason:  "agent is not active",
			})
		} else if err != nil {
			// The agent could not be read, so its access is unknown: the caller is told so rather than that the agent
			// does not exist.
			slog.Error("Failed to read the agent of an API key", "keyOwner", owner.String(), "mcpID", req.MCPID, "error", err)
			return types2.NewErrHTTP(http.StatusInternalServerError, "failed to read the API key's agent")
		}
		response = apiKeyAuthResponse{
			Name: agent.DisplayName,
			// The scopes the authenticator honors for an agent's key, which never include API or device scan access.
			Scopes: agentKeyScopes(apiKey.APIKeyScopes),
		}
	}

	hasWildcard := slices.Contains(apiKey.MCPServerIDs, "*")
	if !req.ValidateOnly && !system.IsWebhookSystemMCPServerID(req.MCPID) {
		// The owner's storage encoding finds the owner's own vMCP instance, so a user and an agent with the same
		// numeric ID never resolve each other's.
		hasAccess, err := authz.MCPIDIsAuthorized(apiContext.Context(), apiContext.Storage, apiKey.MCPServerIDs, owner.String(), req.MCPID)
		if !hasAccess || err != nil {
			slog.Info("Denied API key auth request", "reason", "api_key_scope_mismatch", "keyOwner", owner.String(), "mcpID", req.MCPID)
			return apiContext.Write(apiKeyAuthResponse{
				Allowed: false,
				Reason:  "API key does not have access to this MCP server",
			})
		}
	}

	slog.Debug("Authorized API key request", "keyOwner", owner.String(), "mcpID", req.MCPID, "wildcardScope", hasWildcard, "validateOnly", req.ValidateOnly)

	// Update key's last used time
	if keyErr := apiContext.GatewayClient.UpdateAPIKeyLastUsed(apiContext.Context(), apiKey); keyErr != nil {
		slog.Error("failed to update API key last used time", "error", keyErr)
	}

	response.Allowed = true
	response.Subject = owner.String()
	return apiContext.Write(response)
}

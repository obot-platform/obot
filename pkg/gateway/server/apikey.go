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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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

	response, err := apiContext.GatewayClient.CreateAPIKey(apiContext.Context(), userID, req.Name, req.Description, req.ExpiresAt, req.MCPServerIDs)
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

var gatewayTracer = otel.Tracer("obot/gateway")

func recordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func writeAPIKeyAuthResponse(apiContext api.Context, response apiKeyAuthResponse, reasonCode string) error {
	_, span := gatewayTracer.Start(apiContext.Context(), "gateway.authenticate_api_key.write_response")
	span.SetAttributes(attribute.Bool("gateway.auth.allowed", response.Allowed))
	if reasonCode != "" {
		span.SetAttributes(attribute.String("gateway.auth.reason_code", reasonCode))
	}
	defer span.End()

	err := apiContext.Write(response)
	recordSpanError(span, err)
	return err
}

func (s *Server) authenticateAPIKey(apiContext api.Context) error {
	ctx, span := gatewayTracer.Start(apiContext.Context(), "gateway.authenticate_api_key")
	defer span.End()

	apiContext.Request = apiContext.Request.WithContext(ctx)

	deny := func(reasonCode, reason string) error {
		span.SetAttributes(
			attribute.Bool("gateway.auth.allowed", false),
			attribute.String("gateway.auth.reason_code", reasonCode),
		)
		return writeAPIKeyAuthResponse(apiContext, apiKeyAuthResponse{
			Allowed: false,
			Reason:  reason,
		}, reasonCode)
	}

	// Extract API key from header
	_, authHeaderSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.parse_authorization")
	authHeader := apiContext.Request.Header.Get("Authorization")
	authHeaderSpan.SetAttributes(attribute.Bool("gateway.auth.authorization_header_present", authHeader != ""))
	if authHeader == "" {
		authHeaderSpan.End()
		pkgLog.Infof("Denied API key auth request: reason=missing_authorization_header")
		return deny("missing_authorization_header", "missing Authorization header")
	}

	bearer, ok := strings.CutPrefix(authHeader, "Bearer ")
	hasAPIKeyPrefix := strings.HasPrefix(bearer, "ok1-")
	authHeaderSpan.SetAttributes(
		attribute.Bool("gateway.auth.bearer_prefix_valid", ok),
		attribute.Bool("gateway.auth.api_key_prefix_valid", hasAPIKeyPrefix),
	)
	authHeaderSpan.End()
	if !ok || !hasAPIKeyPrefix {
		pkgLog.Infof("Denied API key auth request: reason=invalid_api_key_format")
		return deny("invalid_api_key_format", "invalid API key format")
	}

	// Parse request body for MCP server info
	_, readRequestSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.read_request")
	var req apiKeyAuthRequest
	if err := apiContext.Read(&req); err != nil {
		recordSpanError(readRequestSpan, err)
		readRequestSpan.End()
		pkgLog.Infof("Denied API key auth request: reason=invalid_request_body")
		return deny("invalid_request_body", "invalid request body")
	}
	requestTargetsServer := system.IsMCPServerID(req.MCPID)
	readRequestSpan.SetAttributes(
		attribute.Bool("gateway.auth.has_mcp_id", req.MCPID != ""),
		attribute.Bool("gateway.auth.request_targets_mcp_server", requestTargetsServer),
	)
	readRequestSpan.End()

	// Validate the API key
	_, validateKeySpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.validate_key")
	apiKey, err := apiContext.GatewayClient.ValidateAPIKey(apiContext.Context(), bearer)
	if err != nil {
		validateKeySpan.End()
		pkgLog.Infof("Denied API key auth request: reason=invalid_or_expired_api_key mcpID=%s", req.MCPID)
		return deny("invalid_or_expired_api_key", "invalid or expired API key")
	}
	validateKeySpan.SetAttributes(attribute.Int("gateway.auth.scope_count", len(apiKey.MCPServerIDs)))
	validateKeySpan.End()

	// Get user info
	_, loadUserSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.load_user")
	user, err := apiContext.GatewayClient.UserByID(apiContext.Context(), strconv.FormatUint(uint64(apiKey.UserID), 10))
	if err != nil {
		recordSpanError(loadUserSpan, err)
		loadUserSpan.End()
		pkgLog.Infof("Denied API key auth request: reason=user_not_found keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
		return deny("user_not_found", "user not found")
	}
	loadUserSpan.End()

	// Check if this server is in the key's allowed list
	// "*" is a special wildcard that grants access to all servers the user can access
	scopeCtx, scopeSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.check_scope")
	hasWildcard := slices.Contains(apiKey.MCPServerIDs, "*")
	directScopeMatch := slices.Contains(apiKey.MCPServerIDs, req.MCPID)
	scopeSpan.SetAttributes(
		attribute.Bool("gateway.auth.scope_has_wildcard", hasWildcard),
		attribute.Bool("gateway.auth.scope_direct_match", directScopeMatch),
	)
	if !hasWildcard && !directScopeMatch {
		// Check if this is a component server - if so, check the composite server ID
		_, componentScopeSpan := gatewayTracer.Start(scopeCtx, "gateway.authenticate_api_key.lookup_component_scope")
		var mcpServer v1.MCPServer
		err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: req.MCPID}, &mcpServer)
		componentScopeMatch := err == nil && mcpServer.Spec.CompositeName != "" && slices.Contains(apiKey.MCPServerIDs, mcpServer.Spec.CompositeName)
		componentScopeSpan.SetAttributes(
			attribute.Bool("gateway.auth.component_server_found", err == nil),
			attribute.Bool("gateway.auth.component_has_composite_name", err == nil && mcpServer.Spec.CompositeName != ""),
			attribute.Bool("gateway.auth.component_scope_match", componentScopeMatch),
		)
		componentScopeSpan.End()
		if !componentScopeMatch {
			scopeSpan.End()
			pkgLog.Infof("Denied API key auth request: reason=api_key_scope_mismatch keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
			return deny("api_key_scope_mismatch", "API key does not have access to this MCP server")
		}
	}
	scopeSpan.End()

	// Verify user still has access to the server
	if requestTargetsServer {
		verifyAccessCtx, verifyAccessSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.verify_server_access")
		var server v1.MCPServer
		_, loadServerSpan := gatewayTracer.Start(verifyAccessCtx, "gateway.authenticate_api_key.load_server")
		err := apiContext.Storage.Get(apiContext.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: req.MCPID}, &server)
		loadServerSpan.SetAttributes(attribute.Bool("gateway.auth.server_found", err == nil))
		loadServerSpan.End()
		if err != nil {
			verifyAccessSpan.End()
			pkgLog.Infof("Denied API key auth request: reason=mcp_server_not_found keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
			return deny("mcp_server_not_found", "MCP server not found")
		}

		accessCheckCtx, accessCheckSpan := gatewayTracer.Start(verifyAccessCtx, "gateway.authenticate_api_key.check_user_access")
		accessCheckContext := apiContext
		accessCheckContext.Request = accessCheckContext.Request.WithContext(accessCheckCtx)
		hasAccess, err := s.userHasAccessToMCPServerByUserID(accessCheckContext, &server, apiKey.UserID)
		if err != nil {
			recordSpanError(accessCheckSpan, err)
		}
		accessCheckSpan.SetAttributes(attribute.Bool("gateway.auth.user_has_access", hasAccess))
		accessCheckSpan.End()
		verifyAccessSpan.SetAttributes(attribute.Bool("gateway.auth.user_has_access", hasAccess))
		verifyAccessSpan.End()
		if err != nil {
			pkgLog.Infof("Denied API key auth request: reason=access_check_failed keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
			return deny("access_check_failed", fmt.Sprintf("failed to verify access: %v", err))
		}
		if !hasAccess {
			pkgLog.Infof("Denied API key auth request: reason=user_lost_mcp_access keyUserID=%d mcpID=%s", apiKey.UserID, req.MCPID)
			return deny("user_lost_mcp_access", "user does not have access to this MCP server")
		}
	}
	pkgLog.Debugf("Authorized API key request: keyUserID=%d mcpID=%s wildcardScope=%v", apiKey.UserID, req.MCPID, hasWildcard)
	span.SetAttributes(
		attribute.Bool("gateway.auth.allowed", true),
		attribute.Bool("gateway.auth.scope_has_wildcard", hasWildcard),
		attribute.Bool("gateway.auth.request_targets_mcp_server", requestTargetsServer),
	)

	err = writeAPIKeyAuthResponse(apiContext, apiKeyAuthResponse{
		Allowed:           true,
		Subject:           fmt.Sprintf("%d", apiKey.UserID),
		Name:              user.DisplayName,
		PreferredUsername: user.Username,
		Email:             user.Email,
	}, "")

	// Update key's last used time
	if keyErr := s.updateKeyLastUsedTime(apiContext, apiKey); keyErr != nil {
		logger.Errorf("failed to update API key last used time: %v", keyErr)
	}

	return err
}

// userHasAccessToMCPServerByUserID checks if a specific user has access to the given MCPServer.
// This is used in the auth webhook where we don't have an authenticated api.Context.
func (s *Server) userHasAccessToMCPServerByUserID(apiContext api.Context, server *v1.MCPServer, userID uint) (bool, error) {
	ctx, span := gatewayTracer.Start(apiContext.Context(), "gateway.authenticate_api_key.user_has_access")
	span.SetAttributes(
		attribute.Bool("gateway.auth.server_catalog_scoped", server.Spec.MCPCatalogID != ""),
		attribute.Bool("gateway.auth.server_workspace_scoped", server.Spec.PowerUserWorkspaceID != ""),
	)
	defer span.End()

	userIDStr := strconv.FormatUint(uint64(userID), 10)

	// Owner always has access
	if server.Spec.UserID == userIDStr {
		span.SetAttributes(
			attribute.Bool("gateway.auth.owner_match", true),
			attribute.Bool("gateway.auth.user_has_access", true),
		)
		return true, nil
	}
	span.SetAttributes(attribute.Bool("gateway.auth.owner_match", false))

	// Get user info with groups for ACR checks
	_, userInfoSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.load_user_info")
	userInfo, err := apiContext.GatewayClient.UserInfoByID(apiContext.Context(), userID)
	if err != nil {
		recordSpanError(userInfoSpan, err)
		userInfoSpan.End()
		recordSpanError(span, err)
		return false, fmt.Errorf("failed to get user info: %w", err)
	}
	userInfoSpan.End()

	// Check ACR for catalog-scoped servers
	if server.Spec.MCPCatalogID != "" {
		_, catalogAccessSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.check_catalog_access")
		hasAccess, err := s.acrHelper.UserHasAccessToMCPServerInCatalog(userInfo, server.Name, server.Spec.MCPCatalogID)
		if err != nil {
			recordSpanError(catalogAccessSpan, err)
			recordSpanError(span, err)
		}
		catalogAccessSpan.SetAttributes(attribute.Bool("gateway.auth.user_has_access", hasAccess))
		catalogAccessSpan.End()
		span.SetAttributes(attribute.Bool("gateway.auth.user_has_access", hasAccess))
		return hasAccess, err
	}

	// Check ACR for workspace-scoped servers
	if server.Spec.PowerUserWorkspaceID != "" {
		_, workspaceAccessSpan := gatewayTracer.Start(ctx, "gateway.authenticate_api_key.check_workspace_access")
		hasAccess, err := s.acrHelper.UserHasAccessToMCPServerInWorkspace(userInfo, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
		if err != nil {
			recordSpanError(workspaceAccessSpan, err)
			recordSpanError(span, err)
		}
		workspaceAccessSpan.SetAttributes(attribute.Bool("gateway.auth.user_has_access", hasAccess))
		workspaceAccessSpan.End()
		span.SetAttributes(attribute.Bool("gateway.auth.user_has_access", hasAccess))
		return hasAccess, err
	}

	// If not owner and not in catalog/workspace, no access
	span.SetAttributes(attribute.Bool("gateway.auth.user_has_access", false))
	return false, nil
}

// updateKeyLastUsedTime updates the last used timestamp for an API key.
func (s *Server) updateKeyLastUsedTime(apiContext api.Context, apiKey *types.APIKey) error {
	_, span := gatewayTracer.Start(apiContext.Context(), "gateway.authenticate_api_key.update_last_used")
	defer span.End()

	err := apiContext.GatewayClient.UpdateAPIKeyLastUsed(apiContext.Context(), apiKey)
	recordSpanError(span, err)
	return err
}

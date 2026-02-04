package server

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server"
	"github.com/obot-platform/obot/pkg/system"
)

func (s *Server) AddRoutes(mux *server.Server) {
	wrap := func(h api.HandlerFunc) api.HandlerFunc {
		return apply(h, addRequestID, addLogger, logRequest, contentType("application/json"))
	}

	// Health endpoint
	mux.HTTPHandle("GET /api/healthz", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.db.Check(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
		} else if !router.GetHealthy() {
			http.Error(w, "controllers not ready", http.StatusServiceUnavailable)
		} else {
			_, _ = w.Write([]byte("ok"))
		}
	}))
	// All the routes served by the API will start with `/api`
	mux.HandleFunc("GET /api/me", wrap(s.getCurrentUser))
	mux.HandleFunc("DELETE /api/me", wrap(s.deleteUser))
	mux.HandleFunc("PATCH /api/me", wrap(s.updateUser))
	mux.HandleFunc("POST /api/logout-all", wrap(s.logoutAll))
	mux.HandleFunc("GET /api/users", wrap(s.getUsers))
	mux.HandleFunc("GET /api/groups", wrap(s.listAuthGroups))
	mux.HandleFunc("GET /api/group-role-assignments", wrap(s.getGroupRoleAssignments))
	mux.HandleFunc("GET /api/group-role-assignments/{groupName}", wrap(s.getGroupRoleAssignment))
	mux.HandleFunc("POST /api/group-role-assignments", wrap(s.createGroupRoleAssignment))
	mux.HandleFunc("PUT /api/group-role-assignments/{groupName}", wrap(s.updateGroupRoleAssignment))
	mux.HandleFunc("DELETE /api/group-role-assignments/{groupName}", wrap(s.deleteGroupRoleAssignment))
	mux.HandleFunc("POST /api/encrypt-all-users", wrap(s.encryptAllUsersAndIdentities))
	mux.HandleFunc("GET /api/users/{user_id}", wrap(s.getUser))
	mux.HandleFunc("GET /api/users/{user_id}/activities", wrap(s.activitiesByUser))
	mux.HandleFunc("GET /api/users/{user_id}/token-usage", wrap(s.usageForUser))
	mux.HandleFunc("GET /api/users/{user_id}/total-token-usage", wrap(s.totalUsageForUser))
	mux.HandleFunc("GET /api/users/{user_id}/remaining-token-usage", wrap(s.remainingUsageForUser))
	mux.HandleFunc("PATCH /api/users/{user_id}", wrap(s.updateUser))
	mux.HandleFunc("POST /api/users/{user_id}/internal", wrap(s.markUserInternal))
	mux.HandleFunc("POST /api/users/{user_id}/external", wrap(s.markUserExternal))
	mux.HandleFunc("DELETE /api/users/{user_id}", wrap(s.deleteUser))
	mux.HandleFunc("GET /api/active-users", wrap(s.activeUsers))

	mux.HandleFunc("GET /api/token-usage", wrap(s.systemTokenUsageByUser))
	mux.HandleFunc("GET /api/total-token-usage", wrap(s.totalSystemTokenUsage))

	mux.HandleFunc("POST /api/token-request", s.tokenRequest)
	mux.HandleFunc("GET /api/token-request/{id}", s.checkForToken)
	mux.HandleFunc("GET /api/token-request/{id}/{namespace}/{name}", s.redirectForTokenRequest)

	mux.HandleFunc("GET /api/tokens", wrap(s.getTokens))
	mux.HandleFunc("DELETE /api/tokens/{id}", wrap(s.deleteToken))

	mux.HandleFunc("GET /api/oauth/start/{id}/{namespace}/{name}", wrap(s.oauth))
	mux.HandleFunc("/api/oauth/redirect/{namespace}/{name}", wrap(s.redirect))

	// CRUD routes for OAuth Apps (integrations with other services such as Microsoft 365)
	mux.HandleFunc("GET /api/oauth-apps", wrap(s.listOAuthApps))
	mux.HandleFunc("GET /api/oauth-apps/{id}", wrap(s.oauthAppByID))
	mux.HandleFunc("POST /api/oauth-apps", wrap(s.createOAuthApp))
	mux.HandleFunc("PATCH /api/oauth-apps/{id}", wrap(s.updateOAuthApp))
	mux.HandleFunc("DELETE /api/oauth-apps/{id}", wrap(s.deleteOAuthApp))

	// Routes for OAuth authorization code flow
	mux.HandleFunc("GET /api/app-oauth/authorize/{id}", wrap(s.authorizeOAuthApp))
	mux.HandleFunc("GET /api/app-oauth/refresh/{id}", wrap(s.refreshOAuthApp))
	mux.HandleFunc("GET /api/app-oauth/callback/{id}", wrap(s.callbackOAuthApp))

	// Route for credential tools to get their OAuth tokens
	mux.HandleFunc("GET /api/app-oauth/get-token", wrap(s.getTokenOAuthApp))
	mux.HandleFunc("GET /api/app-oauth/get-token/{id}", wrap(s.getTokenOAuthApp))

	// Handle updates to the file scanner configuration
	mux.HandleFunc("GET /api/file-scanner-config", wrap(s.getFileScannerConfig))
	mux.HandleFunc("PUT /api/file-scanner-config", wrap(s.updateFileScannerConfig))

	// LLM proxy
	mux.HandleFunc("/api/llm-proxy/openai/{path...}", s.newLLMProviderProxy(mustParseURL(openAIBaseURL), system.OpenAIModelProviderTool).proxy)
	mux.HandleFunc("/api/llm-proxy/anthropic/{path...}", s.newLLMProviderProxy(mustParseURL(anthropicBaseURL), system.AnthropicModelProviderTool).proxy)
	mux.HandleFunc("/api/llm-proxy/{path...}", s.dispatchLLMProxy)

	// API Keys for MCP server access - user's own keys
	mux.HandleFunc("POST /api/api-keys", wrap(s.createAPIKey))
	mux.HandleFunc("GET /api/api-keys", wrap(s.listAPIKeys))
	mux.HandleFunc("GET /api/api-keys/{id}", wrap(s.getAPIKey))
	mux.HandleFunc("DELETE /api/api-keys/{id}", wrap(s.deleteAPIKey))

	// API Keys admin endpoints - for managing any user's keys (admin/owner only)
	mux.HandleFunc("GET /api/admin-api-keys", wrap(s.listAllAPIKeys))
	mux.HandleFunc("GET /api/admin-api-keys/{id}", wrap(s.getAnyAPIKey))
	mux.HandleFunc("DELETE /api/admin-api-keys/{id}", wrap(s.deleteAnyAPIKey))

	// API Key authentication webhook (called by nanobot shim)
	// This endpoint is unauthenticated - it validates the API key passed in the header
	mux.HandleFunc("POST /api/api-keys/auth", wrap(s.authenticateAPIKey))

	// MCP Tokens for integrated MCP server access - user's own tokens
	// Only enabled when Nanobot integration is enabled
	if s.nanobotIntegration {
		mux.HandleFunc("POST /api/mcp-tokens", wrap(s.createObotMCPToken))
		mux.HandleFunc("GET /api/mcp-tokens", wrap(s.listObotMCPTokens))
		mux.HandleFunc("GET /api/mcp-tokens/{id}", wrap(s.getObotMCPToken))
		mux.HandleFunc("DELETE /api/mcp-tokens/{id}", wrap(s.deleteObotMCPToken))
	}
}

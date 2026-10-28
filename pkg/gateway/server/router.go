package server

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server"
)

func (s *Server) AddRoutes(mux *server.Server, tunnelBridge http.Handler) {
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
	if tunnelBridge != nil {
		mux.HTTPHandle("/tunnel/bridge/{target}", tunnelBridge)
	}
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
	mux.HandleFunc("PATCH /api/users/{user_id}", wrap(s.updateUser))
	mux.HandleFunc("POST /api/users/{user_id}/internal", wrap(s.markUserInternal))
	mux.HandleFunc("POST /api/users/{user_id}/external", wrap(s.markUserExternal))
	mux.HandleFunc("DELETE /api/users/{user_id}", wrap(s.deleteUser))
	mux.HandleFunc("POST /api/users/{user_id}/enable", wrap(s.enableUser))
	mux.HandleFunc("GET /api/active-users", wrap(s.activeUsers))

	mux.HandleFunc("GET /api/token-usage", wrap(s.systemTokenUsageByUser))
	mux.HandleFunc("GET /api/total-token-usage", wrap(s.totalSystemTokenUsage))
	mux.HandleFunc("GET /api/token-usage-stats", wrap(s.tokenUsageStats))

	mux.HandleFunc("POST /api/token-request", s.tokenRequest)
	mux.HandleFunc("GET /api/token-request/{id}", s.checkForToken)
	mux.HandleFunc("GET /api/token-request/{id}/{namespace}/{name}", s.redirectForTokenRequest)
	mux.HandleFunc("POST /api/token-request/verify", wrap(s.verifyDeviceCode))

	mux.HandleFunc("GET /api/tokens", wrap(s.getTokens))
	mux.HandleFunc("DELETE /api/tokens/{id}", wrap(s.deleteToken))

	mux.HandleFunc("GET /api/oauth/start/{id}/{namespace}/{name}", wrap(s.oauth))
	mux.HandleFunc("/api/oauth/redirect/{namespace}/{name}", wrap(s.redirect))

	// LLM proxy. The LLM gateway serves every model, in every API format, at the root of the
	// LLM proxy.
	gatewayProxy := s.newBifrostLLMProviderProxy()
	mux.HandleFunc("/api/llm-proxy/v1/{path...}", func(req api.Context) error {
		req.SetPathValue("path", "v1/"+req.PathValue("path"))
		return gatewayProxy.proxy(req)
	})
	// Claude Code sends some Anthropic API requests that name no model to its API base URL too.
	anthropic := newAnthropicProxy()
	mux.HandleFunc("/api/llm-proxy/v1/files", anthropic.files)
	mux.HandleFunc("/api/llm-proxy/v1/files/{path...}", anthropic.files)
	mux.HandleFunc("GET /api/llm-proxy/v1/code/agent-proxy/ca-cert", anthropic.agentProxyCACert)
	mux.HandleFunc("GET /api/llm-proxy/v1/code/agent-proxy/ws", anthropic.agentProxyWebSocket)
	// Clients set up for the former OpenAI and Anthropic routes speak those APIs, which the
	// gateway serves too, so they keep working.
	for _, prefix := range []string{"/api/llm-proxy/openai/", "/api/llm-proxy/anthropic/"} {
		mux.HandleFunc(prefix+"{path...}", gatewayProxy.proxy)
	}
	// The other former per-provider routes spoke provider-specific APIs.
	for _, prefix := range []string{"/api/llm-proxy/generic-responses/", "/api/llm-proxy/aws-bedrock/", "/api/llm-proxy/aws-bedrock-api-key/", "/api/llm-proxy/azure/", "/api/llm-proxy/azure-entra/"} {
		mux.HandleFunc(prefix+"{path...}", func(api.Context) error {
			return types2.NewErrHTTP(http.StatusGone, "this LLM proxy route was removed; send requests to /api/llm-proxy/v1, which serves every model in the OpenAI and Anthropic API formats")
		})
	}

	// API Keys for MCP server access - user's own keys
	mux.HandleFunc("POST /api/api-keys", wrap(s.createAPIKey))
	mux.HandleFunc("GET /api/api-keys", wrap(s.listAPIKeys))
	mux.HandleFunc("GET /api/api-keys/{id}", wrap(s.getAPIKey))
	mux.HandleFunc("DELETE /api/api-keys/{id}", wrap(s.revokeAPIKey))

	// API Keys admin endpoints - for managing any user's keys (admin/owner only)
	mux.HandleFunc("GET /api/admin-api-keys", wrap(s.listAllAPIKeys))
	mux.HandleFunc("GET /api/admin-api-keys/{id}", wrap(s.getAnyAPIKey))
	mux.HandleFunc("DELETE /api/admin-api-keys/{id}", wrap(s.deleteAnyAPIKey))

	// API Key authentication webhook (called by nanobot shim and the CLI)
	// This endpoint is unauthenticated - it validates the API key passed in the header
	mux.HandleFunc("POST /api/api-keys/auth", wrap(s.authenticateAPIKey))
}

package server

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/server"
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
	mux.HandleFunc("POST /api/logout-all", wrap(s.logoutAll))
	mux.HandleFunc("GET /api/users", wrap(s.getUsers))
	mux.HandleFunc("POST /api/encrypt-all-users", wrap(s.encryptAllUsersAndIdentities))
	mux.HandleFunc("GET /api/users/{username_or_id}", wrap(s.getUser))
	mux.HandleFunc("GET /api/users/{user_id}/activities", wrap(s.activitiesByUser))
	mux.HandleFunc("PATCH /api/users/{username}", wrap(s.updateUser))
	mux.HandleFunc("DELETE /api/users/{username}", wrap(s.deleteUser))
	mux.HandleFunc("GET /api/active-users", wrap(s.activeUsers))

	mux.HandleFunc("POST /api/token-request", s.tokenRequest)
	mux.HandleFunc("GET /api/token-request/{id}", s.checkForToken)
	mux.HandleFunc("GET /api/token-request/{id}/{namespace}/{name}", s.redirectForTokenRequest)

	mux.HandleFunc("GET /api/tokens", wrap(s.getTokens))
	mux.HandleFunc("DELETE /api/tokens/{id}", wrap(s.deleteToken))
	mux.HandleFunc("POST /api/tokens", wrap(s.newToken))

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
	mux.HandleFunc("POST /api/llm-proxy/{path...}", s.llmProxy)
}

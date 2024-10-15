package server

import (
	_ "embed"
	"net/http"
	"net/http/httputil"

	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	kcontext "github.com/otto8-ai/otto8/pkg/gateway/context"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
)

func (s *Server) AddRoutes(w func(api.HandlerFunc) http.Handler, mux *http.ServeMux) {
	wrap := func(h api.HandlerFunc) http.Handler {
		return w(apply(h, addRequestID, addLogger, logRequest, contentType("application/json")))
	}
	// All the routes served by the API will start with `/api`
	mux.Handle("GET /api/me", wrap(s.authFunc(types2.RoleBasic)(s.getCurrentUser)))
	mux.Handle("GET /api/users", wrap(s.authFunc(types2.RoleAdmin)(s.getUsers)))
	mux.Handle("GET /api/users/{username}", wrap(s.authFunc(types2.RoleAdmin)(s.getUser)))
	// Any user can update their own username, admins can update any user
	mux.Handle("PATCH /api/users/{username}", wrap(s.authFunc(types2.RoleBasic)(s.updateUser)))
	mux.Handle("DELETE /api/users/{username}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteUser)))

	mux.HandleFunc("POST /api/token-request", s.tokenRequest)
	mux.HandleFunc("GET /api/token-request/{id}", s.checkForToken)
	mux.HandleFunc("GET /api/token-request/{id}/{service}", s.redirectForTokenRequest)

	mux.Handle("GET /api/tokens", wrap(s.authFunc(types2.RoleBasic)(s.getTokens)))
	mux.Handle("DELETE /api/tokens/{id}", wrap(s.authFunc(types2.RoleBasic)(s.deleteToken)))
	mux.Handle("POST /api/tokens", wrap(s.authFunc(types2.RoleBasic)(s.newToken)))

	mux.HandleFunc("GET /api/supported-auth-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedAuthTypeConfigs())
	})
	mux.HandleFunc("GET /api/supported-oauth-app-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedOAuthAppTypeConfigs())
	})

	mux.Handle("POST /api/auth-providers", wrap(s.authFunc(types2.RoleAdmin)(s.createAuthProvider)))
	mux.Handle("PATCH /api/auth-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.updateAuthProvider)))
	mux.Handle("DELETE /api/auth-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteAuthProvider)))
	mux.HandleFunc("GET /api/auth-providers", s.getAuthProviders)
	mux.HandleFunc("GET /api/auth-providers/{slug}", s.getAuthProvider)
	mux.Handle("POST /api/auth-providers/{slug}/disable", wrap(s.authFunc(types2.RoleAdmin)(s.disableAuthProvider)))
	mux.Handle("POST /api/auth-providers/{slug}/enable", wrap(s.authFunc(types2.RoleAdmin)(s.enableAuthProvider)))

	mux.Handle("POST /api/llm-providers", wrap(s.authFunc(types2.RoleAdmin)(s.createLLMProvider)))
	mux.Handle("PATCH /api/llm-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.updateLLMProvider)))
	mux.Handle("DELETE /api/llm-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteLLMProvider)))
	mux.Handle("GET /api/llm-providers", wrap(s.authFunc(types2.RoleBasic)(s.getLLMProviders)))
	mux.Handle("GET /api/llm-providers/{slug}", wrap(s.authFunc(types2.RoleBasic)(s.getLLMProvider)))
	mux.Handle("POST /api/llm-providers/{slug}/disable", wrap(s.authFunc(types2.RoleAdmin)(s.disableLLMProvider)))
	mux.Handle("POST /api/llm-providers/{slug}/enable", wrap(s.authFunc(types2.RoleAdmin)(s.enableLLMProvider)))

	mux.Handle("POST /api/models", wrap(s.authFunc(types2.RoleAdmin)(s.createModel)))
	mux.Handle("PATCH /api/models/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.updateModel)))
	mux.Handle("DELETE /api/models/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteModel)))
	mux.Handle("GET /api/models", wrap(s.authFunc(types2.RoleBasic)(s.getModels)))
	mux.Handle("GET /api/models/{id}", wrap(s.authFunc(types2.RoleBasic)(s.getModel)))
	mux.Handle("POST /api/models/{id}/disable", wrap(s.authFunc(types2.RoleAdmin)(s.disableModel)))
	mux.Handle("POST /api/models/{id}/enable", wrap(s.authFunc(types2.RoleAdmin)(s.enableModel)))

	mux.Handle("GET /api/oauth/start/{id}/{service}", wrap(s.oauth))
	mux.Handle("/api/oauth/redirect/{service}", wrap(s.redirect))

	// CRUD routes for OAuth Apps (integrations with other service such as Microsoft 365)
	mux.Handle("GET /api/oauth-apps", wrap(s.authFunc(types2.RoleBasic)(s.listOAuthApps)))
	mux.Handle("GET /api/oauth-apps/{id}", wrap(s.authFunc(types2.RoleBasic)(s.oauthAppByID)))
	mux.Handle("POST /api/oauth-apps", wrap(s.authFunc(types2.RoleAdmin)(s.createOAuthApp)))
	mux.Handle("PATCH /api/oauth-apps/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.updateOAuthApp)))
	mux.Handle("DELETE /api/oauth-apps/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteOAuthApp)))

	// Routes for OAuth authorization code flow
	mux.Handle("GET /api/app-oauth/authorize/{id}", wrap(s.authorizeOAuthApp))
	mux.Handle("GET /api/app-oauth/refresh/{id}", wrap(s.refreshOAuthApp))
	mux.Handle("GET /api/app-oauth/callback/{id}", wrap(s.callbackOAuthApp))

	// Route for credential tools to get their OAuth tokens
	mux.Handle("GET /api/app-oauth/get-token", wrap(s.getTokenOAuthApp))

	// Handle the proxy to the LLM provider.
	mux.Handle("/api/llm/{provider}/{path...}", w(s.auth(false)(apply(httpToApiHandlerFunc(&httputil.ReverseProxy{
		Rewrite:      s.proxyToProvider,
		ErrorHandler: s.proxyError,
	}), addRequestID, addLogger, logRequest, s.monitor))))
	mux.Handle("/api/llm/{provider}", w(s.auth(false)(apply(httpToApiHandlerFunc(&httputil.ReverseProxy{
		Rewrite:      s.proxyToProvider,
		ErrorHandler: s.proxyError,
	}), addRequestID, addLogger, logRequest, s.monitor))))
}

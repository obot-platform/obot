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
	mux.Handle("GET /me", wrap(s.authFunc(types2.RoleBasic)(s.getCurrentUser)))
	mux.Handle("GET /users", wrap(s.authFunc(types2.RoleAdmin)(s.getUsers)))
	mux.Handle("GET /users/{username}", wrap(s.authFunc(types2.RoleAdmin)(s.getUser)))
	// Any user can update their own username, admins can update any user
	mux.Handle("PATCH /users/{username}", wrap(s.authFunc(types2.RoleBasic)(s.updateUser)))
	mux.Handle("DELETE /users/{username}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteUser)))

	mux.HandleFunc("POST /token-request", s.tokenRequest)
	mux.HandleFunc("GET /token-request/{id}", s.checkForToken)
	mux.HandleFunc("GET /token-request/{id}/{service}", s.redirectForTokenRequest)

	mux.Handle("GET /tokens", wrap(s.authFunc(types2.RoleBasic)(s.getTokens)))
	mux.Handle("DELETE /tokens/{id}", wrap(s.authFunc(types2.RoleBasic)(s.deleteToken)))
	mux.Handle("POST /tokens", wrap(s.authFunc(types2.RoleBasic)(s.newToken)))

	mux.HandleFunc("GET /supported-auth-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedAuthTypeConfigs())
	})
	mux.HandleFunc("GET /supported-oauth-app-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedOAuthAppTypeConfigs())
	})

	mux.Handle("POST /auth-providers", wrap(s.authFunc(types2.RoleAdmin)(s.createAuthProvider)))
	mux.Handle("PATCH /auth-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.updateAuthProvider)))
	mux.Handle("DELETE /auth-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteAuthProvider)))
	mux.HandleFunc("GET /auth-providers", s.getAuthProviders)
	mux.HandleFunc("GET /auth-providers/{slug}", s.getAuthProvider)
	mux.Handle("POST /auth-providers/{slug}/disable", wrap(s.authFunc(types2.RoleAdmin)(s.disableAuthProvider)))
	mux.Handle("POST /auth-providers/{slug}/enable", wrap(s.authFunc(types2.RoleAdmin)(s.enableAuthProvider)))

	mux.Handle("POST /llm-providers", wrap(s.authFunc(types2.RoleAdmin)(s.createLLMProvider)))
	mux.Handle("PATCH /llm-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.updateLLMProvider)))
	mux.Handle("DELETE /llm-providers/{slug}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteLLMProvider)))
	mux.Handle("GET /llm-providers", wrap(s.authFunc(types2.RoleBasic)(s.getLLMProviders)))
	mux.Handle("GET /llm-providers/{slug}", wrap(s.authFunc(types2.RoleBasic)(s.getLLMProvider)))
	mux.Handle("POST /llm-providers/{slug}/disable", wrap(s.authFunc(types2.RoleAdmin)(s.disableLLMProvider)))
	mux.Handle("POST /llm-providers/{slug}/enable", wrap(s.authFunc(types2.RoleAdmin)(s.enableLLMProvider)))

	mux.Handle("POST /models", wrap(s.authFunc(types2.RoleAdmin)(s.createModel)))
	mux.Handle("PATCH /models/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.updateModel)))
	mux.Handle("DELETE /models/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteModel)))
	mux.Handle("GET /models", wrap(s.authFunc(types2.RoleBasic)(s.getModels)))
	mux.Handle("GET /models/{id}", wrap(s.authFunc(types2.RoleBasic)(s.getModel)))
	mux.Handle("POST /models/{id}/disable", wrap(s.authFunc(types2.RoleAdmin)(s.disableModel)))
	mux.Handle("POST /models/{id}/enable", wrap(s.authFunc(types2.RoleAdmin)(s.enableModel)))

	oauthMux := http.NewServeMux()
	oauthMux.Handle("GET /start/{id}/{service}", wrap(s.oauth))
	oauthMux.Handle("/redirect/{service}", wrap(s.redirect))
	mux.Handle("/oauth/", http.StripPrefix("/oauth", oauthMux))

	// CRUD routes for OAuth Apps (integrations with other service such as Microsoft 365)
	mux.Handle("GET /oauth-apps", wrap(s.authFunc(types2.RoleBasic)(s.listOAuthApps)))
	mux.Handle("GET /oauth-apps/{id}", wrap(s.authFunc(types2.RoleBasic)(s.oauthAppByID)))
	mux.Handle("POST /oauth-apps", wrap(s.authFunc(types2.RoleAdmin)(s.createOAuthApp)))
	mux.Handle("PATCH /oauth-apps", wrap(s.authFunc(types2.RoleAdmin)(s.updateOAuthApp)))
	mux.Handle("DELETE /oauth-apps/{id}", wrap(s.authFunc(types2.RoleAdmin)(s.deleteOAuthApp)))

	// Routes for OAuth authorization code flow
	oauthAppsMux := http.NewServeMux()
	oauthAppsMux.Handle("GET /authorize/{id}", wrap(s.authorizeOAuthApp))
	oauthAppsMux.Handle("GET /refresh/{id}", wrap(s.refreshOAuthApp))
	oauthAppsMux.Handle("GET /callback/{id}", wrap(s.callbackOAuthApp))
	mux.Handle("/app-oauth/", http.StripPrefix("/app-oauth", oauthAppsMux))

	// Route for credential tools to get their OAuth tokens
	mux.Handle("GET /app-oauth/get-token", wrap(s.getTokenOAuthApp))

	// Handle the proxy to the LLM provider.
	llmMux := http.NewServeMux()
	llmMux.Handle("/{provider}/{path...}", &httputil.ReverseProxy{
		Rewrite:      s.proxyToProvider,
		ErrorHandler: s.proxyError,
	})
	llmMux.Handle("/{provider}", &httputil.ReverseProxy{
		Rewrite:      s.proxyToProvider,
		ErrorHandler: s.proxyError,
	})
	mux.Handle("/llm/", http.StripPrefix("/llm", wrap(s.auth(false)(apply(httpToApiHandlerFunc(llmMux), s.monitor)))))
}

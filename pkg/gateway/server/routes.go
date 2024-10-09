package server

import (
	_ "embed"
	"net/http"
	"net/http/httputil"

	"github.com/gptscript-ai/otto/pkg/api"
	kcontext "github.com/gptscript-ai/otto/pkg/gateway/context"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
)

func (s *Server) AddRoutes(authed func(api.HandlerFunc) http.Handler, mux *http.ServeMux) {
	// All the routes served by the API will start with `/api`
	mux.Handle("GET /me", authed(s.authFunc(types.RoleBasic)(s.getCurrentUser)))
	mux.Handle("GET /users", authed(s.authFunc(types.RoleAdmin)(s.getUsers)))
	mux.Handle("GET /users/{username}", authed(s.authFunc(types.RoleAdmin)(s.getUser)))
	// Any user can update their own username, admins can update any user
	mux.Handle("PATCH /users/{username}", authed(s.authFunc(types.RoleBasic)(s.updateUser)))
	mux.Handle("DELETE /users/{username}", authed(s.authFunc(types.RoleAdmin)(s.deleteUser)))

	mux.HandleFunc("POST /token-request", s.tokenRequest)
	mux.HandleFunc("GET /token-request/{id}", s.checkForToken)
	mux.HandleFunc("GET /token-request/{id}/{service}", s.redirectForTokenRequest)

	mux.Handle("GET /tokens", authed(s.authFunc(types.RoleBasic)(s.getTokens)))
	mux.Handle("DELETE /tokens/{id}", authed(s.authFunc(types.RoleBasic)(s.deleteToken)))
	mux.Handle("POST /tokens", authed(s.authFunc(types.RoleBasic)(s.newToken)))

	mux.HandleFunc("GET /supported-auth-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedAuthTypeConfigs())
	})
	mux.HandleFunc("GET /supported-oauth-app-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedOAuthAppTypeConfigs())
	})

	mux.Handle("POST /auth-providers", authed(s.authFunc(types.RoleAdmin)(s.createAuthProvider)))
	mux.Handle("PATCH /auth-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.updateAuthProvider)))
	mux.Handle("DELETE /auth-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.deleteAuthProvider)))
	mux.HandleFunc("GET /auth-providers", s.getAuthProviders)
	mux.HandleFunc("GET /auth-providers/{slug}", s.getAuthProvider)
	mux.Handle("POST /auth-providers/{slug}/disable", authed(s.authFunc(types.RoleAdmin)(s.disableAuthProvider)))
	mux.Handle("POST /auth-providers/{slug}/enable", authed(s.authFunc(types.RoleAdmin)(s.enableAuthProvider)))

	mux.Handle("POST /llm-providers", authed(s.authFunc(types.RoleAdmin)(s.createLLMProvider)))
	mux.Handle("PATCH /llm-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.updateLLMProvider)))
	mux.Handle("DELETE /llm-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.deleteLLMProvider)))
	mux.Handle("GET /llm-providers", authed(s.authFunc(types.RoleBasic)(s.getLLMProviders)))
	mux.Handle("GET /llm-providers/{slug}", authed(s.authFunc(types.RoleBasic)(s.getLLMProvider)))
	mux.Handle("POST /llm-providers/{slug}/disable", authed(s.authFunc(types.RoleAdmin)(s.disableLLMProvider)))
	mux.Handle("POST /llm-providers/{slug}/enable", authed(s.authFunc(types.RoleAdmin)(s.enableLLMProvider)))

	mux.Handle("POST /models", authed(s.authFunc(types.RoleAdmin)(s.createModel)))
	mux.Handle("PATCH /models/{id}", authed(s.authFunc(types.RoleAdmin)(s.updateModel)))
	mux.Handle("DELETE /models/{id}", authed(s.authFunc(types.RoleAdmin)(s.deleteModel)))
	mux.Handle("GET /models", authed(s.authFunc(types.RoleBasic)(s.getModels)))
	mux.Handle("GET /models/{id}", authed(s.authFunc(types.RoleBasic)(s.getModel)))
	mux.Handle("POST /models/{id}/disable", authed(s.authFunc(types.RoleAdmin)(s.disableModel)))
	mux.Handle("POST /models/{id}/enable", authed(s.authFunc(types.RoleAdmin)(s.enableModel)))

	oauthMux := http.NewServeMux()
	oauthMux.HandleFunc("GET /start/{id}/{service}", s.oauth)
	oauthMux.HandleFunc("/redirect/{service}", s.redirect)
	mux.Handle("/oauth/", http.StripPrefix("/oauth", apply(oauthMux, addRequestID, addLogger, logRequest, contentType("application/json"))))

	// CRUD routes for OAuth Apps (integrations with other service such as Microsoft 365)
	mux.Handle("GET /oauth-apps", authed(s.authFunc(types.RoleBasic)(s.listOAuthApps)))
	mux.Handle("GET /oauth-apps/{id}", authed(s.authFunc(types.RoleBasic)(s.oauthAppByID)))
	mux.Handle("POST /oauth-apps", authed(s.authFunc(types.RoleAdmin)(s.createOAuthApp)))
	mux.Handle("PATCH /oauth-apps", authed(s.authFunc(types.RoleAdmin)(s.updateOAuthApp)))
	mux.Handle("DELETE /oauth-apps/{id}", authed(s.authFunc(types.RoleAdmin)(s.deleteOAuthApp)))

	// Routes for OAuth authorization code flow
	oauthAppsMux := http.NewServeMux()
	oauthAppsMux.Handle("GET /authorize/{id}", authed(s.authorizeOAuthApp))
	oauthAppsMux.Handle("GET /refresh/{id}", authed(s.refreshOAuthApp))
	oauthAppsMux.Handle("GET /callback/{id}", authed(s.callbackOAuthApp))
	mux.Handle("/app-oauth/", http.StripPrefix("/app-oauth", apply(oauthAppsMux, addRequestID, addLogger, logRequest, contentType("application/json"))))

	// Route for credential tools to get their OAuth tokens
	mux.Handle("GET /app-oauth/get-token", authed(s.getTokenOAuthApp))

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
	mux.Handle("/llm/", http.StripPrefix("/llm", authed(s.auth(types.RoleBasic)(httpToApiHandlerFunc(apply(llmMux, addRequestID, addLogger, logRequest, s.monitor))))))
}

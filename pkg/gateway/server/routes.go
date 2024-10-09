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
	apiMux := http.NewServeMux()
	apiMux.Handle("GET /me", authed(s.authFunc(types.RoleBasic)(s.getCurrentUser)))
	apiMux.Handle("GET /users", authed(s.authFunc(types.RoleAdmin)(s.getUsers)))
	apiMux.Handle("GET /users/{username}", authed(s.authFunc(types.RoleAdmin)(s.getUser)))
	// Any user can update their own username, admins can update any user
	apiMux.Handle("PATCH /users/{username}", authed(s.authFunc(types.RoleBasic)(s.updateUser)))
	apiMux.Handle("DELETE /users/{username}", authed(s.authFunc(types.RoleAdmin)(s.deleteUser)))

	apiMux.HandleFunc("POST /token-request", s.tokenRequest)
	apiMux.HandleFunc("GET /token-request/{id}", s.checkForToken)
	apiMux.HandleFunc("GET /token-request/{id}/{service}", s.redirectForTokenRequest)

	apiMux.Handle("GET /tokens", authed(s.authFunc(types.RoleBasic)(s.getTokens)))
	apiMux.Handle("DELETE /tokens/{id}", authed(s.authFunc(types.RoleBasic)(s.deleteToken)))
	apiMux.Handle("POST /tokens", authed(s.authFunc(types.RoleBasic)(s.newToken)))

	apiMux.HandleFunc("GET /supported-auth-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedAuthTypeConfigs())
	})
	apiMux.HandleFunc("GET /supported-oauth-app-types", func(writer http.ResponseWriter, r *http.Request) {
		writeResponse(r.Context(), kcontext.GetLogger(r.Context()), writer, types.SupportedOAuthAppTypeConfigs())
	})

	apiMux.Handle("POST /auth-providers", authed(s.authFunc(types.RoleAdmin)(s.createAuthProvider)))
	apiMux.Handle("PATCH /auth-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.updateAuthProvider)))
	apiMux.Handle("DELETE /auth-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.deleteAuthProvider)))
	apiMux.HandleFunc("GET /auth-providers", s.getAuthProviders)
	apiMux.HandleFunc("GET /auth-providers/{slug}", s.getAuthProvider)
	apiMux.Handle("POST /auth-providers/{slug}/disable", authed(s.authFunc(types.RoleAdmin)(s.disableAuthProvider)))
	apiMux.Handle("POST /auth-providers/{slug}/enable", authed(s.authFunc(types.RoleAdmin)(s.enableAuthProvider)))

	apiMux.Handle("POST /llm-providers", authed(s.authFunc(types.RoleAdmin)(s.createLLMProvider)))
	apiMux.Handle("PATCH /llm-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.updateLLMProvider)))
	apiMux.Handle("DELETE /llm-providers/{slug}", authed(s.authFunc(types.RoleAdmin)(s.deleteLLMProvider)))
	apiMux.Handle("GET /llm-providers", authed(s.authFunc(types.RoleBasic)(s.getLLMProviders)))
	apiMux.Handle("GET /llm-providers/{slug}", authed(s.authFunc(types.RoleBasic)(s.getLLMProvider)))
	apiMux.Handle("POST /llm-providers/{slug}/disable", authed(s.authFunc(types.RoleAdmin)(s.disableLLMProvider)))
	apiMux.Handle("POST /llm-providers/{slug}/enable", authed(s.authFunc(types.RoleAdmin)(s.enableLLMProvider)))

	apiMux.Handle("POST /models", authed(s.authFunc(types.RoleAdmin)(s.createModel)))
	apiMux.Handle("PATCH /models/{id}", authed(s.authFunc(types.RoleAdmin)(s.updateModel)))
	apiMux.Handle("DELETE /models/{id}", authed(s.authFunc(types.RoleAdmin)(s.deleteModel)))
	apiMux.Handle("GET /models", authed(s.authFunc(types.RoleBasic)(s.getModels)))
	apiMux.Handle("GET /models/{id}", authed(s.authFunc(types.RoleBasic)(s.getModel)))
	apiMux.Handle("POST /models/{id}/disable", authed(s.authFunc(types.RoleAdmin)(s.disableModel)))
	apiMux.Handle("POST /models/{id}/enable", authed(s.authFunc(types.RoleAdmin)(s.enableModel)))

	oauthMux := http.NewServeMux()
	oauthMux.HandleFunc("GET /start/{id}/{service}", s.oauth)
	oauthMux.HandleFunc("/redirect/{service}", s.redirect)
	mux.Handle("/oauth/", http.StripPrefix("/oauth", apply(oauthMux, addRequestID, addLogger, logRequest, contentType("application/json"))))

	// CRUD routes for OAuth Apps (integrations with other service such as Microsoft 365)
	apiMux.Handle("GET /oauth-apps", authed(s.authFunc(types.RoleBasic)(s.listOAuthApps)))
	apiMux.Handle("GET /oauth-apps/{id}", authed(s.authFunc(types.RoleBasic)(s.oauthAppByID)))
	apiMux.Handle("POST /oauth-apps", authed(s.authFunc(types.RoleAdmin)(s.createOAuthApp)))
	apiMux.Handle("PATCH /oauth-apps", authed(s.authFunc(types.RoleAdmin)(s.updateOAuthApp)))
	apiMux.Handle("DELETE /oauth-apps/{id}", authed(s.authFunc(types.RoleAdmin)(s.deleteOAuthApp)))

	// Routes for OAuth authorization code flow
	oauthAppsMux := http.NewServeMux()
	oauthAppsMux.Handle("GET /{id}/authorize", authed(s.authorizeOAuthApp))
	oauthAppsMux.Handle("GET /{id}/refresh", authed(s.refreshOAuthApp))
	oauthAppsMux.Handle("GET /{id}/callback", authed(s.callbackOAuthApp))
	mux.Handle("/oauth-apps/", http.StripPrefix("/oauth-apps", apply(oauthAppsMux, addRequestID, addLogger, logRequest, contentType("application/json"))))

	// Route for credential tools to get their OAuth tokens
	apiMux.Handle("GET /oauth-apps/get-token", authed(s.getTokenOAuthApp))

	mux.Handle("/api/", http.StripPrefix("/api", apply(apiMux, addRequestID, addLogger, logRequest, contentType("application/json"))))

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

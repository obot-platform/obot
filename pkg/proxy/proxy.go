package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/metrics"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var log = logger.Package()

const (
	CurrentAuthProviderCookie  = "current_auth_provider"
	ObotAccessTokenCookie      = "obot_access_token"
	ObotAccessTokenCookieZero  = "obot_access_token_0"
	ObotAuthProviderQueryParam = "obot-auth-provider"
)

var ErrInvalidSession = errors.New("invalid session")

type Manager struct {
	dispatcher *dispatcher.Dispatcher
	gptClient  *gptscript.GPTScript
}

func NewProxyManager(dispatcher *dispatcher.Dispatcher, gptClient *gptscript.GPTScript) *Manager {
	m := &Manager{
		dispatcher: dispatcher,
		gptClient:  gptClient,
	}

	return m
}

func (pm *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	// Check for the access token cookie.
	// This authenticator requires the cookie in order to authenticate any users.
	if _, err := req.Cookie(ObotAccessTokenCookie); errors.Is(err, http.ErrNoCookie) {
		return nil, false, nil
	}

	configuredProvider, err := pm.dispatcher.GetConfiguredAuthProvider(req.Context())
	if err != nil {
		return nil, false, err
	} else if configuredProvider == "" {
		// No provider is configured, but the user has a session cookie.
		// Probably the old provider was deconfigured.
		return nil, false, ErrInvalidSession
	}

	proxy, err := pm.createProxy(req.Context(), pm.gptClient, system.DefaultNamespace+"/"+configuredProvider)
	if err != nil {
		return nil, false, err
	}

	return proxy.authenticateRequest(req)
}

func (pm *Manager) HandlerFunc(ctx api.Context) error {
	pm.ServeHTTP(ctx.User, ctx.ResponseWriter, ctx.Request)
	return nil
}

func (pm *Manager) ServeHTTP(user user.Info, w http.ResponseWriter, r *http.Request) {
	// If the proxy manager is not set up, just redirect the user.
	// This can happen when auth is disabled.
	if pm == nil {
		rd := r.URL.Query().Get("rd")
		if rd == "" || !strings.HasPrefix(rd, "/") {
			rd = "/"
		}
		http.Redirect(w, r, rd, http.StatusFound)
		return
	}

	// Determine which auth provider to use.
	var (
		provider string
		err      error
	)
	if len(user.GetExtra()["auth_provider_name"]) > 0 && len(user.GetExtra()["auth_provider_namespace"]) > 0 {
		provider = fmt.Sprintf("%s/%s", user.GetExtra()["auth_provider_namespace"][0], user.GetExtra()["auth_provider_name"][0])
	} else if r.URL.Path == "/oauth2/callback" {
		// Check for the current auth provider cookie.
		if cookie, err := r.Cookie(CurrentAuthProviderCookie); err == nil {
			provider = cookie.Value

			// Now delete the current auth provider cookie so that it doesn't interfere with anything.
			http.SetCookie(w, &http.Cookie{
				Name:   CurrentAuthProviderCookie,
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})
		} else {
			http.Error(w, "Login timed out. Please try again.", http.StatusUnauthorized)
			return
		}
	} else if param := r.URL.Query().Get(ObotAuthProviderQueryParam); param != "" {
		// If the provider is set in the query params, use that.
		provider = param
	}

	// Save the redirect target for later.
	rdParam := r.URL.Query().Get("rd")
	if rdParam == "" {
		rdParam = "/"
	}

	// If no provider is set, just use the alphabetically first provider.
	if provider == "" {
		configuredProvider, err := pm.dispatcher.GetConfiguredAuthProvider(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get configured auth provider: %v", err), http.StatusInternalServerError)
			return
		}
		if configuredProvider == "" {
			// There aren't any auth providers configured. Return an error, unless the user is signing out, in which case, just redirect.
			if r.URL.Path == "/oauth2/sign_out" {
				http.Redirect(w, r, rdParam, http.StatusFound)
				return
			}

			http.Error(w, "no auth providers configured", http.StatusBadRequest)
			return
		}

		provider = system.DefaultNamespace + "/" + configuredProvider
	} else {
		namespace, name, _ := strings.Cut(provider, "/")
		if namespace == "" || name == "" {
			http.Error(w, "invalid auth provider:"+provider, http.StatusBadRequest)
			return
		}

		// Check if the provider is configured.
		configuredProvider, err := pm.dispatcher.GetConfiguredAuthProvider(r.Context())
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get configured auth provider: %v", err), http.StatusInternalServerError)
			return
		}
		if configuredProvider != "" && configuredProvider != name {
			http.Error(w, "auth provider not configured: "+provider, http.StatusBadRequest)
			return
		}
	}

	proxy, err := pm.createProxy(r.Context(), pm.gptClient, provider)
	if err != nil {
		if r.URL.Path != "/oauth2/sign_out" {
			http.Error(w, fmt.Sprintf("failed to create proxy: %v", err), http.StatusInternalServerError)
		} else {
			// If the user is signing out, and we failed to start the proxy,
			// it's probably because their auth provider got deconfigured.
			// Just redirect them to where they are supposed to go.
			http.Redirect(w, r, rdParam, http.StatusFound)
		}
		return
	}

	// If this is a sign in request, set the "current_auth_provider" cookie.
	if r.URL.Path == "/oauth2/start" {
		http.SetCookie(w, &http.Cookie{
			Name:   CurrentAuthProviderCookie,
			Value:  provider,
			Path:   "/oauth2/callback",
			MaxAge: 60 * 15, // 15 minutes should be plenty of time to do oauth
		})
	}

	log.Infof("forwarding request for %s to provider %s", r.URL.Path, provider)

	proxy.serveHTTP(w, r)
}

func (pm *Manager) createProxy(ctx context.Context, gptClient *gptscript.GPTScript, provider string) (*Proxy, error) {
	parts := strings.Split(provider, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid provider: %s", provider)
	}

	providerURL, err := pm.dispatcher.URLForAuthProvider(ctx, gptClient, parts[0], parts[1])
	if err != nil {
		return nil, err
	}

	return newProxy(parts[0], parts[1], providerURL.String())
}

type Proxy struct {
	proxy                *httputil.ReverseProxy
	url, name, namespace string
}

func newProxy(providerNamespace, providerName, providerURL string) (*Proxy, error) {
	u, err := url.Parse(providerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse provider URL: %w", err)
	}

	return &Proxy{
		proxy:     httputil.NewSingleHostReverseProxy(u),
		url:       providerURL,
		name:      providerName,
		namespace: providerNamespace,
	}, nil
}

func (p *Proxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// Make sure the path is something that we expect.
	switch r.URL.Path {
	case "/oauth2/start":
	case "/oauth2/redirect":
	case "/oauth2/sign_out":
	case "/oauth2/callback":
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	p.proxy.ServeHTTP(w, r)
}

// serializableRequest represents an HTTP request that can be serialized for authentication flows
type serializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

// serializableState represents the authentication state returned from auth providers
type serializableState struct {
	ExpiresOn         *time.Time `json:"expiresOn"`
	AccessToken       string     `json:"accessToken"`
	PreferredUsername string     `json:"preferredUsername"`
	User              string     `json:"user"`
	Email             string     `json:"email"`
	SetCookies        []string   `json:"setCookies"`
}

func (p *Proxy) authenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	// Track authentication attempt start time for metrics
	startTime := time.Now()

	sr := serializableRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header,
	}

	srJSON, err := json.Marshal(sr)
	if err != nil {
		metrics.RecordAuthenticationError(p.name)
		return nil, false, err
	}

	stateRequest, err := http.NewRequest(http.MethodPost, p.url+"/obot-get-state", strings.NewReader(string(srJSON)))
	if err != nil {
		metrics.RecordAuthenticationError(p.name)
		return nil, false, err
	}

	stateResponse, err := http.DefaultClient.Do(stateRequest)
	if err != nil {
		metrics.RecordAuthenticationError(p.name)
		return nil, false, err
	}
	defer stateResponse.Body.Close()
	body, err := io.ReadAll(stateResponse.Body)
	if err != nil {
		metrics.RecordAuthenticationError(p.name)
		return nil, false, err
	}

	// Handle session-related errors that should redirect to login
	// (instead of returning HTTP 500 to the client)
	if IsSessionError(stateResponse.StatusCode, string(body)) {
		// Record token refresh failure (session error indicates token refresh failed)
		duration := time.Since(startTime).Seconds()
		metrics.RecordTokenRefreshFailure(p.name, duration)
		metrics.RecordSessionStorageError(p.name, "token_refresh")

		log.Warnf("session error detected for provider %s (status %d), redirecting to login: %s",
			p.name, stateResponse.StatusCode, string(body))
		return nil, false, ErrInvalidSession
	}

	var ss serializableState
	if err = json.Unmarshal(body, &ss); err != nil {
		return nil, false, err
	}

	userName := getUsername(p.name, ss)
	u := &user.DefaultInfo{
		UID:  ss.User,
		Name: userName,
		Extra: map[string][]string{
			"email":                   {ss.Email},
			"auth_provider_name":      {p.name},
			"auth_provider_namespace": {p.namespace},
			"auth_provider_user_id":   {ss.User},
		},
	}

	if len(ss.SetCookies) != 0 {
		// This is set if the auth provider needed to refresh the token.
		u.Extra["set-cookies"] = ss.SetCookies
	}

	// Put the access token and provider URL on the context so that the profile icon and group info can be fetched.
	*req = *req.WithContext(accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken))
	*req = *req.WithContext(auth.ContextWithProviderURL(req.Context(), p.url))

	// Record successful authentication
	duration := time.Since(startTime).Seconds()
	metrics.RecordAuthenticationSuccess(p.name)

	// If SetCookies is populated, a token refresh occurred
	if len(ss.SetCookies) != 0 {
		metrics.RecordTokenRefreshSuccess(p.name, duration)
	}

	return &authenticator.Response{
		User: u,
	}, true, nil
}

// Important: do not change the order of these checks.
// We rely on the preferred username from GitHub being the user ID in the sessions table.
// See pkg/gateway/server/logout_all.go for more details, as well as the GitHub auth provider code.
// Entra and Keycloak follow the same pattern: ss.User contains the stable provider ID (Azure OID / Keycloak Subject),
// while ss.PreferredUsername contains the human-readable username (UPN / preferred_username).
func getUsername(providerName string, ss serializableState) string {
	if providerName == "github-auth-provider" || providerName == "entra-auth-provider" || providerName == "keycloak-auth-provider" {
		if ss.PreferredUsername != "" {
			return ss.PreferredUsername
		}
		// Fallback to email if PreferredUsername is empty
		return ss.Email
	}

	userName := ss.User
	if userName == "" {
		userName = ss.Email
	}

	return userName
}

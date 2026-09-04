package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const (
	CurrentAuthProviderCookie  = "current_auth_provider"
	ObotAccessTokenCookie      = "obot_access_token"
	ObotAuthProviderQueryParam = "obot-auth-provider"
)

var (
	ErrInvalidSession = errors.New("invalid session")
)

type Manager struct {
	dispatcher *dispatcher.Dispatcher
}

type Proxy struct {
	proxy                               *httputil.ReverseProxy
	url, name, namespace, groupIDPrefix string
}

// serializableRequest represents an HTTP request that can be serialized for authentication flows
type serializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

// serializableState represents the authentication state returned from auth providers
type serializableState struct {
	ExpiresOn             *time.Time `json:"expiresOn"`
	AccessToken           string     `json:"accessToken"`
	PreferredUsername     string     `json:"preferredUsername"`
	User                  string     `json:"user"`
	Email                 string     `json:"email"`
	SetCookies            []string   `json:"setCookies"`
	RequirePasswordChange bool       `json:"requirePasswordChange"`
}

func NewProxyManager(dispatcher *dispatcher.Dispatcher) *Manager {
	m := &Manager{
		dispatcher: dispatcher,
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

	proxy, err := pm.createProxy(req.Context(), system.DefaultNamespace+"/"+configuredProvider)
	if err != nil {
		return nil, false, err
	}

	resp, ok, err := proxy.authenticateRequest(req)
	if ok || !errors.Is(err, ErrInvalidSession) {
		return resp, ok, err
	}

	// A verification login is served by the staged provider, so the active one cannot validate its
	// session. Fall back only for the browser holding the verification.
	staged, stagedErr := pm.dispatcher.GetStagedAuthProvider(req.Context())
	if stagedErr != nil || staged == "" {
		return resp, ok, err
	}
	if loginable, loginErr := pm.dispatcher.LoginableAuthProvider(req.Context(), req, staged); loginErr != nil || !loginable {
		return resp, ok, err
	}
	stagedProxy, stagedErr := pm.createProxy(req.Context(), system.DefaultNamespace+"/"+staged)
	if stagedErr != nil {
		return resp, ok, err
	}

	return stagedProxy.authenticateRequest(req)
}

func (pm *Manager) HandlerFunc(ctx api.Context) error {
	pm.ServeHTTP(ctx.User, ctx.ResponseWriter, ctx.Request)
	return nil
}

// stagedVerificationProvider returns the staged provider when this request belongs to a
// verification round trip, and "" otherwise. The round trip is identified by the verify cookie
// rather than a query parameter because a provider's own login pages redirect through paths that
// name no provider of their own.
func (pm *Manager) stagedVerificationProvider(r *http.Request) string {
	if cookie, err := r.Cookie(auth.AuthProviderVerifyCookie); err != nil || cookie.Value == "" {
		return ""
	}

	staged, err := pm.dispatcher.GetStagedAuthProvider(r.Context())
	if err != nil || staged == "" {
		return ""
	}

	// LoginableAuthProvider re-checks that cookie against an open verification, so a stale or
	// forged one cannot turn the staged provider into a login path of its own.
	loginable, err := pm.dispatcher.LoginableAuthProvider(r.Context(), r, staged)
	if err != nil || !loginable {
		return ""
	}

	return system.DefaultNamespace + "/" + staged
}

func clearCurrentAuthProviderCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:   CurrentAuthProviderCookie,
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
}

func (pm *Manager) ServeHTTP(user user.Info, w http.ResponseWriter, r *http.Request) {
	// If the proxy manager is not set up, just redirect the user.
	// This can happen when auth is disabled.
	if pm == nil {
		http.Redirect(w, r, auth.SafeRedirectPath(r.URL.Query().Get("rd")), http.StatusFound)
		return
	}

	// Determine which auth provider to use.
	var (
		provider string
		err      error
	)
	if requested := pm.stagedVerificationProvider(r); requested != "" {
		// A verification deliberately signs in through a provider other than the caller's, so it is
		// the only case where the provider a request names beats the one its session uses.
		provider = requested
		if r.URL.Path == "/oauth2/callback" {
			clearCurrentAuthProviderCookie(w)
		}
	} else if len(user.GetExtra()["auth_provider_name"]) > 0 && len(user.GetExtra()["auth_provider_namespace"]) > 0 {
		provider = fmt.Sprintf("%s/%s", user.GetExtra()["auth_provider_namespace"][0], user.GetExtra()["auth_provider_name"][0])
	} else if r.URL.Path == "/oauth2/callback" {
		// Check for the current auth provider cookie.
		if cookie, err := r.Cookie(CurrentAuthProviderCookie); err == nil {
			provider = cookie.Value

			// Now delete the current auth provider cookie so that it doesn't interfere with anything.
			clearCurrentAuthProviderCookie(w)
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
		loginable, err := pm.dispatcher.LoginableAuthProvider(r.Context(), r, name)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to get configured auth provider: %v", err), http.StatusInternalServerError)
			return
		}
		if !loginable {
			http.Error(w, "auth provider not configured: "+provider, http.StatusBadRequest)
			return
		}
	}

	proxy, err := pm.createProxy(r.Context(), provider)
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

	slog.Info("forwarding request to provider", "path", r.URL.Path, "provider", provider)

	proxy.serveHTTP(w, r)
}

func (pm *Manager) createProxy(ctx context.Context, provider string) (*Proxy, error) {
	parts := strings.Split(provider, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid provider: %s", provider)
	}

	providerURL, err := pm.dispatcher.URLForAuthProvider(ctx, parts[0], parts[1])
	if err != nil {
		return nil, err
	}
	groupIDPrefix, err := pm.dispatcher.GroupIDPrefixForAuthProvider(ctx, parts[0], parts[1])
	if err != nil {
		return nil, err
	}

	return newProxy(parts[0], parts[1], providerURL.String(), groupIDPrefix)
}

func newProxy(providerNamespace, providerName, providerURL, groupIDPrefix string) (*Proxy, error) {
	u, err := url.Parse(providerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse provider URL: %w", err)
	}

	return &Proxy{
		proxy:         httputil.NewSingleHostReverseProxy(u),
		url:           providerURL,
		name:          providerName,
		namespace:     providerNamespace,
		groupIDPrefix: groupIDPrefix,
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

func (p *Proxy) authenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	sr := serializableRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header,
	}

	srJSON, err := json.Marshal(sr)
	if err != nil {
		return nil, false, err
	}

	ctx, cancel := context.WithTimeout(req.Context(), 15*time.Second)
	defer cancel()

	stateRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url+"/obot-get-state", strings.NewReader(string(srJSON)))
	if err != nil {
		return nil, false, err
	}

	stateResponse, err := http.DefaultClient.Do(stateRequest)
	if err != nil {
		return nil, false, err
	}
	defer stateResponse.Body.Close()
	body, err := io.ReadAll(stateResponse.Body)
	if err != nil {
		return nil, false, err
	}

	if stateResponse.StatusCode == http.StatusInternalServerError && (strings.Contains(string(body), "record not found") || strings.Contains(string(body), "session ticket cookie failed validation")) {
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
	if ss.RequirePasswordChange {
		u.Extra["password_change_required"] = []string{"true"}
	}

	if len(ss.SetCookies) != 0 {
		// This is set if the auth provider needed to refresh the token.
		u.Extra["set-cookies"] = ss.SetCookies
	}

	// Put the access token and provider metadata on the context so that the profile icon and group
	// info can be fetched and validated.
	providerContext := accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken)
	providerContext = auth.ContextWithProviderURL(providerContext, p.url)
	providerContext = auth.ContextWithProviderGroupIDPrefix(providerContext, p.groupIDPrefix)
	*req = *req.WithContext(providerContext)

	return &authenticator.Response{
		User: u,
	}, true, nil
}

// Important: do not change the order of these checks.
// We rely on the preferred username from GitHub being the user ID in the sessions table.
// See pkg/gateway/server/logout_all.go for more details, as well as the GitHub auth provider code.
func getUsername(providerName string, ss serializableState) string {
	if providerName == "github-auth-provider" {
		return ss.PreferredUsername
	}

	userName := ss.User
	if userName == "" {
		userName = ss.Email
	}

	return userName
}

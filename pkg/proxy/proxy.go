package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var log = logger.Package()

const (
	CurrentAuthProviderCookie   = "current_auth_provider"
	ObotAccessTokenCookiePrefix = "obot_access_token_"
	ObotAuthProviderQueryParam  = "obot-auth-provider"
)

type Manager struct {
	dispatcher *dispatcher.Dispatcher
}

func NewProxyManager(dispatcher *dispatcher.Dispatcher) *Manager {
	return &Manager{
		dispatcher: dispatcher,
	}
}

func getModelProviderFromCookies(req *http.Request) (string, error) {
	// Get all the access token cookies.
	cookies := req.Cookies()
	var accessTokenCookies []http.Cookie
	for _, cookie := range cookies {
		if strings.HasPrefix(cookie.Name, ObotAccessTokenCookiePrefix) {
			accessTokenCookies = append(accessTokenCookies, *cookie)
		}
	}

	if len(accessTokenCookies) == 0 {
		return "", errors.New("no access token cookie found")
	}

	// Sort the cookies by latest expiration.
	// The one that expires farthest is the one we want.
	sort.Slice(accessTokenCookies, func(i, j int) bool {
		return accessTokenCookies[i].Expires.After(accessTokenCookies[j].Expires)
	})
	cookieName := accessTokenCookies[0].Name

	// Strip the suffixes "_0" and "_1" if they exist.
	// These are added on when the token is too large to fit in one cookie.
	if strings.HasSuffix(cookieName, "_0") {
		cookieName = strings.TrimSuffix(cookieName, "_0")
	} else if strings.HasSuffix(cookieName, "_1") {
		cookieName = strings.TrimSuffix(cookieName, "_1")
	}

	_, provider, exists := strings.Cut(cookieName, ObotAccessTokenCookiePrefix)
	if !exists {
		// This should be impossible, but we'll account for it anyway.
		return "", fmt.Errorf("failed to find provider in cookie name %s", cookieName)
	}

	// The provider namespace and name should be sparated by a double underscore.
	namespace, name, exists := strings.Cut(provider, "__")
	if !exists {
		return "", fmt.Errorf("failed to find provider in cookie name %s", cookieName)
	}

	return fmt.Sprintf("%s/%s", namespace, name), nil
}

func clearAccessTokenCookies(w http.ResponseWriter, req *http.Request) {
	for _, cookie := range req.Cookies() {
		if strings.HasPrefix(cookie.Name, ObotAccessTokenCookiePrefix) {
			http.SetCookie(w, &http.Cookie{
				Name:   cookie.Name,
				Value:  "",
				Path:   cookie.Path,
				MaxAge: -1,
			})
		}
	}
}

func (pm *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	provider, err := getModelProviderFromCookies(req)
	if err != nil {
		return nil, false, err
	}

	proxy, err := pm.createProxy(req.Context(), provider)
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
		provider   string
		fromCookie bool
		err        error
	)
	if len(user.GetExtra()["auth_provider_name"]) > 0 && len(user.GetExtra()["auth_provider_namespace"]) > 0 {
		fromCookie = true
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
	} else if providerFromCookie, err := getModelProviderFromCookies(r); err == nil {
		fromCookie = true
		provider = providerFromCookie
	}

	// Save the redirect target for later.
	rdParam := r.URL.Query().Get("rd")
	if rdParam == "" {
		rdParam = "/"
	}

	// If no provider is set, just use the alphabetically first provider.
	if provider == "" {
		configuredProviders, err := pm.dispatcher.ListConfiguredAuthProviders(r.Context(), "default")
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list configured auth providers: %v", err), http.StatusInternalServerError)
			return
		} else if len(configuredProviders) == 0 {
			// There aren't any auth providers configured. Return an error, unless the user is signing out, in which case, just redirect.
			if r.URL.Path == "/oauth2/sign_out" {
				http.Redirect(w, r, rdParam, http.StatusFound)
				return
			}

			http.Error(w, "no auth providers configured", http.StatusBadRequest)
			return
		}

		sort.Slice(configuredProviders, func(i, j int) bool {
			return configuredProviders[i] < configuredProviders[j]
		})
		provider = "default/" + configuredProviders[0]
	} else {
		namespace, name, _ := strings.Cut(provider, "/")
		if namespace == "" {
			http.Error(w, "invalid auth provider:"+provider, http.StatusBadRequest)
			return
		}

		// Check if the provider is configured.
		configuredProviders, err := pm.dispatcher.ListConfiguredAuthProviders(r.Context(), namespace)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list configured auth providers: %v", err), http.StatusInternalServerError)
			return
		}

		if !slices.Contains(configuredProviders, name) {
			// The requested auth provider isn't configured. Return an error, unless the user is signing out, in which case, just redirect.
			if r.URL.Path == "/oauth2/sign_out" {
				// Clear all the access tokens.
				clearAccessTokenCookies(w, r)
				http.Redirect(w, r, rdParam, http.StatusFound)
				return
			}

			if fromCookie {
				// Clear the access token cookies, since they are bad.
				clearAccessTokenCookies(w, r)

				// Just refresh the page and try again.
				http.Redirect(w, r, r.URL.String(), http.StatusFound)
				return
			}

			http.Error(w, "auth provider not configured: "+provider, http.StatusBadRequest)
			return
		}
	}

	// If the legacy auth provider cookie exists, delete it.
	http.SetCookie(w, &http.Cookie{
		Name:   "obot_access_token",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})

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

	log.Infof("forwarding request for %s to provider %s", r.URL.Path, provider)

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

type SerializableRequest struct {
	Method string              `json:"method"`
	URL    string              `json:"url"`
	Header map[string][]string `json:"header"`
}

type SerializableState struct {
	ExpiresOn         *time.Time `json:"expiresOn"`
	AccessToken       string     `json:"accessToken"`
	PreferredUsername string     `json:"preferredUsername"`
	User              string     `json:"user"`
	Email             string     `json:"email"`
	SetCookies        []string   `json:"setCookies"`
}

func (p *Proxy) authenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	sr := SerializableRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: req.Header,
	}

	srJSON, err := json.Marshal(sr)
	if err != nil {
		return nil, false, err
	}

	stateRequest, err := http.NewRequest(http.MethodPost, p.url+"/obot-get-state", strings.NewReader(string(srJSON)))
	if err != nil {
		return nil, false, err
	}

	stateResponse, err := http.DefaultClient.Do(stateRequest)
	if err != nil {
		return nil, false, err
	}
	defer stateResponse.Body.Close()

	var ss SerializableState
	if err = json.NewDecoder(stateResponse.Body).Decode(&ss); err != nil {
		return nil, false, err
	}

	userName := ss.PreferredUsername
	if userName == "" {
		userName = ss.User
		if userName == "" {
			userName = ss.Email
		}
	}

	u := &user.DefaultInfo{
		UID:  ss.User,
		Name: userName,
		Extra: map[string][]string{
			"email":                   {ss.Email},
			"auth_provider_name":      {p.name},
			"auth_provider_namespace": {p.namespace},
		},
	}

	if ss.SetCookies != nil {
		// This is set if the auth provider needed to refresh the token.
		u.Extra["set-cookies"] = ss.SetCookies
	}

	if req.URL.Path == "/api/me" {
		// Put the access token on the context so that the profile icon can be fetched.
		*req = *req.WithContext(accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken))
	}

	return &authenticator.Response{
		User: u,
	}, true, nil
}

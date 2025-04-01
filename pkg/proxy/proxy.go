package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/ratelimit"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
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

type cacheObject struct {
	provider  string
	createdAt time.Time
}

type Manager struct {
	dispatcher               *dispatcher.Dispatcher
	tokenHashToProviderCache map[string]cacheObject
	lock                     sync.RWMutex
}

func NewProxyManager(ctx context.Context, dispatcher *dispatcher.Dispatcher) *Manager {
	m := &Manager{
		dispatcher:               dispatcher,
		tokenHashToProviderCache: make(map[string]cacheObject),
		lock:                     sync.RWMutex{},
	}

	go m.cacheCleanup(ctx)

	return m
}

func (pm *Manager) cacheCleanup(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			pm.lock.Lock()
			for tokenHash, obj := range pm.tokenHashToProviderCache {
				if time.Since(obj.createdAt) > 24*time.Hour {
					delete(pm.tokenHashToProviderCache, tokenHash)
				}
			}
			pm.lock.Unlock()
		}
	}
}

func getTokenCookie(req *http.Request) (*http.Cookie, error) {
	c, err := req.Cookie(ObotAccessTokenCookie)
	if errors.Is(err, http.ErrNoCookie) {
		// Check the zero cookie. This one is present when the token is too large to fit in one cookie and
		// must be split into two.
		c, err = req.Cookie(ObotAccessTokenCookieZero)
	}
	if err != nil {
		return nil, err
	}

	return c, nil
}

func getTokenHash(c *http.Cookie) (string, error) {
	h := sha256.New()
	h.Write([]byte(c.Value))
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func (pm *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	tokenCookie, err := getTokenCookie(req)
	if err != nil {
		return nil, false, nil
	}

	tokenHash, err := getTokenHash(tokenCookie)
	if err != nil {
		return nil, false, nil
	}

	pm.lock.RLock()
	cached, found := pm.tokenHashToProviderCache[tokenHash]
	pm.lock.RUnlock()

	if !found {
		// Try the token with each configured auth provider to see if one of them recognizes the user.
		configuredProviders := pm.dispatcher.ListConfiguredAuthProviders(system.DefaultNamespace)
		for _, configuredProvider := range configuredProviders {
			if proxy, err := pm.createProxy(req.Context(), system.DefaultNamespace+"/"+configuredProvider); err == nil {
				if resp, good, err := proxy.authenticateRequest(req); good && err == nil {
					pm.lock.Lock()
					pm.tokenHashToProviderCache[tokenHash] = cacheObject{
						provider:  system.DefaultNamespace + "/" + configuredProvider,
						createdAt: time.Now(),
					}
					ratelimit.EnableAuthGroupRateLimit(
						ratelimit.CredSourceTypeCookie,
						tokenCookie.Name,
						tokenCookie.Value,
						resp.User.(*user.DefaultInfo),
					)
					pm.lock.Unlock()
					return resp, true, nil
				}
			}
		}

		// No provider was found that recognized the user.
		return nil, false, nil
	}

	proxy, err := pm.createProxy(req.Context(), cached.provider)
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
		configuredProviders := pm.dispatcher.ListConfiguredAuthProviders(system.DefaultNamespace)
		if len(configuredProviders) == 0 {
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
		provider = system.DefaultNamespace + "/" + configuredProviders[0]
	} else {
		namespace, name, _ := strings.Cut(provider, "/")
		if namespace == "" || name == "" {
			http.Error(w, "invalid auth provider:"+provider, http.StatusBadRequest)
			return
		}

		// Check if the provider is configured.
		configuredProviders := pm.dispatcher.ListConfiguredAuthProviders(namespace)

		if !slices.Contains(configuredProviders, name) {
			// The requested auth provider isn't configured. Return an error, unless the user is signing out, in which case, just redirect.
			if r.URL.Path == "/oauth2/sign_out" {
				http.Redirect(w, r, rdParam, http.StatusFound)
				return
			}

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

	if len(ss.SetCookies) != 0 {
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

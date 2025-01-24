package proxy

import (
	"context"
	"encoding/json"
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
	ObotAccessTokenCookie      = "obot_access_token"
	ObotAuthProviderQueryParam = "obot-auth-provider"
)

type CookieContents struct {
	AuthProvider string `json:"authProvider"`
	Token        string `json:"token"`
}

type Manager struct {
	dispatcher *dispatcher.Dispatcher
}

func NewProxyManager(dispatcher *dispatcher.Dispatcher) *Manager {
	return &Manager{
		dispatcher: dispatcher,
	}
}

func (pm *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	cookie, err := req.Cookie(ObotAccessTokenCookie)
	if err != nil {
		return nil, false, nil
	}

	cookieOriginalValue := cookie.Value
	cookieValue, err := url.QueryUnescape(cookie.Value)
	if err != nil {
		return nil, false, nil
	}

	var contents CookieContents
	if err = json.Unmarshal([]byte(cookieValue), &contents); err != nil {
		return nil, false, nil
	}

	proxy, err := pm.createProxy(req.Context(), contents.AuthProvider)
	if err != nil {
		return nil, false, err
	}

	// Overwrite the cookie with just the token.
	if err := replaceTokenCookie(contents.Token, req); err != nil {
		return nil, false, err
	}

	// Reset the cookie value after authenticating.
	defer func() {
		if err := replaceTokenCookie(cookieOriginalValue, req); err != nil {
			log.Errorf("failed to reset cookie value: %v", err)
		}
	}()

	return proxy.authenticateRequest(req)
}

func (pm *Manager) HandlerFunc(ctx api.Context) error {
	pm.ServeHTTP(ctx.ResponseWriter, ctx.Request)
	return nil
}

func (pm *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
	var provider string
	if param := r.URL.Query().Get(ObotAuthProviderQueryParam); param != "" {
		// If the provider is set in the query params, use that.
		provider = param
	} else if cookie, err := r.Cookie(ObotAccessTokenCookie); err == nil {
		// Extract the provider from the cookie, if it's there.
		cookieValue, err := url.QueryUnescape(cookie.Value)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to unescape cookie value: %v", err), http.StatusBadRequest)
			return
		}

		var contents CookieContents
		if err = json.Unmarshal([]byte(cookieValue), &contents); err == nil {
			provider = contents.AuthProvider

			// Update the cookie to just be the token, which is what the auth provider expects.
			if err := replaceTokenCookie(contents.Token, r); err != nil {
				http.Error(w, fmt.Sprintf("failed to replace token cookie: %v", err), http.StatusInternalServerError)
			}
		}
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
				http.Redirect(w, r, rdParam, http.StatusFound)
				return
			}

			// Delete the cookie since it is bad.
			http.SetCookie(w, &http.Cookie{
				Name:   ObotAccessTokenCookie,
				Value:  "",
				Path:   "/",
				MaxAge: -1,
			})

			http.Error(w, "auth provider not configured: "+provider, http.StatusBadRequest)
			return
		}
	}

	// If the legacy auth provider cookie exists, delete it.
	http.SetCookie(w, &http.Cookie{
		Name:   ObotAuthProviderQueryParam,
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

	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.ModifyResponse = func(r *http.Response) error {
		// See which cookies the auth provider is setting.
		// We need to update the access token cookie to add information about the auth provider
		if headers := r.Header.Values("Set-Cookie"); headers != nil {
			for i, h := range headers {
				parts := strings.Split(h, "; ")
				for i, part := range parts {
					if strings.HasPrefix(part, ObotAccessTokenCookie+"=") {
						token := strings.TrimPrefix(part, ObotAccessTokenCookie+"=")

						if token != "" {
							newValue := fmt.Sprintf("{\"authProvider\":\"%s\",\"token\":\"%s\"}", providerNamespace+"/"+providerName, token)
							parts[i] = fmt.Sprintf("%s=%s", ObotAccessTokenCookie, url.QueryEscape(newValue))
						}
						break
					}
				}
				headers[i] = strings.Join(parts, "; ")
			}
		}
		return nil
	}

	return &Proxy{
		proxy:     proxy,
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
	SetCookie         string     `json:"setCookie"`
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

	if ss.SetCookie != "" {
		// This is set if the auth provider needed to refresh the token.
		u.Extra["set-cookie"] = []string{url.QueryEscape(fmt.Sprintf("{\"authProvider\":\"%s\",\"token\":\"%s\"}", p.namespace+"/"+p.name, ss.SetCookie))}
	}

	if req.URL.Path == "/api/me" {
		// Put the access token on the context so that the profile icon can be fetched.
		*req = *req.WithContext(accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken))
	}

	return &authenticator.Response{
		User: u,
	}, true, nil
}

func replaceTokenCookie(token string, req *http.Request) error {
	tokenCookie, err := req.Cookie(ObotAccessTokenCookie)
	if err != nil {
		return fmt.Errorf("failed to get token cookie: %w", err)
	}

	tokenCookie.Value = token

	otherCookies := make([]http.Cookie, 0, len(req.Cookies()))
	for _, c := range req.Cookies() {
		if c.Name != ObotAccessTokenCookie {
			otherCookies = append(otherCookies, *c)
		}
	}

	req.Header.Del("Cookie")

	for _, c := range otherCookies {
		req.AddCookie(&c)
	}
	req.AddCookie(tokenCookie)

	return nil
}

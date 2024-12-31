package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

const authProviderCookie = "obot-auth-provider"

type Manager struct {
	dispatcher *dispatcher.Dispatcher
}

func NewProxyManager(dispatcher *dispatcher.Dispatcher) *Manager {
	return &Manager{
		dispatcher: dispatcher,
	}
}

func (pm *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	c, err := req.Cookie(authProviderCookie)
	if err != nil {
		return nil, false, nil
	}

	proxy, err := pm.createProxy(req.Context(), c.Value)
	if err != nil {
		return nil, false, err
	}

	return proxy.authenticateRequest(req)
}

func (pm *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var provider string

	c, err := r.Cookie(authProviderCookie)
	if err != nil {
		provider = r.URL.Query().Get(authProviderCookie)
		if provider == "" {
			http.Error(w, "missing auth provider", http.StatusBadRequest)
			return
		}

		// Set it as a cookie for the future.
		http.SetCookie(w, &http.Cookie{
			Name:  authProviderCookie,
			Value: provider,
			Path:  "/",
		})
	} else {
		provider = c.Value
		if provider == "" {
			http.Error(w, "missing auth provider", http.StatusBadRequest)
			return
		}
	}

	proxy, err := pm.createProxy(r.Context(), provider)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to create proxy: %v", err), http.StatusInternalServerError)
		return
	}

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

func newProxy(providerName, providerNamespace, providerURL string) (*Proxy, error) {
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
}

func (p *Proxy) authenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	sr := SerializableRequest{
		Method: req.Method,
		URL:    req.URL.String(),
		Header: make(map[string][]string),
	}
	for k, v := range req.Header {
		sr.Header[k] = v
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

	var ss SerializableState
	if err := json.NewDecoder(stateResponse.Body).Decode(&ss); err != nil {
		return nil, false, err
	}

	userName := ss.PreferredUsername
	if userName == "" {
		userName = ss.User
		if userName == "" {
			userName = ss.Email
		}
	}

	if req.URL.Path == "/api/me" {
		// Put the access token on the context so that the profile icon can be fetched.
		*req = *req.WithContext(accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken))
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:  ss.User,
			Name: userName,
			Extra: map[string][]string{
				"email":                   {ss.Email},
				"auth_provider_name":      {p.name},
				"auth_provider_namespace": {p.namespace},
			},
		},
	}, true, nil
}

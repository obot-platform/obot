package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
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

const AuthProviderCookie = "obot-auth-provider"

type Manager struct {
	dispatcher *dispatcher.Dispatcher
}

func NewProxyManager(dispatcher *dispatcher.Dispatcher) *Manager {
	return &Manager{
		dispatcher: dispatcher,
	}
}

func (pm *Manager) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	c, err := req.Cookie(AuthProviderCookie)
	if err != nil {
		return nil, false, nil
	}

	proxy, err := pm.createProxy(req.Context(), c.Value)
	if err != nil {
		return nil, false, err
	}

	return proxy.authenticateRequest(req)
}

func (pm *Manager) HandlerFunc(ctx api.Context) error {
	pm.ServeHTTP(ctx.ResponseWriter, ctx.Request)
	return nil
}

func (pm *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var provider string

	if provider = r.URL.Query().Get(AuthProviderCookie); provider != "" {
		// Set it as a cookie for the future.
		http.SetCookie(w, &http.Cookie{
			Name:  AuthProviderCookie,
			Value: provider,
			Path:  "/",
		})
	} else if c, err := r.Cookie(AuthProviderCookie); err == nil {
		provider = c.Value
	}

	// If no provider is set, just use the alphabetically first provider.
	if provider == "" {
		providers, err := pm.dispatcher.ListConfiguredAuthProviders(r.Context(), "default")
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to list configured auth providers: %v", err), http.StatusInternalServerError)
			return
		}
		if len(providers) == 0 {
			// There aren't any auth providers configured. Return an error, unless the user is signing out, in which case, just redirect.
			if r.URL.Path == "/oauth2/sign_out" {
				rdParam := r.URL.Query().Get("rd")
				if rdParam == "" {
					rdParam = "/"
				}

				http.Redirect(w, r, rdParam, http.StatusFound)
				return
			}

			http.Error(w, "no auth providers configured", http.StatusBadRequest)
			return
		}
		sort.Slice(providers, func(i, j int) bool {
			return providers[i] < providers[j]
		})
		provider = "default/" + providers[0]
	}

	log.Infof("forwarding request for %s to provider %s", r.URL.Path, provider)

	// If signing out, delete the auth provider cookie.
	if r.URL.Path == "/oauth2/sign_out" {
		http.SetCookie(w, &http.Cookie{
			Name:   AuthProviderCookie,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
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
		u.Extra["set-cookie"] = []string{ss.SetCookie}
	}

	if req.URL.Path == "/api/me" {
		// Put the access token on the context so that the profile icon can be fetched.
		*req = *req.WithContext(accesstoken.ContextWithAccessToken(req.Context(), ss.AccessToken))
	}

	return &authenticator.Response{
		User: u,
	}, true, nil
}

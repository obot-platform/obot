package proxy

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gptscript-ai/otto/pkg/mvl"
	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var log = mvl.Package()

type Config struct {
	AuthBaseURI        string   `usage:"Base URI for authentication" default:"http://localhost:8080"`
	AuthCookieSecret   string   `usage:"Secret used to encrypt cookie"`
	AuthEmailDomains   string   `usage:"Email domains allowed for authentication" default:"*"`
	AuthAdminEmails    []string `usage:"Emails admin users"`
	GoogleClientID     string   `usage:"Google client ID"`
	GoogleClientSecret string   `usage:"Google client secret"`
}

type Proxy struct {
	proxy          *oauth2proxy.OAuthProxy
	authProviderID string
}

func New(authProviderID uint, cfg Config) (*Proxy, error) {
	oauthProxyOpts, err := options.NewLegacyOptions().ToOptions()
	if err != nil {
		return nil, err
	}

	// Don't need to bind to a port
	oauthProxyOpts.Server.BindAddress = ""
	oauthProxyOpts.MetricsServer.BindAddress = ""
	oauthProxyOpts.Cookie.Refresh = time.Hour
	oauthProxyOpts.Cookie.Name = "otto_access_token"
	oauthProxyOpts.Cookie.Secret = cfg.AuthCookieSecret
	oauthProxyOpts.Cookie.Secure = strings.HasPrefix(cfg.AuthBaseURI, "https://")
	oauthProxyOpts.UpstreamServers = options.UpstreamConfig{
		Upstreams: []options.Upstream{
			{
				ID:            "default",
				URI:           "http://localhost:8080/",
				Path:          "(.*)",
				RewriteTarget: "$1",
			},
		},
	}

	oauthProxyOpts.RawRedirectURL = cfg.AuthBaseURI + "/oauth2/callback"
	oauthProxyOpts.Providers[0].ClientID = cfg.GoogleClientID
	oauthProxyOpts.Providers[0].ClientSecret = cfg.GoogleClientSecret
	oauthProxyOpts.ReverseProxy = true
	if cfg.AuthEmailDomains != "" {
		oauthProxyOpts.EmailDomains = strings.Split(cfg.AuthEmailDomains, ",")
	}

	if err = validation.Validate(oauthProxyOpts); err != nil {
		log.Fatalf("%s", err)
	}

	oauthProxy, err := oauth2proxy.NewOAuthProxy(oauthProxyOpts, oauth2proxy.NewValidator(oauthProxyOpts.EmailDomains, oauthProxyOpts.AuthenticatedEmailsFile))
	if err != nil {
		return nil, fmt.Errorf("failed to create oauth2 proxy: %w", err)
	}

	return &Proxy{
		proxy:          oauthProxy,
		authProviderID: fmt.Sprint(authProviderID),
	}, nil
}

func (p *Proxy) Wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasPrefix(r.URL.Path, "/ui/login/complete") {
			// No authentication required
			h.ServeHTTP(w, r)
			return
		}

		session, err := p.proxy.LoadCookiedSession(r)
		if err != nil || session == nil {
			p.proxy.ServeHTTP(w, r)
		} else {
			h.ServeHTTP(w, r)
		}
	})
}

func (p *Proxy) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	state, err := p.proxy.LoadCookiedSession(req)
	if err != nil || state == nil {
		return nil, false, err
	}

	userName := state.PreferredUsername
	if userName == "" {
		userName = state.User
		if userName == "" {
			userName = state.Email
		}
	}

	return &authenticator.Response{
		User: &user.DefaultInfo{
			UID:  state.User,
			Name: userName,
			Extra: map[string][]string{
				"email":            {state.Email},
				"auth_provider_id": {p.authProviderID},
			},
		},
	}, true, nil
}

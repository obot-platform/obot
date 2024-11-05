package proxy

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	oauth2proxy "github.com/oauth2-proxy/oauth2-proxy/v7"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/apis/options"
	"github.com/oauth2-proxy/oauth2-proxy/v7/pkg/validation"
	"github.com/otto8-ai/otto8/pkg/mvl"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

var log = mvl.Package()

type Config struct {
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

func New(serverURL string, authProviderID uint, cfg Config) (*Proxy, error) {
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
	oauthProxyOpts.Cookie.Secure = strings.HasPrefix(serverURL, "https://")
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

	oauthProxyOpts.RawRedirectURL = serverURL + "/oauth2/callback"
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

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.proxy.ServeHTTP(w, r)
}

func (p *Proxy) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	state, err := p.proxy.LoadCookiedSession(req)
	if err != nil || state == nil || state.IsExpired() {
		return nil, false, err
	}

	userName := state.PreferredUsername
	if userName == "" {
		userName = state.User
		if userName == "" {
			userName = state.Email
		}
	}

	if req.URL.Path == "/api/me" {
		// Put the access token on the context so that the profile icon can be fetched.
		*req = *req.WithContext(contextWithAccessToken(req.Context(), state.AccessToken))
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

type accessTokenKey struct{}

func contextWithAccessToken(ctx context.Context, accessToken string) context.Context {
	return context.WithValue(ctx, accessTokenKey{}, accessToken)
}

func GetAccessToken(ctx context.Context) string {
	accessToken, _ := ctx.Value(accessTokenKey{}).(string)
	return accessToken
}

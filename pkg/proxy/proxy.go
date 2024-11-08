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
	AuthCookieSecret string   `usage:"Secret used to encrypt cookie"`
	AuthEmailDomains string   `usage:"Email domains allowed for authentication" default:"*"`
	AuthAdminEmails  []string `usage:"Emails admin users"`
	AuthConfigType   string   `usage:"Type of OAuth configuration" default:"google"`
	AuthClientID     string   `usage:"Client ID for OAuth"`
	AuthClientSecret string   `usage:"Client secret for OAuth"`

	// Type-specific config
	GithubConfig
}

type GithubConfig struct {
	AuthGithubOrg        string   `usage:"Restrict logins to members of this organization"`
	AuthGithubTeams      []string `usage:"Restrict logins to members of any of these teams (slug)"`
	AuthGithubRepo       string   `usage:"Restrict logins to collaborators of this repository formatted as org/repo"`
	AuthGithubToken      string   `usage:"The token to use when verifying repository collaborators (must have push access to the repository)"`
	AuthGithubAllowUsers []string `usage:"Users allowed to login even if they don't belong to the organization or team(s)"`
}

type Proxy struct {
	proxy          *oauth2proxy.OAuthProxy
	authProviderID string
}

func New(serverURL string, authProviderID uint, cfg Config) (*Proxy, error) {
	legacyOpts := options.NewLegacyOptions()
	legacyOpts.LegacyProvider.ProviderType = cfg.AuthConfigType
	legacyOpts.LegacyProvider.ProviderName = cfg.AuthConfigType
	legacyOpts.LegacyProvider.ClientID = cfg.AuthClientID
	legacyOpts.LegacyProvider.ClientSecret = cfg.AuthClientSecret
	legacyOpts.LegacyProvider.GitHubTeam = strings.Join(cfg.AuthGithubTeams, ",")
	legacyOpts.LegacyProvider.GitHubOrg = cfg.AuthGithubOrg
	legacyOpts.LegacyProvider.GitHubRepo = cfg.AuthGithubRepo
	legacyOpts.LegacyProvider.GitHubToken = cfg.AuthGithubToken
	legacyOpts.LegacyProvider.GitHubUsers = cfg.AuthGithubAllowUsers

	oauthProxyOpts, err := legacyOpts.ToOptions()
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

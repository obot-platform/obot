package types

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	ktime "github.com/otto8-ai/otto8/pkg/gateway/time"
	"gorm.io/gorm"
)

const (
	GitHubOAuthURL = "https://github.com/login/oauth/authorize"
	GitHubTokenURL = "https://github.com/login/oauth/access_token"

	GoogleOAuthURL = "https://accounts.google.com/o/oauth2/auth"
	GoogleJWKSURL  = "https://www.googleapis.com/oauth2/v3/certs"

	AzureOauthURL = "https://login.microsoftonline.com/{tenantID}/oauth2/v2.0/authorize"
	AzureJWKSURL  = "https://login.microsoftonline.com/{tenantID}/discovery/v2.0/keys"

	AuthTypeGitHub      = "github"
	AuthTypeAzureAD     = "azuread"
	AuthTypeGoogle      = "google"
	AuthTypeGenericOIDC = "genericOIDC"
)

var tokenURLByType = map[string]string{
	AuthTypeGitHub: GitHubTokenURL,
	AuthTypeGoogle: GoogleTokenURL,
}

var oauthURLByType = map[string]string{
	AuthTypeGitHub:  GitHubOAuthURL,
	AuthTypeGoogle:  GoogleOAuthURL,
	AuthTypeAzureAD: AzureOauthURL,
}

var jwksURLByType = map[string]string{
	AuthTypeAzureAD: AzureJWKSURL,
	AuthTypeGoogle:  GoogleJWKSURL,
}

var defaultScopesByType = map[string]string{
	AuthTypeGitHub:  "user:email",
	AuthTypeAzureAD: "openid+profile+email",
	AuthTypeGoogle:  "openid profile email",
}

var defaultUsernameClaimByType = map[string]string{
	AuthTypeAzureAD: "preferred_username",
	AuthTypeGoogle:  "name",
}

var defaultEmailClaimByType = map[string]string{
	AuthTypeAzureAD: "email",
	AuthTypeGoogle:  "email",
}

func OAuthURLByType(t string) string {
	return oauthURLByType[t]
}

func JWKSURLByType(t string) string {
	return jwksURLByType[t]
}

func TokenURLByType(t string) string {
	return tokenURLByType[t]
}

func ScopesByType(t string) string {
	return defaultScopesByType[t]
}

func UsernameClaimByType(t string) string {
	return defaultUsernameClaimByType[t]
}

func EmailClaimByType(t string) string {
	return defaultEmailClaimByType[t]
}

type AuthTypeConfig struct {
	DisplayName string            `json:"displayName"`
	Required    map[string]string `json:"required"`
	Advanced    map[string]string `json:"advanced"`
}

type AuthProvider struct {
	// These fields are set for every auth provider
	gorm.Model    `json:",inline"`
	Type          string        `json:"type"`
	ServiceName   string        `json:"serviceName"`
	Slug          string        `json:"slug" gorm:"unique"`
	ClientID      string        `json:"clientID"`
	ClientSecret  string        `json:"clientSecret"`
	OAuthURL      string        `json:"oauthURL"`
	Scopes        string        `json:"scopes,omitempty"`
	Expiration    string        `json:"expiration,omitempty"`
	ExpirationDur time.Duration `json:"-"`
	Disabled      bool          `json:"disabled"`
	// Not needed for OIDC type flows
	TokenURL string `json:"tokenURL"`
	// These fields are only set for AzureAD
	TenantID string `json:"tenantID,omitempty"`
	// These fields are only set for OIDC providers, including AzureAD
	JWKSURL       string `json:"jwksURL,omitempty"`
	UsernameClaim string `json:"usernameClaim,omitempty"`
	EmailClaim    string `json:"emailClaim,omitempty"`
}

func (ap *AuthProvider) ValidateAndSetDefaults() error {
	var (
		errs []error
		err  error
	)
	ap.Type = strings.ToLower(ap.Type)
	if ap.Type == "" {
		errs = append(errs, fmt.Errorf("auth provider type is required"))
	}
	if ap.ServiceName == "" {
		errs = append(errs, fmt.Errorf("auth provider service name is required"))
	}
	if ap.ClientID == "" {
		errs = append(errs, fmt.Errorf("auth provider client id is required"))
	}
	if ap.ClientSecret == "" {
		errs = append(errs, fmt.Errorf("auth provider client secret is required"))
	}
	if ap.Type == AuthTypeAzureAD && ap.TenantID == "" {
		ap.TenantID = "common"
	}
	if ap.OAuthURL == "" {
		ap.OAuthURL = oauthURLByType[strings.ToLower(ap.Type)]
		if ap.OAuthURL == "" {
			errs = append(errs, fmt.Errorf("cannot determine OAuth URL for type: %s", ap.Type))
		} else if ap.Type == AuthTypeAzureAD {
			ap.OAuthURL = strings.ReplaceAll(ap.OAuthURL, "{tenantID}", ap.TenantID)
		}
	}
	if ap.TokenURL == "" {
		ap.TokenURL = tokenURLByType[strings.ToLower(ap.Type)]
		if ap.Type == AuthTypeGitHub && ap.TokenURL == "" {
			errs = append(errs, fmt.Errorf("cannot determine token URL for type: %s", ap.Type))
		}
	}

	if ap.JWKSURL == "" {
		ap.JWKSURL = jwksURLByType[strings.ToLower(ap.Type)]
		if ap.Type == AuthTypeAzureAD {
			ap.JWKSURL = strings.ReplaceAll(ap.JWKSURL, "{tenantID}", ap.TenantID)
		}
		if (ap.Type == AuthTypeGenericOIDC || ap.Type == AuthTypeAzureAD || ap.Type == AuthTypeGoogle) && ap.JWKSURL == "" {
			errs = append(errs, fmt.Errorf("cannot determine JWKS URL for type: %s", ap.Type))
		}
	}

	if ap.Slug == "" {
		ap.Slug = url.PathEscape(strings.ReplaceAll(strings.ToLower(ap.ServiceName), " ", "-"))
	} else {
		ap.Slug = url.PathEscape(ap.Slug)
	}

	if ap.Expiration == "" {
		ap.Expiration = "1d"
	}

	if ap.ExpirationDur, err = ktime.ParseDuration(ap.Expiration); err != nil {
		errs = append(errs, fmt.Errorf("invalid expiration: %w", err))
	}

	if ap.Scopes == "" {
		ap.Scopes = defaultScopesByType[ap.Type]
	}

	if ap.UsernameClaim == "" {
		ap.UsernameClaim = defaultUsernameClaimByType[ap.Type]
	}
	if ap.EmailClaim == "" {
		ap.EmailClaim = defaultEmailClaimByType[ap.Type]
	}

	if ap.Type == AuthTypeGenericOIDC && ap.UsernameClaim == "" {
		errs = append(errs, fmt.Errorf("username claim is required for type: %s", ap.Type))
	}
	if ap.Type == AuthTypeGenericOIDC && ap.EmailClaim == "" {
		errs = append(errs, fmt.Errorf("email claim is required for type: %s", ap.Type))
	}

	return errors.Join(errs...)
}

func (ap *AuthProvider) RedirectURL(baseURL string) string {
	return fmt.Sprintf("%s/oauth/redirect/%s", baseURL, ap.Slug)
}

func (ap *AuthProvider) AuthURL(baseURL string, state, nonce string) string {
	switch ap.Type {
	case AuthTypeGitHub:
		return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
			ap.OAuthURL,
			ap.ClientID,
			ap.RedirectURL(baseURL),
			ap.Scopes,
			state,
		)
	case AuthTypeAzureAD, AuthTypeGoogle, AuthTypeGenericOIDC:
		return fmt.Sprintf("%s?client_id=%s&redirect_uri=%s&scope=%s&state=%s&nonce=%s&response_type=id_token&response_mode=form_post",
			ap.OAuthURL,
			ap.ClientID,
			ap.RedirectURL(baseURL),
			ap.Scopes,
			state,
			nonce,
		)
	default:
		return ""
	}
}

type LLMProvider struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug" gorm:"unique;index"`
	BaseURL   string    `json:"baseURL"`
	Token     string    `json:"token"`
	Disabled  bool      `json:"disabled"`
}

func (lp *LLMProvider) Validate() error {
	var errs []error
	if lp.Name == "" {
		errs = append(errs, fmt.Errorf("provider name is required"))
	}
	if lp.BaseURL == "" {
		errs = append(errs, fmt.Errorf("provider base URL is required"))
	}
	if lp.Token == "" {
		errs = append(errs, fmt.Errorf("provider token is required"))
	}

	if lp.Slug == "" {
		lp.Slug = url.PathEscape(strings.ReplaceAll(strings.ToLower(lp.Name), " ", "-"))
	} else {
		lp.Slug = url.PathEscape(lp.Slug)
	}

	return errors.Join(errs...)
}

func (lp *LLMProvider) RequestBaseURL(serverBase string) string {
	return fmt.Sprintf("%s/llm/%s", serverBase, lp.Slug)
}

func (lp *LLMProvider) URL() string {
	return fmt.Sprintf("%s/llm/%s", lp.BaseURL, lp.Slug)
}

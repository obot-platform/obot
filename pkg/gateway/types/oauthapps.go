package types

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
)

const (
	AtlassianAuthorizeURL = "https://auth.atlassian.com/authorize"
	AtlassianTokenURL     = "https://auth.atlassian.com/oauth/token"

	SlackAuthorizeURL = "https://slack.com/oauth/v2/authorize"
	SlackTokenURL     = "https://slack.com/api/oauth.v2.access"

	NotionAuthorizeURL = "https://api.notion.com/v1/oauth/authorize"
	NotionTokenURL     = "https://api.notion.com/v1/oauth/token"

	HubSpotAuthorizeURL = "https://app.hubspot.com/oauth/authorize"
	HubSpotTokenURL     = "https://api.hubapi.com/oauth/v1/token"

	GoogleAuthorizeURL = "https://accounts.google.com/o/oauth2/v2/auth"
	GoogleTokenURL     = "https://oauth2.googleapis.com/token"

	GitHubAuthorizeURL = "https://github.com/login/oauth/authorize"
	GitHubTokenURL     = "https://github.com/login/oauth/access_token"

	ZoomAuthorizeURL = "https://zoom.us/oauth/authorize"
	ZoomTokenURL     = "https://zoom.us/oauth/token"

	LinkedInAuthorizeURL = "https://www.linkedin.com/oauth/v2/authorization"
	LinkedInTokenURL     = "https://www.linkedin.com/oauth/v2/accessToken"

	PagerDutyAuthorizeURL = "https://identity.pagerduty.com/oauth/authorize"
	PagerDutyTokenURL     = "https://identity.pagerduty.com/oauth/token"

	SmartThingsAuthorizeURL = "https://api.smartthings.com/oauth/authorize"
	SmartThingsTokenURL     = "https://auth-global.api.smartthings.com/oauth/token"

	GitLabAuthorizeURL = "https://gitlab.com/oauth/authorize"
	GitLabTokenURL     = "https://gitlab.com/oauth/token"
)

var (
	alphaNumericRegexp = regexp.MustCompile(`^[a-zA-Z0-9\-]*$`)
)

type OAuthAppTypeConfig struct {
	DisplayName string            `json:"displayName"`
	Parameters  map[string]string `json:"parameters"`
}

func ValidateAndSetDefaultsOAuthAppManifest(r *types.OAuthAppManifest, create bool) error {
	var errs []error
	if r.Alias == "" {
		errs = append(errs, fmt.Errorf("missing alias"))
	} else if !alphaNumericRegexp.MatchString(r.Alias) {
		errs = append(errs, fmt.Errorf("alias name can only contain alphanumeric characters and hyphens: %s", r.Alias))
	}

	var defaultAuthURL, defaultTokenURL string

	switch r.Type {
	case types.OAuthAppTypeAtlassian:
		defaultAuthURL = AtlassianAuthorizeURL
		defaultTokenURL = AtlassianTokenURL
	case types.OAuthAppTypeMicrosoft365:
		tenantID := "common"
		if r.TenantID != nil && *r.TenantID != "" {
			tenantID = *r.TenantID
		}
		defaultAuthURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", tenantID)
		defaultTokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	case types.OAuthAppTypeSlack:
		defaultAuthURL = SlackAuthorizeURL
		defaultTokenURL = SlackTokenURL
	case types.OAuthAppTypeNotion:
		defaultAuthURL = NotionAuthorizeURL
		defaultTokenURL = NotionTokenURL
	case types.OAuthAppTypeHubSpot:
		defaultAuthURL = HubSpotAuthorizeURL
		defaultTokenURL = HubSpotTokenURL
	case types.OAuthAppTypeGoogle:
		defaultAuthURL = GoogleAuthorizeURL
		defaultTokenURL = GoogleTokenURL
	case types.OAuthAppTypeGitHub:
		defaultAuthURL = GitHubAuthorizeURL
		defaultTokenURL = GitHubTokenURL
	case types.OAuthAppTypeZoom:
		defaultAuthURL = ZoomAuthorizeURL
		defaultTokenURL = ZoomTokenURL
	case types.OAuthAppTypeLinkedIn:
		defaultAuthURL = LinkedInAuthorizeURL
		defaultTokenURL = LinkedInTokenURL
	case types.OAuthAppTypePagerDuty:
		defaultAuthURL = PagerDutyAuthorizeURL
		defaultTokenURL = PagerDutyTokenURL
	case types.OAuthAppTypeSmartThings:
		defaultAuthURL = SmartThingsAuthorizeURL
		defaultTokenURL = SmartThingsTokenURL
	case types.OAuthAppTypeSalesforce:
		salesforceAuthorizeFragment := "/services/oauth2/authorize"
		salesforceTokenFragment := "/services/oauth2/token"

		if r.InstanceURL == "" {
			errs = append(errs, fmt.Errorf("missing instanceURL"))
		} else {
			instanceURL, err := url.Parse(r.InstanceURL)
			if err != nil {
				errs = append(errs, err)
			}
			if instanceURL.Scheme == "" {
				instanceURL.Scheme = "https"
			}

			defaultAuthURL, err = url.JoinPath(instanceURL.String(), salesforceAuthorizeFragment)
			if err != nil {
				errs = append(errs, err)
			}
			defaultTokenURL, err = url.JoinPath(instanceURL.String(), salesforceTokenFragment)
			if err != nil {
				errs = append(errs, err)
			}
		}
	case types.OAuthAppTypeGitLab:
		gitlabAuthorizeFragment := "/oauth/authorize"
		gitlabTokenFragment := "/oauth/token"

		// Default to public GitLab if no base URL is provided
		baseURL := "https://gitlab.com"
		if r.GitLabBaseURL != "" {
			u, err := url.Parse(r.GitLabBaseURL)
			if err != nil {
				errs = append(errs, err)
				break
			}
			if u.Scheme == "" {
				u.Scheme = "https"
			}
			baseURL = u.String()
		}

		var err error
		if r.AuthURL, err = url.JoinPath(baseURL, gitlabAuthorizeFragment); err != nil {
			errs = append(errs, err)
		}
		if r.TokenURL, err = url.JoinPath(baseURL, gitlabTokenFragment); err != nil {
			errs = append(errs, err)
		}
	}

	if r.AuthURL == "" {
		r.AuthURL = defaultAuthURL
	}
	if r.TokenURL == "" {
		r.TokenURL = defaultTokenURL
	}

	if r.AuthURL == "" {
		errs = append(errs, fmt.Errorf("missing authURL"))
	}
	if r.TokenURL == "" {
		errs = append(errs, fmt.Errorf("missing tokenURL"))
	}

	if r.AuthURL != "" && r.TokenURL != "" {
		// Validate URLs.
		// If the URLs are empty, then we don't need to validate them and an error would already be returned.
		if _, err := url.Parse(r.AuthURL); err != nil {
			errs = append(errs, fmt.Errorf("invalid authURL: %w", err))
		} else if _, err := url.Parse(r.TokenURL); err != nil {
			errs = append(errs, fmt.Errorf("invalid tokenURL: %w", err))
		}
	}

	// Users are allowed to create OAuth Apps without specifying the fields that they would get from the provider.
	// Things like client ID, client secret, app ID, tenant ID, etc.
	// They will then have to update the Oauth App to add these fields.
	if !create {
		if r.ClientID == "" {
			errs = append(errs, fmt.Errorf("missing clientID"))
		}
		if r.ClientSecret == "" {
			errs = append(errs, fmt.Errorf("missing clientSecret"))
		}
		if r.Type == types.OAuthAppTypeHubSpot && r.AppID == "" {
			errs = append(errs, fmt.Errorf("missing appID"))
		}
	}

	return errors.Join(errs...)
}

func MergeOAuthAppManifests(r, other types.OAuthAppManifest) types.OAuthAppManifest {
	retVal := r

	if other.ClientID != "" {
		retVal.ClientID = other.ClientID
	}
	if other.ClientSecret != "" {
		retVal.ClientSecret = other.ClientSecret
	}
	if other.AuthURL != "" && other.GitLabBaseURL == "" {
		// Only keep custom auth URL if GitLabBaseURL isn't changing
		retVal.AuthURL = other.AuthURL
	}
	if other.TokenURL != "" && other.GitLabBaseURL == "" {
		// Only keep custom token URL if GitLabBaseURL isn't changing
		retVal.TokenURL = other.TokenURL
	}
	if other.Type != "" {
		retVal.Type = other.Type
	}
	if other.TenantID != nil {
		retVal.TenantID = other.TenantID
	}
	if other.Name != "" {
		retVal.Name = other.Name
	}
	if other.Alias != "" {
		retVal.Alias = other.Alias
	}
	if other.AppID != "" {
		retVal.AppID = other.AppID
	}
	if other.OptionalScope != "" {
		retVal.OptionalScope = other.OptionalScope
	}
	if other.Global != nil {
		retVal.Global = other.Global
	}
	if other.InstanceURL != "" {
		retVal.InstanceURL = other.InstanceURL
	}
	if other.GitLabBaseURL != "" {
		retVal.GitLabBaseURL = other.GitLabBaseURL
		//Will reset during validation
		retVal.AuthURL = ""
		retVal.TokenURL = ""
	}

	return retVal
}

// OAuthTokenResponse represents a response from the /token endpoint on an OAuth server.
// These do not get stored in the database.
type OAuthTokenResponse struct {
	State        string `json:"state"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	ExtExpiresIn int    `json:"ext_expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Ok           bool   `json:"ok"`
	Error        string `json:"error"`
	CreatedAt    time.Time
	Extras       map[string]string `json:"extras" gorm:"serializer:json"`
	Data         map[string]string `json:"data" gorm:"serializer:json"`
}

type GoogleOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type SalesforceOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	Signature    string `json:"signature"`
	Scope        string `json:"scope"`
	IDToken      string `json:"id_token"`
	InstanceURL  string `json:"instance_url"`
	ID           string `json:"id"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	IssuedAt     string `json:"issued_at"`
}

type SlackOAuthTokenResponse struct {
	Ok    bool   `json:"ok"`
	Error string `json:"error"`
	AppID string `json:"app_id"`
	Team  struct {
		Name string `json:"name"`
		ID   string `json:"id"`
	} `json:"team"`
	AuthedUser struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		AccessToken string `json:"access_token"`
	} `json:"authed_user"`
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
}

type OAuthTokenRequestChallenge struct {
	State     string    `json:"state" gorm:"primaryKey"`
	Challenge string    `json:"challenge"`
	CreatedAt time.Time `json:"createdAt"`
}

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

	switch r.Type {
	case types.OAuthAppTypeAtlassian:
		r.AuthURL = AtlassianAuthorizeURL
		r.TokenURL = AtlassianTokenURL
	case types.OAuthAppTypeMicrosoft365:
		r.AuthURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/authorize", r.TenantID)
		r.TokenURL = fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", r.TenantID)
	case types.OAuthAppTypeSlack:
		r.AuthURL = SlackAuthorizeURL
		r.TokenURL = SlackTokenURL
	case types.OAuthAppTypeNotion:
		r.AuthURL = NotionAuthorizeURL
		r.TokenURL = NotionTokenURL
	case types.OAuthAppTypeHubSpot:
		r.AuthURL = HubSpotAuthorizeURL
		r.TokenURL = HubSpotTokenURL
	case types.OAuthAppTypeGoogle:
		r.AuthURL = GoogleAuthorizeURL
		r.TokenURL = GoogleTokenURL
	case types.OAuthAppTypeGitHub:
		r.AuthURL = GitHubAuthorizeURL
		r.TokenURL = GitHubTokenURL
	case types.OAuthAppTypeZoom:
		r.AuthURL = ZoomAuthorizeURL
		r.TokenURL = ZoomTokenURL
	case types.OAuthAppTypeLinkedIn:
		r.AuthURL = LinkedInAuthorizeURL
		r.TokenURL = LinkedInTokenURL
	case types.OAuthAppTypeSalesforce:
		salesforceAuthorizeFragment := "/services/oauth2/authorize"
		salesforceTokenFragment := "/services/oauth2/token"
		instanceURL, err := url.Parse(r.InstanceURL)
		if err != nil {
			errs = append(errs, err)
		}
		if instanceURL.Scheme != "" {
			instanceURL.Scheme = "https"
		}

		r.AuthURL, err = url.JoinPath(instanceURL.String(), salesforceAuthorizeFragment)
		if err != nil {
			errs = append(errs, err)
		}
		r.TokenURL, err = url.JoinPath(instanceURL.String(), salesforceTokenFragment)
		if err != nil {
			errs = append(errs, err)
		}
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
		} else if r.Type == types.OAuthAppTypeMicrosoft365 && r.TenantID == "" {
			errs = append(errs, fmt.Errorf("missing tenantID"))
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
	if other.AuthURL != "" {
		retVal.AuthURL = other.AuthURL
	}
	if other.TokenURL != "" {
		retVal.TokenURL = other.TokenURL
	}
	if other.Type != "" {
		retVal.Type = other.Type
	}
	if other.TenantID != "" {
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
	if other.Global {
		retVal.Global = other.Global
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
	Ok         bool   `json:"ok"`
	Error      string `json:"error"`
	AuthedUser struct {
		ID          string `json:"id"`
		Scope       string `json:"scope"`
		AccessToken string `json:"access_token"`
	} `json:"authed_user"`
}

type OAuthTokenRequestChallenge struct {
	State     string    `json:"state" gorm:"primaryKey"`
	Challenge string    `json:"challenge"`
	CreatedAt time.Time `json:"createdAt"`
}

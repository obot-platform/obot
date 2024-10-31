package types

const (
	OAuthAppTypeMicrosoft365 OAuthAppType = "microsoft365"
	OAuthAppTypeSlack        OAuthAppType = "slack"
	OAuthAppTypeNotion       OAuthAppType = "notion"
	OAuthAppTypeHubSpot      OAuthAppType = "hubspot"
	OAuthAppTypeGitHub       OAuthAppType = "github"
	OAuthAppTypeGoogle       OAuthAppType = "google"
	OAuthAppTypeCustom       OAuthAppType = "custom"
)

type OAuthAppType string

type OAuthApp struct {
	OAuthAppManifest
}

type OAuthAppManifest struct {
	Metadata
	Type         OAuthAppType `json:"type"`
	Name         string       `json:"name,omitempty"`
	ClientID     string       `json:"clientID"`
	ClientSecret string       `json:"clientSecret,omitempty"`
	// These fields are only needed for custom OAuth apps.
	AuthURL  string `json:"authURL,omitempty"`
	TokenURL string `json:"tokenURL,omitempty"`
	// This field is only needed for Microsoft 365 OAuth apps.
	TenantID string `json:"tenantID,omitempty"`
	// This field is only needed for HubSpot OAuth apps.
	AppID string `json:"appID,omitempty"`
	// This field is optional for HubSpot OAuth apps.
	OptionalScope string `json:"optionalScope,omitempty"`
	// This field is required, it correlates to the integration name in the gptscript oauth cred tool
	Integration string `json:"integration,omitempty"`
	// Global indicates if the OAuth app is globally applied to all agents.
	Global *bool `json:"global,omitempty"`
}

type OAuthAppList List[OAuthApp]

type OAuthAppLoginAuthStatus struct {
	URL           string `json:"url,omitempty"`
	Authenticated bool   `json:"authenticated,omitempty"`
	Required      *bool  `json:"required,omitempty"`
	Error         string `json:"error,omitempty"`
}

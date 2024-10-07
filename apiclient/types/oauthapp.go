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
	OAuthAppExternalStatus
}

type OAuthAppManifest struct {
	Metadata
	Type         OAuthAppType `json:"type"`
	RefName      string       `json:"refName"`
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
}

type OAuthAppExternalStatus struct {
	RefNameAssigned bool `json:"refNameAssigned,omitempty"`
}

type OAuthAppList List[OAuthApp]

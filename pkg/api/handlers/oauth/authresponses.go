package oauth

import "time"

// tokenResponse represents a response from the /token endpoint on an OAuth server.
// These do not get stored in the database.
type tokenResponse struct {
	State        string `json:"state"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int64  `json:"expires_in"`
	ExtExpiresIn int64  `json:"ext_expires_in"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Ok           bool   `json:"ok"`
	Error        string `json:"error"`
	CreatedAt    time.Time
	Extras       map[string]string `json:"extras" gorm:"serializer:json"`
	Data         map[string]string `json:"data" gorm:"serializer:json"`
}

type googleOAuthTokenResponse struct {
	AccessToken  string `json:"access_token"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	TokenType    string `json:"token_type"`
}

type salesforceOAuthTokenResponse struct {
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

type slackOAuthTokenResponse struct {
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

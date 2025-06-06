package types

type MCPServerConfig struct {
	AccessTokens map[string]AccessToken `json:"accessTokens,omitempty"`
	EnvVars      map[string]string      `json:"envVars,omitempty"`
}

type AccessToken struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpiresAt    *Time  `json:"expiresAt"`
}

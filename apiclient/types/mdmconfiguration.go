package types

import "encoding/json"

// MDMConfiguration is a fleet grouping that devices enroll into. AssetDigest,
// Platform, OS, and Values are the saved configuration. Instructions and Error
// are derived by the server from the pinned asset and are ignored on writes.
type MDMConfiguration struct {
	ID           uint            `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	CreatedAt    Time            `json:"createdAt"`
	AssetDigest  string          `json:"assetDigest,omitempty"`
	Platform     string          `json:"platform,omitempty"`
	OS           string          `json:"os,omitempty"`
	Values       json.RawMessage `json:"values,omitempty"`
	Instructions string          `json:"instructions,omitempty"`
	Error        string          `json:"error,omitempty"`
}

type MDMConfigurationList List[MDMConfiguration]

type MDMConfigurationCreateResponse struct {
	MDMConfiguration
	EnrollmentCredential string `json:"enrollmentCredential"`
}

type MDMEnrollmentKey struct {
	ID         uint   `json:"id"`
	Name       string `json:"name,omitempty"`
	CreatedAt  Time   `json:"createdAt"`
	LastUsedAt *Time  `json:"lastUsedAt,omitempty"`
	ExpiresAt  *Time  `json:"expiresAt,omitempty"`
}

type MDMEnrollmentKeyList List[MDMEnrollmentKey]

type MDMEnrollmentKeyCreateRequest struct {
	Name      string `json:"name,omitempty"`
	ExpiresAt *Time  `json:"expiresAt,omitempty"`
}

type MDMEnrollmentKeyCreateResponse struct {
	MDMEnrollmentKey
	EnrollmentCredential string `json:"enrollmentCredential"`
}

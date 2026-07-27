package types

import "encoding/json"

type MDMConfigurationManifest struct {
	AssetDigest string          `json:"assetDigest,omitempty"`
	Values      json.RawMessage `json:"values,omitempty"`
}

// MDMConfiguration is a fleet grouping that devices enroll into. AssetDigest
// identifies the asset bundle whose fields Values conform to. Artifacts are
// rendered for every platform and OS in that bundle when Values are saved.
type MDMConfiguration struct {
	MDMConfigurationManifest `json:",inline"`

	ID        uint `json:"id"`
	IsDefault bool `json:"isDefault"`
	CreatedAt Time `json:"createdAt"`

	// ObotSentryVersion is copied from the source bundle's manifest when the
	// artifacts are rendered. It is server-owned and reports the version the
	// saved packages were generated with.
	ObotSentryVersion string `json:"obotSentryVersion,omitempty"`

	Artifacts []MDMConfigurationArtifact `json:"artifacts"`
}

type MDMConfigurationList List[MDMConfiguration]

// MDMConfigurationArtifact is one rendered deployment option. Slug selects its
// download endpoint; ZIP content, content digest, and filename remain private.
type MDMConfigurationArtifact struct {
	Slug         string `json:"slug"`
	Platform     string `json:"platform"`
	OS           string `json:"os"`
	Instructions string `json:"instructions"`
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

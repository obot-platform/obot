//nolint:revive
package types

import (
	"fmt"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

// MDMConfigurationPrincipalPrefix namespaces the principal identity (Name/UID)
// of a MDM configuration so it never collides with user UIDs or device
// principals. An enrollment credential authenticates as its configuration.
const MDMConfigurationPrincipalPrefix = "mdm-configuration"

// MDMConfigurationPrincipalName returns the stable principal identity for a
// configuration, e.g. "mdm-configuration:12".
func MDMConfigurationPrincipalName(id uint) string {
	return fmt.Sprintf("%s:%d", MDMConfigurationPrincipalPrefix, id)
}

// MDMConfiguration is a fleet grouping that devices enroll into. Enrollment is
// authorized by one or more DeviceEnrollmentKeys attached to it; a device
// belongs to the configuration itself, not to any particular key.
type MDMConfiguration struct {
	ID uint `json:"id" gorm:"primaryKey;autoIncrement"`

	// There may be no default before device management is configured, but there
	// can never be more than one. The backend assigns the first configuration as
	// the default; clients cannot change this field.
	IsDefault bool `json:"-" gorm:"not null;default:false;uniqueIndex:idx_mdm_configurations_default,where:is_default = true"`

	CreatedBy uint      `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`

	// Name is a vestigial column retained so the pre-existing NOT NULL "name"
	// column keeps accepting inserts on databases created before the
	// configuration rework. It is not part of the public API and is stored
	// empty. The nullable "description" column from that same schema is left in
	// place; it is harmless and never read.
	Name string `json:"-" gorm:"not null"`

	// AssetDigest identifies the bundle against which Values were validated.
	// ObotSentryVersion is copied from that bundle's manifest when artifacts are
	// rendered, so the generated version is known without reopening the bundle.
	// Artifacts are loaded separately from their explicit table. ZIP bytes never
	// pass through the public configuration API.
	AssetDigest       string                     `json:"-" gorm:"size:64;index"`
	ObotSentryVersion string                     `json:"-" gorm:"size:64"`
	Values            string                     `json:"-" gorm:"type:text"`
	Artifacts         []MDMConfigurationArtifact `json:"-" gorm:"-"`

	EnforcementEnabled   bool                        `json:"enforcementEnabled,omitempty"`
	EnforcementAllowlist types2.EnforcementAllowlist `json:"-" gorm:"serializer:json"`
}

// MDMConfigurationArtifact stores one rendered download. The configuration
// relationship is maintained explicitly by the gateway client without a
// database foreign-key constraint.
type MDMConfigurationArtifact struct {
	ID                 uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	MDMConfigurationID uint   `json:"mdmConfigurationID" gorm:"not null;index;uniqueIndex:idx_mdm_configuration_artifact_slug,priority:1"`
	Slug               string `json:"slug" gorm:"not null;uniqueIndex:idx_mdm_configuration_artifact_slug,priority:2"`
	Platform           string `json:"platform"`
	OS                 string `json:"os"`
	Instructions       string `json:"instructions"`
	Digest             string `json:"digest" gorm:"size:64;not null"`
	Content            []byte `json:"-" gorm:"not null"`
}

// MDMAssetBundle is one immutable validated source snapshot, addressed by the
// lowercase SHA-256 of Content. Rendered configuration artifacts are stored in
// their own table rather than sharing a generic blob abstraction.
type MDMAssetBundle struct {
	Digest  string `json:"digest" gorm:"primaryKey;size:64"`
	Content []byte `json:"-" gorm:"not null"`
}

// DeviceEnrollmentKey is one credential that authorizes enrolling a device into
// its configuration. A configuration can have several at once, added and removed
// independently. Deleting a key only stops it from enrolling new devices — it
// never affects already-enrolled devices (they authenticate with their own
// keys). Rotation is therefore: add a new key, distribute it, delete the old.
//
// The credential format is: ode1-<configuration_id>-<key_id>-<secret>
// Lookup is by key ID (scoped to the configuration), then bcrypt verifies the secret.
type DeviceEnrollmentKey struct {
	ID                 uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	MDMConfigurationID uint       `json:"mdmConfigurationID" gorm:"index;not null"`
	Name               string     `json:"name,omitempty"` // optional, admin-provided
	HashedSecret       string     `json:"-"`              // bcrypt hash of the secret portion only
	CreatedBy          uint       `json:"createdBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	LastUsedAt         *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt          *time.Time `json:"expiresAt,omitempty"` // nil means no expiration
}

// DeviceEnrollmentKeyCreateResponse is returned when minting a key. The full
// enrollment credential is only visible here.
type DeviceEnrollmentKeyCreateResponse struct {
	DeviceEnrollmentKey
	EnrollmentCredential string `json:"enrollmentCredential"` // ode1-..., shown once
}

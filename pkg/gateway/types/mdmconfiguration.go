//nolint:revive
package types

import (
	"fmt"
	"time"
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
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description,omitempty"`
	CreatedBy   uint      `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`

	// The optional MDM asset selection and JSON-encoded template values. The
	// digest, platform, and OS are either all set or all blank.
	AssetDigest string `json:"-" gorm:"size:64;index"`
	Platform    string `json:"platform,omitempty"`
	OS          string `json:"os,omitempty"`
	Values      string `json:"-"`
}

// MDMAssetBundle is one immutable, validated MDM asset snapshot. Digest is the
// lowercase SHA-256 of Content and is also the stable identity configurations
// pin. Content is a canonical ZIP consumed directly by mdmassets.OpenArchive.
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

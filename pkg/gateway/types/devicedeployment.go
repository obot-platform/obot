//nolint:revive
package types

import (
	"fmt"
	"time"
)

// DeviceDeploymentPrincipalPrefix namespaces the principal identity (Name/UID)
// of a device deployment so it never collides with user UIDs or device
// principals. An enrollment credential authenticates as its deployment.
const DeviceDeploymentPrincipalPrefix = "device-deployment"

// DeviceDeploymentPrincipalName returns the stable principal identity for a
// deployment, e.g. "device-deployment:12".
func DeviceDeploymentPrincipalName(id uint) string {
	return fmt.Sprintf("%s:%d", DeviceDeploymentPrincipalPrefix, id)
}

// DeviceDeployment is a fleet grouping that devices enroll into. Enrollment is
// authorized by one or more DeviceEnrollmentKeys attached to it; a device
// belongs to the deployment itself, not to any particular key.
type DeviceDeployment struct {
	ID          uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name        string    `json:"name" gorm:"not null"`
	Description string    `json:"description,omitempty"`
	CreatedBy   uint      `json:"createdBy"`
	CreatedAt   time.Time `json:"createdAt"`
}

// DeviceEnrollmentKey is one credential that authorizes enrolling a device into
// its deployment. A deployment can have several at once, added and removed
// independently. Deleting a key only stops it from enrolling new devices — it
// never affects already-enrolled devices (they authenticate with their own
// keys). Rotation is therefore: add a new key, distribute it, delete the old.
//
// The credential format is: ode1-<deployment_id>-<key_id>-<secret>
// Lookup is by key ID (scoped to the deployment), then bcrypt verifies the secret.
type DeviceEnrollmentKey struct {
	ID                 uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	DeviceDeploymentID uint       `json:"deviceDeploymentID" gorm:"index;not null"`
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

//nolint:revive
package types

import (
	"fmt"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

// DevicePrincipalPrefix namespaces the principal identity (Name/UID) of an
// enrolled device so it never collides with user UIDs or configuration principals.
const DevicePrincipalPrefix = "device"

// DevicePrincipalName returns the stable principal identity for an enrolled
// device, e.g. "device:abc-123". This is what a device authenticates as when
// submitting a scan; the scan itself is identified by DeviceScan.DeviceID
// (device submissions leave SubmittedBy empty).
func DevicePrincipalName(deviceID string) string {
	return fmt.Sprintf("%s:%s", DevicePrincipalPrefix, deviceID)
}

// Device is one machine that belongs to a MDMConfiguration, identified by a
// stable, client-computed DeviceID.
//
// PublicKey is the device's identity key, registered trust-on-first-use at
// enrollment. The device proves possession of it by signing short-lived JWTs
// that it presents directly when submitting scans; a request to bind a
// different key to an existing DeviceID is rejected (anti-takeover).
type Device struct {
	ID                 uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	DeviceID           string     `json:"deviceID" gorm:"uniqueIndex;not null"`                                                   // client-computed, stable
	MDMConfigurationID uint       `json:"mdmConfigurationID" gorm:"index:idx_devices_configuration_enrolled,priority:1;not null"` // the configuration this device belongs to
	PublicKey          []byte     `json:"-"`                                                                                      // DER SubjectPublicKeyInfo (PKIX) of the identity key
	Hostname           string     `json:"hostname,omitempty"`
	OS                 string     `json:"os,omitempty"`
	OSVersion          string     `json:"osVersion,omitempty"`
	EnrolledAt         time.Time  `json:"enrolledAt" gorm:"index:idx_devices_configuration_enrolled,priority:2"`
	LastSeenAt         *time.Time `json:"lastSeenAt,omitempty"`
}

// ConvertDevice maps a gateway device record to its API representation,
// dropping the registered public key.
func ConvertDevice(d Device) types2.Device {
	out := types2.Device{
		ID:                 d.ID,
		DeviceID:           d.DeviceID,
		MDMConfigurationID: d.MDMConfigurationID,
		Hostname:           d.Hostname,
		OS:                 d.OS,
		OSVersion:          d.OSVersion,
		EnrolledAt:         *types2.NewTime(d.EnrolledAt),
	}
	if d.LastSeenAt != nil {
		out.LastSeenAt = types2.NewTime(*d.LastSeenAt)
	}
	return out
}

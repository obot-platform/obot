//nolint:revive
package types

import "time"

// EnforcementDecisionLog is one recorded allow/deny decision made by the
// enforcement decision endpoint for a device's tool call.
type EnforcementDecisionLog struct {
	ID        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time `json:"createdAt" gorm:"index"`

	// Fields describing the device:
	MDMConfigurationID uint   `json:"mdmConfigurationID" gorm:"index"`
	DeviceID           string `json:"deviceID,omitempty" gorm:"index"`
	ClientIP           string `json:"clientIP,omitempty" gorm:"index"`

	// Fields describing the tool call:
	Agent      string `json:"agent,omitempty" gorm:"index"`
	Tool       string `json:"tool,omitempty" gorm:"index"`
	Kind       string `json:"kind,omitempty" gorm:"index"`
	ServerName string `json:"serverName,omitempty" gorm:"index"`
	ObotHosted bool   `json:"obotHosted,omitempty"`

	// Resolved server identity.
	ServerURL            string `json:"serverURL,omitempty"`
	ServerHostname       string `json:"serverHostname,omitempty"`
	ServerCommand        string `json:"serverCommand,omitempty"`
	ServerPackageSource  string `json:"serverPackageSource,omitempty"`
	ServerPackageName    string `json:"serverPackageName,omitempty"`
	ServerPackageVersion string `json:"serverPackageVersion,omitempty"`
	ServerConnector      string `json:"serverConnector,omitempty"`

	// Unresolved records that the device could not establish what the call
	// targeted, with UnresolvedReason naming the cause it reported. The row is
	// always a deny; these distinguish "we could not identify this" from "this
	// is identified and not allowlisted".
	Unresolved       bool   `json:"unresolved,omitempty"`
	UnresolvedReason string `json:"unresolvedReason,omitempty"`

	// Decision is the evaluator verdict: types.EnforcementDecisionAllow or
	// types.EnforcementDecisionDeny. Reason is the evaluator's human-readable
	// justification.
	Decision string `json:"decision" gorm:"index"`
	Reason   string `json:"reason,omitempty"`
}

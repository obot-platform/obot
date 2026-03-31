//nolint:revive
package types

import (
	"encoding/json"
	"time"
)

// MessagePolicyViolation represents a record of a message policy violation.
type MessagePolicyViolation struct {
	ID                   uint            `json:"id" gorm:"primaryKey"`
	CreatedAt            time.Time       `json:"createdAt" gorm:"index"`
	UserID               string          `json:"userID" gorm:"index"`
	PolicyID             string          `json:"policyID" gorm:"index"`
	PolicyName           string          `json:"policyName" gorm:"index"`
	PolicyDefinition     string          `json:"policyDefinition"`
	Direction            string          `json:"direction" gorm:"index"`
	ViolationExplanation string          `json:"violationExplanation"`
	BlockedContent       json.RawMessage `json:"blockedContent,omitempty"`
	ProjectID            string          `json:"projectID" gorm:"index"`
	ThreadID             string          `json:"threadID" gorm:"index"`
	Encrypted            bool            `json:"encrypted"`
}

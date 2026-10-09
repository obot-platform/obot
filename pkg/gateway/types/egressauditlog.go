package types

import (
	"encoding/json"
	"time"
)

// EgressAuditLog snapshots attribution so session cleanup cannot change history.
type EgressAuditLog struct {
	ID                string          `json:"id" gorm:"primaryKey"`
	CreatedAt         time.Time       `json:"createdAt" gorm:"index"`
	UpdatedAt         time.Time       `json:"updatedAt" gorm:"index"`
	UserID            string          `json:"userID" gorm:"index"`
	ClaudeAccount     string          `json:"claudeAccount" gorm:"index"`
	EnvironmentID     string          `json:"environmentID" gorm:"index"`
	ClaudeSession     string          `json:"claudeSession" gorm:"index"`
	Actor             string          `json:"actor" gorm:"index"`
	Method            string          `json:"method"`
	Scheme            string          `json:"scheme"`
	Host              string          `json:"host" gorm:"index"`
	Path              string          `json:"path"`
	ResponseStatus    int             `json:"responseStatus" gorm:"index"`
	Outcome           string          `json:"outcome" gorm:"index"`
	DurationMs        int64           `json:"durationMs"`
	RequestBytes      int64           `json:"requestBytes"`
	ResponseBytes     int64           `json:"responseBytes"`
	RequestTruncated  bool            `json:"requestTruncated"`
	ResponseTruncated bool            `json:"responseTruncated"`
	RequestHeaders    json.RawMessage `json:"requestHeaders,omitempty"`
	ResponseHeaders   json.RawMessage `json:"responseHeaders,omitempty"`
	// Bodies are JSON-encoded byte slices (base64), including binary responses.
	RequestBody  json.RawMessage `json:"requestBody,omitempty"`
	ResponseBody json.RawMessage `json:"responseBody,omitempty"`
	// Finalized preserves transport status when capture and access logs arrive out of order.
	Finalized bool `json:"-"`
	Encrypted bool `json:"-"`
}

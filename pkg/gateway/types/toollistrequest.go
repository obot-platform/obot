package types

import (
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

// ToolListRequest statuses.
const (
	ToolListRequestPending   = types2.ToolListRequestPending
	ToolListRequestCompleted = types2.ToolListRequestCompleted
	ToolListRequestFailed    = types2.ToolListRequestFailed
)

// ToolListRequest is a request, usually from the obot CLI, to list a catalog entry's tools. Listing
// launches the entry's server, so the user who made the request confirms it in the Obot UI, and the
// CLI waits for the result.
type ToolListRequest struct {
	ID        string `gorm:"primaryKey"`
	UserID    string `gorm:"index"`
	EntryName string
	Status    string
	Tools     []types2.MCPServerTool `gorm:"serializer:json"`
	Error     string
	CreatedAt time.Time `gorm:"index"`
	ExpiresAt time.Time
}

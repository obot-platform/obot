package types

// UserPreferences represents user-specific preferences and settings
type UserPreferences struct {
	// AutoApproveAllToolCalls enables automatic approval of all tool calls
	// for this user account. When true, tools execute without user confirmation.
	AutoApproveAllToolCalls bool `json:"autoApproveAllToolCalls"`

	Metadata Metadata `json:"metadata,omitempty"`
}

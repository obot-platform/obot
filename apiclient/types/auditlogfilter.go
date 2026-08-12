package types

// AuditLogAPIKeyFilterOption describes an API key that appears in the currently
// visible audit-log result set. Value is the stable filter value; the remaining
// fields are non-secret display context.
type AuditLogAPIKeyFilterOption struct {
	Value           string `json:"value"`
	Name            string `json:"name"`
	MaskedKey       string `json:"maskedKey"`
	UserID          string `json:"userID"`
	UserDisplayName string `json:"userDisplayName"`
	Revoked         bool   `json:"revoked"`
}

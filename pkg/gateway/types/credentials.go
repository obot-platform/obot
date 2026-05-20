package types

import "time"

// Credential stores secret environment variables for an Obot resource.
// List operations intentionally return Secrets keys with blank values; use RevealCredential to read values.
type Credential struct {
	ID        uint              `json:"id" gorm:"primaryKey;autoIncrement"`
	CreatedAt time.Time         `json:"createdAt"`
	Context   string            `json:"context" gorm:"uniqueIndex:idx_credentials_context_name;not null"`
	Name      string            `json:"name" gorm:"uniqueIndex:idx_credentials_context_name;not null"`
	Secrets   map[string]string `json:"secrets" gorm:"serializer:json"`
	Encrypted bool              `json:"-"`
}

// Package types defines the LLM gateway's persisted configuration and snapshots.
package types

import (
	"encoding/json"
	"time"

	"github.com/maximhq/bifrost/core/schemas"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
)

// Provider is an upstream provider the gateway routes to. Its configuration is the engine's own
// provider configuration as JSON, which is also the shape the admin UI sends today.
type Provider struct {
	Name      string    `gorm:"primaryKey"`
	Config    string    `gorm:"type:text;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// ProviderKey is one upstream API key of a provider, as the engine's key JSON.
//
// The complete document is protected by the configured credential codec.
type ProviderKey struct {
	ID        string    `gorm:"primaryKey"`
	Provider  string    `gorm:"not null;index"`
	Key       string    `gorm:"type:text;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// RoutingRule is a routing rule as the engine's rule JSON, targets and fallbacks included.
type RoutingRule struct {
	ID        string    `gorm:"primaryKey"`
	Rule      string    `gorm:"type:text;not null"`
	UpdatedAt time.Time `gorm:"not null"`
}

// Revision coordinates configuration edits and replica cache refreshes across the
// provider, key, routing-rule, and setting tables. There is one row, with ID 1.
//
// Edits read and replace a whole configuration snapshot. Writers increment this
// row before reading the snapshot; the UPDATE holds a database row lock until
// commit, preventing concurrent editors from overwriting each other's changes.
// The increment and configuration changes commit or roll back together.
//
// Replicas compare this counter with their applied revision to avoid loading and
// decrypting unchanged configuration on every poll. A replica only acknowledges
// a revision after applying it successfully. This is not configuration history.
type Revision struct {
	ID       int   `gorm:"primaryKey"`
	Revision int64 `gorm:"not null"`
}

// Config is one consistent read of the whole configuration.
type Config struct {
	Revision  int64
	Providers map[schemas.ModelProvider]schemas.ProviderConfig
	Keys      map[schemas.ModelProvider][]schemas.Key
	Rules     map[string]configstoreTables.TableRoutingRule
	Settings  map[string]json.RawMessage
}

// Setting stores client configuration and pricing overrides in Obot's database.
type Setting struct {
	Name  string `gorm:"primaryKey"`
	Value string `gorm:"type:text;not null"`
}

func (Provider) TableName() string    { return "llm_core_providers" }
func (ProviderKey) TableName() string { return "llm_core_provider_keys" }
func (RoutingRule) TableName() string { return "llm_core_routing_rules" }
func (Revision) TableName() string    { return "llm_core_revision" }
func (Setting) TableName() string     { return "llm_core_settings" }

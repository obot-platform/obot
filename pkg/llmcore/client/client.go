// Package client provides persistence for LLM gateway configuration.
package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/maximhq/bifrost/core/schemas"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
	"github.com/obot-platform/obot/pkg/llmcore/types"
	"gorm.io/gorm"
)

// Client stores LLM gateway configuration.
type Client struct {
	db    *gorm.DB
	codec ConfigCodec
}

// ConfigCodec protects complete configuration documents with Obot's credential encryption.
type ConfigCodec interface {
	SealGatewayConfig(context.Context, string, []byte) ([]byte, error)
	OpenGatewayConfig(context.Context, string, []byte) ([]byte, error)
}

func New(db *gorm.DB, codecs ...ConfigCodec) *Client {
	s := &Client{db: db}
	if len(codecs) > 0 {
		s.codec = codecs[0]
	}
	return s
}

func (s *Client) encode(ctx context.Context, id string, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	if s.codec != nil {
		data, err = s.codec.SealGatewayConfig(ctx, id, data)
	}
	return string(data), err
}

func (s *Client) decode(ctx context.Context, id, raw string, v any) error {
	data := []byte(raw)
	if s.codec != nil {
		var err error
		data, err = s.codec.OpenGatewayConfig(ctx, id, data)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(data, v)
}

// Revision returns the current configuration revision.
func (s *Client) Revision(ctx context.Context) (int64, error) {
	var rev types.Revision
	err := s.db.WithContext(ctx).First(&rev, 1).Error
	return rev.Revision, err
}

// Load reads the whole configuration in one transaction.
func (s *Client) Load(ctx context.Context) (types.Config, error) {
	return s.load(ctx, s.db.WithContext(ctx), true)
}

func (s *Client) load(ctx context.Context, db *gorm.DB, snapshot bool) (types.Config, error) {
	cfg := types.Config{
		Providers: map[schemas.ModelProvider]schemas.ProviderConfig{},
		Keys:      map[schemas.ModelProvider][]schemas.Key{},
		Rules:     map[string]configstoreTables.TableRoutingRule{},
		Settings:  map[string]json.RawMessage{},
	}
	var opts *sql.TxOptions
	if s.db.Name() == "postgres" {
		// PostgreSQL's default READ COMMITTED can see a different commit on each
		// query. Repeatable read keeps the revision and all configuration tables
		// in the same snapshot, even if another replica commits during Load.
		// Load only reads, so mark this transaction read-only.
		opts = &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true}
	}

	read := func(tx *gorm.DB) error {
		var rev types.Revision
		if err := tx.First(&rev, 1).Error; err != nil {
			return err
		}
		cfg.Revision = rev.Revision

		var providers []types.Provider
		if err := tx.Find(&providers).Error; err != nil {
			return err
		}

		for _, p := range providers {
			var pc schemas.ProviderConfig
			if err := s.decode(ctx, "provider/"+p.Name, p.Config, &pc); err != nil {
				return fmt.Errorf("provider %s: %w", p.Name, err)
			}
			cfg.Providers[schemas.ModelProvider(p.Name)] = pc
		}

		var keys []types.ProviderKey
		if err := tx.Order("id").Find(&keys).Error; err != nil {
			return err
		}

		for _, k := range keys {
			var key schemas.Key
			if err := s.decode(ctx, "key/"+k.ID, k.Key, &key); err != nil {
				return fmt.Errorf("key %s: %w", k.ID, err)
			}
			key.ID = k.ID
			cfg.Keys[schemas.ModelProvider(k.Provider)] = append(cfg.Keys[schemas.ModelProvider(k.Provider)], key)
		}

		var rules []types.RoutingRule
		if err := tx.Find(&rules).Error; err != nil {
			return err
		}

		for _, r := range rules {
			rule, err := decodeRule(r)
			if err != nil {
				return err
			}
			cfg.Rules[rule.ID] = rule
		}

		var settings []types.Setting
		if err := tx.Find(&settings).Error; err != nil {
			return err
		}

		for _, v := range settings {
			var raw json.RawMessage
			if err := s.decode(ctx, "setting/"+v.Name, v.Value, &raw); err != nil {
				return err
			}
			cfg.Settings[v.Name] = raw
		}
		return nil
	}

	var err error
	if snapshot {
		err = db.Transaction(read, opts)
	} else {
		err = read(db)
	}

	return cfg, err
}

func (s *Client) PutProvider(ctx context.Context, name string, config schemas.ProviderConfig) error {
	data, err := s.encode(ctx, "provider/"+name, config)
	if err != nil {
		return err
	}
	return s.write(ctx, func(tx *gorm.DB) error {
		return tx.Save(&types.Provider{Name: name, Config: data}).Error
	})
}

// DeleteProvider deletes a provider and its keys.
func (s *Client) DeleteProvider(ctx context.Context, name string) error {
	return s.write(ctx, func(tx *gorm.DB) error {
		if err := tx.Where("provider = ?", name).Delete(&types.ProviderKey{}).Error; err != nil {
			return err
		}
		return tx.Delete(&types.Provider{Name: name}).Error
	})
}

func (s *Client) PutKey(ctx context.Context, provider string, key schemas.Key) error {
	if key.ID == "" {
		return errors.New("a key needs an ID")
	}

	data, err := s.encode(ctx, "key/"+key.ID, key)
	if err != nil {
		return err
	}

	return s.write(ctx, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&types.Provider{}).Where("name = ?", provider).Count(&count).Error; err != nil {
			return err
		}

		if count == 0 {
			return fmt.Errorf("provider %q not found", provider)
		}
		return tx.Save(&types.ProviderKey{ID: key.ID, Provider: provider, Key: data}).Error
	})
}

func (s *Client) DeleteKey(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *gorm.DB) error {
		return tx.Delete(&types.ProviderKey{ID: id}).Error
	})
}

func (s *Client) PutRule(ctx context.Context, rule configstoreTables.TableRoutingRule) error {
	if rule.ID == "" {
		return errors.New("a routing rule needs an ID")
	}

	data, err := json.Marshal(rule)
	if err != nil {
		return err
	}

	return s.write(ctx, func(tx *gorm.DB) error {
		return tx.Save(&types.RoutingRule{ID: rule.ID, Rule: string(data)}).Error
	})
}

func (s *Client) DeleteRule(ctx context.Context, id string) error {
	return s.write(ctx, func(tx *gorm.DB) error {
		return tx.Delete(&types.RoutingRule{ID: id}).Error
	})
}

// write applies a change and advances the revision in one transaction. The UPDATE takes a row
// lock on the revision, so concurrent writers from any replica are serialized by the database
// itself, with no advisory lock and no reload before writing.
func (s *Client) write(ctx context.Context, change func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&types.Revision{}).Where("id = 1").Update("revision", gorm.Expr("revision + 1"))
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("gateway revision row missing")
		}
		return change(tx)
	})
}

func decodeRule(r types.RoutingRule) (configstoreTables.TableRoutingRule, error) {
	var rule configstoreTables.TableRoutingRule
	if err := json.Unmarshal([]byte(r.Rule), &rule); err != nil {
		return rule, fmt.Errorf("routing rule %s: %w", r.ID, err)
	}
	rule.ID = r.ID

	// The rule JSON carries scope_id but not the targets' rule IDs, which the engine keys on.
	for i := range rule.Targets {
		rule.Targets[i].RuleID = rule.ID
	}
	if rule.Scope == "" {
		rule.Scope = "global"
	}
	return rule, nil
}

// Mutate reads and validates the latest configuration under the revision row lock.
// The entire change, including confirmed key removal, commits atomically.
func (s *Client) Mutate(ctx context.Context, change func(*types.Config) error) error {
	return s.write(ctx, func(tx *gorm.DB) error {
		cfg, err := s.load(ctx, tx, false)
		if err != nil {
			return err
		}
		if err := change(&cfg); err != nil {
			return err
		}

		// Enforce the same key invariants as PutKey before replacing any rows.
		for provider, keys := range cfg.Keys {
			if _, exists := cfg.Providers[provider]; !exists && len(keys) > 0 {
				return fmt.Errorf("provider %q not found", provider)
			}
			for _, key := range keys {
				if key.ID == "" {
					return errors.New("a key needs an ID")
				}
			}
		}

		for _, model := range []any{&types.Provider{}, &types.ProviderKey{}, &types.RoutingRule{}, &types.Setting{}} {
			if err := tx.Where("1 = 1").Delete(model).Error; err != nil {
				return err
			}
		}

		for name, v := range cfg.Providers {
			data, err := s.encode(ctx, "provider/"+string(name), v)
			if err != nil {
				return err
			}
			if err := tx.Create(&types.Provider{Name: string(name), Config: data}).Error; err != nil {
				return err
			}
		}

		for provider, keys := range cfg.Keys {
			for _, v := range keys {
				data, err := s.encode(ctx, "key/"+v.ID, v)
				if err != nil {
					return err
				}
				if err := tx.Create(&types.ProviderKey{ID: v.ID, Provider: string(provider), Key: data}).Error; err != nil {
					return err
				}
			}
		}

		for id, v := range cfg.Rules {
			data, err := json.Marshal(v)
			if err != nil {
				return err
			}
			if err := tx.Create(&types.RoutingRule{ID: id, Rule: string(data)}).Error; err != nil {
				return err
			}
		}

		for name, v := range cfg.Settings {
			data, err := s.encode(ctx, "setting/"+name, v)
			if err != nil {
				return err
			}
			if err := tx.Create(&types.Setting{Name: name, Value: data}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

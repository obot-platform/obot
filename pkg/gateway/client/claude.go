package client

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// ClaudeLLMCredentialContext stores each session's LLM proxy key apart from its MCP key,
	// so MCP grant changes never revoke LLM access.
	ClaudeLLMCredentialContext = "substrate-agent-llm"
)

var (
	ErrClaudeCredentialRevoked = errors.New("the Claude session API key has been revoked")
)

// ponytail: global admission lock; use per-account/session locks if provisioning throughput warrants it.
// Serialize admission and order deduplication across Obot replicas. Provisioning only
// writes desired state; this lock is never held while waiting for a runner to boot.
func (c *Client) WithClaudeProvisionLock(ctx context.Context, fn func() error) error {
	// The lock's transaction holds a database connection while it waits, and fn's queries
	// use others. Waiting here first keeps a replica's waiters from filling the pool and
	// stalling the holder: only one caller per replica holds or waits for the lock.
	gate := c.claudeProvisionGate()
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-gate }()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(672046923)).Error; err != nil {
			return err
		}
		return fn()
	})
}

func (c *Client) claudeProvisionGate() chan struct{} {
	c.claudeProvisionGateOnce.Do(func() {
		c.claudeProvisionGateSlot = make(chan struct{}, 1)
	})
	return c.claudeProvisionGateSlot
}

func (c *Client) ClaudeWorkOrderSubmitted(ctx context.Context, name string) (bool, error) {
	var order types.ClaudeWorkOrder
	err := c.db.WithContext(ctx).Where("name = ?", name).First(&order).Error
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	return err == nil, err
}

func (c *Client) MarkClaudeWorkOrderSubmitted(ctx context.Context, name string) error {
	return c.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&types.ClaudeWorkOrder{Name: name, CreatedAt: time.Now()}).Error
}

func (c *Client) SaveClaudeWorkOrder(ctx context.Context, credential types.Credential) error {
	if err := c.encryptCredential(ctx, &credential); err != nil {
		return err
	}
	return c.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&credential).Error
}

// EnsureClaudeCredential reuses a valid key or atomically replaces an expired key.
// The caller must verify the session and account under WithClaudeProvisionLock.
func (c *Client) EnsureClaudeCredential(ctx context.Context, userID uint, name string, servers []string, expires time.Time) (types.Credential, error) {
	if len(servers) == 0 {
		return types.Credential{}, fmt.Errorf("an unexpired session and explicit MCP scopes are required")
	}
	return c.ensureClaudeCredential(ctx, "substrate-agent", userID, name, "Claude session "+name, types.APIKeyScopes{MCPServerIDs: servers}, expires)
}

// EnsureClaudeLLMCredential is EnsureClaudeCredential for the session's LLM proxy key.
func (c *Client) EnsureClaudeLLMCredential(ctx context.Context, userID uint, name string, expires time.Time) (types.Credential, error) {
	return c.ensureClaudeCredential(ctx, ClaudeLLMCredentialContext, userID, name, "Claude session LLM "+name, types.APIKeyScopes{CanAccessLLMProxy: true}, expires)
}

func (c *Client) ensureClaudeCredential(ctx context.Context, credentialContext string, userID uint, name, keyName string, scopes types.APIKeyScopes, expires time.Time) (types.Credential, error) {
	var credential types.Credential
	var oldID uint
	now := time.Now()
	if !expires.After(now) || !scopes.HasSomeScope() {
		return credential, fmt.Errorf("an unexpired session and explicit scopes are required")
	}

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("context = ? AND name = ?", credentialContext, name).First(&credential).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			if err := c.decryptCredential(ctx, &credential); err != nil {
				return err
			}
			id, err := strconv.ParseUint(credential.Secrets["keyID"], 10, 64)
			if err != nil {
				return err
			}
			var old types.APIKey
			// Expiration cleanup may already have removed the previous key.
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userID).First(&old).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if old.RevokedAt != nil {
				return ErrClaudeCredentialRevoked
			}
			if claudeKeyReusable(old, scopes, expires, now) {
				return nil
			}

			oldID = old.ID
			if old.ID != 0 {
				if old.ExpiresAt != nil && !old.ExpiresAt.After(now) {
					err = tx.Delete(&old).Error
				} else {
					err = tx.Model(&old).Update("revoked_at", now).Error
				}
				if err != nil {
					return err
				}
			}
		}

		key, err := c.createAPIKey(tx, types.APIKey{
			UserID:            userID,
			Name:              keyName,
			Description:       "Self-hosted Claude session",
			ExpiresAt:         &expires,
			APIKeyScopes:      scopes,
			ClaudeEnvironment: true,
		})
		if err != nil {
			return err
		}

		credential = types.Credential{Context: credentialContext, Name: name, Secrets: map[string]string{"key": key.Key, "keyID": fmt.Sprint(key.ID)}}
		stored := credential
		if err := c.encryptCredential(ctx, &stored); err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "context"}, {Name: "name"}},
			DoUpdates: clause.AssignmentColumns([]string{"secrets", "encrypted"}),
		}).Create(&stored).Error
	})
	if err == nil && oldID != 0 {
		c.invalidateValidatedAPIKeysByID(oldID)
	}
	return credential, err
}

// claudeKeyReusable reports whether a session's unrevoked key can be returned instead of
// issuing a new one: it has the scopes asked for and expires neither too soon nor after
// expires.
func claudeKeyReusable(key types.APIKey, scopes types.APIKeyScopes, expires, now time.Time) bool {
	return key.ClaudeEnvironment && key.ExpiresAt != nil && key.ExpiresAt.After(now.Add(30*time.Second)) && !key.ExpiresAt.After(expires) && reflect.DeepEqual(key.APIKeyScopes, scopes)
}

// CurrentClaudeCredential returns the session's MCP key when EnsureClaudeCredential would
// reuse it, without the provisioning lock. Returning a key that was already issued grants
// nothing new, and a key revoked meanwhile is rejected wherever it is used. It reports false
// when the key must be issued or rotated, which needs EnsureClaudeCredential and the lock.
func (c *Client) CurrentClaudeCredential(ctx context.Context, userID uint, name string, servers []string, expires time.Time) (types.Credential, bool, error) {
	return c.currentClaudeCredential(ctx, "substrate-agent", userID, name, types.APIKeyScopes{MCPServerIDs: servers}, expires)
}

// CurrentClaudeLLMCredential is CurrentClaudeCredential for the session's LLM proxy key.
func (c *Client) CurrentClaudeLLMCredential(ctx context.Context, userID uint, name string, expires time.Time) (types.Credential, bool, error) {
	return c.currentClaudeCredential(ctx, ClaudeLLMCredentialContext, userID, name, types.APIKeyScopes{CanAccessLLMProxy: true}, expires)
}

func (c *Client) currentClaudeCredential(ctx context.Context, credentialContext string, userID uint, name string, scopes types.APIKeyScopes, expires time.Time) (types.Credential, bool, error) {
	credential, err := c.RevealCredential(ctx, []string{credentialContext}, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return types.Credential{}, false, nil
	}
	if err != nil {
		return types.Credential{}, false, err
	}
	id, err := strconv.ParseUint(credential.Secrets["keyID"], 10, 64)
	if err != nil {
		return types.Credential{}, false, err
	}
	var key types.APIKey
	if err := c.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&key).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		// Expiration cleanup may already have removed the key.
		return types.Credential{}, false, nil
	} else if err != nil {
		return types.Credential{}, false, err
	}
	if key.RevokedAt != nil {
		// A concurrent rotation revokes the old key as it stores the new one. Report
		// revocation only when the session still refers to the revoked key.
		current, err := c.RevealCredential(ctx, []string{credentialContext}, name)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return types.Credential{}, false, err
		}
		if err != nil || current.Secrets["keyID"] != credential.Secrets["keyID"] {
			return types.Credential{}, false, nil
		}
		return types.Credential{}, false, ErrClaudeCredentialRevoked
	}
	if !claudeKeyReusable(key, scopes, expires, time.Now()) {
		return types.Credential{}, false, nil
	}
	return credential, true, nil
}

// RevokeClaudeCredentials revokes a session's MCP and LLM keys. The revoked keys stay
// referenced as revocation markers, so neither can be reissued for the session.
func (c *Client) RevokeClaudeCredentials(ctx context.Context, name string) error {
	for _, credentialContext := range []string{"substrate-agent", ClaudeLLMCredentialContext} {
		if err := c.RevokeClaudeCredential(ctx, credentialContext, name); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) RevokeClaudeCredential(ctx context.Context, credentialContext, name string) error {
	credential, err := c.RevealCredential(ctx, []string{credentialContext}, name)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	id, err := strconv.ParseUint(credential.Secrets["keyID"], 10, 64)
	if err != nil {
		return err
	}
	return c.RevokeAPIKeyByID(ctx, uint(id))
}

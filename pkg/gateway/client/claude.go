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

var (
	ErrClaudeCredentialRevoked = errors.New("the Claude session API key has been revoked")
)

// ponytail: global admission lock; use per-account/session locks if provisioning throughput warrants it.
// Serialize admission and order deduplication across Obot replicas. Provisioning only
// writes desired state; this lock is never held while waiting for a runner to boot.
func (c *Client) WithClaudeProvisionLock(ctx context.Context, fn func() error) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(672046923)).Error; err != nil {
			return err
		}
		return fn()
	})
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
	var credential types.Credential
	var oldID uint
	now := time.Now()
	if !expires.After(now) || len(servers) == 0 {
		return credential, fmt.Errorf("an unexpired session and explicit MCP scopes are required")
	}
	scopes := types.APIKeyScopes{MCPServerIDs: servers}

	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("context = ? AND name = ?", "substrate-agent", name).First(&credential).Error
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
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", id, userID).First(&old).Error; err != nil {
				return err
			}
			if old.RevokedAt != nil {
				return ErrClaudeCredentialRevoked
			}
			if old.ClaudeEnvironment && old.ExpiresAt != nil && old.ExpiresAt.After(now.Add(30*time.Second)) && !old.ExpiresAt.After(expires) && reflect.DeepEqual(old.APIKeyScopes, scopes) {
				return nil
			}

			oldID = old.ID
			if err := tx.Model(&old).Update("revoked_at", now).Error; err != nil {
				return err
			}
		}

		key, err := c.createAPIKey(tx, types.APIKey{
			UserID:            userID,
			Name:              "Claude session " + name,
			Description:       "Self-hosted Claude session",
			ExpiresAt:         &expires,
			APIKeyScopes:      scopes,
			ClaudeEnvironment: true,
		})
		if err != nil {
			return err
		}

		credential = types.Credential{Context: "substrate-agent", Name: name, Secrets: map[string]string{"key": key.Key, "keyID": fmt.Sprint(key.ID)}}
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

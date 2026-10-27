package client

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (

	// UpstreamOAuthScopePrefix identifies temporary context, never a permission grant.
	UpstreamOAuthScopePrefix = "obot:upstream:"
)

var (
	ErrMCPOAuthChallengeBinding = errors.New("upstream authorization challenge binding mismatch")
)

func (c *Client) CreateMCPOAuthChallenge(ctx context.Context, batch *types.MCPOAuthChallenge) error {
	batch.ID = strings.ToLower(rand.Text())
	batch.CreatedAt = time.Now()
	batch.ExpiresAt = batch.CreatedAt.Add(pendingStateTTL)

	return c.db.WithContext(ctx).Create(batch).Error
}

// ClaimMCPOAuthChallenge validates before CAS. The same transaction can retry a
// failed handoff; a different transaction cannot steal or delete its payload.
func (c *Client) ClaimMCPOAuthChallenge(ctx context.Context, id, userID, namespace, mcpID, clientID, resource, requestID string) (*types.MCPOAuthChallenge, error) {
	if requestID == "" {
		return nil, fmt.Errorf("OAuth request ID is required")
	}

	var batch types.MCPOAuthChallenge
	if err := c.db.WithContext(ctx).Where("id = ? AND expires_at > ?", id, time.Now()).First(&batch).Error; err != nil {
		return nil, err
	}

	if batch.UserID != userID || batch.Namespace != namespace || batch.MCPID != mcpID || batch.Resource != resource || batch.ClientID != "" && batch.ClientID != clientID {
		return nil, fmt.Errorf("%w: challenge does not belong to this connection", ErrMCPOAuthChallengeBinding)
	}

	result := c.db.WithContext(ctx).Model(&types.MCPOAuthChallenge{}).
		Where("id = ? AND expires_at > ? AND (auth_request_id = '' OR auth_request_id = ?)", id, time.Now(), requestID).
		Update("auth_request_id", requestID)
	if result.Error != nil {
		return nil, result.Error
	}

	if result.RowsAffected != 1 {
		return nil, fmt.Errorf("%w: challenge is already owned by another transaction", ErrMCPOAuthChallengeBinding)
	}

	batch.AuthRequestID = requestID

	return &batch, nil
}

func (c *Client) DeleteClaimedMCPOAuthChallenge(ctx context.Context, id, requestID string) error {
	return c.db.WithContext(ctx).Where("id = ? AND auth_request_id = ?", id, requestID).Delete(&types.MCPOAuthChallenge{}).Error
}

// ReserveMCPOAuthAttempt counts starts by stable authenticated binding, not ticket ID.
func (c *Client) ReserveMCPOAuthAttempt(ctx context.Context, key string) error {
	now := time.Now()

	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		budget := types.MCPOAuthAttemptBudget{
			ID:        key,
			ExpiresAt: now.Add(5 * time.Minute),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&budget).Error; err != nil {
			return err
		}

		if err := tx.Model(&budget).Where("expires_at <= ?", now).Updates(map[string]any{
			"starts":     0,
			"expires_at": budget.ExpiresAt,
		}).Error; err != nil {
			return err
		}

		result := tx.Model(&budget).Where("starts < ?", 3).UpdateColumn("starts", gorm.Expr("starts + 1"))
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected != 1 {
			return fmt.Errorf("upstream OAuth attempt limit reached; retry after five minutes")
		}

		return nil
	})
}

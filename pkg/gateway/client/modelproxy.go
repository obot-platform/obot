package client

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	modelProxyEnabledKey = "model_proxy_enabled"
)

// ModelProxyEnabled reads shared state on every call. Only an absent property
// uses the backwards-compatible default.
func (c *Client) ModelProxyEnabled(ctx context.Context) (bool, error) {
	p, err := c.GetProperty(ctx, modelProxyEnabledKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true, nil
	}

	if err != nil {
		return false, errors.New("model proxy settings unavailable")
	}

	switch p.Value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, errors.New("invalid model proxy settings")
	}
}

// SetModelProxyEnabled uses an atomic upsert so concurrent first writes cannot
// race to create the singleton. Preserve the existing property encryption path.
func (c *Client) SetModelProxyEnabled(ctx context.Context, enabled bool) error {
	now := time.Now()
	p := types.Property{
		Key:       modelProxyEnabledKey,
		Value:     strconv.FormatBool(enabled),
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := c.encryptProperty(ctx, &p); err != nil {
		return errors.New("model proxy settings unavailable")
	}

	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "encrypted", "updated_at"}),
	}).Create(&p).Error; err != nil {
		return errors.New("model proxy settings unavailable")
	}

	return nil
}

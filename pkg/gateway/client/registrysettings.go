package client

import (
	"context"
	"errors"
	"strconv"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	registryAnonymousAccessEnabledKey = "registry_anonymous_access_enabled"
)

// AnonymousRegistryAccessEnabled reads shared state on every call. Only an absent property
// uses the default of disabled.
func (c *Client) AnonymousRegistryAccessEnabled(ctx context.Context) (bool, error) {
	p, err := c.GetProperty(ctx, registryAnonymousAccessEnabledKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, errors.New("registry settings unavailable")
	}

	enabled, err := strconv.ParseBool(p.Value)
	if err != nil {
		return false, errors.New("invalid registry settings")
	}
	return enabled, nil
}

// SetAnonymousRegistryAccessEnabled uses an atomic upsert so concurrent first writes cannot
// race to create the singleton. Preserve the existing property encryption path.
func (c *Client) SetAnonymousRegistryAccessEnabled(ctx context.Context, enabled bool) error {
	p := types.Property{
		Key:   registryAnonymousAccessEnabledKey,
		Value: strconv.FormatBool(enabled),
	}

	if err := c.encryptProperty(ctx, &p); err != nil {
		return errors.New("registry settings unavailable")
	}

	if err := c.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "encrypted", "updated_at"}),
	}).Create(&p).Error; err != nil {
		return errors.New("registry settings unavailable")
	}

	return nil
}

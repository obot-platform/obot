package client

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) AddActivityForToday(ctx context.Context, userID string) error {
	today := time.Now().UTC().Truncate(24 * time.Hour)
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check for an existing activity
		if err := tx.Where("user_id = ? AND date = ?", userID, today).First(new(types.APIActivity)).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Create an activity tracker for this user on this day.
		return tx.Create(&types.APIActivity{UserID: userID, Date: today}).Error
	})
}

func (c *Client) ActivitiesByUser(ctx context.Context, userID string, start, end time.Time) ([]types.APIActivity, error) {
	var activities []types.APIActivity
	return activities, c.db.WithContext(ctx).Where("user_id = ?", userID).Where("date >= ? AND date < ?", start, end).Order("id DESC").Find(&activities).Error
}

func (c *Client) ActiveUsersByDate(ctx context.Context, start, end time.Time) ([]types.User, error) {
	var users []types.User
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []string
		if err := tx.Model(new(types.APIActivity)).
			Distinct("user_id").
			Where("date >= ? AND date < ?", start, end).
			Where("user_id != ?", "bootstrap").
			Where("user_id != ?", "anonymous").
			Where("user_id != ?", "").
			Pluck("user_id", &ids).Error; err != nil {
			return err
		}
		return tx.Where("id IN (?) AND NOT internal", ids).Find(&users).Error
	}); err != nil {
		return nil, err
	}

	for i := range users {
		if err := c.decryptUser(ctx, &users[i]); err != nil {
			return nil, fmt.Errorf("failed to decrypt user: %w", err)
		}
	}

	return users, nil
}

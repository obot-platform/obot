package client

import (
	"context"
	"errors"
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
	return users, c.db.WithContext(ctx).Where("last_active_time >= ? AND last_active_time < ?", start, end).Find(&users).Error
}

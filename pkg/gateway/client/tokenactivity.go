package client

import (
	"context"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func (c *Client) TokenUsageForUser(ctx context.Context, userID string, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	return activities, c.db.WithContext(ctx).Model(new(types.RunState)).Where("user_id = ?", userID).Where("created_at >= ? AND created_at < ?", start, end).Order("created_at DESC").Find(&activities).Error
}

func (c *Client) TotalTokenUsageForUser(ctx context.Context, userID string, start, end time.Time) (types.TokenActivity, error) {
	activity, err := c.tokenUsageByUser(ctx, userID, start, end)
	if err != nil || len(activity) == 0 {
		return types.TokenActivity{
			RunTokenActivity: types.RunTokenActivity{
				UserID: userID,
			},
		}, err
	}

	return activity[0], nil
}

func (c *Client) TokenUsageByUser(ctx context.Context, start, end time.Time) ([]types.TokenActivity, error) {
	return c.tokenUsageByUser(ctx, "", start, end)
}

func (c *Client) tokenUsageByUser(ctx context.Context, userID string, start, end time.Time) ([]types.TokenActivity, error) {
	var activities []types.TokenActivity
	db := c.db.WithContext(ctx).Model(new(types.RunState)).
		Select("user_id, COUNT(name) as run_count, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(total_tokens) as total_tokens, MIN(created_at) as created_at").
		Where("created_at >= ? AND created_at < ?", start, end)
	if userID != "" {
		db = db.Where("user_id = ?", userID)
	} else {
		db = db.Where("user_id IS NOT NULL")
	}
	return activities, db.Group("user_id").Scan(&activities).Error
}

package client

import (
	"context"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) UpsertTokenUsage(ctx context.Context, activity *types.RunTokenActivity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if activity.ID == 0 {
			return tx.Create(activity).Error
		}
		return tx.Updates(activity).Error
	})
}

func (c *Client) TokenUsageForUser(ctx context.Context, userID string, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	return activities, c.db.WithContext(ctx).Where("user_id = ?", userID).Where("created_at >= ? AND created_at < ?", start, end).Order("created_at DESC").Find(&activities).Error
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

func (c *Client) RemainingTokenUsageForUser(ctx context.Context, userID string, period time.Duration, promptTokenLimit, completionTokenLimit int) (*types.RemainingTokenUsage, error) {
	r := &types.RemainingTokenUsage{
		UnlimitedCompletionTokens: completionTokenLimit < 0,
		UnlimitedPromptTokens:     promptTokenLimit < 0,
	}
	// Check if both "limits" are less than 0. If so, then the user has unlimited tokens.
	if promptTokenLimit < 0 && completionTokenLimit < 0 {
		return r, nil
	}

	user, err := c.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.Role.HasRole(types2.RoleAdmin) {
		// Admins always have unlimited tokens.
		return r, nil
	}

	// If either user-based limit is negative, then the user has unlimited tokens.
	r.UnlimitedPromptTokens = user.DailyPromptTokensLimit < 0
	r.UnlimitedCompletionTokens = user.DailyCompletionTokensLimit < 0

	// If both "limits" are less than or equal to 0. If so, then the user has unlimited tokens.
	if r.UnlimitedPromptTokens && r.UnlimitedCompletionTokens {
		return r, nil
	}

	r.PromptTokens = promptTokenLimit
	r.CompletionTokens = completionTokenLimit

	end := time.Now()
	activity, err := c.tokenUsageByUser(ctx, userID, end.Add(-period), end)
	if err != nil || len(activity) == 0 {
		return r, err
	}

	r.PromptTokens = promptTokenLimit - activity[0].PromptTokens
	r.CompletionTokens = completionTokenLimit - activity[0].CompletionTokens

	return r, nil
}

func (c *Client) tokenUsageByUser(ctx context.Context, userID string, start, end time.Time) ([]types.TokenActivity, error) {
	var activities []types.TokenActivity
	db := c.db.WithContext(ctx).Model(new(types.RunTokenActivity)).
		Select("user_id, COUNT(name) as run_count, SUM(prompt_tokens) as prompt_tokens, SUM(completion_tokens) as completion_tokens, SUM(total_tokens) as total_tokens").
		Where("created_at >= ? AND created_at < ?", start, end)
	if userID != "" {
		db = db.Where("user_id = ?", userID)
	} else {
		db = db.Where("user_id IS NOT NULL")
	}
	return activities, db.Group("user_id").Scan(&activities).Error
}

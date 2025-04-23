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

func (c *Client) RemainingTokenUsageForUser(ctx context.Context, userID string, period time.Duration, promptTokenLimit, completionTokenLimit int) (int, int, error) {
	// Check if both "limits" are less than or equal to 0. If so, then the user has unlimited tokens.
	if promptTokenLimit <= 0 && completionTokenLimit <= 0 {
		return 1, 1, nil
	}

	user, err := c.UserByID(ctx, userID)
	if err != nil {
		return 0, 0, err
	}

	if user.Role.HasRole(types2.RoleAdmin) {
		// Admins always have unlimited tokens.
		return promptTokenLimit, completionTokenLimit, nil
	}

	// If the user has unlimited tokens (specified by a negative limit on the user object),
	// then set the limit to 0 here. The logic below will make it such that the user will always
	// have available tokens.
	if user.DailyPromptTokensLimit < 0 {
		promptTokenLimit = 0
	}
	if user.DailyCompletionTokensLimit < 0 {
		completionTokenLimit = 0
	}

	// If both "limits" are less than or equal to 0. If so, then the user has unlimited tokens.
	if promptTokenLimit <= 0 && completionTokenLimit <= 0 {
		return 1, 1, nil
	}

	end := time.Now()
	activity, err := c.tokenUsageByUser(ctx, userID, end.Add(-period), end)
	if err != nil || len(activity) == 0 {
		return promptTokenLimit, completionTokenLimit, err
	}

	if promptTokenLimit == 0 {
		promptTokenLimit = activity[0].PromptTokens + 1
	}
	if completionTokenLimit == 0 {
		completionTokenLimit = activity[0].CompletionTokens + 1
	}

	return promptTokenLimit - activity[0].PromptTokens, completionTokenLimit - activity[0].CompletionTokens, nil
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

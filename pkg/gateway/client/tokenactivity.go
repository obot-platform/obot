package client

import (
	"context"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) InsertTokenUsage(ctx context.Context, activity *types.RunTokenActivity) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if activity.ID == 0 {
			return tx.Create(activity).Error
		}
		return tx.Updates(activity).Error
	})
}

func (c *Client) TokenUsageForUser(ctx context.Context, userID string, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	return activities, c.db.WithContext(ctx).Where("user_id = ?", userID).Where("created_at >= ? AND created_at <= ?", start, end).Order("created_at DESC").Find(&activities).Error
}

func (c *Client) TotalTokenUsageForUser(ctx context.Context, userID string, start, end time.Time) (types.RunTokenActivity, error) {
	activity, err := c.tokenUsageByUser(ctx, userID, start, end)
	if err != nil || len(activity) == 0 {
		return types.RunTokenActivity{
			UserID: userID,
		}, err
	}

	return activity[0], nil
}

func (c *Client) TokenUsageByUser(ctx context.Context, start, end time.Time) ([]types.RunTokenActivity, error) {
	return c.tokenUsageByUser(ctx, "", start, end)
}

// TokenUsageSeriesInRange returns all individual token usage records in the time range for all users.
// Results are ordered by created_at descending.
// The range is [start, end] inclusive so that the requested end time is the last moment included.
func (c *Client) TokenUsageSeriesInRange(ctx context.Context, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	err := c.db.WithContext(ctx).Where("created_at >= ? AND created_at <= ?", start, end).
		Where("user_id IS NOT NULL").
		Order("created_at DESC").
		Find(&activities).Error
	return activities, err
}

func (c *Client) RemainingTokenUsageForUser(ctx context.Context, userID string, period time.Duration, inputTokenLimit, outputTokenLimit int) (*types.RemainingTokenUsage, error) {
	r := &types.RemainingTokenUsage{}

	user, err := c.UserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.Role.HasRole(types2.RoleAdmin) {
		// Admins always have unlimited tokens.
		r.UnlimitedInputTokens = true
		r.UnlimitedOutputTokens = true
		return r, nil
	}

	// Resolve the effective per-dimension limit, folding in the per-user setting: a positive
	// per-user limit overrides the server limit, 0 inherits it, and a negative one disables it.
	inputLimit, inputUnlimited := effectiveTokenLimit(inputTokenLimit, user.DailyInputTokensLimit)
	outputLimit, outputUnlimited := effectiveTokenLimit(outputTokenLimit, user.DailyOutputTokensLimit)
	r.UnlimitedInputTokens = inputUnlimited
	r.UnlimitedOutputTokens = outputUnlimited
	if inputUnlimited && outputUnlimited {
		return r, nil
	}

	// Seed with the full limit so a user with no usage this period sees their whole budget.
	r.InputTokens = inputLimit
	r.OutputTokens = outputLimit

	end := time.Now()
	activity, err := c.tokenUsageByUser(ctx, userID, end.Add(-period), end)
	if err != nil || len(activity) == 0 {
		return r, err
	}

	// Remaining = limit - used. A negative value is how far over the limit the user went
	// (only meaningful for limited dimensions; ignore it when the Unlimited flag is set).
	r.InputTokens = inputLimit - activity[0].Usage.InputTokens
	r.OutputTokens = outputLimit - activity[0].Usage.OutputTokens

	return r, nil
}

// effectiveTokenLimit resolves one dimension's daily token limit. A positive per-user limit
// overrides the server limit; a per-user limit of 0 inherits the server limit; any negative
// limit (server or per-user) disables the limit (unlimited).
func effectiveTokenLimit(serverLimit, userLimit int) (limit int, unlimited bool) {
	if userLimit > 0 {
		return userLimit, false
	}
	if userLimit < 0 || serverLimit < 0 {
		return 0, true
	}
	return serverLimit, false
}

func (c *Client) tokenUsageByUser(ctx context.Context, userID string, start, end time.Time) ([]types.RunTokenActivity, error) {
	var activities []types.RunTokenActivity
	db := c.db.WithContext(ctx).Model(new(types.RunTokenActivity)).
		Select("user_id, "+
			"SUM(input_tokens) as input_tokens, "+
			"SUM(cache_read_tokens) as cache_read_tokens, "+
			"SUM(cache_write_tokens) as cache_write_tokens, "+
			"SUM(output_tokens) as output_tokens, "+
			"SUM(thinking_tokens) as thinking_tokens, "+
			"SUM(total_tokens) as total_tokens, "+
			"SUM(input_spend) as input_spend, "+
			"SUM(cache_read_spend) as cache_read_spend, "+
			"SUM(cache_write_spend) as cache_write_spend, "+
			"SUM(output_spend) as output_spend, "+
			"SUM(total_spend) as total_spend").
		Where("created_at >= ? AND created_at <= ?", start, end)
	if userID != "" {
		db = db.Where("user_id = ?", userID)
	} else {
		db = db.Where("user_id IS NOT NULL")
	}
	return activities, db.Group("user_id").Scan(&activities).Error
}

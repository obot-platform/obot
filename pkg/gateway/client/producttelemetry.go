package client

import (
	"context"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func (c *Client) ActiveUserCountByDate(ctx context.Context, start, end time.Time) (int64, error) {
	var count int64
	err := c.db.WithContext(ctx).
		Model(new(types.User)).
		Joins("JOIN api_activities ON api_activities.user_id = CAST(users.id AS TEXT)").
		Where("api_activities.date >= ? AND api_activities.date < ?", start.UTC(), end.UTC()).
		Where("NOT users.internal AND users.deleted_at IS NULL").
		Distinct("users.id").
		Count(&count).Error
	return count, err
}

func (c *Client) MCPToolCallCount(ctx context.Context, start, end time.Time) (int64, error) {
	var count int64
	err := c.db.WithContext(ctx).
		Model(new(types.MCPAuditLog)).
		Where("source_type = ?", clienttypes.AuditLogSourceTypeMCP).
		Where("call_type = ?", "tools/call").
		Where("created_at >= ? AND created_at < ?", start.UTC(), end.UTC()).
		Count(&count).Error
	return count, err
}

func (c *Client) LLMAuditLogCount(ctx context.Context, start, end time.Time) (int64, error) {
	var count int64
	err := c.db.WithContext(ctx).
		Model(new(types.LLMAuditLog)).
		Where("created_at >= ? AND created_at < ?", start.UTC(), end.UTC()).
		Count(&count).Error
	return count, err
}

func (c *Client) DeviceScanCount(ctx context.Context, start, end time.Time) (int64, error) {
	var count int64
	err := c.db.WithContext(ctx).
		Model(new(types.DeviceScan)).
		Where("created_at >= ? AND created_at < ?", start.UTC(), end.UTC()).
		Count(&count).Error
	return count, err
}

func (c *Client) EnforcementDecisionCount(ctx context.Context, start, end time.Time) (int64, error) {
	var count int64
	err := c.db.WithContext(ctx).
		Model(new(types.EnforcementDecisionLog)).
		Where("created_at >= ? AND created_at < ?", start.UTC(), end.UTC()).
		Count(&count).Error
	return count, err
}

package client

import (
	"context"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
)

func (c *Client) ActiveUserCountByDate(ctx context.Context, start, end time.Time) (int64, error) {
	activeUserIDs := c.db.WithContext(ctx).
		Model(new(types.APIActivity)).
		Distinct("user_id").
		Where("date >= ? AND date < ?", start.UTC(), end.UTC()).
		Where("user_id != ?", system.BootstrapName).
		Where("user_id != ?", "anonymous").
		Where("user_id != ?", "").
		Select("user_id")

	var count int64
	err := c.db.WithContext(ctx).
		Model(new(types.User)).
		Where("id IN (?) AND NOT internal AND deleted_at IS NULL", activeUserIDs).
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

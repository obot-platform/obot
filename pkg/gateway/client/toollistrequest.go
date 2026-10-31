package client

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

const (
	// ToolListRequestTTL is how long the user has to confirm a tool list request.
	ToolListRequestTTL = 10 * time.Minute
)

var (
	// ErrToolListRequestNotPending is returned when a tool list request was already answered.
	ErrToolListRequestNotPending = errors.New("tool list request is no longer pending")
)

// CreateToolListRequest creates a pending request by userID to list the tools of entryName.
func (c *Client) CreateToolListRequest(ctx context.Context, userID, entryName string) (*types.ToolListRequest, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("failed to generate tool list request id: %w", err)
	}
	now := time.Now()
	request := &types.ToolListRequest{
		ID:        "tlr" + hex.EncodeToString(id),
		UserID:    userID,
		EntryName: entryName,
		Status:    types.ToolListRequestPending,
		CreatedAt: now,
		ExpiresAt: now.Add(ToolListRequestTTL),
	}
	if err := c.db.WithContext(ctx).Create(request).Error; err != nil {
		return nil, fmt.Errorf("failed to create tool list request: %w", err)
	}
	return request, nil
}

// ToolListRequest returns userID's tool list request with id. A request that expired, or belongs
// to another user, is not found.
func (c *Client) ToolListRequest(ctx context.Context, userID, id string) (*types.ToolListRequest, error) {
	request := new(types.ToolListRequest)
	if err := c.db.WithContext(ctx).Where("id = ? AND user_id = ? AND expires_at > ?", id, userID, time.Now()).First(request).Error; err != nil {
		return nil, err
	}
	return request, nil
}

// CompleteToolListRequest records the result of userID's pending tool list request with id: its
// tools, or why they couldn't be listed. A request is answered once.
func (c *Client) CompleteToolListRequest(ctx context.Context, userID, id string, tools []types2.MCPServerTool, listErr error) error {
	update := types.ToolListRequest{Status: types.ToolListRequestCompleted, Tools: tools}
	if listErr != nil {
		update = types.ToolListRequest{Status: types.ToolListRequestFailed, Error: listErr.Error()}
	}
	result := c.db.WithContext(ctx).Model(&types.ToolListRequest{}).
		Where("id = ? AND user_id = ? AND status = ? AND expires_at > ?", id, userID, types.ToolListRequestPending, time.Now()).
		Select("status", "tools", "error").
		Updates(&update)
	if result.Error != nil {
		return fmt.Errorf("failed to complete tool list request: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		if _, err := c.ToolListRequest(ctx, userID, id); errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		return ErrToolListRequestNotPending
	}
	return nil
}

// CleanupExpiredToolListRequests deletes tool list requests that expired.
func (c *Client) CleanupExpiredToolListRequests(ctx context.Context) error {
	return c.db.WithContext(ctx).Delete(&types.ToolListRequest{}, "expires_at < ?", time.Now()).Error
}

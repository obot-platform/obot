package client

import (
	"context"
	"fmt"
	"strconv"

	api "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"gorm.io/gorm"
)

// EnsureClaudeTagUser runs under the provisioning lock. Service users have no
// login identity or verified email, so provider login cannot adopt the account.
func (c *Client) EnsureClaudeTagUser(ctx context.Context, environment *types.ClaudeEnvironment) error {
	if environment.ClaudeTagUserID != "" {
		user, err := c.UserByID(ctx, environment.ClaudeTagUserID)
		if err != nil {
			return err
		}
		if !user.Internal {
			return gorm.ErrRecordNotFound
		}
		return nil
	}
	verified := false
	user := types.User{
		Username:       "claude-tag:" + environment.ID,
		HashedUsername: hash.String("claude-tag:" + environment.ID),
		DisplayName:    "Claude Tag — " + environment.DisplayName,
		Role:           api.RoleBasic,
		Internal:       true,
		VerifiedEmail:  &verified,
	}
	if err := c.encryptUser(ctx, &user); err != nil {
		return err
	}
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		environment.ClaudeTagUserID = fmt.Sprint(user.ID)
		return tx.Model(&types.ClaudeEnvironment{}).Where("id = ?", environment.ID).Update("claude_tag_user_id", environment.ClaudeTagUserID).Error
	})
}

// Claude Tag identities are owned and deleted by their environment, not user management.
func refuseClaudeTagUserMutation(tx *gorm.DB, userID string) error {
	id, err := strconv.ParseUint(userID, 10, 64)
	if err != nil {
		return api.NewErrHTTP(400, "invalid user ID")
	}

	var count int64
	if err := tx.Model(&types.ClaudeEnvironment{}).Where("claude_tag_user_id = ?", strconv.FormatUint(id, 10)).Count(&count).Error; err != nil {
		return err
	}
	if count != 0 {
		return api.NewErrHTTP(403, "Claude Tag service users are managed through their Claude environment")
	}

	return nil
}

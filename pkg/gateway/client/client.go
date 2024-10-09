package client

import (
	"context"
	"errors"
	"strconv"

	"github.com/gptscript-ai/otto/pkg/gateway/db"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"k8s.io/apiserver/pkg/authentication/authenticator"
)

type Client struct {
	db          *db.DB
	adminEmails map[string]struct{}
	nextAuth    authenticator.Request
}

func New(db *db.DB, adminEmails []string) *Client {
	adminEmailsSet := make(map[string]struct{}, len(adminEmails))
	for _, email := range adminEmails {
		adminEmailsSet[email] = struct{}{}
	}
	return &Client{
		db:          db,
		adminEmails: adminEmailsSet,
	}
}

func (c *Client) Close() error {
	return c.db.Close()
}

// EnsureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func (c *Client) EnsureIdentity(ctx context.Context, id *types.Identity) (*types.User, error) {
	role := types.RoleBasic
	if _, ok := c.adminEmails[id.Email]; ok {
		role = types.RoleAdmin
	}

	var user *types.User
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		user, err = EnsureIdentity(tx, id, role)
		return err
	}); err != nil {
		return nil, err
	}

	return user, nil
}

// EnsureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func EnsureIdentity(tx *gorm.DB, id *types.Identity, role types.Role) (*types.User, error) {
	if err := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider_username"}, {Name: "auth_provider_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"email"}),
	}, clause.Returning{Columns: []clause.Column{{Name: "user_id"}}}).Create(id).Error; err != nil {
		return nil, err
	}

	user := &types.User{
		ID:       id.UserID,
		Username: id.ProviderUsername,
		Email:    id.Email,
		Role:     role,
	}
	if user.Role == types.RoleUnknown {
		user.Role = types.RoleBasic
	}

	userQuery := tx
	if user.ID != 0 {
		userQuery = userQuery.Where("id = ?", user.ID)
	} else {
		userQuery = userQuery.Where("username = ?", user.Username)
	}

	if err := userQuery.First(&user).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.Create(&user).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if user.Role != role {
		user.Role = role
		if err := tx.Updates(user).Error; err != nil {
			return nil, err
		}
	}

	if id.UserID != user.ID {
		id.UserID = user.ID
		if err := tx.Updates(id).Error; err != nil {
			return nil, err
		}
	}

	return user, nil
}

func firstValue(m map[string][]string, key string) string {
	values := m[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func firstValueAsInt(m map[string][]string, key string) int {
	value := firstValue(m, key)
	v, _ := strconv.Atoi(value)
	return v
}

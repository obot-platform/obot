package client

import (
	"context"
	"errors"

	types2 "github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/gateway/types"
	"gorm.io/gorm"
)

// EnsureIdentity ensures that the given identity exists in the database, and returns the user associated with it.
func (c *Client) EnsureIdentity(ctx context.Context, id *types.Identity) (*types.User, error) {
	role := types2.RoleBasic
	if _, ok := c.adminEmails[id.Email]; ok {
		role = types2.RoleAdmin
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
func EnsureIdentity(tx *gorm.DB, id *types.Identity, role types2.Role) (*types.User, error) {
	email := id.Email
	if err := tx.First(id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		if err = tx.Create(id).Error; err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	} else if id.Email != email {
		id.Email = email
		if err = tx.Updates(id).Error; err != nil {
			return nil, err
		}
	}

	user := &types.User{
		ID:       id.UserID,
		Username: id.ProviderUsername,
		Email:    id.Email,
		Role:     role,
	}
	if user.Role == types2.RoleUnknown {
		user.Role = types2.RoleBasic
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

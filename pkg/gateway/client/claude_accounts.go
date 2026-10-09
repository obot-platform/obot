package client

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func (c *Client) ClaudeOrganizationAccounts(ctx context.Context, organization string) ([]types.ClaudeOrganizationAccount, error) {
	accounts := []types.ClaudeOrganizationAccount{}
	err := c.db.WithContext(ctx).Where("organization_id = ?", organization).Order("account").Find(&accounts).Error
	return accounts, err
}

func (c *Client) ClaudeOrganizationAccountForUser(ctx context.Context, organization, user string) (*types.ClaudeOrganizationAccount, error) {
	var account types.ClaudeOrganizationAccount
	if user == "" {
		return nil, gorm.ErrRecordNotFound
	}
	err := c.db.WithContext(ctx).Where("organization_id = ? AND user_id = ?", organization, user).First(&account).Error
	return &account, err
}

// SaveClaudeOrganizationAccounts runs under the provisioning lock. Omitted accounts
// are preserved; an explicit empty UserID removes a mapping. Validate the merged
// result before writing so reassignments and swaps are atomic.
func (c *Client) SaveClaudeOrganizationAccounts(ctx context.Context, organization string, accounts []types.ClaudeOrganizationAccount) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing []types.ClaudeOrganizationAccount
		if err := tx.Where("organization_id = ?", organization).Find(&existing).Error; err != nil {
			return err
		}
		merged := map[string]types.ClaudeOrganizationAccount{}
		for _, account := range existing {
			merged[account.Account] = account
		}
		for _, account := range accounts {
			account.OrganizationID = organization
			merged[account.Account] = account
		}

		users := map[string]bool{}
		for _, account := range merged {
			if account.UserID == "" {
				continue
			}
			if users[account.UserID] {
				return gorm.ErrDuplicatedKey
			}
			users[account.UserID] = true
			var user types.User
			if err := tx.Where("id = ? AND deleted_at IS NULL", account.UserID).First(&user).Error; err != nil {
				return fmt.Errorf("mapped Obot user %s: %w", account.UserID, err)
			}
			if user.Internal {
				var serviceEnvironments int64
				if err := tx.Model(&types.ClaudeEnvironment{}).Where("claude_tag_user_id = ?", account.UserID).Count(&serviceEnvironments).Error; err != nil {
					return err
				}
				if serviceEnvironments != 0 {
					return fmt.Errorf("service users cannot be mapped to human accounts: %w", gorm.ErrRecordNotFound)
				}
			}
		}

		for _, account := range accounts {
			account.OrganizationID = organization
			if err := tx.Save(&account).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Client) ClaudeOrganizationAccount(ctx context.Context, organization, account string) (*types.ClaudeOrganizationAccount, error) {
	var mapping types.ClaudeOrganizationAccount
	err := c.db.WithContext(ctx).Where("organization_id = ? AND account = ? AND user_id <> ''", organization, account).First(&mapping).Error
	return &mapping, err
}

// MatchClaudeOrganizationAccount runs under the provisioning lock shared with admin
// edits. Only the authenticated orchestrator supplies the account and email.
func (c *Client) MatchClaudeOrganizationAccount(ctx context.Context, organization, account, email string) (*types.ClaudeOrganizationAccount, error) {
	var mapping types.ClaudeOrganizationAccount
	err := c.db.WithContext(ctx).Where("organization_id = ? AND account = ?", organization, account).First(&mapping).Error
	if err == nil && mapping.UserID != "" {
		return &mapping, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	email = strings.TrimSpace(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 320 || account == "" {
		return nil, gorm.ErrRecordNotFound
	}

	// Decrypt through Users: stored emails may be encrypted and hashes preserve case.
	users, err := c.Users(ctx, types.UserQuery{})
	if err != nil {
		return nil, err
	}
	var owner string
	for _, user := range users {
		if user.Internal || user.DisabledAt != nil || !strings.EqualFold(strings.TrimSpace(user.Email), email) {
			continue
		}
		if owner != "" {
			return nil, gorm.ErrRecordNotFound
		}
		owner = fmt.Sprint(user.ID)
	}
	if owner == "" {
		return nil, gorm.ErrRecordNotFound
	}
	if _, err := c.ClaudeOrganizationAccountForUser(ctx, organization, owner); err == nil {
		return nil, gorm.ErrRecordNotFound
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	mapping.OrganizationID = organization
	mapping.Account = account
	mapping.Email = email
	mapping.UserID = owner
	if err := c.SaveClaudeOrganizationAccounts(ctx, organization, []types.ClaudeOrganizationAccount{mapping}); err != nil {
		return nil, err
	}
	return &mapping, nil
}

package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestClaudeOrganizationMappings(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	for _, user := range []types.User{
		{ID: 1, Username: "first", HashedUsername: "first"},
		{ID: 2, Username: "second", HashedUsername: "second"},
	} {
		require.NoError(t, c.db.WithContext(ctx).Create(&user).Error)
	}
	require.NoError(t, c.SaveClaudeOrganizationAccounts(ctx, "org", []types.ClaudeOrganizationAccount{
		{Account: "account", UserID: "1"},
		{Account: "unmapped", UserID: ""},
	}))
	// Reimports preserve omitted rows.
	require.NoError(t, c.SaveClaudeOrganizationAccounts(ctx, "org", nil))
	accounts, err := c.ClaudeOrganizationAccounts(ctx, "org")
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	_, err = c.ClaudeOrganizationAccount(ctx, "org", "account")
	require.NoError(t, err)

	// A conflict or missing user rolls back the whole submission.
	require.ErrorIs(t, c.SaveClaudeOrganizationAccounts(ctx, "org", []types.ClaudeOrganizationAccount{
		{Account: "unmapped", UserID: "1"},
	}), gorm.ErrDuplicatedKey)
	require.ErrorIs(t, c.SaveClaudeOrganizationAccounts(ctx, "org", []types.ClaudeOrganizationAccount{
		{Account: "account", UserID: "2"},
		{Account: "unmapped", UserID: "999"},
	}), gorm.ErrRecordNotFound)
	mapping, err := c.ClaudeOrganizationAccountForUser(ctx, "org", "1")
	require.NoError(t, err)
	require.Equal(t, "account", mapping.Account)

	// Reassignment revokes the old owner's mapping.
	require.NoError(t, c.SaveClaudeOrganizationAccounts(ctx, "org", []types.ClaudeOrganizationAccount{
		{Account: "account", UserID: "2"},
	}))
	_, err = c.ClaudeOrganizationAccountForUser(ctx, "org", "1")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	require.NoError(t, c.SaveClaudeOrganizationAccounts(ctx, "org", []types.ClaudeOrganizationAccount{
		{Account: "account", UserID: ""},
	}))
	_, err = c.ClaudeOrganizationAccountForUser(ctx, "org", "2")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = c.ClaudeOrganizationAccountForUser(ctx, "org", "")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
}

func TestMatchClaudeOrganizationAccount(t *testing.T) {
	for _, tc := range []struct {
		name    string
		email   string
		prepare func(*testing.T, *Client)
		want    string
	}{
		{
			name:  "case insensitive trimmed email",
			email: "  PERSON@EXAMPLE.COM  ",
			want:  "1",
		},
		{
			name: "missing email",
		},
		{
			name:  "unknown email",
			email: "missing@example.com",
		},
		{
			name:  "malformed email",
			email: "Person <person@example.com>",
		},
		{
			name:  "disabled user",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.db.WithContext(t.Context()).Model(&types.User{}).Where("id = 1").Update("disabled_at", time.Now()).Error)
			},
		},
		{
			name:  "deleted user",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.db.WithContext(t.Context()).Model(&types.User{}).Where("id = 1").Update("deleted_at", time.Now()).Error)
			},
		},
		{
			name:  "service user",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.db.WithContext(t.Context()).Model(&types.User{}).Where("id = 1").Update("internal", true).Error)
			},
		},
		{
			name:  "ambiguous email",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.db.WithContext(t.Context()).Model(&types.User{}).Where("id = 2").Update("email", "PERSON@example.com").Error)
			},
		},
		{
			name:  "user already mapped in this organization",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.SaveClaudeOrganizationAccounts(t.Context(), "org", []types.ClaudeOrganizationAccount{{Account: "another", UserID: "1"}}))
			},
		},
		{
			name:  "mapping in another organization is independent",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.SaveClaudeOrganizationAccounts(t.Context(), "other-org", []types.ClaudeOrganizationAccount{{Account: "another", UserID: "1"}}))
			},
			want: "1",
		},
		{
			name:  "saved unmapped account matches by email",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.SaveClaudeOrganizationAccounts(t.Context(), "org", []types.ClaudeOrganizationAccount{{Account: "user_test", UUID: "uuid", DisplayName: "Imported name"}}))
			},
			want: "1",
		},
		{
			name:  "admin mapping wins over email",
			email: "person@example.com",
			prepare: func(t *testing.T, c *Client) {
				t.Helper()

				require.NoError(t, c.SaveClaudeOrganizationAccounts(t.Context(), "org", []types.ClaudeOrganizationAccount{{Account: "user_test", UserID: "2"}}))
			},
			want: "2",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := newTestClient(t)
			require.NoError(t, c.db.WithContext(t.Context()).Create(&types.User{ID: 1, Username: "person", HashedUsername: "person", Email: "person@example.com"}).Error)
			require.NoError(t, c.db.WithContext(t.Context()).Create(&types.User{ID: 2, Username: "other", HashedUsername: "other", Email: "other@example.com"}).Error)
			if tc.prepare != nil {
				tc.prepare(t, c)
			}
			mapping, err := c.MatchClaudeOrganizationAccount(t.Context(), "org", "user_test", tc.email)
			if tc.want == "" {
				require.ErrorIs(t, err, gorm.ErrRecordNotFound)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, mapping.UserID)
			stored, err := c.ClaudeOrganizationAccount(t.Context(), "org", "user_test")
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.UserID)
			if tc.name == "saved unmapped account matches by email" {
				require.Equal(t, "uuid", stored.UUID)
				require.Equal(t, "Imported name", stored.DisplayName)
			}
		})
	}
}

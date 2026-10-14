package client

import (
	"context"
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

func TestListCredentialsContextPrefixIsLiteralAndScoped(t *testing.T) {
	c := newTestClient(t)
	for _, context := range []string{"owner_%!one", "owner_%!two", "owner-other", "another-owner"} {
		require.NoError(t, c.UpsertCredential(t.Context(), types.Credential{Context: context, Name: "oauth", Secrets: map[string]string{"CLIENT_SECRET": "secret"}}))
	}
	credentials, err := c.ListCredentials(t.Context(), ListCredentialsOptions{CredentialContextPrefix: "owner_%!"})
	require.NoError(t, err)
	require.Len(t, credentials, 2)
	for _, credential := range credentials {
		require.Contains(t, []string{"owner_%!one", "owner_%!two"}, credential.Context)
		require.NotEqual(t, "secret", credential.Secrets["CLIENT_SECRET"])
	}
}

func TestDeleteCredentialAndOAuthAuthorizationIsAtomic(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	credential := types.Credential{
		Context: "component",
		Name:    "oauth",
		Secrets: map[string]string{"CLIENT_ID": "client"},
	}
	conf := &oauth2.Config{ClientID: "client"}
	require.NoError(t, c.UpsertCredential(ctx, credential))
	require.NoError(t, c.ReplaceMCPOAuthToken(ctx, "user", "mcp", "url", "", conf, &oauth2.Token{AccessToken: "token"}))
	require.NoError(t, c.CreateMCPOAuthPendingState(ctx, "user", "mcp", "url", "", "pending", "verifier", "", conf))
	require.NoError(t, c.ReplaceMCPOAuthToken(ctx, "user", "other", "url", "", conf, &oauth2.Token{AccessToken: "unrelated"}))
	notifications := 0
	c.mcpOAuthTokenTrigger = func(context.Context, string) error {
		notifications++
		return nil
	}
	failure := errors.New("token cleanup failed")
	require.NoError(t, c.db.WithContext(ctx).Callback().Delete().Before("gorm:delete").Register("test:fail_token_cleanup", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "MCPOAuthToken" {
			_ = tx.AddError(failure)
		}
	}))
	t.Cleanup(func() { _ = c.db.WithContext(ctx).Callback().Delete().Remove("test:fail_token_cleanup") })
	deleted, err := c.DeleteCredentialAndMCPOAuthTokens(ctx, credential.Context, credential.Name, []string{"mcp"})
	require.ErrorIs(t, err, failure)
	require.False(t, deleted)
	_, err = c.RevealCredential(ctx, []string{credential.Context}, credential.Name)
	require.NoError(t, err, "credential deletion must roll back with failed cleanup")
	_, err = c.GetMCPOAuthPendingState(ctx, "pending")
	require.NoError(t, err, "pending state deletion must also roll back")
	require.Zero(t, notifications)
	require.NoError(t, c.db.WithContext(ctx).Callback().Delete().Remove("test:fail_token_cleanup"))
	deleted, err = c.DeleteCredentialAndMCPOAuthTokens(ctx, credential.Context, credential.Name, []string{"mcp"})
	require.NoError(t, err)
	require.True(t, deleted)
	require.Equal(t, 1, notifications)
	_, err = c.GetMCPOAuthToken(ctx, "user", "mcp", "url")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = c.GetMCPOAuthPendingState(ctx, "pending")
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = c.GetMCPOAuthToken(ctx, "user", "other", "url")
	require.NoError(t, err)
	require.NoError(t, c.ReplaceMCPOAuthToken(ctx, "user", "mcp", "url", "", conf, &oauth2.Token{AccessToken: "reauthorized"}))
	notifications = 0
	deleted, err = c.DeleteCredentialAndMCPOAuthTokens(ctx, credential.Context, credential.Name, []string{"mcp"})
	require.NoError(t, err)
	require.False(t, deleted)
	token, err := c.GetMCPOAuthToken(ctx, "user", "mcp", "url")
	require.NoError(t, err)
	require.Equal(t, "reauthorized", token.AccessToken)
	require.Zero(t, notifications)
}

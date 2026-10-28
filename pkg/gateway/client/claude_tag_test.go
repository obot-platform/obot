package client

import (
	"fmt"
	"testing"
	"time"

	api "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	"k8s.io/apiserver/pkg/storage/value"
)

func TestClaudeTagServiceUser(t *testing.T) {
	c := newTestClient(t)
	c.encryptionConfig = &encryptionconfig.EncryptionConfiguration{
		Transformers: map[schema.GroupResource]value.Transformer{credentialGroupResource: testTransformer{}},
	}
	ctx := t.Context()
	environment := &types.ClaudeEnvironment{
		ID:             "env",
		DisplayName:    "Test environment",
		EnvironmentID:  "ccpool_test",
		OrganizationID: "org_test",
	}
	require.NoError(t, c.SaveClaudeEnvironment(ctx, environment, nil))
	require.NoError(t, c.EnsureClaudeTagUser(ctx, environment))
	user, err := c.UserByID(ctx, environment.ClaudeTagUserID)
	require.NoError(t, err)
	require.True(t, user.Internal)
	require.Equal(t, api.RoleBasic, user.Role)
	require.Empty(t, user.Email)
	require.NotNil(t, user.VerifiedEmail)
	require.False(t, *user.VerifiedEmail)
	credential, err := c.EnsureClaudeCredential(ctx, user.ID, "tag-session", []string{"vmcp_test"}, time.Now().Add(time.Hour))
	require.NoError(t, err)
	key, err := c.ValidateAPIKey(ctx, credential.Secrets["key"])
	require.NoError(t, err)
	require.Equal(t, user.ID, key.UserID)
	require.Equal(t, []string{"vmcp_test"}, key.MCPServerIDs)

	// An active session can renew after retention cleanup removes its expired key.
	require.NoError(t, c.db.WithContext(ctx).Model(key).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	_, err = c.ValidateAPIKey(ctx, credential.Secrets["key"])
	require.ErrorIs(t, err, ErrInvalidAPIKey)
	credential, err = c.EnsureClaudeCredential(ctx, user.ID, "tag-session", []string{"vmcp_test"}, time.Now().Add(time.Hour))
	require.NoError(t, err)
	renewed, err := c.ValidateAPIKey(ctx, credential.Secrets["key"])
	require.NoError(t, err)
	require.NotEqual(t, key.ID, renewed.ID)
	require.EqualValues(t, 1, countAPIKeys(t, c))
	// Keep only the referenced revocation marker, including beyond normal retention.
	credential, err = c.EnsureClaudeCredential(ctx, user.ID, "tag-session", []string{"vmcp_changed"}, time.Now().Add(time.Hour))
	require.NoError(t, err)
	current, err := c.ValidateAPIKey(ctx, credential.Secrets["key"])
	require.NoError(t, err)
	require.NoError(t, c.RevokeAPIKeyByID(ctx, current.ID))
	require.NoError(t, c.db.WithContext(ctx).Model(&types.APIKey{}).
		Where("id IN ?", []uint{renewed.ID, current.ID}).Update("expires_at", time.Now().Add(-time.Minute)).Error)
	c.auditLogDeleteBatchSize = 1
	require.NoError(t, c.db.WithContext(ctx).Create(&types.APIKey{
		ClaudeEnvironment: true,
		ExpiresAt:         new(time.Now().Add(-time.Minute)),
	}).Error)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	require.EqualValues(t, 1, countAPIKeys(t, c))
	require.NoError(t, c.deleteOldRevokedAPIKeys(ctx, time.Now().AddDate(0, 0, 100), 90))
	_, err = c.EnsureClaudeCredential(ctx, user.ID, "tag-session", []string{"vmcp_changed"}, time.Now().Add(time.Hour))
	require.ErrorIs(t, err, ErrClaudeCredentialRevoked)
	_, err = c.DeleteCredential(ctx, "substrate-agent", "tag-session")
	require.NoError(t, err)
	require.NoError(t, c.cleanupRetainedData(ctx, time.Now(), 0, 0))
	require.EqualValues(t, 0, countAPIKeys(t, c))
	info, err := c.UserInfoByID(ctx, user.ID)
	require.NoError(t, err)
	require.NotContains(t, info.GetGroups(), api.GroupAdmin)

	originalID := environment.ClaudeTagUserID
	require.NoError(t, c.EnsureClaudeTagUser(ctx, environment))
	require.Equal(t, originalID, environment.ClaudeTagUserID)

	// Human member mappings must not claim a service identity.
	require.ErrorIs(t, c.SaveClaudeOrganizationAccounts(ctx, environment.OrganizationID, []types.ClaudeOrganizationAccount{
		{Account: "user", UserID: originalID},
	}), gorm.ErrRecordNotFound)

	other := &types.ClaudeEnvironment{
		ID:            "other",
		DisplayName:   "Other environment",
		EnvironmentID: "ccpool_other",
	}
	require.NoError(t, c.SaveClaudeEnvironment(ctx, other, nil))
	require.NoError(t, c.EnsureClaudeTagUser(ctx, other))
	require.NotEqual(t, originalID, other.ClaudeTagUserID)
	require.NoError(t, c.DeleteClaudeEnvironment(ctx, environment.ID))
	_, err = c.UserByID(ctx, originalID)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	_, err = c.UserByID(ctx, other.ClaudeTagUserID)
	require.NoError(t, err)
}

func TestClaudeTagUsersAreManagedByEnvironment(t *testing.T) {
	c := newTestClient(t)
	ctx := t.Context()
	environment := &types.ClaudeEnvironment{ID: "env", EnvironmentID: "ccpool_test"}
	require.NoError(t, c.SaveClaudeEnvironment(ctx, environment, nil))
	require.NoError(t, c.EnsureClaudeTagUser(ctx, environment))

	for _, id := range []string{environment.ClaudeTagUserID, "0" + environment.ClaudeTagUserID} {
		_, err := c.UpdateUser(ctx, true, &types.User{Role: api.RoleAdmin}, id)
		require.ErrorContains(t, err, "managed through their Claude environment")
		require.ErrorContains(t, c.DeleteUser(ctx, id), "managed through their Claude environment")
		require.ErrorContains(t, c.UpdateUserInternalStatus(ctx, id, false), "managed through their Claude environment")
	}

	// The service identity remains available for attribution and profile selection.
	user, err := c.UserByID(ctx, environment.ClaudeTagUserID)
	require.NoError(t, err)
	require.Equal(t, api.RoleBasic, user.Role)
	require.True(t, user.Internal)
	require.NoError(t, c.EnsureClaudeTagUser(ctx, environment))
}

func TestClaudeTagUsersDoNotConsumeSeats(t *testing.T) {
	c := newIdentityUserLimitTestClient(t)
	ctx := t.Context()
	environment := &types.ClaudeEnvironment{ID: "env", EnvironmentID: "ccpool_test"}
	require.NoError(t, c.SaveClaudeEnvironment(ctx, environment, nil))
	require.NoError(t, c.EnsureClaudeTagUser(ctx, environment))

	user, err := ensureUserLimitTestIdentity(ctx, c, "human", "human@example.com", UserLimit{Maximum: 1})
	require.NoError(t, err)
	// Marking a human internal must not bypass seat limits.
	require.NoError(t, c.UpdateUserInternalStatus(ctx, fmt.Sprint(user.ID), true))
	count, err := c.UserCount(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	count, err = countUsersTowardLimit(c.db.WithContext(ctx))
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	_, err = ensureUserLimitTestIdentity(ctx, c, "other", "other@example.com", UserLimit{Maximum: 1})
	requireIdentityUserLimitForbiddenError(t, err)
}

func TestClaudeTagGuardRejectsMalformedUserID(t *testing.T) {
	for _, id := range []string{"not-a-number", "-1", "18446744073709551616"} {
		t.Run(id, func(t *testing.T) {
			err := refuseClaudeTagUserMutation(nil, id)
			var httpErr *api.ErrHTTP
			require.ErrorAs(t, err, &httpErr)
			require.Equal(t, 400, httpErr.Code)
		})
	}
}

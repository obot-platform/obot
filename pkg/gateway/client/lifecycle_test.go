package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

var (
	lifecycleTestProvider = AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}
	lifecycleTestLocalProvider = AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      system.LocalAuthProvider,
	}
)

func newLifecycleTestClient(t *testing.T, objects ...kclient.Object) *Client {
	t.Helper()

	c := newTestClient(t)
	c.storageClient = fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(append([]kclient.Object{
			&v1.UserDefaultRoleSetting{
				Namespace: system.DefaultNamespace,
				Name:      system.DefaultRoleSettingName,
				Spec: v1.UserDefaultRoleSettingSpec{
					Role: apitypes.RoleBasic,
				},
			},
		}, objects...)...).
		Build()
	return c
}

// createLifecycleTestUser creates a user with an identity for provider.
func createLifecycleTestUser(t *testing.T, c *Client, username string, provider AuthProviderRef) *types.User {
	t.Helper()

	email := username + "@example.com"
	user := &types.User{
		Username:       username,
		HashedUsername: hash.String(username),
		Email:          email,
		HashedEmail:    hash.String(email),
		Role:           apitypes.RoleBasic,
	}
	if err := c.db.WithContext(t.Context()).Create(user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	providerUserID := "00u-" + username
	if err := c.db.WithContext(t.Context()).Create(&types.Identity{
		AuthProviderName:      provider.Name,
		AuthProviderNamespace: provider.Namespace,
		ProviderUsername:      username,
		ProviderUserID:        providerUserID,
		HashedProviderUserID:  hash.String(providerUserID),
		Email:                 email,
		HashedEmail:           hash.String(email),
		UserID:                user.ID,
	}).Error; err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}

	return user
}

// disableUser disables a user of provider in a transaction of its own, as SCIM does within its own transactions when
// the identity provider deactivates them, and returns the decrypted user. A user who is already disabled only gets
// the new reason.
func disableUser(t *testing.T, c *Client, provider AuthProviderRef, userID uint, reason types.UserDisabledReason) (*types.User, error) {
	t.Helper()

	var (
		user    *types.User
		changed bool
	)
	if err := c.db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		user, changed, err = disableUserTx(tx, provider, userID, reason)
		return err
	}); err != nil {
		return nil, err
	}
	if changed {
		c.kickUserLifecycleDelivery()
	}
	return user, c.decryptUser(t.Context(), user)
}

// reactivateUser reactivates a disabled user of provider in a transaction of its own, as SCIM does within its own
// transactions when the identity provider reactivates them, and returns the decrypted user.
func reactivateUser(t *testing.T, c *Client, provider AuthProviderRef, userID uint) (*types.User, error) {
	t.Helper()

	var user *types.User
	if err := c.db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		var err error
		user, _, err = reactivateUserTx(tx, provider, userID)
		return err
	}); err != nil {
		return nil, err
	}
	return user, c.decryptUser(t.Context(), user)
}

func storedLifecycleUser(t *testing.T, c *Client, userID uint) types.User {
	t.Helper()

	var user types.User
	if err := c.db.WithContext(t.Context()).Where("id = ?", userID).Take(&user).Error; err != nil {
		t.Fatalf("failed to read user %d: %v", userID, err)
	}
	return user
}

func lifecycleEvents(t *testing.T, c *Client, userID uint) []types.UserLifecycleEvent {
	t.Helper()

	var events []types.UserLifecycleEvent
	if err := c.db.WithContext(t.Context()).Where("user_id = ?", userID).Order("id").Find(&events).Error; err != nil {
		t.Fatalf("failed to list lifecycle events: %v", err)
	}
	return events
}

func TestDisableUserKeepsTheAccountAndReactivateRestoresIt(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)

	if err := c.db.WithContext(ctx).Create(&types.GroupMemberships{
		UserID:  user.ID,
		GroupID: "okta/00g-engineering",
	}).Error; err != nil {
		t.Fatalf("failed to create membership: %v", err)
	}
	if err := c.db.WithContext(ctx).Create(&types.AuthToken{
		ID:          "token-1",
		UserID:      user.ID,
		HashedToken: hash.String("secret"),
	}).Error; err != nil {
		t.Fatalf("failed to create auth token: %v", err)
	}
	apiKey, err := c.CreateAPIKey(ctx, user.ID, "cli", "", nil, types.APIKeyScopes{
		CanAccessAPI: true,
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	disabled, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive)
	if err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if disabled.ID != user.ID || disabled.Status() != apitypes.UserStatusDisabled || disabled.DisabledReason != types.UserDisabledReasonSCIMInactive {
		t.Fatalf("disabled user = %+v, want user %d disabled as %q", disabled, user.ID, types.UserDisabledReasonSCIMInactive)
	}
	if disabled.Email != user.Email {
		t.Errorf("disabled user email = %q, want the decrypted %q", disabled.Email, user.Email)
	}

	stored := storedLifecycleUser(t, c, user.ID)
	if stored.DisabledAt == nil || stored.DisabledReason != types.UserDisabledReasonSCIMInactive || stored.DeletedAt != nil {
		t.Fatalf("stored user after disable = %+v, want disabled and not deleted", stored)
	}
	if stored.Username != user.Username || stored.HashedUsername != user.HashedUsername || stored.Role != user.Role {
		t.Errorf("stored user profile changed after disable: %+v", stored)
	}

	var authTokens int64
	if err := c.db.WithContext(ctx).Model(new(types.AuthToken)).Where("user_id = ?", user.ID).Count(&authTokens).Error; err != nil {
		t.Fatalf("failed to count auth tokens: %v", err)
	}
	if authTokens != 0 {
		t.Errorf("auth tokens after disable = %d, want 0", authTokens)
	}

	var memberships int64
	if err := c.db.WithContext(ctx).Model(new(types.GroupMemberships)).Where("user_id = ?", user.ID).Count(&memberships).Error; err != nil {
		t.Fatalf("failed to count memberships: %v", err)
	}
	if memberships != 1 {
		t.Errorf("memberships after disable = %d, want 1", memberships)
	}

	if _, err := c.GetAPIKeyByID(ctx, apiKey.ID); err != nil {
		t.Errorf("API key after disable: %v, want it kept", err)
	}

	events := lifecycleEvents(t, c, user.ID)
	if len(events) != 1 || events[0].Type != types.UserLifecycleEventDisabled || events[0].DeliveredAt != nil {
		t.Fatalf("lifecycle events after disable = %+v, want one undelivered disabled event", events)
	}

	reactivated, err := reactivateUser(t, c, lifecycleTestProvider, user.ID)
	if err != nil {
		t.Fatalf("failed to reactivate user: %v", err)
	}
	if reactivated.ID != user.ID || reactivated.Status() != apitypes.UserStatusActive || reactivated.DisabledReason != "" {
		t.Fatalf("reactivated user = %+v, want user %d active", reactivated, user.ID)
	}

	stored = storedLifecycleUser(t, c, user.ID)
	if stored.DisabledAt != nil || stored.DisabledReason != "" {
		t.Errorf("stored user after reactivation = %+v, want enabled", stored)
	}
	if got := lifecycleEvents(t, c, user.ID); len(got) != 1 {
		t.Errorf("lifecycle events after reactivation = %d, want 1", len(got))
	}
	if _, err := c.GetAPIKeyByID(ctx, apiKey.ID); err != nil {
		t.Errorf("API key after reactivation: %v, want it kept", err)
	}
}

func TestDisableUserTwiceOnlyUpdatesTheReason(t *testing.T) {
	c := newLifecycleTestClient(t)
	user := createLifecycleTestUser(t, c, "bob", lifecycleTestProvider)

	first, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMUnprovisioned)
	if err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	second, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive)
	if err != nil {
		t.Fatalf("failed to disable user again: %v", err)
	}

	if second.DisabledReason != types.UserDisabledReasonSCIMInactive {
		t.Errorf("disable reason = %q, want %q", second.DisabledReason, types.UserDisabledReasonSCIMInactive)
	}
	if !second.DisabledAt.Equal(*first.DisabledAt) {
		t.Errorf("disabled at changed from %v to %v", first.DisabledAt, second.DisabledAt)
	}
	if events := lifecycleEvents(t, c, user.ID); len(events) != 1 {
		t.Errorf("lifecycle events = %d, want 1", len(events))
	}
}

func TestLifecycleChangesAreScopedToTheAuthProvider(t *testing.T) {
	c := newLifecycleTestClient(t)
	localUser := createLifecycleTestUser(t, c, "carol", lifecycleTestLocalProvider)

	_, err := disableUser(t, c, lifecycleTestProvider, localUser.ID, types.UserDisabledReasonSCIMInactive)
	if _, ok := errors.AsType[*UserOutsideAuthProviderError](err); !ok {
		t.Fatalf("disable error = %v, want *UserOutsideAuthProviderError", err)
	}
	_, err = reactivateUser(t, c, lifecycleTestProvider, localUser.ID)
	if _, ok := errors.AsType[*UserOutsideAuthProviderError](err); !ok {
		t.Fatalf("reactivate error = %v, want *UserOutsideAuthProviderError", err)
	}

	if stored := storedLifecycleUser(t, c, localUser.ID); stored.DisabledAt != nil || stored.DisabledReason != "" {
		t.Errorf("local user after refused disable = %+v, want unchanged", stored)
	}
	if events := lifecycleEvents(t, c, localUser.ID); len(events) != 0 {
		t.Errorf("lifecycle events = %d, want 0", len(events))
	}
}

func TestLifecycleChangesLeaveDeletedUsersDeleted(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "dave", lifecycleTestProvider)
	createLifecycleTestUser(t, c, "owner", lifecycleTestProvider)

	if err := c.DeleteUser(ctx, fmt.Sprint(user.ID)); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}

	_, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive)
	if _, ok := errors.AsType[*UserDeletedError](err); !ok {
		t.Fatalf("disable error = %v, want *UserDeletedError", err)
	}
	_, err = reactivateUser(t, c, lifecycleTestProvider, user.ID)
	if _, ok := errors.AsType[*UserDeletedError](err); !ok {
		t.Fatalf("reactivate error = %v, want *UserDeletedError", err)
	}

	if stored := storedLifecycleUser(t, c, user.ID); stored.Status() != apitypes.UserStatusDeleted || stored.DisabledAt != nil {
		t.Errorf("deleted user after refused lifecycle changes = %+v, want deleted and never disabled", stored)
	}
}

func TestDisableUserRejectsUnknownReasons(t *testing.T) {
	c := newLifecycleTestClient(t)
	user := createLifecycleTestUser(t, c, "erin", lifecycleTestProvider)

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, "admin_block"); err == nil {
		t.Fatal("expected an unknown disable reason to be refused")
	}
	if stored := storedLifecycleUser(t, c, user.ID); stored.DisabledAt != nil {
		t.Errorf("user after refused disable = %+v, want enabled", stored)
	}
}

func TestLoginProfileRefreshNeverChangesLifecycleState(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/obot-get-user-info" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"name": "Frank",
		})
	}))
	t.Cleanup(provider.Close)

	c := newLifecycleTestClient(t)
	ctx := accesstoken.ContextWithAccessToken(t.Context(), "provider-access-token")
	user := createLifecycleTestUser(t, c, "frank", lifecycleTestProvider)

	refreshProfile := func(stale types.User) {
		t.Helper()
		if err := c.UpdateProfileIfNeeded(ctx, &stale, lifecycleTestProvider.Name, lifecycleTestProvider.Namespace, provider.URL); err != nil {
			t.Fatalf("failed to refresh profile: %v", err)
		}
		// Force the next refresh to fetch the profile again.
		if err := c.db.WithContext(ctx).Model(new(types.Identity)).Where("user_id = ?", user.ID).
			UpdateColumn("icon_last_checked", time.Time{}).Error; err != nil {
			t.Fatalf("failed to reset profile check: %v", err)
		}
	}

	// A copy read while the user was disabled must not re-disable them after reactivation.
	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	staleDisabled := storedLifecycleUser(t, c, user.ID)
	if _, err := reactivateUser(t, c, lifecycleTestProvider, user.ID); err != nil {
		t.Fatalf("failed to reactivate user: %v", err)
	}
	refreshProfile(staleDisabled)
	stored := storedLifecycleUser(t, c, user.ID)
	if stored.DisplayName != "Frank" {
		t.Fatalf("display name after profile refresh = %q, want the refreshed profile", stored.DisplayName)
	}
	if stored.DisabledAt != nil || stored.DisabledReason != "" {
		t.Fatalf("user after profile refresh = %+v, want still enabled", stored)
	}

	// And a copy read while the user was enabled must not re-enable them after they are disabled.
	staleEnabled := storedLifecycleUser(t, c, user.ID)
	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	refreshProfile(staleEnabled)
	if stored := storedLifecycleUser(t, c, user.ID); stored.DisabledAt == nil || stored.DisabledReason != types.UserDisabledReasonSCIMInactive {
		t.Fatalf("user after profile refresh = %+v, want still disabled", stored)
	}
}

func TestCredentialsAreNotIssuedToInactiveUsers(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	disabled := createLifecycleTestUser(t, c, "gina", lifecycleTestProvider)
	deleted := createLifecycleTestUser(t, c, "hank", lifecycleTestProvider)
	createLifecycleTestUser(t, c, "owner", lifecycleTestProvider)

	if _, err := disableUser(t, c, lifecycleTestProvider, disabled.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if err := c.DeleteUser(ctx, fmt.Sprint(deleted.ID)); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}

	for _, tt := range []struct {
		name       string
		userID     uint
		wantStatus apitypes.UserStatus
	}{
		{
			name:       "disabled",
			userID:     disabled.ID,
			wantStatus: apitypes.UserStatusDisabled,
		},
		{
			name:       "deleted",
			userID:     deleted.ID,
			wantStatus: apitypes.UserStatusDeleted,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for name, create := range map[string]func() error{
				"personal API key": func() error {
					_, err := c.CreateAPIKey(ctx, tt.userID, "cli", "", nil, types.APIKeyScopes{
						CanAccessAPI: true,
					})
					return err
				},
				"hosted agent API key": func() error {
					_, err := c.CreateHostedAgentAPIKey(ctx, "hai1-test", tt.userID, "agent")
					return err
				},
				"persistent token": func() error {
					return c.CheckCredentialOwner(ctx, tt.userID)
				},
			} {
				err := create()
				denied, ok := errors.AsType[*UserAccessDeniedError](err)
				if !ok || denied.Status != tt.wantStatus {
					t.Errorf("%s: error = %v, want a denial for a %s user", name, err, tt.wantStatus)
				}
			}
		})
	}
}

func TestValidateAPIKeyReportsItsOwnersCurrentStatus(t *testing.T) {
	c := newLifecycleTestClient(t)
	c.apiKeyCache = make(map[[32]byte]apiKeyValidationCacheEntry)
	c.apiKeyCacheTTL = time.Minute
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "ivan", lifecycleTestProvider)

	created, err := c.CreateAPIKey(ctx, user.ID, "cli", "", nil, types.APIKeyScopes{
		CanAccessAPI: true,
		MCPServerIDs: []string{"ms1-first", "ms1-second"},
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	assertOwnerStatus := func(want apitypes.UserStatus) {
		t.Helper()
		key, err := c.ValidateAPIKey(ctx, created.Key)
		if err != nil {
			t.Fatalf("failed to validate API key: %v", err)
		}
		if key.OwnerStatus != want {
			t.Fatalf("owner status = %q, want %q", key.OwnerStatus, want)
		}
		// The key is read together with its user, and must come back whole.
		if key.ID != created.ID || key.UserID != user.ID || !key.CanAccessAPI || !slices.Equal(key.MCPServerIDs, []string{"ms1-first", "ms1-second"}) {
			t.Fatalf("validated key = %+v, want the key as created", key)
		}
	}

	// The first validation misses the cache, and the rest hit it. Both read the owner with the key.
	assertOwnerStatus(apitypes.UserStatusActive)
	assertOwnerStatus(apitypes.UserStatusActive)

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	assertOwnerStatus(apitypes.UserStatusDisabled)

	c.invalidateValidatedAPIKeysByID(created.ID)
	assertOwnerStatus(apitypes.UserStatusDisabled)

	if _, err := reactivateUser(t, c, lifecycleTestProvider, user.ID); err != nil {
		t.Fatalf("failed to reactivate user: %v", err)
	}
	assertOwnerStatus(apitypes.UserStatusActive)

	// A key that outlived its user row has no owner, which is treated as deleted.
	if err := c.db.WithContext(ctx).Where("id = ?", user.ID).Delete(new(types.User)).Error; err != nil {
		t.Fatalf("failed to remove user row: %v", err)
	}
	assertOwnerStatus(apitypes.UserStatusDeleted)
	c.invalidateValidatedAPIKeysByID(created.ID)
	assertOwnerStatus(apitypes.UserStatusDeleted)
}

func TestValidateAPIKeyDistinguishesInvalidKeysFromLookupFailures(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "jane", lifecycleTestProvider)

	created, err := c.CreateAPIKey(ctx, user.ID, "cli", "", nil, types.APIKeyScopes{
		CanAccessAPI: true,
	})
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	for _, key := range []string{
		"ok1-not-a-key",
		fmt.Sprintf("ok1-%d-%d-wrong-secret", user.ID, created.ID),
		fmt.Sprintf("ok1-%d-%d-secret", user.ID, created.ID+100),
	} {
		if _, err := c.ValidateAPIKey(ctx, key); !errors.Is(err, ErrInvalidAPIKey) {
			t.Errorf("validate %q error = %v, want ErrInvalidAPIKey", key, err)
		}
	}

	if err := c.RevokeAPIKeyByID(ctx, created.ID); err != nil {
		t.Fatalf("failed to revoke API key: %v", err)
	}
	if _, err := c.ValidateAPIKey(ctx, created.Key); !errors.Is(err, ErrInvalidAPIKey) {
		t.Errorf("validate revoked key error = %v, want ErrInvalidAPIKey", err)
	}

	if err := c.db.Close(); err != nil {
		t.Fatalf("failed to close database: %v", err)
	}
	if _, err := c.ValidateAPIKey(ctx, created.Key); err == nil || errors.Is(err, ErrInvalidAPIKey) {
		t.Errorf("validate with a closed database error = %v, want a lookup failure", err)
	}
}

func TestDeliverDisabledEventDeletesOnlyTheUsersRefreshTokens(t *testing.T) {
	c := newLifecycleTestClient(t,
		&v1.OAuthToken{
			Namespace: system.DefaultNamespace,
			Name:      "disabled-user-token",
			Spec: v1.OAuthTokenSpec{
				ClientID: "client",
				UserID:   1,
			},
		},
		&v1.OAuthToken{
			Namespace: system.DefaultNamespace,
			Name:      "other-user-token",
			Spec: v1.OAuthTokenSpec{
				ClientID: "client",
				UserID:   2,
			},
		},
	)
	ctx := t.Context()
	disabled := createLifecycleTestUser(t, c, "kate", lifecycleTestProvider)
	other := createLifecycleTestUser(t, c, "liam", lifecycleTestProvider)
	if disabled.ID != 1 || other.ID != 2 {
		t.Fatalf("user IDs = %d and %d, want 1 and 2", disabled.ID, other.ID)
	}

	if _, err := disableUser(t, c, lifecycleTestProvider, disabled.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	assertOAuthTokenExists(t, c, "disabled-user-token", false)
	assertOAuthTokenExists(t, c, "other-user-token", true)
	events := lifecycleEvents(t, c, disabled.ID)
	if len(events) != 1 || events[0].DeliveredAt == nil {
		t.Fatalf("lifecycle events after delivery = %+v, want one delivered event", events)
	}

	// A repeated delivery, from another replica or after a crash before the event was marked delivered, finds
	// nothing left to delete.
	if err := c.deliverUserDisabledEvent(ctx, events[0], c.listOAuthTokensOnce(ctx)); err != nil {
		t.Fatalf("failed to deliver the event again: %v", err)
	}
	assertOAuthTokenExists(t, c, "other-user-token", true)
}

func TestDeliverDisabledEventForAReactivatedUserDeletesNothing(t *testing.T) {
	c := newLifecycleTestClient(t, &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      "reactivated-user-token",
		Spec: v1.OAuthTokenSpec{
			ClientID: "client",
			UserID:   1,
		},
	})
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "mia", lifecycleTestProvider)

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if _, err := reactivateUser(t, c, lifecycleTestProvider, user.ID); err != nil {
		t.Fatalf("failed to reactivate user: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	assertOAuthTokenExists(t, c, "reactivated-user-token", true)
	if events := lifecycleEvents(t, c, user.ID); len(events) != 1 || events[0].DeliveredAt == nil {
		t.Fatalf("lifecycle events after delivery = %+v, want one delivered event", events)
	}
}

func TestFailedLifecycleDeliveryIsRetriedAfterItsClaimExpires(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "noah", lifecycleTestProvider)

	failing := true
	c.storageClient = interceptor.NewClient(fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(), interceptor.Funcs{
		List: func(ctx context.Context, client kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if failing {
				return errors.New("storage unavailable")
			}
			return client.List(ctx, list, opts...)
		},
	})

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	events := lifecycleEvents(t, c, user.ID)
	if len(events) != 1 || events[0].DeliveredAt != nil || events[0].Attempts != 1 || events[0].LastError == "" {
		t.Fatalf("lifecycle events after failed delivery = %+v, want one undelivered event with one failed attempt", events)
	}
	if events[0].ClaimedUntil == nil || !events[0].ClaimedUntil.After(time.Now()) {
		t.Fatalf("claim after failed delivery = %v, want it held until the retry", events[0].ClaimedUntil)
	}

	// The claim holds the event until it expires, even once storage recovers.
	failing = false
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}
	if events := lifecycleEvents(t, c, user.ID); events[0].DeliveredAt != nil {
		t.Fatal("event was delivered while another attempt still held its claim")
	}

	if err := c.db.WithContext(ctx).Model(new(types.UserLifecycleEvent)).Where("id = ?", events[0].ID).
		UpdateColumn("claimed_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("failed to expire claim: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}
	if events := lifecycleEvents(t, c, user.ID); events[0].DeliveredAt == nil || events[0].LastError != "" {
		t.Fatalf("lifecycle events after retry = %+v, want the event delivered", events)
	}
}

func TestLifecycleEventClaimsAreExclusive(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "olga", lifecycleTestProvider)

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	event := lifecycleEvents(t, c, user.ID)[0]

	now := time.Now()
	claimed, err := c.claimUserLifecycleEvents(ctx, []uint{event.ID}, now)
	if err != nil || len(claimed) != 1 || claimed[0].ID != event.ID {
		t.Fatalf("first claim = %+v, %v; want the event claimed", claimed, err)
	}
	claimed, err = c.claimUserLifecycleEvents(ctx, []uint{event.ID}, now)
	if err != nil || len(claimed) != 0 {
		t.Fatalf("second claim = %+v, %v; want refused", claimed, err)
	}
}

func TestDeliverReconcileEventCreatesRoleAndGroupChanges(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	stayed := createLifecycleTestUser(t, c, "paul", lifecycleTestProvider)
	left := createLifecycleTestUser(t, c, "quinn", lifecycleTestProvider)

	for user, groupsRemoved := range map[*types.User]bool{
		stayed: false,
		left:   true,
	} {
		if err := recordUserReconcileEvent(c.db.WithContext(ctx), user.ID, groupsRemoved); err != nil {
			t.Fatalf("failed to record reconcile event: %v", err)
		}
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	var events []types.UserLifecycleEvent
	for _, user := range []*types.User{stayed, left} {
		events = append(events, lifecycleEvents(t, c, user.ID)...)
	}
	if len(events) != 2 {
		t.Fatalf("lifecycle events = %d, want 2", len(events))
	}
	for _, event := range events {
		user := stayed
		if event.UserID == left.ID {
			user = left
		}
		if event.DeliveredAt == nil {
			t.Fatalf("event %d was not delivered", event.ID)
		}

		var roleChange v1.UserRoleChange
		if err := c.storageClient.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      userLifecycleObjectName(system.UserRoleChangePrefix, event.ID),
		}, &roleChange); err != nil || roleChange.Spec.UserID != user.ID {
			t.Errorf("role change for event %d = %+v, %v; want one for user %d", event.ID, roleChange.Spec, err, user.ID)
		}

		var groupChange v1.UserGroupChange
		err := c.storageClient.Get(ctx, kclient.ObjectKey{
			Namespace: system.DefaultNamespace,
			Name:      userLifecycleObjectName(system.UserGroupChangePrefix, event.ID),
		}, &groupChange)
		if event.GroupsRemoved && (err != nil || groupChange.Spec.UserID != user.ID) {
			t.Errorf("group change for event %d = %+v, %v; want one for user %d", event.ID, groupChange.Spec, err, user.ID)
		} else if !event.GroupsRemoved && !apierrors.IsNotFound(err) {
			t.Errorf("group change for event %d: %v, want none", event.ID, err)
		}
	}

	// A repeated delivery creates nothing new.
	if err := c.deliverUserReconcileEvent(ctx, events[1]); err != nil {
		t.Fatalf("failed to deliver the event again: %v", err)
	}
}

func TestSignInRecordsTheFirstSignInOnce(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	provider := lifecycleTestLocalProvider

	signIn := func() *types.Identity {
		t.Helper()
		identity := &types.Identity{
			AuthProviderName:      provider.Name,
			AuthProviderNamespace: provider.Namespace,
			ProviderUsername:      "quinn",
			ProviderUserID:        "quinn",
			Email:                 "quinn@example.com",
		}
		if _, err := c.EnsureIdentity(ctx, identity, "", UserLimit{Unlimited: true}); err != nil {
			t.Fatalf("failed to sign in: %v", err)
		}
		return storedIdentity(t, c, provider, "quinn")
	}

	first := signIn()
	if first.FirstSignInAt == nil {
		t.Fatal("first sign-in was not recorded")
	}
	second := signIn()
	if second.FirstSignInAt == nil || !second.FirstSignInAt.Equal(*first.FirstSignInAt) {
		t.Fatalf("first sign-in after another sign-in = %v, want %v", second.FirstSignInAt, first.FirstSignInAt)
	}
}

func TestSignInRecordsTheFirstSignInOfAnExistingIdentity(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "rosa", lifecycleTestProvider)

	if identity := storedIdentity(t, c, lifecycleTestProvider, "00u-rosa"); identity.FirstSignInAt != nil {
		t.Fatalf("identity created before sign-in has first sign-in %v, want none", identity.FirstSignInAt)
	}

	signedIn, err := c.EnsureIdentity(ctx, &types.Identity{
		AuthProviderName:      lifecycleTestProvider.Name,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		ProviderUsername:      "rosa",
		ProviderUserID:        "00u-rosa",
		Email:                 "rosa@example.com",
	}, "", UserLimit{Unlimited: true})
	if err != nil {
		t.Fatalf("failed to sign in: %v", err)
	}
	if signedIn.ID != user.ID {
		t.Fatalf("signed in as user %d, want %d", signedIn.ID, user.ID)
	}
	if identity := storedIdentity(t, c, lifecycleTestProvider, "00u-rosa"); identity.FirstSignInAt == nil {
		t.Fatal("first sign-in of an existing identity was not recorded")
	}
}

func TestSignInOfADisabledUserReturnsTheirStatus(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "sam", lifecycleTestProvider)

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}

	signedIn, err := c.EnsureIdentity(ctx, &types.Identity{
		AuthProviderName:      lifecycleTestProvider.Name,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		ProviderUsername:      "sam",
		ProviderUserID:        "00u-sam",
		Email:                 "sam@example.com",
	}, "", UserLimit{Unlimited: true})
	if err != nil {
		t.Fatalf("failed to sign in: %v", err)
	}
	if signedIn.ID != user.ID || signedIn.Status() != apitypes.UserStatusDisabled {
		t.Fatalf("signed-in user = %+v, want user %d disabled", signedIn, user.ID)
	}
	if stored := storedLifecycleUser(t, c, user.ID); stored.DisabledAt == nil {
		t.Fatal("sign-in re-enabled a disabled user")
	}
}

func TestSignInCreatesAUserWithTheExplicitRoleOfTheAssertedEmail(t *testing.T) {
	c := newLifecycleTestClient(t)
	c.emailsWithExplicitRoles = map[string]apitypes.Role{
		"owner@example.com": apitypes.RoleOwner,
	}

	user, err := c.EnsureIdentity(t.Context(), &types.Identity{
		AuthProviderName:      lifecycleTestProvider.Name,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		ProviderUsername:      "owner",
		ProviderUserID:        "00u-owner",
		Email:                 "owner@example.com",
	}, "", UserLimit{
		Unlimited: true,
	})
	if err != nil {
		t.Fatalf("failed to sign in: %v", err)
	}
	if stored := storedLifecycleUser(t, c, user.ID); !stored.Role.HasRole(apitypes.RoleOwner) {
		t.Fatalf("role of the user that sign-in created = %d, want Owner", stored.Role)
	}
}

func TestSignInThatRaisesARoleRecordsAReconcileEvent(t *testing.T) {
	c := newLifecycleTestClient(t)
	c.emailsWithExplicitRoles = map[string]apitypes.Role{
		"tara@example.com": apitypes.RoleAdmin,
	}
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "tara", lifecycleTestProvider)

	signIn := func() {
		t.Helper()
		if _, err := c.EnsureIdentity(ctx, &types.Identity{
			AuthProviderName:      lifecycleTestProvider.Name,
			AuthProviderNamespace: lifecycleTestProvider.Namespace,
			ProviderUsername:      "tara",
			ProviderUserID:        "00u-tara",
			Email:                 "tara@example.com",
		}, "", UserLimit{Unlimited: true}); err != nil {
			t.Fatalf("failed to sign in: %v", err)
		}
	}

	signIn()
	if stored := storedLifecycleUser(t, c, user.ID); !stored.Role.HasRole(apitypes.RoleAdmin) {
		t.Fatalf("role after sign-in = %d, want Admin", stored.Role)
	}
	events := lifecycleEvents(t, c, user.ID)
	if len(events) != 1 || events[0].Type != types.UserLifecycleEventReconcile || events[0].GroupsRemoved {
		t.Fatalf("lifecycle events after a role-raising sign-in = %+v, want one reconcile event", events)
	}

	// A sign-in that leaves the role unchanged records nothing.
	signIn()
	if events := lifecycleEvents(t, c, user.ID); len(events) != 1 {
		t.Fatalf("lifecycle events after a second sign-in = %d, want 1", len(events))
	}
}

func TestHasSignedInOwner(t *testing.T) {
	provider := lifecycleTestProvider

	for _, tt := range []struct {
		name     string
		role     apitypes.Role
		username string
		provider AuthProviderRef
		signedIn bool
		disabled bool
		want     bool
	}{
		{
			name:     "owner who signed in",
			role:     apitypes.RoleOwner,
			username: "owner",
			provider: provider,
			signedIn: true,
			want:     true,
		},
		{
			name:     "owner and auditor who signed in",
			role:     apitypes.RoleOwner | apitypes.RoleAuditor,
			username: "owner",
			provider: provider,
			signedIn: true,
			want:     true,
		},
		{
			name:     "owner who never signed in",
			role:     apitypes.RoleOwner,
			username: "owner",
			provider: provider,
			want:     false,
		},
		{
			name:     "disabled owner who signed in",
			role:     apitypes.RoleOwner,
			username: "owner",
			provider: provider,
			signedIn: true,
			disabled: true,
			want:     true,
		},
		{
			name:     "owner of another provider",
			role:     apitypes.RoleOwner,
			username: "owner",
			provider: lifecycleTestLocalProvider,
			signedIn: true,
			want:     false,
		},
		{
			name:     "admin who signed in",
			role:     apitypes.RoleAdmin,
			username: "admin",
			provider: provider,
			signedIn: true,
			want:     false,
		},
		{
			name:     "bootstrap user",
			role:     apitypes.RoleOwner,
			username: system.BootstrapName,
			provider: provider,
			signedIn: true,
			want:     false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newLifecycleTestClient(t)
			ctx := t.Context()
			user := createLifecycleTestUser(t, c, tt.username, tt.provider)
			if err := c.db.WithContext(ctx).Model(user).UpdateColumn("role", tt.role).Error; err != nil {
				t.Fatalf("failed to set role: %v", err)
			}
			if tt.signedIn {
				if err := c.db.WithContext(ctx).Model(new(types.Identity)).Where("user_id = ?", user.ID).
					UpdateColumn("first_sign_in_at", time.Now()).Error; err != nil {
					t.Fatalf("failed to record sign-in: %v", err)
				}
			}
			if tt.disabled {
				if _, err := disableUser(t, c, tt.provider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
					t.Fatalf("failed to disable user: %v", err)
				}
			}

			got, err := c.HasSignedInOwner(ctx, provider.Name)
			if err != nil {
				t.Fatalf("failed to check for an owner: %v", err)
			}
			if got != tt.want {
				t.Errorf("HasSignedInOwner = %v, want %v", got, tt.want)
			}
		})
	}
}

func storedIdentity(t *testing.T, c *Client, provider AuthProviderRef, providerUserID string) *types.Identity {
	t.Helper()

	var identity types.Identity
	if err := c.db.WithContext(t.Context()).
		Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", provider.Namespace, provider.Name, hash.String(providerUserID)).
		Take(&identity).Error; err != nil {
		t.Fatalf("failed to read identity: %v", err)
	}
	return &identity
}

func assertOAuthTokenExists(t *testing.T, c *Client, name string, want bool) {
	t.Helper()

	err := c.storageClient.Get(t.Context(), kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      name,
	}, new(v1.OAuthToken))
	if want && err != nil {
		t.Errorf("OAuth token %q: %v, want it kept", name, err)
	} else if !want && !apierrors.IsNotFound(err) {
		t.Errorf("OAuth token %q: %v, want it deleted", name, err)
	}
}

func TestUserDecoratorRecordsTheUsersStatusOverAnyProviderValue(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "uma", lifecycleTestProvider)

	decorator := NewUserDecorator(
		authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{
				User: &kuser.DefaultInfo{
					Name: "uma",
					UID:  "00u-uma",
					Extra: map[string][]string{
						"email":                   {"uma@example.com"},
						"auth_provider_name":      {lifecycleTestProvider.Name},
						"auth_provider_namespace": {lifecycleTestProvider.Namespace},
						// An auth provider must not be able to vouch for the user's status.
						principal.UserStatusExtra: {string(apitypes.UserStatusActive)},
					},
				},
			}, true, nil
		}),
		c,
		userLimitProviderFunc(func(context.Context) (UserLimit, error) {
			return UserLimit{Unlimited: true}, nil
		}),
	)

	authenticate := func() apitypes.UserStatus {
		t.Helper()
		resp, ok, err := decorator.AuthenticateRequest(httptest.NewRequestWithContext(ctx, http.MethodGet, "/", nil))
		if err != nil || !ok {
			t.Fatalf("authenticate = %v, %v; want the principal returned for the admission check", ok, err)
		}
		if resp.User.GetUID() != fmt.Sprint(user.ID) {
			t.Fatalf("principal UID = %q, want %d", resp.User.GetUID(), user.ID)
		}
		status, recorded := principal.UserStatus(resp.User)
		if !recorded {
			t.Fatal("no status was recorded")
		}
		return status
	}

	if got := authenticate(); got != apitypes.UserStatusActive {
		t.Fatalf("status of an active user = %q, want %q", got, apitypes.UserStatusActive)
	}

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if got := authenticate(); got != apitypes.UserStatusDisabled {
		t.Fatalf("status of a disabled user = %q, want %q", got, apitypes.UserStatusDisabled)
	}
}

func TestUserDecoratorFailsWhenItCannotReadTheUser(t *testing.T) {
	c := newLifecycleTestClient(t)
	decorator := NewUserDecorator(
		authenticator.RequestFunc(func(*http.Request) (*authenticator.Response, bool, error) {
			return &authenticator.Response{
				User: &kuser.DefaultInfo{
					Name: "vera",
					UID:  "00u-vera",
					Extra: map[string][]string{
						"email":                   {"vera@example.com"},
						"auth_provider_name":      {lifecycleTestProvider.Name},
						"auth_provider_namespace": {lifecycleTestProvider.Namespace},
					},
				},
			}, true, nil
		}),
		c,
		userLimitProviderFunc(func(context.Context) (UserLimit, error) {
			return UserLimit{Unlimited: true}, nil
		}),
	)

	if err := c.db.Close(); err != nil {
		t.Fatalf("failed to close database: %v", err)
	}

	resp, ok, err := decorator.AuthenticateRequest(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil))
	if _, isLookup := errors.AsType[*UserAccessLookupError](err); resp != nil || ok || !isLookup {
		t.Fatalf("authenticate with an unreadable database = %v, %v, %v; want a lookup failure", resp, ok, err)
	}
}

func TestDeliverDisabledEventEndsTheUsersSessions(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "wes", lifecycleTestLocalProvider)

	localUser, err := c.CreateLocalAuthUser(ctx, user.Email, "password-hash", false)
	if err != nil {
		t.Fatalf("failed to create local auth user: %v", err)
	}
	if err := c.CreateLocalAuthSession(ctx, "session-hash", localUser.ID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	if _, err := disableUser(t, c, lifecycleTestLocalProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	// The session is gone, so reactivating the user does not bring it back.
	if _, _, err := c.LocalAuthSession(ctx, "session-hash"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("session after delivery: %v, want it deleted", err)
	}
	if events := lifecycleEvents(t, c, user.ID); len(events) != 1 || events[0].DeliveredAt == nil {
		t.Fatalf("lifecycle events after delivery = %+v, want one delivered event", events)
	}
}

func TestSessionDeletionUnsupported(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "unsupported",
			err:  errors.Join(LogoutAllErr{}),
			want: true,
		},
		{
			name: "unsupported and failed",
			err:  errors.Join(errors.New("failed to delete local auth sessions"), LogoutAllErr{}),
			want: false,
		},
		{
			name: "failed",
			err:  errors.New("failed to get auth provider"),
			want: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionDeletionUnsupported(tt.err); got != tt.want {
				t.Errorf("sessionDeletionUnsupported(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestGatewayTokenOfADeletedUserIsDenied(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "xena", lifecycleTestProvider)
	createLifecycleTestUser(t, c, "owner", lifecycleTestProvider)
	if err := c.db.WithContext(ctx).Create(&types.AuthToken{
		ID:                    "token-1",
		UserID:                user.ID,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		AuthProviderName:      lifecycleTestProvider.Name,
		HashedToken:           hash.String("secret"),
	}).Error; err != nil {
		t.Fatalf("failed to create auth token: %v", err)
	}

	if u, _, _, _, _, err := c.UserFromToken(ctx, "token-1:secret"); err != nil || u.ID != user.ID {
		t.Fatalf("user from token = %v, %v; want user %d", u, err, user.ID)
	}
	if _, _, _, _, _, err := c.UserFromToken(ctx, "token-1:wrong"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("user from an unknown token: %v, want not found", err)
	}

	if err := c.DeleteUser(ctx, fmt.Sprint(user.ID)); err != nil {
		t.Fatalf("failed to delete user: %v", err)
	}
	_, _, _, _, _, err := c.UserFromToken(ctx, "token-1:secret")
	if denied, ok := errors.AsType[*UserAccessDeniedError](err); !ok || denied.Status != apitypes.UserStatusDeleted {
		t.Fatalf("user from a deleted user's token: %v, want a denial", err)
	}
}

func TestADeniedSignInIsNotRecordedAsASignIn(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	// An Owner whose identity exists before they ever signed in, as provisioning creates it.
	owner := createLifecycleTestUser(t, c, "yuri", lifecycleTestProvider)
	if err := c.db.WithContext(ctx).Model(owner).UpdateColumn("role", apitypes.RoleOwner).Error; err != nil {
		t.Fatalf("failed to make the user an Owner: %v", err)
	}
	if _, err := disableUser(t, c, lifecycleTestProvider, owner.ID, types.UserDisabledReasonSCIMUnprovisioned); err != nil {
		t.Fatalf("failed to disable the Owner: %v", err)
	}

	signIn := func() *types.User {
		t.Helper()
		u, err := c.EnsureIdentity(ctx, &types.Identity{
			AuthProviderName:      lifecycleTestProvider.Name,
			AuthProviderNamespace: lifecycleTestProvider.Namespace,
			ProviderUsername:      "yuri",
			ProviderUserID:        "00u-yuri",
			Email:                 "yuri@example.com",
		}, "", UserLimit{Unlimited: true})
		if err != nil {
			t.Fatalf("failed to sign in: %v", err)
		}
		return u
	}
	assertHasSignedInOwner := func(want bool) {
		t.Helper()
		got, err := c.HasSignedInOwner(ctx, lifecycleTestProvider.Name)
		if err != nil {
			t.Fatalf("failed to check for an owner: %v", err)
		}
		if got != want {
			t.Fatalf("HasSignedInOwner = %v, want %v", got, want)
		}
	}

	// The disabled Owner's attempt is denied, so it must not count as signing in.
	if u := signIn(); u.Status() != apitypes.UserStatusDisabled {
		t.Fatalf("signed-in user status = %q, want disabled", u.Status())
	}
	if identity := storedIdentity(t, c, lifecycleTestProvider, "00u-yuri"); identity.FirstSignInAt != nil {
		t.Fatalf("a denied sign-in was recorded at %v", identity.FirstSignInAt)
	}

	if _, err := reactivateUser(t, c, lifecycleTestProvider, owner.ID); err != nil {
		t.Fatalf("failed to reactivate the Owner: %v", err)
	}
	assertHasSignedInOwner(false)

	signIn()
	if identity := storedIdentity(t, c, lifecycleTestProvider, "00u-yuri"); identity.FirstSignInAt == nil {
		t.Fatal("the reactivated Owner's sign-in was not recorded")
	}
	assertHasSignedInOwner(true)
}

func TestEnableUser(t *testing.T) {
	github := AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "github-auth-provider",
	}

	tests := []struct {
		name string
		// setup returns the user to enable.
		setup       func(t *testing.T, c *Client) uint
		wantManaged bool
		wantDeleted bool
		wantEnabled bool
	}{
		{
			name: "a user that SCIM disabled, whose provider has no SCIM connection any more",
			setup: func(t *testing.T, c *Client) uint {
				t.Helper()
				user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)
				if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
					t.Fatal(err)
				}
				return user.ID
			},
			wantEnabled: true,
		},
		{
			name: "a user who is not disabled",
			setup: func(t *testing.T, c *Client) uint {
				t.Helper()
				return createLifecycleTestUser(t, c, "alice", lifecycleTestProvider).ID
			},
			wantEnabled: true,
		},
		{
			name: "a user whom SCIM provisions, and deactivated",
			setup: func(t *testing.T, c *Client) uint {
				t.Helper()
				conn, _ := createTestSCIMConnection(t, c, true)
				user := provisionTestSCIMUser(t, c, conn, "00u-alice", "alice@example.com")
				if _, err := disableUser(t, c, lifecycleTestProvider, user.UserID, types.UserDisabledReasonSCIMInactive); err != nil {
					t.Fatal(err)
				}
				return user.UserID
			},
			wantManaged: true,
		},
		{
			name: "an unprovisioned user of a provider that has a SCIM connection",
			setup: func(t *testing.T, c *Client) uint {
				t.Helper()
				user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)
				if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMUnprovisioned); err != nil {
					t.Fatal(err)
				}
				createTestSCIMConnection(t, c, true)
				return user.ID
			},
			wantManaged: true,
		},
		{
			name: "a user of another provider than the SCIM connection's",
			setup: func(t *testing.T, c *Client) uint {
				t.Helper()
				createTestSCIMConnection(t, c, true)
				user := createLifecycleTestUser(t, c, "alice", github)
				if _, err := disableUser(t, c, github, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
					t.Fatal(err)
				}
				return user.ID
			},
			wantEnabled: true,
		},
		{
			name: "a deleted user",
			setup: func(t *testing.T, c *Client) uint {
				t.Helper()
				user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)
				if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
					t.Fatal(err)
				}
				if err := c.db.WithContext(t.Context()).Model(user).UpdateColumn("deleted_at", time.Now()).Error; err != nil {
					t.Fatal(err)
				}
				return user.ID
			},
			wantDeleted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newLifecycleTestClient(t)
			userID := tt.setup(t, c)
			before := storedLifecycleUser(t, c, userID)

			enabled, err := c.EnableUser(t.Context(), userID)
			if _, managed := errors.AsType[*SCIMManagedUserError](err); managed != tt.wantManaged {
				t.Fatalf("EnableUser() error = %v, want SCIM managed %v", err, tt.wantManaged)
			}
			if _, deleted := errors.AsType[*UserDeletedError](err); deleted != tt.wantDeleted {
				t.Fatalf("EnableUser() error = %v, want deleted %v", err, tt.wantDeleted)
			}
			if tt.wantEnabled && (err != nil || enabled.DisabledAt != nil || enabled.Email != "alice@example.com") {
				t.Fatalf("EnableUser() = %+v, %v, want the decrypted user enabled", enabled, err)
			}

			got := storedLifecycleUser(t, c, userID)
			if tt.wantEnabled && (got.DisabledAt != nil || got.DisabledReason != "") {
				t.Fatalf("user after EnableUser() = %+v, want enabled", got)
			}
			if !tt.wantEnabled && (got.DisabledAt == nil || got.DisabledReason != before.DisabledReason) {
				t.Fatalf("user after a refused EnableUser() = %+v, want unchanged", got)
			}
		})
	}

	if _, err := newLifecycleTestClient(t).EnableUser(t.Context(), 4242); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("EnableUser() of an unknown user = %v, want gorm.ErrRecordNotFound", err)
	}
}

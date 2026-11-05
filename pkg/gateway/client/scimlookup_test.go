package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"github.com/obot-platform/obot/pkg/scim/adapter/adaptertest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	lookupTestProviderName = "lookup-test-auth-provider"
	lookupTestGroupPrefix  = "lookup/"
)

var (
	lookupTestProvider = AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      lookupTestProviderName,
	}
)

// lookupIDStub stands in for an auth provider whose user info reports the access token as the user's group lookup ID,
// so that each test sign-in chooses its own.
type lookupIDStub struct {
	*httptest.Server
	// failing makes the user info endpoint fail.
	failing atomic.Bool
}

// entraPictureTest is a user that SCIM provisioned through Entra, and an Entra stub whose user info reports the
// picture in iconURL, running during while it answers.
type entraPictureTest struct {
	c       *Client
	userID  uint
	srv     *httptest.Server
	iconURL atomic.Value
	during  atomic.Pointer[func()]
}

func newLookupIDStub(t *testing.T) *lookupIDStub {
	t.Helper()

	stub := new(lookupIDStub)
	stub.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/obot-get-user-info":
			if stub.failing.Load() {
				http.Error(w, "unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "),
				"name": "Login Name",
			})
		case "/obot-list-user-auth-groups":
			_ = json.NewEncoder(w).Encode([]auth.GroupInfo{})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(stub.Close)
	return stub
}

// newLookupTestClient returns a client that encrypts identities and users, for an auth provider whose adapter matches
// SCIM users by group lookup ID, binds groups by externalId, and deletes users.
func newLookupTestClient(t *testing.T) *Client {
	t.Helper()

	adapter.RegisterForTest(t, lookupTestProviderName, adaptertest.Adapter{
		Users:  adapter.UserKeyGroupLookupID,
		Groups: adapter.GroupKeyExternalID,
		SharedRules: adapter.Rules{
			DeletesUsers: true,
		},
	})

	c := newLifecycleTestClient(t)
	encryptResources(t, c, identityGroupResource, userGroupResource)
	// SCIM matches the provider's users by group lookup ID only once the backfill of their hashes has run.
	require.NoError(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()))
	return c
}

func createLookupTestConnection(t *testing.T, c *Client) *types.SCIMConnection {
	t.Helper()

	conn, _, err := c.CreateSCIMConnection(t.Context(), CreateSCIMConnectionOptions{
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		GroupIDPrefix:         lookupTestGroupPrefix,
		Issuer:                "tenant",
		Origin:                types.SCIMConnectionOriginMigrated,
		IssueToken:            true,
	})
	require.NoError(t, err)
	return conn
}

// lookupSignIn signs in through the lookup test provider, with subject sub, as the person whose group lookup ID is
// objectID.
func lookupSignIn(t *testing.T, c *Client, stub *lookupIDStub, sub, objectID string) (*types.User, error) {
	t.Helper()

	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), stub.URL), lookupTestGroupPrefix), objectID)
	return c.EnsureIdentity(ctx, &types.Identity{
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		ProviderUsername:      sub,
		ProviderUserID:        sub,
		Email:                 sub + "@example.com",
	}, "", UserLimit{
		Unlimited: true,
	})
}

func provisionLookupTestUser(t *testing.T, c *Client, conn *types.SCIMConnection, objectID string) (*SCIMUser, error) {
	t.Helper()

	return c.CreateSCIMUser(t.Context(), conn, SCIMUserInput{
		UserName:   objectID + "@example.com",
		ExternalID: objectID,
		Profile: types.SCIMUserProfile{
			DisplayName: "SCIM Name",
		},
	}, SCIMUserCreateOptions{
		UserLimit: UserLimit{
			Unlimited: true,
		},
		DefaultRole: apitypes.RoleBasic,
	})
}

// pendingIdentityCount returns the number of identities that SCIM created for the person with the group lookup ID, which
// wait for their first sign-in.
func pendingIdentityCount(t *testing.T, c *Client, objectID string) int64 {
	t.Helper()

	return countRows(t, c, new(types.Identity), "hashed_provider_user_id = ?", hash.String(adapter.PendingProviderUserID(objectID)))
}

func decryptedUser(t *testing.T, c *Client, userID uint) types.User {
	t.Helper()

	user := storedLifecycleUser(t, c, userID)
	require.NoError(t, c.decryptUser(t.Context(), &user))
	return user
}

func TestSCIMFirstUserIsClaimedAtFirstSignIn(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)

	// SCIM creates the user before they ever sign in, with an identity that waits for their subject.
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-1")
	require.NoError(t, err)
	require.EqualValues(t, 1, pendingIdentityCount(t, c, "oid-1"))
	require.Equal(t, "oid-1", decryptedUser(t, c, provisioned.UserID).Username)

	// SCIM is enforced, which a claimed identity satisfies.
	setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)

	// The first sign-in claims the identity: it is keyed by the subject, and its user gets the subject as username.
	user, err := lookupSignIn(t, c, stub, "sub-1", "oid-1")
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, user.ID)
	require.Zero(t, pendingIdentityCount(t, c, "oid-1"))
	require.EqualValues(t, 1, countRows(t, c, new(types.Identity), "hashed_provider_user_id = ?", hash.String("sub-1")))

	stored := *storedIdentity(t, c, lookupTestProvider, "sub-1")
	require.True(t, stored.Encrypted)
	require.Equal(t, hash.String("oid-1"), stored.HashedProviderGroupLookupID)
	require.NotNil(t, stored.FirstSignInAt)
	require.NoError(t, c.decryptIdentity(t.Context(), &stored))
	require.Equal(t, "sub-1", stored.ProviderUserID)
	require.Equal(t, "sub-1", stored.ProviderUsername)
	require.Equal(t, "oid-1", stored.ProviderGroupLookupID)

	renamed := decryptedUser(t, c, provisioned.UserID)
	require.Equal(t, "sub-1", renamed.Username)
	require.Equal(t, hash.String("sub-1"), renamed.HashedUsername)
	// The rest of the user is encrypted again with the new username's context, and still reads.
	require.Equal(t, "SCIM Name", renamed.DisplayName)

	// Later sign-ins find the identity by the subject.
	again, err := lookupSignIn(t, c, stub, "sub-1", "oid-1")
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, again.ID)

	// SCIM still finds the user by the group lookup ID.
	got, err := c.GetSCIMUser(t.Context(), conn.ID, provisioned.ID)
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, got.UserID)
}

func TestSCIMCreateBindsTheIdentityThatSignInCreated(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)

	// A sign-in before SCIM provisions the person stores the group lookup ID with the identity.
	signedIn, err := lookupSignIn(t, c, stub, "sub-2", "oid-2")
	require.NoError(t, err)
	require.Equal(t, hash.String("oid-2"), storedIdentity(t, c, lookupTestProvider, "sub-2").HashedProviderGroupLookupID)

	// SCIM binds to that user rather than creating another.
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-2")
	require.NoError(t, err)
	require.Equal(t, signedIn.ID, provisioned.UserID)
	require.Zero(t, pendingIdentityCount(t, c, "oid-2"))

	// A second create of the same person is a conflict.
	_, err = provisionLookupTestUser(t, c, conn, "oid-2")
	var conflict *SCIMConflictError
	require.ErrorAs(t, err, &conflict)
}

func TestFirstSignInFailsWithoutAGroupLookupIDOnlyWithAConnection(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	stub.failing.Store(true)

	// Without a connection, there is nothing to claim, so the sign-in goes ahead. SCIM cannot be set up until the
	// user signs in again, which stores the lookup ID.
	_, err := lookupSignIn(t, c, stub, "sub-4", "oid-4")
	require.NoError(t, err)
	stub.failing.Store(false)
	_, err = lookupSignIn(t, c, stub, "sub-4", "oid-4")
	require.NoError(t, err)
	stub.failing.Store(true)

	// With a connection, a first sign-in that cannot learn its group lookup ID could duplicate a provisioned user.
	createLookupTestConnection(t, c)
	_, err = lookupSignIn(t, c, stub, "sub-5", "oid-5")
	require.ErrorContains(t, err, "group lookup ID")
	require.Zero(t, countRows(t, c, new(types.Identity), "hashed_provider_user_id = ?", hash.String("sub-5")))

	// A sign-in whose identity exists does not need it.
	_, err = lookupSignIn(t, c, stub, "sub-4", "oid-4")
	require.NoError(t, err)

	// An auth provider that reports no ID is the same as one that cannot be reached.
	stub.failing.Store(false)
	_, err = lookupSignIn(t, c, stub, "sub-6", "")
	require.ErrorContains(t, err, "group lookup ID")
	require.Zero(t, countRows(t, c, new(types.Identity), "hashed_provider_user_id = ?", hash.String("sub-6")))
}

func TestSCIMCreateRefusesDuplicateUsersWithTheSameGroupLookupID(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)

	// Two users have identities with the same group lookup ID, as a legacy duplicate does.
	_, err := lookupSignIn(t, c, stub, "sub-6a", "oid-6")
	require.NoError(t, err)
	_, err = lookupSignIn(t, c, stub, "sub-6b", "oid-6")
	require.NoError(t, err)

	conn := createLookupTestConnection(t, c)
	_, err = provisionLookupTestUser(t, c, conn, "oid-6")
	var conflict *SCIMConflictError
	require.ErrorAs(t, err, &conflict)
	require.Contains(t, conflict.Message, "duplicates")
}

func TestSCIMCreateStartsANewAccountForADeletedUser(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)

	signedIn, err := lookupSignIn(t, c, stub, "sub-7", "oid-7")
	require.NoError(t, err)
	require.NoError(t, c.DeleteUser(t.Context(), strconv.FormatUint(uint64(signedIn.ID), 10)))

	conn := createLookupTestConnection(t, c)
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-7")
	require.NoError(t, err)
	require.NotEqual(t, signedIn.ID, provisioned.UserID)

	// The identity moved to the new account, which the person's next sign-in reaches.
	require.Equal(t, provisioned.UserID, storedIdentity(t, c, lookupTestProvider, "sub-7").UserID)
	user, err := lookupSignIn(t, c, stub, "sub-7", "oid-7")
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, user.ID)
	// The new account has the username that sign-in gives, not the group lookup ID.
	require.Equal(t, "sub-7", decryptedUser(t, c, provisioned.UserID).Username)
}

func TestDeleteSCIMUser(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)
	ctx := t.Context()

	// An owner who stays, so that the deleted user is not the last one.
	owner, err := lookupSignIn(t, c, stub, "sub-owner", "oid-owner")
	require.NoError(t, err)
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.User)).Where("id = ?", owner.ID).UpdateColumn("role", apitypes.RoleOwner).Error)

	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-8")
	require.NoError(t, err)
	_, err = lookupSignIn(t, c, stub, "sub-8", "oid-8")
	require.NoError(t, err)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.AuthToken{
		ID:                    "token-8",
		UserID:                provisioned.UserID,
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		HashedToken:           hash.String("token-8"),
	}).Error)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
		ID:                    lookupTestGroupPrefix + "g-8",
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		Name:                  "team",
	}).Error)
	require.NoError(t, c.db.WithContext(ctx).Create(&types.GroupMemberships{
		UserID:  provisioned.UserID,
		GroupID: lookupTestGroupPrefix + "g-8",
	}).Error)

	userID, err := c.DeleteSCIMUser(ctx, conn, provisioned.ID)
	require.NoError(t, err)
	require.Equal(t, provisioned.UserID, userID)

	// The user is disabled and deleted, their tokens are gone, and their SCIM ID is retired, all at once.
	deleted := storedLifecycleUser(t, c, userID)
	require.NotNil(t, deleted.DeletedAt)
	require.NotNil(t, deleted.DisabledAt)
	require.Equal(t, types.UserDisabledReasonSCIMDeleted, deleted.DisabledReason)
	require.Zero(t, countRows(t, c, new(types.AuthToken), "user_id = ?", userID))
	require.Zero(t, countRows(t, c, new(types.GroupMemberships), "user_id = ?", userID))
	_, err = c.GetSCIMUser(ctx, conn.ID, provisioned.ID)
	var notFound *SCIMNotFoundError
	require.ErrorAs(t, err, &notFound)

	// The deletion event creates one cleanup, however often it is delivered.
	var events []types.UserLifecycleEvent
	require.NoError(t, c.db.WithContext(ctx).Where("user_id = ? AND type = ?", userID, types.UserLifecycleEventDeleted).Find(&events).Error)
	require.Len(t, events, 1)
	require.NoError(t, c.deliverUserLifecycleEvent(ctx, events[0], c.listOAuthTokensOnce(ctx)))
	require.NoError(t, c.deliverUserLifecycleEvent(ctx, events[0], c.listOAuthTokensOnce(ctx)))
	var cleanups v1.UserDeleteList
	require.NoError(t, c.storageClient.List(ctx, &cleanups))
	require.Len(t, cleanups.Items, 1)
	require.Equal(t, userID, cleanups.Items[0].Spec.UserID)

	// A retired or unknown SCIM ID is not found.
	_, err = c.DeleteSCIMUser(ctx, conn, provisioned.ID)
	require.ErrorAs(t, err, &notFound)
	_, err = c.DeleteSCIMUser(ctx, conn, "unknown")
	require.ErrorAs(t, err, &notFound)

	// Provisioning the same person again starts a new account.
	again, err := provisionLookupTestUser(t, c, conn, "oid-8")
	require.NoError(t, err)
	require.NotEqual(t, userID, again.UserID)
}

func TestDeleteSCIMUserRefusesTheLastOwnerAndAdmin(t *testing.T) {
	tests := []struct {
		name string
		role apitypes.Role
	}{
		{
			name: "owner",
			role: apitypes.RoleOwner,
		},
		{
			name: "admin",
			role: apitypes.RoleAdmin,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newLookupTestClient(t)
			conn := createLookupTestConnection(t, c)

			provisioned, err := provisionLookupTestUser(t, c, conn, "oid-9")
			require.NoError(t, err)
			require.NoError(t, c.db.WithContext(t.Context()).Model(new(types.User)).Where("id = ?", provisioned.UserID).UpdateColumns(map[string]any{
				"role":         tt.role,
				"hashed_email": hash.String("oid-9@example.com"),
			}).Error)

			_, err = c.DeleteSCIMUser(t.Context(), conn, provisioned.ID)
			var conflict *SCIMConflictError
			require.ErrorAs(t, err, &conflict)
			require.False(t, conflict.Uniqueness)

			// Nothing changed.
			user := storedLifecycleUser(t, c, provisioned.UserID)
			require.Nil(t, user.DeletedAt)
			require.Nil(t, user.DisabledAt)
			_, err = c.GetSCIMUser(t.Context(), conn.ID, provisioned.ID)
			require.NoError(t, err)
		})
	}
}

func TestAdminDeletionRecordsTheCleanupEvent(t *testing.T) {
	c := newLifecycleTestClient(t)
	user := createLifecycleTestUser(t, c, "deleted", lifecycleTestProvider)

	require.NoError(t, c.DeleteUser(t.Context(), strconv.FormatUint(uint64(user.ID), 10)))
	var events []types.UserLifecycleEvent
	require.NoError(t, c.db.WithContext(t.Context()).Where("user_id = ? AND type = ?", user.ID, types.UserLifecycleEventDeleted).Find(&events).Error)
	require.Len(t, events, 1)

	require.NoError(t, c.deliverUserLifecycleEvents(t.Context()))
	var cleanups v1.UserDeleteList
	require.NoError(t, c.storageClient.List(t.Context(), &cleanups))
	require.Len(t, cleanups.Items, 1)
	require.Equal(t, user.ID, cleanups.Items[0].Spec.UserID)

	// A refused deletion records nothing.
	require.Error(t, c.DeleteUser(t.Context(), "999"))
	require.Zero(t, countRows(t, c, new(types.UserLifecycleEvent), "user_id = ?", 999))

	// Deleting the user again finds no user, and records no second cleanup.
	require.ErrorIs(t, c.DeleteUser(t.Context(), strconv.FormatUint(uint64(user.ID), 10)), gorm.ErrRecordNotFound)
	require.EqualValues(t, 1, countRows(t, c, new(types.UserLifecycleEvent), "user_id = ? AND type = ?", user.ID, types.UserLifecycleEventDeleted))
}

func TestSCIMGroupsBindByExternalID(t *testing.T) {
	c := newLookupTestClient(t)
	conn := createLookupTestConnection(t, c)
	ctx := t.Context()

	// A group that directory synchronization created, with a grant.
	legacyID := lookupTestGroupPrefix + "g-1"
	require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
		ID:                    legacyID,
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		Name:                  "Old Name",
	}).Error)
	_, err := c.CreateGroupRoleAssignment(ctx, legacyID, apitypes.RoleAdmin, "")
	require.NoError(t, err)

	// The pushed group binds to it by externalId, keeps its ID, and takes the pushed name.
	group, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "Team",
		ExternalID:  "g-1",
	})
	require.NoError(t, err)
	require.Equal(t, legacyID, group.GroupID)
	require.Equal(t, "g-1", group.ExternalID)
	require.Equal(t, "Team", group.DisplayName)

	// It is found by its externalId, exactly as it was sent, since externalId is case exact.
	found, total, err := c.ListSCIMGroups(ctx, conn, SCIMGroupFilter{
		ExternalID: "g-1",
	}, SCIMPage{
		Limit: 10,
	}, false)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, group.ID, found[0].ID)
	require.Equal(t, "g-1", found[0].ExternalID)
	for _, externalID := range []string{" g-1 ", "G-1", "g-other"} {
		_, total, err := c.ListSCIMGroups(ctx, conn, SCIMGroupFilter{
			ExternalID: externalID,
		}, SCIMPage{
			Limit: 10,
		}, false)
		require.NoError(t, err)
		require.Zero(t, total, externalID)
	}

	// Pushing the same group again is a conflict.
	_, err = c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "Team Again",
		ExternalID:  "g-1",
	})
	var conflict *SCIMConflictError
	require.ErrorAs(t, err, &conflict)

	// A group without an existing one is created with the ID that its externalId gives.
	created, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "New Team",
		ExternalID:  "g-2",
	})
	require.NoError(t, err)
	require.Equal(t, lookupTestGroupPrefix+"g-2", created.GroupID)

	// A group without an externalId is refused.
	_, err = c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "No ID",
	})
	var invalid *SCIMInvalidValueError
	require.ErrorAs(t, err, &invalid)

	// So is an externalId that the adapter accepts but that cannot be part of a group ID, and no group has one.
	for _, externalID := range []string{"a/b", "a\tb", strings.Repeat("g", 257)} {
		_, err = c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
			DisplayName: "Bad ID",
			ExternalID:  externalID,
		})
		require.ErrorAs(t, err, &invalid, externalID)
		found, total, err := c.ListSCIMGroups(ctx, conn, SCIMGroupFilter{
			ExternalID: externalID,
		}, SCIMPage{
			Limit: 10,
		}, false)
		require.NoError(t, err)
		require.Zero(t, total)
		require.Empty(t, found)
	}

	// Unassigning the group before SCIM is enforced retires only the binding. Assigning it again binds the same
	// group, which kept its grant.
	require.NoError(t, c.DeleteSCIMGroup(ctx, conn, group.ID))
	rebound, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
		DisplayName: "Team",
		ExternalID:  "g-1",
	})
	require.NoError(t, err)
	require.Equal(t, legacyID, rebound.GroupID)
	require.NotEqual(t, group.ID, rebound.ID)
	require.EqualValues(t, 1, countRows(t, c, new(types.GroupRoleAssignment), "group_name = ?", legacyID))
}

func TestSCIMGroupsByExternalIDWaitForDeletionsAndRefuseOtherProviders(t *testing.T) {
	c := newLookupTestClient(t)
	conn := createLookupTestConnection(t, c)
	ctx := t.Context()

	tests := []struct {
		name    string
		groupID string
		setup   func(t *testing.T)
	}{
		{
			name:    "a group pending deletion for being unreferenced",
			groupID: lookupTestGroupPrefix + "marked",
			setup: func(t *testing.T) {
				t.Helper()

				require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
					ID:                    lookupTestGroupPrefix + "marked",
					AuthProviderNamespace: lookupTestProvider.Namespace,
					AuthProviderName:      lookupTestProvider.Name,
					Name:                  "marked",
				}).Error)
				require.NoError(t, c.db.WithContext(ctx).Create(&types.SCIMPendingGroupDeletion{
					GroupID:      lookupTestGroupPrefix + "marked",
					ConnectionID: conn.ID,
					CreatedAt:    time.Now(),
				}).Error)
			},
		},
		{
			name:    "a deleted group whose subjects are still being removed",
			groupID: lookupTestGroupPrefix + "cleanup",
			setup: func(t *testing.T) {
				t.Helper()

				require.NoError(t, c.db.WithContext(ctx).Create(&types.SCIMGroupSubjectCleanup{
					GroupID:   lookupTestGroupPrefix + "cleanup",
					Namespace: system.DefaultNamespace,
				}).Error)
			},
		},
		{
			name:    "a group of another auth provider",
			groupID: lookupTestGroupPrefix + "other",
			setup: func(t *testing.T) {
				t.Helper()

				require.NoError(t, c.db.WithContext(ctx).Create(&types.Group{
					ID:                    lookupTestGroupPrefix + "other",
					AuthProviderNamespace: system.DefaultNamespace,
					AuthProviderName:      "other-auth-provider",
					Name:                  "other",
				}).Error)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)
			_, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
				DisplayName: tt.name,
				ExternalID:  strings.TrimPrefix(tt.groupID, lookupTestGroupPrefix),
			})
			var conflict *SCIMConflictError
			require.ErrorAs(t, err, &conflict)
			require.Zero(t, countRows(t, c, new(types.SCIMGroupBinding), "group_id = ?", tt.groupID))
		})
	}
}

func TestOktaGroupsKeepTheirExternalID(t *testing.T) {
	c := newLifecycleTestClient(t)
	conn, _ := createTestSCIMConnection(t, c, false)

	group, err := c.CreateSCIMGroup(t.Context(), conn, SCIMGroupInput{
		DisplayName: "team",
		ExternalID:  "00g1a2b3c4d5e6f7g8h9",
	})
	require.NoError(t, err)
	require.Equal(t, "00g1a2b3c4d5e6f7g8h9", group.ExternalID)

	found, total, err := c.ListSCIMGroups(t.Context(), conn, SCIMGroupFilter{
		ExternalID: "00g1a2b3c4d5e6f7g8h9",
	}, SCIMPage{
		Limit: 10,
	}, false)
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, group.ID, found[0].ID)

	// Okta groups bind by name, so their externalId can change, and an update without one removes it.
	require.NoError(t, c.PatchSCIMGroup(t.Context(), conn, group.ID, SCIMGroupPatch{
		ExternalID: "00g-other",
	}))
	got, err := c.GetSCIMGroup(t.Context(), conn, group.ID, false)
	require.NoError(t, err)
	require.Equal(t, "00g-other", got.ExternalID)

	require.NoError(t, c.UpdateSCIMGroup(t.Context(), conn, group.ID, func(current SCIMGroup) (SCIMGroupInput, error) {
		return SCIMGroupInput{
			DisplayName: current.DisplayName,
		}, nil
	}))
	got, err = c.GetSCIMGroup(t.Context(), conn, group.ID, false)
	require.NoError(t, err)
	require.Empty(t, got.ExternalID)
}

// clearGroupLookupID makes the identity of the lookup test provider with the given provider user ID look like one
// from before group lookup IDs were stored: unencrypted, and without one.
func clearGroupLookupID(t *testing.T, c *Client, sub string) {
	t.Helper()

	require.NoError(t, c.db.WithContext(t.Context()).Model(new(types.Identity)).
		Where("auth_provider_namespace = ? AND auth_provider_name = ? AND hashed_provider_user_id = ?", lookupTestProvider.Namespace, lookupTestProvider.Name, hash.String(sub)).
		UpdateColumns(map[string]any{
			"provider_group_lookup_id":        "",
			"hashed_provider_group_lookup_id": "",
			"encrypted":                       false,
			"provider_username":               sub,
			"provider_user_id":                sub,
			"email":                           sub + "@example.com",
			"icon_url":                        "",
		}).Error)
}

func TestSCIMConnectionsWaitForTheBackfillOfGroupLookupIDHashes(t *testing.T) {
	adapter.RegisterForTest(t, lookupTestProviderName, adaptertest.Adapter{
		Users: adapter.UserKeyGroupLookupID,
	})
	c := newLifecycleTestClient(t)
	stub := newLookupIDStub(t)

	// Until the backfill has run, SCIM could not find identities without the hash, so it is not set up.
	_, _, err := c.CreateSCIMConnection(t.Context(), CreateSCIMConnectionOptions{
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		GroupIDPrefix:         lookupTestGroupPrefix,
		Issuer:                "tenant",
		Origin:                types.SCIMConnectionOriginMigrated,
	})
	var notReady *SCIMIdentitiesNotReadyError
	require.ErrorAs(t, err, &notReady)

	// An identity written without the hash after the backfill, as a replica without it writes one, gets it when SCIM
	// is set up.
	require.NoError(t, c.BackfillIdentityGroupLookupIDHashes(t.Context()))
	_, err = lookupSignIn(t, c, stub, "sub-c", "oid-c")
	require.NoError(t, err)
	require.NoError(t, c.db.WithContext(t.Context()).Model(new(types.Identity)).
		Where("hashed_provider_user_id = ?", hash.String("sub-c")).
		UpdateColumn("hashed_provider_group_lookup_id", "").Error)

	conn := createLookupTestConnection(t, c)
	require.Equal(t, hash.String("oid-c"), storedIdentity(t, c, lookupTestProvider, "sub-c").HashedProviderGroupLookupID)
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-c")
	require.NoError(t, err)
	require.Equal(t, storedIdentity(t, c, lookupTestProvider, "sub-c").UserID, provisioned.UserID)
}

func TestClaimRefusesATakenUsername(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)

	// Another user already has the username that the sign-in would give.
	createLifecycleTestUser(t, c, "sub-d", lifecycleTestProvider)
	_, err := provisionLookupTestUser(t, c, conn, "oid-d")
	require.NoError(t, err)

	_, err = lookupSignIn(t, c, stub, "sub-d", "oid-d")
	var httpErr *apitypes.ErrHTTP
	require.ErrorAs(t, err, &httpErr)
	require.Equal(t, http.StatusConflict, httpErr.Code)
	// Nothing was claimed.
	require.EqualValues(t, 1, pendingIdentityCount(t, c, "oid-d"))
}

func TestClaimRefusesAnEncryptedIdentityWithoutEncryption(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	conn := createLookupTestConnection(t, c)
	_, err := provisionLookupTestUser(t, c, conn, "oid-e")
	require.NoError(t, err)

	c.encryptionConfig = nil
	_, err = lookupSignIn(t, c, stub, "sub-e", "oid-e")
	require.ErrorContains(t, err, "encryption is not configured")
	require.EqualValues(t, 1, pendingIdentityCount(t, c, "oid-e"))
}

func TestStoringAGroupLookupIDLeavesTheRestOfTheIdentityAlone(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub := newLookupIDStub(t)
	user := createLifecycleTestUser(t, c, "stale", lifecycleTestProvider)

	// A copy of the identity read before something else moved it to another user.
	identity := *storedIdentity(t, c, lifecycleTestProvider, "00u-stale")
	identity.UserID = user.ID + 100

	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderURL(t.Context(), stub.URL), "lookup-stale")
	require.NoError(t, c.ensureProviderGroupLookupID(ctx, &identity))

	stored := storedIdentity(t, c, lifecycleTestProvider, "00u-stale")
	require.Equal(t, user.ID, stored.UserID)
	require.Equal(t, hash.String("lookup-stale"), stored.HashedProviderGroupLookupID)
}

func TestAGroupLookupIDStoredByAnotherSignInIsUsed(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub := newLookupIDStub(t)
	createLifecycleTestUser(t, c, "racer", lifecycleTestProvider)

	// Two sign-ins read the identity before either stored a lookup ID, and the first one stores its ID.
	first := *storedIdentity(t, c, lifecycleTestProvider, "00u-racer")
	second := first
	ctx := auth.ContextWithProviderURL(t.Context(), stub.URL)
	require.NoError(t, c.ensureProviderGroupLookupID(accesstoken.ContextWithAccessToken(ctx, "lookup-first"), &first))

	// The second one finds the stored ID and uses it, rather than leaving its own copy without one.
	require.NoError(t, c.ensureProviderGroupLookupID(accesstoken.ContextWithAccessToken(ctx, "lookup-second"), &second))
	require.Equal(t, "lookup-first", second.ProviderGroupLookupID)
	require.Equal(t, hash.String("lookup-first"), second.HashedProviderGroupLookupID)
	require.Equal(t, hash.String("lookup-first"), storedIdentity(t, c, lifecycleTestProvider, "00u-racer").HashedProviderGroupLookupID)
}

func newEntraPictureTest(t *testing.T) *entraPictureTest {
	t.Helper()

	test := &entraPictureTest{
		c: newLookupTestClient(t),
	}
	ctx := t.Context()
	conn, _, err := test.c.CreateSCIMConnection(ctx, CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "entra-auth-provider",
		GroupIDPrefix:         "entra/",
		Issuer:                "tenant",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
		IssueToken:            true,
	})
	require.NoError(t, err)

	const objectID = "11111111-1111-4111-8111-111111111111"
	test.iconURL.Store("https://example.com/photo-1")
	test.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if during := test.during.Load(); during != nil {
			(*during)()
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":       objectID,
			"name":     "Login Name",
			"icon_url": test.iconURL.Load(),
		})
	}))
	t.Cleanup(test.srv.Close)

	provisioned, err := test.c.CreateSCIMUser(ctx, conn, SCIMUserInput{
		UserName:   "alice@example.com",
		ExternalID: objectID,
		Profile: types.SCIMUserProfile{
			DisplayName: "SCIM Name",
		},
	}, SCIMUserCreateOptions{
		UserLimit: UserLimit{
			Unlimited: true,
		},
		DefaultRole: apitypes.RoleBasic,
	})
	require.NoError(t, err)
	test.userID = provisioned.UserID
	return test
}

// refresh runs the sign-in profile refresh of the user, and returns its error and the caller's copy of the user.
func (test *entraPictureTest) refresh(t *testing.T) (types.User, error) {
	t.Helper()

	user := test.stored(t)
	err := test.c.UpdateProfileIfNeeded(accesstoken.ContextWithAccessToken(t.Context(), "access-token"), &user, "entra-auth-provider", system.DefaultNamespace, test.srv.URL)
	return user, err
}

// stored returns the user as stored, decrypted.
func (test *entraPictureTest) stored(t *testing.T) types.User {
	t.Helper()

	user := storedLifecycleUser(t, test.c, test.userID)
	require.NoError(t, test.c.decryptUser(t.Context(), &user))
	return user
}

func TestSCIMProvisionedEntraUsersGetTheirProfilePictures(t *testing.T) {
	test := newEntraPictureTest(t)
	refresh := func() types.User {
		t.Helper()
		user, err := test.refresh(t)
		require.NoError(t, err)
		stored := test.stored(t)
		require.Equal(t, user.IconURL, stored.IconURL, "the caller's copy of the user differs from the stored one")
		return stored
	}

	// SCIM sends no picture, so sign-in fills it in, and leaves the SCIM profile alone.
	got := refresh()
	require.Equal(t, "https://example.com/photo-1", got.IconURL)
	require.Equal(t, "SCIM Name", got.DisplayName)
	require.True(t, got.Encrypted)

	// A changed picture replaces the stored one once it is due to be checked again.
	test.iconURL.Store("https://example.com/photo-2")
	require.Equal(t, "https://example.com/photo-1", refresh().IconURL)
	require.NoError(t, test.c.db.WithContext(t.Context()).Model(new(types.Identity)).
		Where("user_id = ?", test.userID).
		UpdateColumn("icon_last_checked", time.Now().Add(-profileIconCheckInterval-time.Hour)).Error)
	got = refresh()
	require.Equal(t, "https://example.com/photo-2", got.IconURL)
	require.Equal(t, "SCIM Name", got.DisplayName)
}

func TestProfilePictureOfAnIdentityMovedDuringTheRefreshIsNotWritten(t *testing.T) {
	test := newEntraPictureTest(t)
	other := createLifecycleTestUser(t, test.c, "other", lifecycleTestProvider)

	// The identity moves to another user while the provider is answering. The stub answers on its own goroutine, so
	// the move's error is checked once the refresh returns.
	moved := make(chan error, 1)
	move := func() {
		moved <- test.c.db.WithContext(t.Context()).Model(new(types.Identity)).
			Where("user_id = ?", test.userID).
			UpdateColumn("user_id", other.ID).Error
	}
	test.during.Store(&move)

	_, err := test.refresh(t)
	select {
	case moveErr := <-moved:
		require.NoError(t, moveErr)
	default:
		t.Fatal("the identity did not move while the provider was answering")
	}
	require.NoError(t, err)
	require.Empty(t, test.stored(t).IconURL)
	var identity types.Identity
	require.NoError(t, test.c.db.WithContext(t.Context()).Where("user_id = ? AND auth_provider_name = ?", other.ID, "entra-auth-provider").Take(&identity).Error)
	require.True(t, identity.IconLastChecked.IsZero(), "the moved identity's picture was checked")
}

func TestProfilePictureRefreshRefusesAnEncryptedUserWithoutUserEncryption(t *testing.T) {
	test := newEntraPictureTest(t)
	before := storedLifecycleUser(t, test.c, test.userID)

	// Identities can still be decrypted, but users cannot.
	delete(test.c.encryptionConfig.Transformers, userGroupResource)
	_, err := test.refresh(t)
	require.ErrorContains(t, err, "user is encrypted but encryption is not configured")
	require.Equal(t, before, storedLifecycleUser(t, test.c, test.userID))
}

func TestDeletionDoesNotCountDisabledOwners(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	deleted := createLifecycleTestUser(t, c, "owner-a", lifecycleTestProvider)
	disabled := createLifecycleTestUser(t, c, "owner-b", lifecycleTestProvider)
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.User)).Where("id IN ?", []uint{deleted.ID, disabled.ID}).UpdateColumn("role", apitypes.RoleOwner).Error)
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.User)).Where("id = ?", disabled.ID).UpdateColumns(map[string]any{
		"disabled_at":     time.Now(),
		"disabled_reason": types.UserDisabledReasonSCIMInactive,
	}).Error)

	// The disabled Owner cannot act as one, so the other is the last.
	var lastOwner *LastOwnerError
	require.ErrorAs(t, c.DeleteUser(ctx, strconv.FormatUint(uint64(deleted.ID), 10)), &lastOwner)
	require.Nil(t, storedLifecycleUser(t, c, deleted.ID).DeletedAt)
}

func TestSCIMConnectionsWaitForUsersWithoutAGroupLookupID(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	ctx := t.Context()

	// Two users have identities without a group lookup ID, as users who have not signed in since Obot started storing
	// it do. One of them is disabled. A third has one, and a fourth, who has none, is deleted.
	missing, err := lookupSignIn(t, c, stub, "sub-m", "oid-m")
	require.NoError(t, err)
	disabled, err := lookupSignIn(t, c, stub, "sub-n", "oid-n")
	require.NoError(t, err)
	_, err = lookupSignIn(t, c, stub, "sub-o", "oid-o")
	require.NoError(t, err)
	deleted, err := lookupSignIn(t, c, stub, "sub-p", "oid-p")
	require.NoError(t, err)
	for _, sub := range []string{"sub-m", "sub-n", "sub-p"} {
		clearGroupLookupID(t, c, sub)
	}
	require.NoError(t, c.db.WithContext(ctx).Model(new(types.User)).Where("id = ?", disabled.ID).UpdateColumns(map[string]any{
		"disabled_at":     time.Now(),
		"disabled_reason": types.UserDisabledReasonSCIMInactive,
	}).Error)
	require.NoError(t, c.DeleteUser(ctx, strconv.FormatUint(uint64(deleted.ID), 10)))

	want := []SCIMUserWithoutGroupLookupID{
		{
			UserID: missing.ID,
			Name:   "sub-m@example.com",
		},
		{
			UserID:   disabled.ID,
			Name:     "sub-n@example.com",
			Disabled: true,
		},
	}
	users, err := c.UsersWithoutGroupLookupID(ctx, lookupTestProvider)
	require.NoError(t, err)
	require.Equal(t, want, users)

	// SCIM is not set up while they have none.
	_, _, err = c.CreateSCIMConnection(ctx, CreateSCIMConnectionOptions{
		AuthProviderNamespace: lookupTestProvider.Namespace,
		AuthProviderName:      lookupTestProvider.Name,
		GroupIDPrefix:         lookupTestGroupPrefix,
		Issuer:                "tenant",
		Origin:                types.SCIMConnectionOriginMigrated,
	})
	var missingErr *SCIMGroupLookupIDsMissingError
	require.ErrorAs(t, err, &missingErr)
	require.Equal(t, want, missingErr.Users)
	require.Zero(t, countRows(t, c, new(types.SCIMConnection), "auth_provider_name = ?", lookupTestProvider.Name))

	// Signing in stores the lookup ID. Once the disabled user is enabled and signs in too, SCIM can be set up.
	_, err = lookupSignIn(t, c, stub, "sub-m", "oid-m")
	require.NoError(t, err)
	_, err = c.EnableUser(ctx, disabled.ID)
	require.NoError(t, err)
	_, err = lookupSignIn(t, c, stub, "sub-n", "oid-n")
	require.NoError(t, err)
	users, err = c.UsersWithoutGroupLookupID(ctx, lookupTestProvider)
	require.NoError(t, err)
	require.Empty(t, users)

	conn := createLookupTestConnection(t, c)
	provisioned, err := provisionLookupTestUser(t, c, conn, "oid-m")
	require.NoError(t, err)
	require.Equal(t, missing.ID, provisioned.UserID)
}

func TestUsersWithoutGroupLookupIDWaitForTheBackfill(t *testing.T) {
	adapter.RegisterForTest(t, lookupTestProviderName, adaptertest.Adapter{
		Users: adapter.UserKeyGroupLookupID,
	})
	c := newLifecycleTestClient(t)

	_, err := c.UsersWithoutGroupLookupID(t.Context(), lookupTestProvider)
	var notReady *SCIMIdentitiesNotReadyError
	require.ErrorAs(t, err, &notReady)
}

func TestSignInNeverCreatesAnIdentityOrUserWithoutAGroupLookupIDOnceSCIMIsSetUp(t *testing.T) {
	c := newLookupTestClient(t)
	stub := newLookupIDStub(t)
	ctx := t.Context()

	// A user whose identity has no lookup ID is deleted, so it no longer blocks setting SCIM up, and the identity is
	// still there because its cleanup has not run.
	deleted, err := lookupSignIn(t, c, stub, "sub-q", "oid-q")
	require.NoError(t, err)
	clearGroupLookupID(t, c, "sub-q")
	require.NoError(t, c.DeleteUser(ctx, strconv.FormatUint(uint64(deleted.ID), 10)))
	createLookupTestConnection(t, c)

	// The sign-in's transaction runs as though fetchSignInGroupLookupID had found no connection, and so no lookup ID
	// was required before it: the connection was created while the sign-in fetched it, and the fetch failed.
	signIn := func(sub string) error {
		return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := c.ensureIdentity(ctx, tx, &types.Identity{
				AuthProviderNamespace: lookupTestProvider.Namespace,
				AuthProviderName:      lookupTestProvider.Name,
				ProviderUsername:      sub,
				ProviderUserID:        sub,
				Email:                 sub + "@example.com",
			}, "", apitypes.RoleUnknown, UserLimit{
				Unlimited: true,
			})
			return err
		})
	}

	// A first sign-in does not create an identity without one.
	require.ErrorIs(t, signIn("sub-r"), errSignInWithoutGroupLookupID)
	require.Zero(t, countRows(t, c, new(types.Identity), "hashed_provider_user_id = ?", hash.String("sub-r")))

	// Nor does a sign-in to the deleted user's identity create a user for it. The refusal of sessions of deleted users
	// applies before the lookup ID is required.
	users := countRows(t, c, new(types.User), "")
	err = signIn("sub-q")
	denied, ok := errors.AsType[*UserAccessDeniedError](err)
	require.Truef(t, ok, "sign-in failed with %v, want a UserAccessDeniedError", err)
	require.Equal(t, apitypes.UserStatusDeleted, denied.Status)
	require.Equal(t, deleted.ID, denied.UserID)
	require.Equal(t, users, countRows(t, c, new(types.User), ""))
}

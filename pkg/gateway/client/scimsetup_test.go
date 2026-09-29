package client

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
)

// enforceFixture is a connected SCIM connection whose auth provider has a provisioned Owner who signed in, an
// unprovisioned user, a deleted user, a local user, a pushed group, and an unbound group that nothing references.
type enforceFixture struct {
	conn          *types.SCIMConnection
	owner         *types.User
	unprovisioned *types.User
	deleted       *types.User
	local         *types.User
	pushed        *SCIMGroup
	actor         SCIMEnforceActor
}

// markSignedIn records that the user's identities have signed in.
func markSignedIn(t *testing.T, c *Client, userID uint) {
	t.Helper()

	if err := c.db.WithContext(t.Context()).Model(new(types.Identity)).Where("user_id = ?", userID).UpdateColumn("first_sign_in_at", time.Now()).Error; err != nil {
		t.Fatalf("failed to mark user %d signed in: %v", userID, err)
	}
}

// createTestGroup creates an unbound group of the lifecycle test provider with members.
func createTestGroup(t *testing.T, c *Client, id, name string, members ...uint) {
	t.Helper()

	if err := c.db.WithContext(t.Context()).Create(&types.Group{
		ID:                    id,
		AuthProviderName:      lifecycleTestProvider.Name,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		Name:                  name,
	}).Error; err != nil {
		t.Fatalf("failed to create group %s: %v", id, err)
	}
	for _, userID := range members {
		if err := c.db.WithContext(t.Context()).Create(&types.GroupMemberships{
			UserID:  userID,
			GroupID: id,
		}).Error; err != nil {
			t.Fatalf("failed to add user %d to group %s: %v", userID, id, err)
		}
	}
}

func memberships(t *testing.T, c *Client) []types.GroupMemberships {
	t.Helper()

	var result []types.GroupMemberships
	if err := c.db.WithContext(t.Context()).Order("group_id, user_id").Find(&result).Error; err != nil {
		t.Fatalf("failed to list memberships: %v", err)
	}
	for i := range result {
		result[i].CreatedAt = time.Time{}
	}
	return result
}

func storedGroupIDs(t *testing.T, c *Client) []string {
	t.Helper()

	var ids []string
	if err := c.db.WithContext(t.Context()).Model(new(types.Group)).Order("id").Pluck("id", &ids).Error; err != nil {
		t.Fatalf("failed to list groups: %v", err)
	}
	return ids
}

// reconcileEvents counts the reconcile events recorded so far.
func reconcileEvents(t *testing.T, c *Client) int64 {
	t.Helper()

	var count int64
	if err := c.db.WithContext(t.Context()).Model(new(types.UserLifecycleEvent)).Where("type = ?", types.UserLifecycleEventReconcile).Count(&count).Error; err != nil {
		t.Fatalf("failed to count reconcile events: %v", err)
	}
	return count
}

func pendingDeletions(t *testing.T, c *Client) []string {
	t.Helper()

	var ids []string
	if err := c.db.WithContext(t.Context()).Model(new(types.SCIMPendingGroupDeletion)).Order("group_id").Pluck("group_id", &ids).Error; err != nil {
		t.Fatalf("failed to list groups pending deletion: %v", err)
	}
	return ids
}

func newEnforceFixture(t *testing.T, c *Client) *enforceFixture {
	t.Helper()

	f := &enforceFixture{}
	f.conn, _ = createTestSCIMConnection(t, c, true)

	f.owner = createLifecycleTestUser(t, c, "owner", lifecycleTestProvider)
	markSignedIn(t, c, f.owner.ID)
	ownerSCIM := provisionTestSCIMUser(t, c, f.conn, "00u-owner", "owner@example.com")
	f.unprovisioned = createLifecycleTestUser(t, c, "unprovisioned", lifecycleTestProvider)
	markSignedIn(t, c, f.unprovisioned.ID)
	f.deleted = createLifecycleTestUser(t, c, "deleted", lifecycleTestProvider)
	if err := c.db.WithContext(t.Context()).Model(f.deleted).UpdateColumn("deleted_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}
	f.local = createLifecycleTestUser(t, c, "local", lifecycleTestLocalProvider)

	var err error
	if f.pushed, err = c.CreateSCIMGroup(t.Context(), f.conn, SCIMGroupInput{
		DisplayName: "Engineering",
		MemberIDs: []string{
			ownerSCIM.ID,
		},
	}); err != nil {
		t.Fatalf("failed to push a group: %v", err)
	}
	createTestGroup(t, c, "okta/00g-stale", "Stale", f.unprovisioned.ID)

	f.actor = SCIMEnforceActor{
		UserID:                f.owner.ID,
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		AuthProviderName:      lifecycleTestProvider.Name,
	}
	return f
}

func (f *enforceFixture) referenced() map[string]struct{} {
	return map[string]struct{}{
		f.pushed.GroupID: {},
	}
}

func TestEnforceSCIMConnection(t *testing.T) {
	c := newLifecycleTestClient(t)
	f := newEnforceFixture(t, c)
	before := memberships(t, c)

	run, err := c.MarkUnreferencedSCIMGroups(t.Context(), f.conn, f.referenced())
	if err != nil {
		t.Fatalf("failed to mark unreferenced groups: %v", err)
	}
	if !slices.Equal(run.GroupIDs, []string{"okta/00g-stale"}) {
		t.Fatalf("groups marked for deletion = %v, want only the unreferenced unbound group", run.GroupIDs)
	}
	reconcileBefore := reconcileEvents(t, c)

	result, err := c.EnforceSCIMConnection(t.Context(), f.conn.ID, EnforceSCIMOptions{
		RunID:              run.ID,
		ReferencedGroupIDs: f.referenced(),
		Actor:              f.actor,
	})
	if err != nil {
		t.Fatalf("failed to enforce: %v", err)
	}

	// Exactly the unprovisioned, live user of the provider is disabled.
	if !slices.Equal(result.DisabledUserIDs, []uint{f.unprovisioned.ID}) {
		t.Fatalf("disabled users = %v, want %d", result.DisabledUserIDs, f.unprovisioned.ID)
	}
	if user := storedLifecycleUser(t, c, f.unprovisioned.ID); user.DisabledAt == nil || user.DisabledReason != types.UserDisabledReasonSCIMUnprovisioned {
		t.Fatalf("unprovisioned user = %+v, want disabled as unprovisioned", user)
	}
	for _, userID := range []uint{f.owner.ID, f.local.ID, f.deleted.ID} {
		if user := storedLifecycleUser(t, c, userID); user.DisabledAt != nil {
			t.Fatalf("user %d was disabled", userID)
		}
	}

	// The unreferenced group is deleted with its memberships. The pushed group's memberships do not change.
	if !slices.Equal(result.DeletedGroupIDs, []string{"okta/00g-stale"}) {
		t.Fatalf("deleted groups = %v", result.DeletedGroupIDs)
	}
	if ids := storedGroupIDs(t, c); !slices.Equal(ids, []string{f.pushed.GroupID}) {
		t.Fatalf("groups after enforcing = %v", ids)
	}
	want := slices.DeleteFunc(slices.Clone(before), func(m types.GroupMemberships) bool {
		return m.GroupID == "okta/00g-stale"
	})
	if got := memberships(t, c); !slices.Equal(got, want) {
		t.Fatalf("memberships after enforcing = %v, want %v", got, want)
	}
	if ids := pendingDeletions(t, c); len(ids) != 0 {
		t.Fatalf("groups still pending deletion: %v", ids)
	}
	// The deleted group granted nothing, so deleting it reconciles no one, and triggers no resource cleanup.
	if after := reconcileEvents(t, c); after != reconcileBefore {
		t.Fatalf("enforcing recorded %d reconcile events", after-reconcileBefore)
	}

	if result.Connection.State != types.SCIMConnectionStateEnforced || result.Connection.EnforcedAt == nil {
		t.Fatalf("connection after enforcing = %+v", result.Connection)
	}
	stored, err := c.SCIMConnection(t.Context(), f.conn.ID)
	if err != nil || stored.State != types.SCIMConnectionStateEnforced || stored.EnforcedAt == nil {
		t.Fatalf("stored connection = %+v, %v", stored, err)
	}

	// Enforcement cannot be repeated or reversed.
	_, err = c.EnforceSCIMConnection(t.Context(), f.conn.ID, EnforceSCIMOptions{
		RunID:              run.ID,
		ReferencedGroupIDs: f.referenced(),
		Actor:              f.actor,
	})
	if _, ok := errors.AsType[*SCIMConnectionStateError](err); !ok {
		t.Fatalf("enforcing again = %v, want a state error", err)
	}

	// Provisioning the disabled user later re-enables them with the same account.
	provisioned := provisionTestSCIMUser(t, c, f.conn, "00u-unprovisioned", "unprovisioned@example.com")
	if provisioned.UserID != f.unprovisioned.ID {
		t.Fatalf("provisioning bound user %d, want %d", provisioned.UserID, f.unprovisioned.ID)
	}
	if user := storedLifecycleUser(t, c, f.unprovisioned.ID); user.DisabledAt != nil || user.DisabledReason != "" {
		t.Fatalf("provisioned user = %+v, want enabled", user)
	}
}

func TestEnforceSCIMConnectionIsRefusedAndClearsItsMarks(t *testing.T) {
	c := newLifecycleTestClient(t)
	f := newEnforceFixture(t, c)
	createTestGroup(t, c, "okta/00g-policy", "Policy")
	bootstrap := createLifecycleTestUser(t, c, system.BootstrapName, AuthProviderRef{
		Namespace: "",
		Name:      "",
	})
	notSignedIn := createLifecycleTestUser(t, c, "not-signed-in", lifecycleTestProvider)
	provisionTestSCIMUser(t, c, f.conn, "00u-not-signed-in", "not-signed-in@example.com")
	unbound := createLifecycleTestUser(t, c, "unbound", lifecycleTestProvider)
	markSignedIn(t, c, unbound.ID)

	withPolicy := f.referenced()
	withPolicy["okta/00g-policy"] = struct{}{}

	tests := []struct {
		name        string
		referenced  map[string]struct{}
		actor       SCIMEnforceActor
		wantGroups  []string
		wantProblem SCIMEnforceActorProblem
	}{
		{
			name:       "a referenced group is unbound",
			referenced: withPolicy,
			actor:      f.actor,
			wantGroups: []string{
				"okta/00g-policy",
			},
		},
		{
			name:       "a marked group gained a reference and is unbound",
			referenced: map[string]struct{}{f.pushed.GroupID: {}, "okta/00g-stale": {}},
			actor:      f.actor,
			wantGroups: []string{
				"okta/00g-stale",
			},
		},
		{
			name:       "the bootstrap user",
			referenced: f.referenced(),
			actor: SCIMEnforceActor{
				UserID:                bootstrap.ID,
				AuthProviderNamespace: "",
				AuthProviderName:      system.BootstrapName,
			},
			wantProblem: SCIMEnforceActorOtherAuthProvider,
		},
		{
			name:       "an Owner who has not signed in through the provider",
			referenced: f.referenced(),
			actor: SCIMEnforceActor{
				UserID:                notSignedIn.ID,
				AuthProviderNamespace: lifecycleTestProvider.Namespace,
				AuthProviderName:      lifecycleTestProvider.Name,
			},
			wantProblem: SCIMEnforceActorNotSignedIn,
		},
		{
			name:       "an unbound Owner",
			referenced: f.referenced(),
			actor: SCIMEnforceActor{
				UserID:                unbound.ID,
				AuthProviderNamespace: lifecycleTestProvider.Namespace,
				AuthProviderName:      lifecycleTestProvider.Name,
			},
			wantProblem: SCIMEnforceActorUnprovisioned,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run, err := c.MarkUnreferencedSCIMGroups(t.Context(), f.conn, f.referenced())
			if err != nil {
				t.Fatal(err)
			}

			_, err = c.EnforceSCIMConnection(t.Context(), f.conn.ID, EnforceSCIMOptions{
				RunID:              run.ID,
				ReferencedGroupIDs: tt.referenced,
				Actor:              tt.actor,
			})
			blocked, ok := errors.AsType[*SCIMEnforceBlockedError](err)
			if !ok {
				t.Fatalf("EnforceSCIMConnection() = %v, want blocked", err)
			}
			var groups []string
			for _, group := range blocked.UnboundGroups {
				groups = append(groups, group.ID)
			}
			if !slices.Equal(groups, tt.wantGroups) || blocked.ActorProblem != tt.wantProblem {
				t.Fatalf("blocked by groups %v and %q, want %v and %q", groups, blocked.ActorProblem, tt.wantGroups, tt.wantProblem)
			}

			// Nothing changed, and the marks are gone, so references to the marked groups are accepted again.
			if ids := pendingDeletions(t, c); len(ids) != 0 {
				t.Fatalf("groups still pending deletion: %v", ids)
			}
			if user := storedLifecycleUser(t, c, f.unprovisioned.ID); user.DisabledAt != nil {
				t.Fatal("a refused Enforce disabled a user")
			}
			if stored, err := c.SCIMConnection(t.Context(), f.conn.ID); err != nil || stored.State != types.SCIMConnectionStateConnected {
				t.Fatalf("connection after a refused Enforce = %+v, %v", stored, err)
			}
			if ids := storedGroupIDs(t, c); !slices.Contains(ids, "okta/00g-stale") {
				t.Fatalf("a refused Enforce deleted a group: %v", ids)
			}
		})
	}
}

func TestEnforcementSurvivesRestarts(t *testing.T) {
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "gateway.db")
	open := func() *Client {
		t.Helper()

		services, err := sservices.New(sservices.Config{
			DSN: dsn,
		})
		if err != nil {
			t.Fatal(err)
		}
		db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := db.AutoMigrate(); err != nil {
			t.Fatal(err)
		}
		c := newLifecycleTestClient(t)
		c.db = db
		t.Cleanup(func() {
			_ = services.DB.SQLDB.Close()
		})
		return c
	}

	c := open()
	f := newEnforceFixture(t, c)
	run, err := c.MarkUnreferencedSCIMGroups(t.Context(), f.conn, f.referenced())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.EnforceSCIMConnection(t.Context(), f.conn.ID, EnforceSCIMOptions{
		RunID:              run.ID,
		ReferencedGroupIDs: f.referenced(),
		Actor:              f.actor,
	}); err != nil {
		t.Fatalf("failed to enforce: %v", err)
	}

	restarted := open()
	stored, err := restarted.SCIMConnection(t.Context(), f.conn.ID)
	if err != nil || stored.State != types.SCIMConnectionStateEnforced {
		t.Fatalf("connection after a restart = %+v, %v", stored, err)
	}

	// Sign-in still requires a binding, and never creates an account.
	_, err = restarted.EnsureIdentity(t.Context(), signInIdentity("00u-newcomer", "newcomer@example.com"), "", UserLimit{
		Unlimited: true,
	})
	if _, ok := errors.AsType[*UserAccessDeniedError](err); !ok {
		t.Fatalf("sign-in of an unprovisioned user after a restart = %v, want denied", err)
	}
}

func TestWithNewSCIMGroupReferences(t *testing.T) {
	c := newLifecycleTestClient(t)
	write := func(groupIDs ...string) (bool, error) {
		t.Helper()
		var wrote bool
		err := c.WithNewSCIMGroupReferences(t.Context(), groupIDs, func() error {
			wrote = true
			return nil
		})
		return wrote, err
	}

	// Without a connection, any group ID may be referenced: login-time synchronization discovers groups later.
	if wrote, err := write("okta/00g-anything"); err != nil || !wrote {
		t.Fatalf("without a connection: wrote %v, %v", wrote, err)
	}

	conn, _ := createTestSCIMConnection(t, c, false)
	createTestGroup(t, c, "okta/00g-existing", "Existing")
	createTestGroup(t, c, "okta/00g-pending", "Pending")
	if _, err := c.MarkUnreferencedSCIMGroups(t.Context(), conn, map[string]struct{}{"okta/00g-existing": {}}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name        string
		groupIDs    []string
		wantMissing []string
		wantPending []string
	}{
		{
			name: "an existing group and a group of another provider",
			groupIDs: []string{
				"okta/00g-existing",
				"entra/engineering",
			},
		},
		{
			name: "a group ID with the prefix that no group has",
			groupIDs: []string{
				"okta/00g-existing",
				"okta/00g-missing",
			},
			wantMissing: []string{
				"okta/00g-missing",
			},
		},
		{
			name: "a group pending deletion",
			groupIDs: []string{
				"okta/00g-pending",
			},
			wantPending: []string{
				"okta/00g-pending",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrote, err := write(tt.groupIDs...)
			if tt.wantMissing == nil && tt.wantPending == nil {
				if err != nil || !wrote {
					t.Fatalf("WithNewSCIMGroupReferences() wrote %v, %v", wrote, err)
				}
				return
			}
			if wrote {
				t.Fatal("a refused reference was written")
			}
			refErr, ok := errors.AsType[*SCIMGroupReferenceError](err)
			if !ok {
				t.Fatalf("WithNewSCIMGroupReferences() = %v, want a reference error", err)
			}
			if !slices.Equal(refErr.Missing, tt.wantMissing) || !slices.Equal(refErr.PendingDeletion, tt.wantPending) {
				t.Fatalf("missing %v and pending %v, want %v and %v", refErr.Missing, refErr.PendingDeletion, tt.wantMissing, tt.wantPending)
			}
		})
	}

	// The write's own error is returned as it is.
	failed := errors.New("storage is unavailable")
	if err := c.WithNewSCIMGroupReferences(t.Context(), []string{"okta/00g-existing"}, func() error {
		return failed
	}); !errors.Is(err, failed) {
		t.Fatalf("WithNewSCIMGroupReferences() = %v, want the write's error", err)
	}
}

// testMarkingWaitsForReferenceWrites shows that marking a group for deletion does not return, so its references are
// not read again, until a write of a reference to it that has already been checked finishes, and that a write
// checked after the marks is refused.
func testMarkingWaitsForReferenceWrites(t *testing.T, c *Client) {
	t.Helper()

	conn, _ := createTestSCIMConnection(t, c, false)
	createTestGroup(t, c, "okta/00g-racing", "Racing")

	checked := make(chan struct{})
	release := make(chan struct{})
	written := make(chan error, 1)
	go func() {
		written <- c.WithNewSCIMGroupReferences(t.Context(), []string{"okta/00g-racing"}, func() error {
			close(checked)
			<-release
			return nil
		})
	}()
	<-checked

	marked := make(chan error, 1)
	go func() {
		// The scan that selected the group ran before the reference was saved.
		_, err := c.MarkUnreferencedSCIMGroups(t.Context(), conn, map[string]struct{}{})
		marked <- err
	}()

	select {
	case err := <-marked:
		t.Fatalf("marking finished while a checked reference was being written: %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	close(release)
	if err := <-written; err != nil {
		t.Fatalf("the reference write failed: %v", err)
	}
	if err := <-marked; err != nil {
		t.Fatalf("failed to mark groups: %v", err)
	}

	// Once the marks commit, a new reference to the group is refused, and the references read afterwards include
	// the one written before them, so Enforce keeps the group.
	err := c.WithNewSCIMGroupReferences(t.Context(), []string{"okta/00g-racing"}, func() error {
		t.Error("a reference to a group pending deletion was written")
		return nil
	})
	if refErr, ok := errors.AsType[*SCIMGroupReferenceError](err); !ok || !slices.Equal(refErr.PendingDeletion, []string{"okta/00g-racing"}) {
		t.Fatalf("WithNewSCIMGroupReferences() after marking = %v", err)
	}
}

func TestMarkingWaitsForReferenceWrites(t *testing.T) {
	testMarkingWaitsForReferenceWrites(t, newLifecycleTestClient(t))
}

func TestMarkingWaitsForReferenceWritesOnPostgres(t *testing.T) {
	testMarkingWaitsForReferenceWrites(t, newPostgresLifecycleTestClient(t))
}

func TestDeleteUnusedSCIMConnection(t *testing.T) {
	scimFirst := testSCIMConnectionOptions(true)
	scimFirst.Origin = types.SCIMConnectionOriginSCIMFirst

	tests := []struct {
		name        string
		opts        CreateSCIMConnectionOptions
		use         func(t *testing.T, c *Client, conn *types.SCIMConnection)
		wantDeleted bool
	}{
		{
			name:        "an unused connection created without directory credentials",
			opts:        scimFirst,
			wantDeleted: true,
		},
		{
			name: "a connection that provisioned a user",
			opts: scimFirst,
			use: func(t *testing.T, c *Client, conn *types.SCIMConnection) {
				t.Helper()
				provisionTestSCIMUser(t, c, conn, "00u-user", "user@example.com")
			},
		},
		{
			name: "a connection that bound a group, even after the group was deleted in the target",
			opts: scimFirst,
			use: func(t *testing.T, c *Client, conn *types.SCIMConnection) {
				t.Helper()
				group, err := c.CreateSCIMGroup(t.Context(), conn, SCIMGroupInput{
					DisplayName: "Engineering",
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := c.DeleteSCIMGroup(t.Context(), conn, group.ID); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "an enforced connection",
			opts: scimFirst,
			use: func(t *testing.T, c *Client, conn *types.SCIMConnection) {
				t.Helper()
				setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)
			},
		},
		{
			name: "a connection that replaced directory synchronization",
			opts: testSCIMConnectionOptions(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newLifecycleTestClient(t)
			conn, _, err := c.CreateSCIMConnection(t.Context(), tt.opts)
			if err != nil {
				t.Fatal(err)
			}
			if tt.use != nil {
				tt.use(t, c, conn)
			}

			deleted, err := c.DeleteUnusedSCIMConnection(t.Context(), lifecycleTestProvider.Namespace, lifecycleTestProvider.Name)
			if err != nil {
				t.Fatalf("DeleteUnusedSCIMConnection() error = %v", err)
			}
			if deleted != tt.wantDeleted {
				t.Fatalf("DeleteUnusedSCIMConnection() = %v, want %v", deleted, tt.wantDeleted)
			}
			remaining, err := c.SCIMConnectionForAuthProvider(t.Context(), lifecycleTestProvider.Namespace, lifecycleTestProvider.Name)
			if err != nil {
				t.Fatal(err)
			}
			if (remaining == nil) != tt.wantDeleted {
				t.Fatalf("connection after DeleteUnusedSCIMConnection() = %+v", remaining)
			}
		})
	}
}

func TestCreateSCIMConnectionRequiringNoGroupData(t *testing.T) {
	opts := testSCIMConnectionOptions(false)
	opts.Origin = types.SCIMConnectionOriginSCIMFirst
	opts.RequireNoGroupData = true

	tests := []struct {
		name  string
		setup func(t *testing.T, c *Client)
	}{
		{
			name: "a group of the provider",
			setup: func(t *testing.T, c *Client) {
				t.Helper()
				createTestGroup(t, c, "okta/00g-group", "Group")
			},
		},
		{
			name: "a membership of a group ID with the provider's prefix",
			setup: func(t *testing.T, c *Client) {
				t.Helper()
				user := createLifecycleTestUser(t, c, "member", lifecycleTestLocalProvider)
				if err := c.db.WithContext(t.Context()).Create(&types.GroupMemberships{
					UserID:  user.ID,
					GroupID: "okta/00g-orphan",
				}).Error; err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "a group role assignment of a group ID with the provider's prefix",
			setup: func(t *testing.T, c *Client) {
				t.Helper()
				if _, err := c.CreateGroupRoleAssignment(t.Context(), "okta/00g-admins", apitypes.RoleAdmin, ""); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newLifecycleTestClient(t)
			tt.setup(t, c)

			_, _, err := c.CreateSCIMConnection(t.Context(), opts)
			if _, ok := errors.AsType[*SCIMResidualGroupDataError](err); !ok {
				t.Fatalf("CreateSCIMConnection() = %v, want residual group data", err)
			}
			if has, err := c.HasSCIMConnections(t.Context()); err != nil || has {
				t.Fatalf("a connection was created: %v, %v", has, err)
			}
		})
	}

	// Group data of other providers does not matter.
	c := newLifecycleTestClient(t)
	if _, err := c.CreateGroupRoleAssignment(t.Context(), "entra/admins", apitypes.RoleAdmin, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.CreateSCIMConnection(t.Context(), opts); err != nil {
		t.Fatalf("CreateSCIMConnection() = %v", err)
	}
}

func TestSCIMReviewUsers(t *testing.T) {
	c := newLifecycleTestClient(t)
	f := newEnforceFixture(t, c)

	provisioned, total, err := c.SCIMProvisionedUsers(t.Context(), f.conn, SCIMPage{
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(provisioned) != 1 || provisioned[0].UserID != f.owner.ID || provisioned[0].SCIMID == "" || !provisioned[0].Active || !provisioned[0].SignedIn {
		t.Fatalf("provisioned users = %+v (%d)", provisioned, total)
	}

	for range 3 {
		createLifecycleTestUser(t, c, "user"+time.Now().Format("150405.000000000"), lifecycleTestProvider)
	}
	unprovisioned, total, err := c.SCIMUnprovisionedUsers(t.Context(), f.conn, SCIMPage{
		Offset: 1,
		Limit:  2,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The unprovisioned user who signed in, and the three who did not, but not the deleted or local user.
	if total != 4 || len(unprovisioned) != 2 {
		t.Fatalf("unprovisioned users = %+v (%d), want the second page of 4", unprovisioned, total)
	}
	if unprovisioned[0].SignedIn || unprovisioned[0].SCIMID != "" {
		t.Fatalf("unprovisioned user = %+v", unprovisioned[0])
	}

	page, total, err := c.SCIMUnprovisionedUsers(t.Context(), f.conn, SCIMPage{
		Offset: 4,
		Limit:  2,
	})
	if err != nil || total != 4 || len(page) != 0 {
		t.Fatalf("page past the end = %+v (%d), %v", page, total, err)
	}
}

func TestEnforceSCIMConnectionOnPostgres(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	f := newEnforceFixture(t, c)

	run, err := c.MarkUnreferencedSCIMGroups(t.Context(), f.conn, f.referenced())
	if err != nil {
		t.Fatalf("failed to mark unreferenced groups: %v", err)
	}
	if users, total, err := c.SCIMUnprovisionedUsers(t.Context(), f.conn, SCIMPage{Limit: 10}); err != nil || total != 1 || len(users) != 1 || !users[0].SignedIn {
		t.Fatalf("unprovisioned users = %+v (%d), %v", users, total, err)
	}

	// A user provisioned while SCIM is enforced either binds first and is never disabled, or binds after being
	// disabled and is re-enabled. Either way, they end up bound and enabled.
	provisioned := make(chan error, 1)
	go func() {
		_, err := c.CreateSCIMUser(t.Context(), f.conn, SCIMUserInput{
			UserName:   "unprovisioned@example.com",
			ExternalID: "00u-unprovisioned",
		}, SCIMUserCreateOptions{
			UserLimit: UserLimit{
				Unlimited: true,
			},
			DefaultRole: apitypes.RoleBasic,
		})
		provisioned <- err
	}()

	result, err := c.EnforceSCIMConnection(t.Context(), f.conn.ID, EnforceSCIMOptions{
		RunID:              run.ID,
		ReferencedGroupIDs: f.referenced(),
		Actor:              f.actor,
	})
	if err != nil {
		t.Fatalf("failed to enforce: %v", err)
	}
	if err := <-provisioned; err != nil {
		t.Fatalf("failed to provision a user while enforcing: %v", err)
	}
	if !slices.Equal(result.DeletedGroupIDs, []string{"okta/00g-stale"}) {
		t.Fatalf("deleted groups = %v", result.DeletedGroupIDs)
	}
	if user := storedLifecycleUser(t, c, f.unprovisioned.ID); user.DisabledAt != nil {
		t.Fatalf("user provisioned during Enforce = %+v, want enabled", user)
	}
	if binding, err := c.SCIMUserBindingForUser(t.Context(), f.unprovisioned.ID); err != nil || binding == nil {
		t.Fatalf("binding of the user provisioned during Enforce = %+v, %v", binding, err)
	}
}

func TestMarkingWaitsOnlyForUnexpiredReferenceWrites(t *testing.T) {
	testMarkingWaitsOnlyForUnexpiredReferenceWrites(t, newLifecycleTestClient(t))
}

func TestMarkingWaitsOnlyForUnexpiredReferenceWritesOnPostgres(t *testing.T) {
	testMarkingWaitsOnlyForUnexpiredReferenceWrites(t, newPostgresLifecycleTestClient(t))
}

// testMarkingWaitsOnlyForUnexpiredReferenceWrites shows that a write that never finished holds up marking only until
// it expires, by the clock that every replica agrees on.
func testMarkingWaitsOnlyForUnexpiredReferenceWrites(t *testing.T, c *Client) {
	t.Helper()

	conn, _ := createTestSCIMConnection(t, c, false)
	createTestGroup(t, c, "okta/00g-stale", "Stale")

	now, err := c.scimReferenceClock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	// A write that crashed long ago does not count, and one that crashed recently counts until it expires.
	if err := c.db.WithContext(t.Context()).Create(&[]types.SCIMReferenceWrite{
		{
			ID:        "crashed-long-ago",
			ExpiresAt: now.Add(-time.Minute),
		},
		{
			ID:        "crashed-recently",
			ExpiresAt: now.Add(300 * time.Millisecond),
		},
	}).Error; err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	run, err := c.MarkUnreferencedSCIMGroups(t.Context(), conn, map[string]struct{}{})
	if err != nil {
		t.Fatalf("failed to mark groups: %v", err)
	}
	if waited := time.Since(start); waited < 250*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("marking waited %v for a write that expires in 300ms", waited)
	}
	if !slices.Equal(run.GroupIDs, []string{"okta/00g-stale"}) {
		t.Fatalf("marked groups = %v", run.GroupIDs)
	}

	var remaining int64
	if err := c.db.WithContext(t.Context()).Model(new(types.SCIMReferenceWrite)).Where("id = ?", "crashed-long-ago").Count(&remaining).Error; err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatal("an expired record of a reference write was kept")
	}
}

func TestEnforceDeletesOnlyTheGroupsItsRunMarked(t *testing.T) {
	c := newLifecycleTestClient(t)
	f := newEnforceFixture(t, c)

	// Two deletions mark the same group. The later one holds the mark, so the earlier one keeps the group: it may have
	// read the references before the later one marked it.
	first, err := c.MarkUnreferencedSCIMGroups(t.Context(), f.conn, f.referenced())
	if err != nil {
		t.Fatal(err)
	}
	second, err := c.MarkUnreferencedSCIMGroups(t.Context(), f.conn, f.referenced())
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID || !slices.Equal(second.GroupIDs, []string{"okta/00g-stale"}) {
		t.Fatalf("deletion runs = %+v and %+v", first, second)
	}

	result, err := c.EnforceSCIMConnection(t.Context(), f.conn.ID, EnforceSCIMOptions{
		RunID:              first.ID,
		ReferencedGroupIDs: f.referenced(),
		Actor:              f.actor,
	})
	if err != nil {
		t.Fatalf("failed to enforce: %v", err)
	}
	if len(result.DeletedGroupIDs) != 0 || !slices.Contains(storedGroupIDs(t, c), "okta/00g-stale") {
		t.Fatalf("Enforce deleted %v, a group that another deletion marked", result.DeletedGroupIDs)
	}
	if ids := pendingDeletions(t, c); len(ids) != 0 {
		t.Fatalf("groups still pending deletion: %v", ids)
	}
}

func TestReferenceWritesNeverStarveThePostgresPool(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	sqlDB, err := c.db.WithContext(t.Context()).DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(2)
	conn, _ := createTestSCIMConnection(t, c, false)
	createTestGroup(t, c, "okta/00g-shared", "Shared")
	createTestGroup(t, c, "okta/00g-stale", "Stale")

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// Each write needs a connection of its own, as saving a policy or a role assignment does, and a deletion runs
	// alongside them.
	const writers = 6
	errs := make(chan error, writers+1)
	for i := range writers {
		go func() {
			errs <- c.WithNewSCIMGroupReferences(ctx, []string{"okta/00g-shared"}, func() error {
				_, err := c.CreateGroupRoleAssignment(ctx, fmt.Sprintf("okta/00g-shared-%d", i), apitypes.RoleBasic, "")
				return err
			})
		}()
	}
	go func() {
		_, err := c.MarkUnreferencedSCIMGroups(ctx, conn, map[string]struct{}{"okta/00g-shared": {}})
		errs <- err
	}()
	for range writers + 1 {
		if err := <-errs; err != nil {
			t.Fatalf("a reference write or a deletion failed with a small pool: %v", err)
		}
	}
}

func TestReferenceWritesExpireByTheDatabaseClockOnPostgres(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)

	var expiresIn time.Duration
	if err := c.WithNewSCIMGroupReferences(t.Context(), []string{"okta/00g-any"}, func() error {
		var row struct {
			ExpiresIn float64
		}
		if err := c.db.WithContext(t.Context()).
			Raw("SELECT EXTRACT(EPOCH FROM (expires_at - now())) AS expires_in FROM scim_reference_writes").
			Scan(&row).Error; err != nil {
			return err
		}
		expiresIn = time.Duration(row.ExpiresIn * float64(time.Second))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if expiresIn < scimReferenceWriteLifetime-5*time.Second || expiresIn > scimReferenceWriteLifetime {
		t.Fatalf("a write expires %v after the database's now, want about %v", expiresIn, scimReferenceWriteLifetime)
	}
}

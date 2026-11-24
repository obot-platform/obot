package client

import (
	"context"
	"sync"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestConcurrentPostgresGroupRefreshesUseConsistentLockOrder(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	groupA := types.Group{
		ID:                    "test:a",
		Name:                  "A",
		AuthProviderName:      testAuthProviderName,
		AuthProviderNamespace: testAuthProviderNamespace,
	}
	groupB := groupA
	groupB.ID = "test:b"
	groupB.Name = "B"
	require.NoError(t, c.db.WithContext(ctx).Create(&[]types.Group{groupA, groupB}).Error)

	first := groupRefreshTestIdentity(newGroupRefreshTestUser(t, c, "first"))
	second := groupRefreshTestIdentity(newGroupRefreshTestUser(t, c, "second"))
	first.AuthProviderGroupsLastChecked = time.Now()
	second.AuthProviderGroupsLastChecked = first.AuthProviderGroupsLastChecked
	first.AuthProviderGroups = []types.Group{groupA, groupB}
	second.AuthProviderGroups = []types.Group{groupA, groupB}
	first.AuthProviderGroups[1].Name = "Updated B"
	second.AuthProviderGroups[0].Name = "Updated A"

	pauseCtx, upserted, release := pausePostgresGroupWrite(ctx, t,
		c.db.WithContext(ctx).Callback().Create().After("gorm:create").Register, "groups")

	type result struct {
		claimed bool
		err     error
	}
	results := make(chan result, 2)
	refresh := func(ctx context.Context, identity *types.Identity) {
		claimed, err := c.persistGroups(ctx, identity, time.Time{})
		results <- result{claimed: claimed, err: err}
	}
	go refresh(pauseCtx, first)

	var firstPID int
	select {
	case firstPID = <-upserted:
	case result := <-results:
		t.Fatalf("first refresh finished before it reached the metadata upsert: %v", result.err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	go refresh(ctx, second)

	// The first refresh has written B. Wait until PostgreSQL reports that the second refresh
	// depends on it before letting the first proceed to its membership writes.
	waitForPostgresGroupWriteBlock(ctx, t, c, firstPID)
	release()

	for range 2 {
		select {
		case result := <-results:
			require.NoError(t, result.err)
			require.True(t, result.claimed)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for _, identity := range []*types.Identity{first, second} {
		ids, err := c.ListGroupIDsForUser(ctx, identity.UserID)
		require.NoError(t, err)
		require.ElementsMatch(t, []string{groupA.ID, groupB.ID}, ids)
	}
}

func TestPostgresMembershipAdditionsShareGroupLock(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	group := types.Group{
		ID:                    "test:shared",
		Name:                  "Shared",
		AuthProviderName:      testAuthProviderName,
		AuthProviderNamespace: testAuthProviderNamespace,
	}
	require.NoError(t, c.db.WithContext(ctx).Create(&group).Error)
	_, err := c.CreateGroupRoleAssignment(ctx, group.ID, apitypes.RoleBasic, "")
	require.NoError(t, err)

	first := groupRefreshTestIdentity(newGroupRefreshTestUser(t, c, "first"))
	second := groupRefreshTestIdentity(newGroupRefreshTestUser(t, c, "second"))
	for _, identity := range []*types.Identity{first, second} {
		identity.AuthProviderGroupsLastChecked = time.Now()
		identity.AuthProviderGroups = []types.Group{group}
	}

	pauseCtx, inserted, release := pausePostgresGroupWrite(ctx, t,
		c.db.WithContext(ctx).Callback().Create().After("gorm:create").Register, "group_memberships")
	firstResult := make(chan error, 1)
	go func() {
		_, err := c.persistGroups(pauseCtx, first, time.Time{})
		firstResult <- err
	}()
	var firstPID int
	select {
	case firstPID = <-inserted:
	case err := <-firstResult:
		t.Fatalf("first refresh finished before inserting its membership: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// The first refresh holds its group lock throughout the second refresh.
	joinCtx, cancelJoin := context.WithTimeout(ctx, 2*time.Second)
	defer cancelJoin()
	claimed, err := c.persistGroups(joinCtx, second, time.Time{})
	require.NoError(t, err, "another user should be able to join the same group before the first refresh commits")
	require.True(t, claimed)

	edited := make(chan error, 1)
	go func() {
		_, err := c.UpdateGroupRoleAssignment(ctx, group.ID, apitypes.RoleAdmin, "", true)
		edited <- err
	}()
	waitForPostgresGroupWriteBlock(ctx, t, c, firstPID)
	release()
	require.NoError(t, <-firstResult)
	require.NoError(t, <-edited)
}

func TestPostgresSCIMGroupDeletionSerializesWithRoleEdit(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	ctx := t.Context()

	conn, _ := createTestSCIMConnection(t, c, false)
	group, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{DisplayName: "Admins"})
	require.NoError(t, err)
	_, err = c.CreateGroupRoleAssignment(ctx, group.GroupID, apitypes.RoleBasic, "")
	require.NoError(t, err)
	setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)

	requirePostgresGroupDeletionSerializesWithRoleEdit(t, c, group.GroupID, func(ctx context.Context) error {
		return c.DeleteSCIMGroup(ctx, conn, group.ID)
	})
}

func TestPostgresAuthProviderGroupDataDeletionSerializesWithRoleEdit(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	group := types.Group{
		ID:                    "entra/engineering",
		AuthProviderNamespace: "default",
		AuthProviderName:      "entra-auth-provider",
		Name:                  "Engineering",
	}
	require.NoError(t, c.db.WithContext(t.Context()).Create(&group).Error)
	_, err := c.CreateGroupRoleAssignment(t.Context(), group.ID, apitypes.RoleBasic, "")
	require.NoError(t, err)

	requirePostgresGroupDeletionSerializesWithRoleEdit(t, c, group.ID, func(ctx context.Context) error {
		return c.DeleteAuthProviderGroupData(ctx, group.AuthProviderNamespace, group.AuthProviderName, "entra/")
	})
}

func requirePostgresGroupDeletionSerializesWithRoleEdit(t *testing.T, c *Client, groupID string, deleteGroup func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()

	pauseCtx, deletedMapping, release := pausePostgresGroupWrite(ctx, t,
		c.db.WithContext(ctx).Callback().Delete().After("gorm:delete").Register, "group_role_assignments")
	deleted := make(chan error, 1)
	go func() {
		deleted <- deleteGroup(pauseCtx)
	}()

	var deletePID int
	select {
	case deletePID = <-deletedMapping:
	case err := <-deleted:
		t.Fatalf("deletion finished before removing the role mapping: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	edited := make(chan error, 1)
	go func() {
		_, err := c.UpdateGroupRoleAssignment(ctx, groupID, apitypes.RoleAdmin, "", true)
		edited <- err
	}()
	waitForPostgresGroupWriteBlock(ctx, t, c, deletePID)
	release()
	require.NoError(t, <-deleted)
	require.ErrorIs(t, <-edited, ErrGroupRoleAssignmentNotFound)
}

// pausePostgresGroupWrite stops one tagged transaction immediately after the selected statement.
func pausePostgresGroupWrite(ctx context.Context, t *testing.T, register func(string, func(*gorm.DB)) error, table string) (context.Context, <-chan int, func()) {
	t.Helper()
	type pauseGroupWriteKey struct{}
	reached := make(chan int, 1)
	release := make(chan struct{})
	resume := sync.OnceFunc(func() { close(release) })
	t.Cleanup(resume)
	require.NoError(t, register("pause_group_write", func(tx *gorm.DB) {
		if tx.Error != nil || tx.Statement.Table != table || tx.Statement.Context.Value(pauseGroupWriteKey{}) == nil {
			return
		}
		var pid int
		if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
			_ = tx.AddError(err)
			return
		}
		reached <- pid
		select {
		case <-release:
		case <-ctx.Done():
			_ = tx.AddError(ctx.Err())
		}
	}))
	return context.WithValue(ctx, pauseGroupWriteKey{}, true), reached, resume
}

func waitForPostgresGroupWriteBlock(ctx context.Context, t *testing.T, c *Client, blockerPID int) {
	t.Helper()
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var blocked bool
		err := c.db.WithContext(ctx).Raw(`SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))
		)`, blockerPID).Scan(&blocked).Error
		assert.NoError(collect, err)
		assert.True(collect, blocked)
	}, 5*time.Second, 10*time.Millisecond)
}

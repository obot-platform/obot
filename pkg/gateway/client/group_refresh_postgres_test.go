package client

import (
	"context"
	"testing"
	"time"

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

	type pauseGroupRefreshKey struct{}
	upserted := make(chan int, 1)
	release := make(chan struct{}, 1)
	defer close(release)
	callbacks := c.db.WithContext(ctx).Callback().Create()
	const callback = "pause_first_group_metadata_upsert"
	require.NoError(t, callbacks.After("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Error != nil || tx.Statement.Table != "groups" || tx.Statement.Context.Value(pauseGroupRefreshKey{}) == nil {
			return
		}
		var pid int
		if err := tx.Session(&gorm.Session{NewDB: true}).Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
			_ = tx.AddError(err)
			return
		}
		upserted <- pid
		select {
		case <-release:
		case <-ctx.Done():
			_ = tx.AddError(ctx.Err())
		}
	}))

	type result struct {
		claimed bool
		err     error
	}
	results := make(chan result, 2)
	refresh := func(ctx context.Context, identity *types.Identity) {
		claimed, err := c.persistGroups(ctx, identity, time.Time{})
		results <- result{claimed: claimed, err: err}
	}
	go refresh(context.WithValue(ctx, pauseGroupRefreshKey{}, true), first)

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
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		var blocked bool
		err := c.db.WithContext(ctx).Raw(`SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))
		)`, firstPID).Scan(&blocked).Error
		assert.NoError(collect, err)
		assert.True(collect, blocked)
	}, 5*time.Second, 10*time.Millisecond)
	release <- struct{}{}

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

package client

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// userGroupProviderStub serves the /obot-list-user-auth-groups contract. It counts requests so
// tests can assert how many reached the provider, and can hold each one open so a burst of callers
// is still in flight when the assertion is made.
type userGroupProviderStub struct {
	lock     sync.Mutex
	requests int

	// block, when non-nil, holds every request until it is closed.
	block chan struct{}

	// arrived is signalled once per request, as it arrives.
	arrived chan struct{}

	// status, when non-zero, is returned instead of a group list.
	status int

	groups []auth.GroupInfo
}

func (s *userGroupProviderStub) server(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/obot-list-user-auth-groups" {
			http.NotFound(w, r)
			return
		}

		s.lock.Lock()
		s.requests++
		status, groups := s.status, s.groups
		s.lock.Unlock()

		if s.arrived != nil {
			s.arrived <- struct{}{}
		}
		if s.block != nil {
			<-s.block
		}

		if status != 0 {
			http.Error(w, "provider is rate limited", status)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(groups)
	}))
	t.Cleanup(srv.Close)

	return srv
}

func (s *userGroupProviderStub) count() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.requests
}

// recover makes the stub start serving groups again, as a provider does once it stops rate
// limiting.
func (s *userGroupProviderStub) recover(groups []auth.GroupInfo) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.status, s.groups = 0, groups
}

// newGroupRefreshTestClient builds a client that can persist a refresh: persistGroups emits events
// through the storage client whenever memberships change.
func newGroupRefreshTestClient(t *testing.T) *Client {
	t.Helper()

	c := newTestClient(t)
	c.storageClient = fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	return c
}

// newGroupRefreshTestUser inserts a user, and the identity persistGroups claims before it changes
// that user's memberships.
func newGroupRefreshTestUser(t *testing.T, c *Client, username string) uint {
	t.Helper()

	user := types.User{Username: username, HashedUsername: username, Email: username + "@example.com"}
	if err := c.db.WithContext(t.Context()).Create(&user).Error; err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := c.db.WithContext(t.Context()).Create(groupRefreshTestIdentity(user.ID)).Error; err != nil {
		t.Fatalf("failed to create identity: %v", err)
	}

	// Identities that predate the check time column carry NULL rather than a zero timestamp, and
	// the claim in persistGroups has to match that or they could never be claimed at all.
	setGroupCheckTime(t, c, user.ID, nil)

	return user.ID
}

func groupRefreshTestIdentity(userID uint) *types.Identity {
	providerUserID := fmt.Sprintf("provider-user-%d", userID)
	return &types.Identity{
		AuthProviderName:      testAuthProviderName,
		AuthProviderNamespace: testAuthProviderNamespace,
		ProviderUserID:        providerUserID,
		HashedProviderUserID:  "hashed-" + providerUserID,
		UserID:                userID,
	}
}

func storedGroupCheckTime(t *testing.T, c *Client, userID uint) time.Time {
	t.Helper()

	// NullTime, not time.Time: identities that predate the column carry NULL.
	var checked []sql.NullTime
	if err := c.db.WithContext(t.Context()).
		Model(new(types.Identity)).
		Where("user_id = ?", userID).
		Pluck(groupsLastCheckedColumn, &checked).Error; err != nil {
		t.Fatalf("failed to read group check time: %v", err)
	}
	if len(checked) != 1 {
		t.Fatalf("identities for user %d = %d, want 1", userID, len(checked))
	}

	return checked[0].Time
}

func setGroupCheckTime(t *testing.T, c *Client, userID uint, checked any) {
	t.Helper()

	if err := c.db.WithContext(t.Context()).
		Model(new(types.Identity)).
		Where("user_id = ?", userID).
		Update(groupsLastCheckedColumn, checked).Error; err != nil {
		t.Fatalf("failed to set group check time: %v", err)
	}
}

// expireGroupCooldown ages every recorded failure so the cooldown lapses without sleeping.
func expireGroupCooldown(c *Client) {
	c.groupCooldown.lock.Lock()
	defer c.groupCooldown.lock.Unlock()

	for key := range c.groupCooldown.failures {
		c.groupCooldown.failures[key] = time.Now().Add(-groupFailureCooldown)
	}
}

// TestEnsureGroupsCoalescesConcurrentRefreshes covers the case that made an Okta org return 429s:
// only a successful refresh advances the check window, so every request in flight when it lapses
// would otherwise call the provider on its own.
func TestEnsureGroupsCoalescesConcurrentRefreshes(t *testing.T) {
	const callers = 20

	stub := &userGroupProviderStub{
		block:   make(chan struct{}),
		arrived: make(chan struct{}, callers),
		groups:  []auth.GroupInfo{{ID: "entra/0001", Name: "group-0001"}},
	}
	srv := stub.server(t)

	c := newGroupRefreshTestClient(t)
	userID := newGroupRefreshTestUser(t, c, "coalesce")
	ctx := auth.ContextWithProviderURL(testGroupContext(t), srv.URL)

	var (
		wg      sync.WaitGroup
		results = make([][]types.Group, callers)
		errs    = make([]error, callers)
	)
	for i := range callers {
		wg.Go(func() {
			// Each caller gets its own identity, since ensureGroups writes to it.
			identity := groupRefreshTestIdentity(userID)
			errs[i] = c.ensureGroups(ctx, identity)
			results[i] = identity.AuthProviderGroups
		})
	}

	// Wait for the leader to reach the provider, then give the rest time to pile up behind it
	// before letting it answer. Without coalescing, every caller's request would be counted.
	<-stub.arrived
	time.Sleep(250 * time.Millisecond)
	close(stub.block)
	wg.Wait()

	if got := stub.count(); got != 1 {
		t.Errorf("auth provider requests = %d, want 1", got)
	}

	for i := range callers {
		if errs[i] != nil {
			t.Fatalf("caller %d: ensureGroups() error = %v", i, errs[i])
		}
		if len(results[i]) != 1 || results[i][0].ID != "entra/0001" {
			t.Errorf("caller %d: groups = %v, want the one group from the provider", i, results[i])
		}
	}
}

// TestEnsureGroupsBacksOffAfterFailedRefresh covers the other half of the same incident: with only
// a successful refresh advancing the check window, a failing provider would otherwise be called
// again by the very next request and never get to recover.
func TestEnsureGroupsBacksOffAfterFailedRefresh(t *testing.T) {
	stub := &userGroupProviderStub{status: http.StatusTooManyRequests}
	srv := stub.server(t)

	c := newGroupRefreshTestClient(t)
	userID := newGroupRefreshTestUser(t, c, "backoff")
	ctx := auth.ContextWithProviderURL(testGroupContext(t), srv.URL)

	// Something is already known about this user, so the cooldown has groups to fall back on.
	seedGroups(t, c, 1)
	if err := c.db.WithContext(ctx).Create(&types.GroupMemberships{UserID: userID, GroupID: "entra/0000"}).Error; err != nil {
		t.Fatalf("failed to seed membership: %v", err)
	}

	if err := c.ensureGroups(ctx, groupRefreshTestIdentity(userID)); err == nil {
		t.Fatal("ensureGroups() error = nil, want the provider's failure to surface")
	}
	if got := stub.count(); got != 1 {
		t.Fatalf("auth provider requests after first call = %d, want 1", got)
	}

	// Every later request inside the cooldown is served from the database instead.
	for i := range 5 {
		identity := groupRefreshTestIdentity(userID)
		if err := c.ensureGroups(ctx, identity); err != nil {
			t.Fatalf("call %d: ensureGroups() error = %v, want the cached groups", i, err)
		}
		if len(identity.AuthProviderGroups) != 1 || identity.AuthProviderGroups[0].ID != "entra/0000" {
			t.Errorf("call %d: groups = %v, want the cached group", i, identity.AuthProviderGroups)
		}
	}

	if got := stub.count(); got != 1 {
		t.Errorf("auth provider requests = %d, want the cooldown to have stopped at 1", got)
	}

	// The cooldown has to lapse too, or a provider that recovers would never be reached again.
	stub.recover([]auth.GroupInfo{{ID: "entra/0001", Name: "group-0001"}})
	expireGroupCooldown(c)

	identity := groupRefreshTestIdentity(userID)
	if err := c.ensureGroups(ctx, identity); err != nil {
		t.Fatalf("ensureGroups() after the cooldown error = %v", err)
	}
	if got := stub.count(); got != 2 {
		t.Errorf("auth provider requests after the cooldown = %d, want 2", got)
	}
	if len(identity.AuthProviderGroups) != 1 || identity.AuthProviderGroups[0].ID != "entra/0001" {
		t.Errorf("groups after the cooldown = %v, want what the provider now returns", identity.AuthProviderGroups)
	}

	// With the provider healthy again, the check window is what holds requests off it.
	identity = groupRefreshTestIdentity(userID)
	identity.AuthProviderGroupsLastChecked = time.Now()
	if err := c.ensureGroups(ctx, identity); err != nil {
		t.Fatalf("ensureGroups() inside the check window error = %v", err)
	}
	if got := stub.count(); got != 2 {
		t.Errorf("auth provider requests inside the check window = %d, want 2", got)
	}
}

// TestEnsureGroupsOutlivesCancelledLeader covers the hazard coalescing introduces: callers share
// one refresh, so it must not be tied to the lifetime of whichever request leads it.
func TestEnsureGroupsOutlivesCancelledLeader(t *testing.T) {
	stub := &userGroupProviderStub{
		block:   make(chan struct{}),
		arrived: make(chan struct{}, 1),
		groups:  []auth.GroupInfo{{ID: "entra/0001", Name: "group-0001"}},
	}
	srv := stub.server(t)

	c := newGroupRefreshTestClient(t)
	userID := newGroupRefreshTestUser(t, c, "cancelled")
	ctx, cancel := context.WithCancel(auth.ContextWithProviderURL(testGroupContext(t), srv.URL))

	identity := groupRefreshTestIdentity(userID)
	done := make(chan error, 1)
	go func() {
		done <- c.ensureGroups(ctx, identity)
	}()

	// Cancel the caller mid-refresh, the way a client that disconnects would.
	<-stub.arrived
	cancel()
	close(stub.block)

	if err := <-done; err != nil {
		t.Fatalf("ensureGroups() error = %v, want the refresh to complete despite the cancellation", err)
	}
	if len(identity.AuthProviderGroups) != 1 || identity.AuthProviderGroups[0].ID != "entra/0001" {
		t.Errorf("groups = %v, want the one group from the provider", identity.AuthProviderGroups)
	}
}

// TestRefreshGroupsDiscardsOvertakenRefresh covers the claim that keeps replicas apart.
// Coalescing and the cooldown are per-process, so another replica can commit a refresh for the
// same identity while this one's fetch is still in flight. The overtaken refresh must neither
// restore the memberships the other removed nor authorize its own caller against them.
func TestRefreshGroupsDiscardsOvertakenRefresh(t *testing.T) {
	stub := &userGroupProviderStub{groups: []auth.GroupInfo{{ID: "entra/0001", Name: "group-0001"}}}
	srv := stub.server(t)

	c := newGroupRefreshTestClient(t)
	userID := newGroupRefreshTestUser(t, c, "overtaken")
	seedGroups(t, c, 2)

	// What the other replica already committed: this membership, and a check time to match.
	if err := c.db.WithContext(t.Context()).Create(&types.GroupMemberships{UserID: userID, GroupID: "entra/0000"}).Error; err != nil {
		t.Fatalf("failed to seed membership: %v", err)
	}
	setGroupCheckTime(t, c, userID, time.Now())
	overtakenBy := storedGroupCheckTime(t, c, userID)

	// An identity that neither refresh touches, so a claim that is not scoped to one row is caught.
	bystander := newGroupRefreshTestUser(t, c, "bystander")

	// This refresh read the identity before that landed, so it still starts from no check time.
	identity := groupRefreshTestIdentity(userID)
	groups, err := c.refreshGroups(testGroupContext(t), srv.URL, "overtaken", *identity, time.Now())
	if err != nil {
		t.Fatalf("refreshGroups() error = %v", err)
	}

	if len(groups) != 1 || groups[0].ID != "entra/0000" {
		t.Errorf("groups = %v, want what the other replica committed, not this refresh's own result", groups)
	}
	stored, err := c.listCachedGroups(t.Context(), *identity)
	if err != nil {
		t.Fatalf("listCachedGroups() error = %v", err)
	}
	if len(stored) != 1 || stored[0].ID != "entra/0000" {
		t.Errorf("stored memberships = %v, want the overtaken refresh to have left them alone", stored)
	}
	if checked := storedGroupCheckTime(t, c, userID); !checked.Equal(overtakenBy) {
		t.Errorf("stored check time = %v, want the other replica's %v", checked, overtakenBy)
	}
	if checked := storedGroupCheckTime(t, c, bystander); !checked.IsZero() {
		t.Errorf("bystander check time = %v, want none: the claim must be scoped to one identity", checked)
	}
}

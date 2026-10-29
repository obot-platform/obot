package client

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	refreshTestProvider = "keycloak-auth-provider"
)

// profileRefreshFixture is one user signed in through a fake auth provider whose profile, status, and fetch count
// each test controls, so that every case starts from its own known state.
type profileRefreshFixture struct {
	c       *Client
	user    *types.User
	url     string
	ctx     context.Context
	fetches atomic.Int32
	status  atomic.Int32

	mu      sync.Mutex
	profile map[string]string
}

func newProfileRefreshFixture(t *testing.T) *profileRefreshFixture {
	t.Helper()
	f := &profileRefreshFixture{
		c: newLifecycleTestClient(t),
		profile: map[string]string{
			"id":       "f:storage:opaque-id",
			"name":     "First Name",
			"icon_url": "https://id.example/first.png",
		},
	}
	f.user = createLifecycleTestUser(t, f.c, "refresh-user", AuthProviderRef{Namespace: system.DefaultNamespace, Name: refreshTestProvider})
	f.status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The handler runs on the server's goroutine, where require cannot stop the test.
		f.fetches.Add(1)
		assert.Equal(t, "/obot-get-user-info", r.URL.Path)
		assert.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		if code := int(f.status.Load()); code != http.StatusOK {
			w.WriteHeader(code)
			return
		}
		f.mu.Lock()
		profile := maps.Clone(f.profile)
		f.mu.Unlock()
		assert.NoError(t, json.NewEncoder(w).Encode(profile))
	}))
	t.Cleanup(server.Close)
	f.url = server.URL
	f.ctx = accesstoken.ContextWithAccessToken(t.Context(), "access-token")
	return f
}

func (f *profileRefreshFixture) setProfile(key, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profile[key] = value
}

func (f *profileRefreshFixture) deleteProfile(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.profile, key)
}

// refresh runs the profile check a page load makes and returns the stored user.
func (f *profileRefreshFixture) refresh(t *testing.T) *types.User {
	t.Helper()
	user, err := f.c.UserByID(f.ctx, fmtUserID(f.user.ID))
	require.NoError(t, err)
	require.NoError(t, f.c.UpdateProfileIfNeeded(f.ctx, user, refreshTestProvider, system.DefaultNamespace, f.url))
	stored, err := f.c.UserByID(f.ctx, fmtUserID(f.user.ID))
	require.NoError(t, err)
	return stored
}

func (f *profileRefreshFixture) setLastChecked(t *testing.T, at time.Time) {
	t.Helper()
	require.NoError(t, f.c.db.WithContext(f.ctx).Model(new(types.Identity)).Where("user_id = ?", f.user.ID).Update("icon_last_checked", at).Error)
}

func (f *profileRefreshFixture) expectFetches(t *testing.T, want int32) {
	t.Helper()
	assert.Equal(t, want, f.fetches.Load(), "profile fetches")
}

// TestProfileRefreshSchedule pins down when a sign-in refreshes a user's display name and avatar from the auth
// provider: on the first profile read, then at most every seven days, and on every read while the user has no
// display name. Each case builds its own user, so any case can run alone.
func TestProfileRefreshSchedule(t *testing.T) {
	t.Run("first read fetches the profile and the native lookup ID", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		stored := f.refresh(t)
		f.expectFetches(t, 1)
		assert.Equal(t, "First Name", stored.DisplayName)
		assert.Equal(t, "https://id.example/first.png", stored.IconURL)
		var identity types.Identity
		require.NoError(t, f.c.db.WithContext(f.ctx).Where("user_id = ?", f.user.ID).Take(&identity).Error)
		assert.Equal(t, "f:storage:opaque-id", identity.ProviderGroupLookupID)
	})

	t.Run("reads within seven days do not fetch", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		f.refresh(t)
		f.setProfile("icon_url", "https://id.example/second.png")

		stored := f.refresh(t)
		f.expectFetches(t, 1)
		assert.Equal(t, "https://id.example/first.png", stored.IconURL)

		f.setLastChecked(t, time.Now().Add(-7*24*time.Hour+time.Hour))
		f.refresh(t)
		f.expectFetches(t, 1)
	})

	t.Run("a read after seven days fetches the profile", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		f.refresh(t)
		f.setProfile("icon_url", "https://id.example/second.png")
		f.setLastChecked(t, time.Now().Add(-7*24*time.Hour-time.Minute))

		stored := f.refresh(t)
		f.expectFetches(t, 2)
		assert.Equal(t, "https://id.example/second.png", stored.IconURL)
	})

	t.Run("reads without an access token do not fetch", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		f.refresh(t)
		f.setLastChecked(t, time.Time{})

		user, err := f.c.UserByID(t.Context(), fmtUserID(f.user.ID))
		require.NoError(t, err)
		require.NoError(t, f.c.UpdateProfileIfNeeded(t.Context(), user, refreshTestProvider, system.DefaultNamespace, f.url))
		f.expectFetches(t, 1)
	})

	t.Run("a failed fetch is retried on the next read", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		f.refresh(t)
		f.setLastChecked(t, time.Time{})

		f.status.Store(http.StatusInternalServerError)
		user, err := f.c.UserByID(f.ctx, fmtUserID(f.user.ID))
		require.NoError(t, err)
		assert.Error(t, f.c.UpdateProfileIfNeeded(f.ctx, user, refreshTestProvider, system.DefaultNamespace, f.url))
		f.expectFetches(t, 2)

		f.status.Store(http.StatusOK)
		f.refresh(t)
		f.expectFetches(t, 3)
	})

	t.Run("a profile without a picture keeps the stored avatar", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		f.refresh(t)
		f.deleteProfile("icon_url")
		f.setLastChecked(t, time.Time{})

		stored := f.refresh(t)
		f.expectFetches(t, 2)
		assert.Equal(t, "https://id.example/first.png", stored.IconURL)
	})

	t.Run("an empty name from the provider keeps the stored display name", func(t *testing.T) {
		f := newProfileRefreshFixture(t)
		f.refresh(t)
		f.setProfile("name", "")
		f.setLastChecked(t, time.Time{})

		stored := f.refresh(t)
		f.expectFetches(t, 2)
		assert.Equal(t, "First Name", stored.DisplayName)
	})

	t.Run("every read fetches while the user has no display name", func(t *testing.T) {
		// The provider has no name for the user either, so no read can fill one in.
		f := newProfileRefreshFixture(t)
		f.setProfile("name", "")

		f.refresh(t)
		f.refresh(t)
		f.refresh(t)
		f.expectFetches(t, 3)
	})
}

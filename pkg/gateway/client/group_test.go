package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	testAuthProviderName      = "entra-auth-provider"
	testAuthProviderNamespace = "default"
)

// seedGroups inserts n groups into the cache table, named so that name ordering matches ID
// ordering.
func seedGroups(t *testing.T, c *Client, n int) {
	t.Helper()

	groups := make([]types.Group, 0, n)
	for i := range n {
		groups = append(groups, types.Group{
			ID:                    fmt.Sprintf("entra/%04d", i),
			AuthProviderName:      testAuthProviderName,
			AuthProviderNamespace: testAuthProviderNamespace,
			Name:                  fmt.Sprintf("group-%04d", i),
		})
	}

	if err := c.db.WithContext(t.Context()).Create(&groups).Error; err != nil {
		t.Fatalf("failed to seed groups: %v", err)
	}
}

// newProviderStub serves the paginated /obot-list-auth-groups contract over `total` groups.
func newProviderStub(t *testing.T, total int) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/obot-list-auth-groups" {
			http.NotFound(w, r)
			return
		}

		all := make([]auth.GroupInfo, 0, total)
		for i := range total {
			all = append(all, auth.GroupInfo{
				ID:   fmt.Sprintf("entra/%04d", i),
				Name: fmt.Sprintf("group-%04d", i),
			})
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

		items := []auth.GroupInfo{}
		if offset < len(all) {
			items = all[offset:min(offset+limit, len(all))]
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "total": len(all)})
	}))
	t.Cleanup(srv.Close)

	return srv
}

// TestListAuthGroupsPagesEntireDirectory is the regression guard for the reported bug: every group
// in a large directory must be reachable through paging.
func TestListAuthGroupsPagesEntireDirectory(t *testing.T) {
	c := newTestClient(t)
	srv := newProviderStub(t, 10000)

	seen := make(map[string]struct{}, 10000)
	for offset := 0; offset < 10000; offset += 50 {
		groups, total, source, degraded, err := c.ListAuthGroups(
			t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName,
			ListAuthGroupsOptions{Limit: 50, Offset: offset},
		)
		if err != nil {
			t.Fatalf("offset %d: unexpected error: %v", offset, err)
		}
		if total != 10000 {
			t.Fatalf("offset %d: total = %d, want 10000", offset, total)
		}
		if source != types.GroupSourceProvider {
			t.Fatalf("offset %d: source = %q, want %q", offset, source, types.GroupSourceProvider)
		}
		if degraded {
			t.Fatalf("offset %d: degraded = true, want false", offset)
		}

		for _, group := range groups {
			if _, dup := seen[group.ID]; dup {
				t.Fatalf("group %s returned on more than one page", group.ID)
			}
			seen[group.ID] = struct{}{}
		}
	}

	if len(seen) != 10000 {
		t.Fatalf("paged over %d groups, want 10000", len(seen))
	}
}

func TestListAuthGroupsFallsBackToCacheOnProviderError(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "directory read permission not granted", http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)

	groups, total, source, degraded, err := c.ListAuthGroups(
		t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName,
		ListAuthGroupsOptions{Limit: 50, Offset: 0},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != types.GroupSourceCache {
		t.Errorf("source = %q, want %q", source, types.GroupSourceCache)
	}
	if !degraded {
		t.Error("degraded = false, want true when the provider fails")
	}
	if total != 120 {
		t.Errorf("total = %d, want 120", total)
	}
	if len(groups) != 50 {
		t.Errorf("len = %d, want 50", len(groups))
	}
}

// A provider without group support 404s. That is not a degraded response: the cached groups are
// all that has ever existed for it.
func TestListAuthGroupsNotFoundIsNotDegraded(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 10)

	srv := httptest.NewServer(http.HandlerFunc(http.NotFound))
	t.Cleanup(srv.Close)

	groups, total, source, degraded, err := c.ListAuthGroups(
		t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName,
		ListAuthGroupsOptions{Limit: 50, Offset: 0},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != types.GroupSourceCache {
		t.Errorf("source = %q, want %q", source, types.GroupSourceCache)
	}
	if degraded {
		t.Error("degraded = true, want false for a provider without group support")
	}
	if total != 10 || len(groups) != 10 {
		t.Errorf("total/len = %d/%d, want 10/10", total, len(groups))
	}
}

// A provider predating pagination ignores the limit and returns a bare array; Obot must page it.
func TestListAuthGroupsHandlesUnpaginatedProvider(t *testing.T) {
	c := newTestClient(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		all := make([]auth.GroupInfo, 0, 300)
		for i := range 300 {
			all = append(all, auth.GroupInfo{ID: fmt.Sprintf("entra/%04d", i), Name: fmt.Sprintf("group-%04d", i)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(all)
	}))
	t.Cleanup(srv.Close)

	groups, total, source, _, err := c.ListAuthGroups(
		t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName,
		ListAuthGroupsOptions{Limit: 25, Offset: 275},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if source != types.GroupSourceProvider {
		t.Errorf("source = %q, want %q", source, types.GroupSourceProvider)
	}
	if total != 300 {
		t.Errorf("total = %d, want 300", total)
	}
	if len(groups) != 25 {
		t.Fatalf("len = %d, want 25", len(groups))
	}
	if groups[0].ID != "entra/0275" {
		t.Errorf("first = %s, want entra/0275", groups[0].ID)
	}
}

func TestListAuthGroupsCachePagingBoundaries(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)

	tests := []struct {
		name          string
		limit, offset int
		wantLen       int
		wantFirst     string
	}{
		{
			name:      "first page",
			limit:     50,
			offset:    0,
			wantLen:   50,
			wantFirst: "entra/0000",
		},
		{
			name:      "partial last page",
			limit:     50,
			offset:    100,
			wantLen:   20,
			wantFirst: "entra/0100",
		},
		{
			name:    "offset at end",
			limit:   50,
			offset:  120,
			wantLen: 0,
		},
		{
			name:    "offset past end",
			limit:   50,
			offset:  9999,
			wantLen: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups, total, _, _, err := c.ListAuthGroups(
				t.Context(), "", testAuthProviderNamespace, testAuthProviderName,
				ListAuthGroupsOptions{Limit: tt.limit, Offset: tt.offset},
			)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if total != 120 {
				t.Errorf("total = %d, want 120", total)
			}
			if len(groups) != tt.wantLen {
				t.Fatalf("len = %d, want %d", len(groups), tt.wantLen)
			}
			if tt.wantLen > 0 && groups[0].ID != tt.wantFirst {
				t.Errorf("first = %s, want %s", groups[0].ID, tt.wantFirst)
			}
		})
	}
}

func TestListAuthGroupsCacheNameFilterAffectsTotal(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)

	_, total, _, _, err := c.ListAuthGroups(
		t.Context(), "", testAuthProviderNamespace, testAuthProviderName,
		ListAuthGroupsOptions{NameFilter: "group-001", Limit: 50, Offset: 0},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// group-0010 through group-0019.
	if total != 10 {
		t.Errorf("total = %d, want 10 (the filtered count, not the full 120)", total)
	}
}

func TestResolveAuthGroups(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 20)

	t.Run("resolves known IDs to names", func(t *testing.T) {
		groups, err := c.ResolveAuthGroups(t.Context(), testAuthProviderNamespace, testAuthProviderName,
			[]string{"entra/0003", "entra/0007"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(groups) != 2 {
			t.Fatalf("len = %d, want 2", len(groups))
		}
		if groups[0].Name != "group-0003" || groups[1].Name != "group-0007" {
			t.Errorf("names = %q/%q, want group-0003/group-0007", groups[0].Name, groups[1].Name)
		}
	})

	// An assignment on a group the cache has never seen must still produce a row, otherwise it
	// disappears from the Group Role Assignments table entirely.
	t.Run("unknown IDs are returned rather than dropped", func(t *testing.T) {
		groups, err := c.ResolveAuthGroups(t.Context(), testAuthProviderNamespace, testAuthProviderName,
			[]string{"entra/0003", "entra/9999"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(groups) != 2 {
			t.Fatalf("len = %d, want 2", len(groups))
		}
		if groups[1].ID != "entra/9999" || groups[1].Name != "entra/9999" {
			t.Errorf("unresolved group = %+v, want ID and Name both entra/9999", groups[1])
		}
	})

	t.Run("preserves requested order", func(t *testing.T) {
		want := []string{"entra/0005", "entra/0001", "entra/0009"}
		groups, err := c.ResolveAuthGroups(t.Context(), testAuthProviderNamespace, testAuthProviderName, want)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for i, id := range want {
			if groups[i].ID != id {
				t.Errorf("position %d = %s, want %s", i, groups[i].ID, id)
			}
		}
	})

	t.Run("empty input", func(t *testing.T) {
		groups, err := c.ResolveAuthGroups(t.Context(), testAuthProviderNamespace, testAuthProviderName, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(groups) != 0 {
			t.Errorf("len = %d, want 0", len(groups))
		}
	})
}

package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	testAuthProviderName      = "entra-auth-provider"
	testAuthProviderNamespace = "default"
)

// providerStub serves the cursor-based /obot-list-auth-groups contract over `total` groups, using
// an offset as its cursor. It records the cursor of every request so tests can assert what the
// client actually sent.
type providerStub struct {
	total    int
	cursors  []string
	requests int
}

// resolverStub serves the /obot-get-auth-groups contract, naming every ID it is asked about except
// those in unknown. It records each batch so tests can assert what the client actually sent.
type resolverStub struct {
	unknown  map[string]bool
	requests [][]string
	status   int
}

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

func (p *providerStub) server(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/obot-list-auth-groups" {
			http.NotFound(w, r)
			return
		}

		p.requests++
		cursor := r.URL.Query().Get("cursor")
		p.cursors = append(p.cursors, cursor)

		all := make([]auth.GroupInfo, 0, p.total)
		for i := range p.total {
			all = append(all, auth.GroupInfo{
				ID:   fmt.Sprintf("entra/%04d", i),
				Name: fmt.Sprintf("group-%04d", i),
			})
		}

		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		start := 0
		if cursor != "" {
			start, _ = strconv.Atoi(cursor)
		}
		start = min(start, len(all))
		end := min(start+limit, len(all))

		body := map[string]any{"items": all[start:end]}
		if end < len(all) {
			body["nextCursor"] = strconv.Itoa(end)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)

	return srv
}

// walkAll pages the whole listing and returns the IDs it saw, failing on a repeat or a runaway.
func walkAll(t *testing.T, c *Client, providerURL string, opts ListAuthGroupsOptions) ([]string, ListAuthGroupsResult) {
	t.Helper()

	var (
		seen []string
		last ListAuthGroupsResult
	)
	unique := map[string]struct{}{}

	for pages := 0; ; pages++ {
		if pages > 1000 {
			t.Fatal("pagination did not terminate")
		}

		result, err := c.ListAuthGroups(t.Context(), providerURL, testAuthProviderNamespace, testAuthProviderName, opts)
		if err != nil {
			t.Fatalf("ListAuthGroups() error = %v", err)
		}
		last = result

		for _, group := range result.Groups {
			if _, ok := unique[group.ID]; ok {
				t.Fatalf("group %s was returned more than once", group.ID)
			}
			unique[group.ID] = struct{}{}
			seen = append(seen, group.ID)
		}

		if result.NextCursor == "" {
			break
		}
		opts.Cursor = result.NextCursor
	}

	return seen, last
}

// TestListAuthGroupsPagesEntireDirectory is the regression guard for the reported bug: every group
// in a large directory must be reachable through paging.
func TestListAuthGroupsPagesEntireDirectory(t *testing.T) {
	c := newTestClient(t)
	stub := &providerStub{total: 10000}
	srv := stub.server(t)

	seen, _ := walkAll(t, c, srv.URL, ListAuthGroupsOptions{Limit: 100})

	if len(seen) != 10000 {
		t.Errorf("collected %d groups, want 10000", len(seen))
	}
	// One request per page and no more: the client must never enumerate on the provider's behalf.
	if stub.requests != 100 {
		t.Errorf("made %d provider requests, want 100", stub.requests)
	}
	if stub.cursors[0] != "" {
		t.Errorf("the first request carried cursor %q, want none", stub.cursors[0])
	}
}

func TestListAuthGroupsForwardsProviderCursor(t *testing.T) {
	c := newTestClient(t)
	stub := &providerStub{total: 500}
	srv := stub.server(t)

	first, err := c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}
	if first.NextCursor == "" {
		t.Fatal("NextCursor should be set when more groups remain")
	}
	// The provider's own cursor must not leak to callers unwrapped.
	if first.NextCursor == "50" {
		t.Error("the provider cursor should be wrapped, not passed through verbatim")
	}

	if _, err = c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, Cursor: first.NextCursor}); err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}
	if got := stub.cursors[1]; got != "50" {
		t.Errorf("the provider received cursor %q, want %q", got, "50")
	}
}

// A cursor is bound to the search it was minted for. Changing the filter restarts the listing
// rather than returning a page from the wrong result set.
func TestListAuthGroupsResetsWhenNameFilterChanges(t *testing.T) {
	c := newTestClient(t)
	stub := &providerStub{total: 500}
	srv := stub.server(t)

	first, err := c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, NameFilter: "eng"})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if _, err = c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{
		Limit:      50,
		NameFilter: "sales",
		Cursor:     first.NextCursor,
	}); err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if got := stub.cursors[1]; got != "" {
		t.Errorf("the provider received cursor %q after the filter changed, want none", got)
	}
}

// A cursor minted while the provider was serving must not be replayed against the cache, and vice
// versa, because neither can interpret the other's position.
func TestListAuthGroupsIgnoresCursorFromTheOtherSource(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)
	stub := &providerStub{total: 500}
	srv := stub.server(t)

	fromProvider, err := c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	// Same cursor, but now there is no provider, so the cache answers.
	fromCache, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, Cursor: fromProvider.NextCursor})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if fromCache.Source != types.GroupSourceCache {
		t.Fatalf("Source = %q, want %q", fromCache.Source, types.GroupSourceCache)
	}
	if len(fromCache.Groups) == 0 {
		t.Fatal("expected a page of cached groups")
	}
	if got, want := fromCache.Groups[0].ID, "entra/0000"; got != want {
		t.Errorf("first group = %q, want %q; a provider cursor should restart the cached listing", got, want)
	}
}

func TestListAuthGroupsFallsBackToCacheOnProviderError(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 10)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	result, err := c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if result.Source != types.GroupSourceCache {
		t.Errorf("Source = %q, want %q", result.Source, types.GroupSourceCache)
	}
	if !result.Degraded {
		t.Error("Degraded should be true when the provider could not be listed")
	}
	if len(result.Groups) != 10 {
		t.Errorf("got %d groups, want 10", len(result.Groups))
	}
}

// A provider that rejects the cursor has usually expired its own continuation token. That is a
// restart, not an outage, so the listing must come back from the provider rather than the cache.
func TestListAuthGroupsRetriesFromFirstPageOnRejectedCursor(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 10)

	var requests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("cursor") != "" {
			http.Error(w, "invalid cursor", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []auth.GroupInfo{{ID: "entra/0000", Name: "group-0000"}},
		})
	}))
	t.Cleanup(srv.Close)

	stale, err := encodeGroupCursor(groupCursor{Source: types.GroupSourceProvider, ProviderCursor: "999"})
	if err != nil {
		t.Fatalf("encodeGroupCursor() error = %v", err)
	}

	result, err := c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, Cursor: stale})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if result.Source != types.GroupSourceProvider {
		t.Errorf("Source = %q, want %q; an expired cursor should restart, not degrade", result.Source, types.GroupSourceProvider)
	}
	if result.Degraded {
		t.Error("Degraded should be false when the retry succeeded")
	}
	if requests != 2 {
		t.Errorf("made %d requests, want 2 (the rejected cursor then the restart)", requests)
	}
}

func TestListAuthGroupsNotFoundIsNotDegraded(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 3)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	result, err := c.ListAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if result.Source != types.GroupSourceCache {
		t.Errorf("Source = %q, want %q", result.Source, types.GroupSourceCache)
	}
	// A provider without group support has nothing to be degraded from.
	if result.Degraded {
		t.Error("Degraded should be false when the provider does not implement group listing")
	}
}

func TestListAuthGroupsCachePagesEveryGroupExactlyOnce(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 250)

	seen, _ := walkAll(t, c, "", ListAuthGroupsOptions{Limit: 50})

	if len(seen) != 250 {
		t.Errorf("collected %d groups, want 250", len(seen))
	}
	for i, id := range seen {
		if want := fmt.Sprintf("entra/%04d", i); id != want {
			t.Fatalf("group at position %d = %q, want %q", i, id, want)
		}
	}
}

// Keyset paging orders by (name, id), so groups sharing a name must not shadow one another.
func TestListAuthGroupsCachePagesDuplicateNames(t *testing.T) {
	c := newTestClient(t)

	groups := []types.Group{
		{ID: "entra/a", AuthProviderName: testAuthProviderName, AuthProviderNamespace: testAuthProviderNamespace, Name: "duplicate"},
		{ID: "entra/b", AuthProviderName: testAuthProviderName, AuthProviderNamespace: testAuthProviderNamespace, Name: "duplicate"},
		{ID: "entra/c", AuthProviderName: testAuthProviderName, AuthProviderNamespace: testAuthProviderNamespace, Name: "duplicate"},
	}
	if err := c.db.WithContext(t.Context()).Create(&groups).Error; err != nil {
		t.Fatalf("failed to seed groups: %v", err)
	}

	// A page size of one forces the tiebreak to do the work.
	seen, _ := walkAll(t, c, "", ListAuthGroupsOptions{Limit: 1})

	if len(seen) != 3 {
		t.Errorf("collected %d groups, want 3; the id tiebreak should separate identical names", len(seen))
	}
}

func TestListAuthGroupsCacheLastPageHasNoCursor(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 50)

	result, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if len(result.Groups) != 50 {
		t.Errorf("got %d groups, want 50", len(result.Groups))
	}
	// Exactly one full page and nothing beyond it.
	if result.NextCursor != "" {
		t.Error("an exactly-full final page should not advertise a successor")
	}
}

func TestListAuthGroupsCacheNameFilter(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)

	seen, _ := walkAll(t, c, "", ListAuthGroupsOptions{Limit: 50, NameFilter: "group-001"})

	// group-0010 through group-0019: ten matches.
	if len(seen) != 10 {
		t.Errorf("collected %d groups, want 10", len(seen))
	}
}

func TestGroupCursorRoundTrip(t *testing.T) {
	original := groupCursor{Source: types.GroupSourceCache, FilterFingerprint: groupFilterFingerprint("eng"), LastName: "group-0010", LastID: "entra/0010"}

	encoded, err := encodeGroupCursor(original)
	if err != nil {
		t.Fatalf("encodeGroupCursor() error = %v", err)
	}

	decoded, ok := decodeGroupCursor(encoded, "eng")
	if !ok {
		t.Fatal("decodeGroupCursor() reported the cursor as unusable")
	}
	if decoded.LastName != original.LastName || decoded.LastID != original.LastID || decoded.Source != original.Source {
		t.Errorf("decoded = %+v, want %+v", decoded, original)
	}
}

func TestDecodeGroupCursorRejectsUnusableCursors(t *testing.T) {
	valid, err := encodeGroupCursor(groupCursor{Source: types.GroupSourceCache, FilterFingerprint: groupFilterFingerprint("eng"), LastName: "n", LastID: "i"})
	if err != nil {
		t.Fatalf("encodeGroupCursor() error = %v", err)
	}

	tests := []struct {
		name   string
		cursor string
		filter string
	}{
		{
			name:   "empty",
			cursor: "",
			filter: "eng",
		},
		{
			name:   "not base64",
			cursor: "not!base64",
			filter: "eng",
		},
		{
			name:   "filter changed",
			cursor: valid,
			filter: "sales",
		},
		{
			name:   "filter dropped",
			cursor: valid,
			filter: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, ok := decodeGroupCursor(tt.cursor, tt.filter); ok {
				t.Error("decodeGroupCursor() accepted a cursor it should have rejected")
			}
		})
	}
}

func TestEncodeGroupCursorEmptyPositionIsEmpty(t *testing.T) {
	encoded, err := encodeGroupCursor(groupCursor{Source: types.GroupSourceCache, FilterFingerprint: groupFilterFingerprint("eng")})
	if err != nil {
		t.Fatalf("encodeGroupCursor() error = %v", err)
	}
	if encoded != "" {
		t.Errorf("encodeGroupCursor() = %q, want an empty cursor when there is no position", encoded)
	}
}

func TestResolveAuthGroups(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 5)

	resolved, err := c.ResolveAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, []string{"entra/0001", "entra/9999"})
	if err != nil {
		t.Fatalf("ResolveAuthGroups() error = %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("got %d groups, want 2", len(resolved))
	}
	if resolved[0].Name != "group-0001" {
		t.Errorf("known group name = %q, want %q", resolved[0].Name, "group-0001")
	}
	// An unknown ID renders as itself rather than being dropped.
	if resolved[1].Name != "entra/9999" {
		t.Errorf("unknown group name = %q, want the ID itself", resolved[1].Name)
	}
}

func (s *resolverStub) server(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/obot-get-auth-groups" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if s.status != 0 {
			w.WriteHeader(s.status)
			return
		}

		ids := strings.Split(r.URL.Query().Get("ids"), ",")
		s.requests = append(s.requests, ids)

		items := make([]auth.GroupInfo, 0, len(ids))
		for _, id := range ids {
			if s.unknown[id] {
				continue
			}
			items = append(items, auth.GroupInfo{ID: id, Name: "Directory " + id})
		}

		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	t.Cleanup(srv.Close)

	return srv
}

func TestResolveAuthGroupsFallsBackToTheProvider(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 5)

	stub := &resolverStub{}
	srv := stub.server(t)

	// entra/0001 is cached; entra/9999 has never been seen at sign-in and only the provider knows it.
	resolved, err := c.ResolveAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, []string{"entra/0001", "entra/9999"})
	if err != nil {
		t.Fatalf("ResolveAuthGroups() error = %v", err)
	}

	if len(resolved) != 2 {
		t.Fatalf("got %d groups, want 2", len(resolved))
	}
	if resolved[0].Name != "group-0001" {
		t.Errorf("cached group name = %q, want %q", resolved[0].Name, "group-0001")
	}
	if resolved[1].Name != "Directory entra/9999" {
		t.Errorf("provider-resolved name = %q, want the directory name", resolved[1].Name)
	}

	// Only the ID the cache could not answer for is worth asking about.
	if len(stub.requests) != 1 {
		t.Fatalf("made %d provider requests, want 1", len(stub.requests))
	}
	if len(stub.requests[0]) != 1 || stub.requests[0][0] != "entra/9999" {
		t.Errorf("asked the provider for %v, want only the uncached id", stub.requests[0])
	}
}

func TestResolveAuthGroupsCachesWhatTheProviderResolved(t *testing.T) {
	c := newTestClient(t)

	stub := &resolverStub{}
	srv := stub.server(t)

	if _, err := c.ResolveAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, []string{"entra/9999"}); err != nil {
		t.Fatalf("ResolveAuthGroups() error = %v", err)
	}

	// The second call is served from the table, so the provider is not asked again.
	resolved, err := c.ResolveAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, []string{"entra/9999"})
	if err != nil {
		t.Fatalf("ResolveAuthGroups() error = %v", err)
	}

	if resolved[0].Name != "Directory entra/9999" {
		t.Errorf("name = %q, want the directory name to have been cached", resolved[0].Name)
	}
	if len(stub.requests) != 1 {
		t.Errorf("made %d provider requests, want 1; the first answer should have been cached", len(stub.requests))
	}
}

func TestResolveAuthGroupsDegradesToIDs(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{
			name:   "provider does not implement resolution",
			status: http.StatusNotFound,
		},
		{
			name:   "provider is failing",
			status: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t)

			stub := &resolverStub{status: tt.status}
			srv := stub.server(t)

			resolved, err := c.ResolveAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, []string{"entra/9999"})
			if err != nil {
				t.Fatalf("resolution is best effort and should not fail the caller: %v", err)
			}

			if len(resolved) != 1 {
				t.Fatalf("got %d groups, want 1", len(resolved))
			}
			if resolved[0].Name != "entra/9999" {
				t.Errorf("name = %q, want the ID itself", resolved[0].Name)
			}
		})
	}
}

func TestResolveAuthGroupsKeepsUnknownIDsWhenTheProviderHasNoAnswer(t *testing.T) {
	c := newTestClient(t)

	stub := &resolverStub{unknown: map[string]bool{"entra/deleted": true}}
	srv := stub.server(t)

	resolved, err := c.ResolveAuthGroups(t.Context(), srv.URL, testAuthProviderNamespace, testAuthProviderName, []string{"entra/deleted"})
	if err != nil {
		t.Fatalf("ResolveAuthGroups() error = %v", err)
	}

	// A group deleted in the directory still has a policy pointing at it, so the row has to render.
	if len(resolved) != 1 || resolved[0].Name != "entra/deleted" {
		t.Errorf("resolved = %+v, want the ID rendered as its own name", resolved)
	}
}

func TestListAuthGroupsCacheEmptyPageIsNotNil(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 5)

	result, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, NameFilter: "nothing-matches-this"})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	// Not just empty: non-nil, so that the response serializes items as [] rather than null.
	if result.Groups == nil {
		t.Error("an empty page should be an empty slice, not nil")
	}
	if len(result.Groups) != 0 {
		t.Errorf("got %d groups, want 0", len(result.Groups))
	}
}

func TestListAuthGroupsCacheDefaultsAnUnsetLimit(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 5)

	// Limit is exported and a caller can leave it unset; the truncation below it must not run off
	// the end of the slice.
	result, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if len(result.Groups) != 5 {
		t.Errorf("got %d groups, want 5", len(result.Groups))
	}
}

func TestListAuthGroupsReportsACursorItCouldNotHonor(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)

	// A cursor minted by the provider says nothing about a position in the cached table, so the
	// cached listing starts over. The caller has to be told, or it labels page one as page two.
	providerCursor, err := encodeGroupCursor(groupCursor{
		Source:         types.GroupSourceProvider,
		ProviderCursor: "some-provider-token",
	})
	if err != nil {
		t.Fatalf("encodeGroupCursor() error = %v", err)
	}

	result, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, Cursor: providerCursor})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}

	if !result.Reset {
		t.Error("a cursor that could not be honored should be reported as a reset")
	}
	if len(result.Groups) == 0 || result.Groups[0].Name != "group-0000" {
		t.Error("the listing should have restarted at the first page")
	}
}

func TestListAuthGroupsDoesNotReportAResetWhenTheCursorWasHonored(t *testing.T) {
	c := newTestClient(t)
	seedGroups(t, c, 120)

	first, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}
	if first.Reset {
		t.Error("a first page was not asked to continue from anywhere and cannot be a reset")
	}

	second, err := c.ListAuthGroups(t.Context(), "", testAuthProviderNamespace, testAuthProviderName, ListAuthGroupsOptions{Limit: 50, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("ListAuthGroups() error = %v", err)
	}
	if second.Reset {
		t.Error("a cursor the cache minted itself should be honored, not reset")
	}
}

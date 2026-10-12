package scim

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

const (
	// fixtureBaseURL is the base URL of the Phase 0 capture responder, as the sanitized fixtures record it.
	fixtureBaseURL = "https://scim-capture.example.invalid/scim/v2"
	// fixtureUserAgent identifies the requests that Okta sent. The others are local smoke checks.
	fixtureUserAgent = "Okta SCIM Client"
)

// fixture is one sanitized request and response captured from Okta in Phase 0.
type fixture struct {
	Sequence     int             `json:"sequence"`
	UserAgent    string          `json:"userAgent"`
	Method       string          `json:"method"`
	Path         string          `json:"path"`
	Query        string          `json:"query"`
	RequestBody  json.RawMessage `json:"requestBody"`
	Status       int             `json:"status"`
	ResponseBody map[string]any  `json:"responseBody"`
}

// phase0Inventory is the Obot data of the Phase 0 development installation, keyed by the pseudonyms the
// fixtures use for native Okta user IDs.
type phase0Inventory struct {
	owner, basic, unprovisioned *types.User
}

// replay is the state of replaying a Phase 0 capture run: the Okta requests are sent with the responder's IDs
// remapped to the IDs this server issues, and every response must match the one Okta received.
type replay struct {
	s *scimTest
	// ids maps responder IDs to the IDs this server issued.
	ids map[string]string
	// eventsAfter is the number of lifecycle events recorded after each sequence number.
	eventsAfter map[int]int64
}

func TestReplayPhase0Run003(t *testing.T) {
	testReplayPhase0Run003(t, newSCIMTest(t))
}

func TestReplayPhase0Run003Postgres(t *testing.T) {
	testReplayPhase0Run003(t, newPostgresSCIMTest(t))
}

func testReplayPhase0Run003(t *testing.T, s *scimTest) {
	t.Helper()

	// Run 003 used an AIW application against legacy data seeded like the Phase 0 inventory, plus a synthetic
	// legacy obot-scim-empty group whose two members are both stale.
	inv := seedPhase0Inventory(s, "okta-user-2", "okta-user-6")
	s.seedGroup("okta/00gEmptyGroup0000001", "obot-scim-empty", inv.owner.ID, inv.basic.ID)
	usersBefore := s.count(new(types.User), "")
	s.enable()

	r := replayFixtures(t, s, "run-003.json")

	// Users are bound by native ID and keep their IDs. The never-provisioned user stays unbound, and one user
	// is created for the new Okta-only user.
	owner := r.boundUser("701e85e1-af8f-4eb1-a208-68f7ea2c2205")
	basic := r.boundUser("1556d517-a8b1-4908-ae01-6a162000118c")
	newUser := r.boundUser("4758eebb-5c1a-4d27-9ad6-af932380308c")
	if owner != inv.owner.ID || basic != inv.basic.ID {
		t.Fatalf("existing users bound to %d and %d, want %d and %d", owner, basic, inv.owner.ID, inv.basic.ID)
	}
	if binding, _ := s.gateway.SCIMUserBindingForUser(t.Context(), inv.unprovisioned.ID); binding != nil {
		t.Fatal("the never-provisioned user was bound")
	}
	if got := s.count(new(types.User), ""); got != usersBefore+1 {
		t.Fatalf("got %d users, want %d", got, usersBefore+1)
	}
	assertNoDataLoss(t, s)

	// obot-scim-empty bound by name, keeping its ID. Its stale members were dropped in the binding write.
	r.assertGroup("a4badfc2-e6d4-4913-9f09-e2ef4f516d43", "okta/00gEmptyGroup0000001", "obot-scim-empty", newUser)

	// The stale name created an accidental group, which was deleted in the target and so is unbound and empty.
	accidental := r.retiredGroup("207e0aa4-90d4-44f7-a28d-78de5db212d9")
	if strings.HasPrefix(accidental, "okta/00g") || len(s.memberships(accidental)) != 0 {
		t.Fatalf("accidental group %s has members %v", accidental, s.memberships(accidental))
	}

	// The fix bound the Okta group to the existing test group, keeping its ID and dropping the stale member, and
	// the rename back changed only the display name.
	r.assertGroup("7ad0801c-386d-4dd7-b05a-17ae2759ff2d", "okta/00gTestGroup00000001", "test-renamed", owner, newUser)

	// A new group, whose members then changed over two AIW read-then-write rounds.
	newGroupID := r.assertGroup("e7f10bd3-ac84-4f90-96f3-68101f3ed5e2", "", "obot-scim-new", newUser)
	if !strings.HasPrefix(newGroupID, "okta/") || newGroupID == "okta/e7f10bd3-ac84-4f90-96f3-68101f3ed5e2" {
		t.Fatalf("new group ID = %q, want okta/<a SCIM ID this server issued>", newGroupID)
	}

	// Groups that Okta never pushed keep their frozen memberships.
	if got := s.memberships("okta/00gAssignedGroup0001"); !slices.Equal(got, sortedIDs(inv.owner.ID, inv.basic.ID)) {
		t.Fatalf("obot-scim-assigned memberships = %v", got)
	}
	if got := s.memberships("okta/00gEveryoneGroup0001"); !slices.Equal(got, sortedIDs(inv.owner.ID, inv.basic.ID, inv.unprovisioned.ID)) {
		t.Fatalf("Everyone memberships = %v", got)
	}

	// The new user was deactivated and reactivated over PUT, with the same account throughout.
	if u := s.user(newUser); u.DisabledAt != nil {
		t.Fatalf("the reactivated user is disabled: %+v", u)
	}
	events := s.events(newUser)
	if !slices.ContainsFunc(events, func(e types.UserLifecycleEvent) bool { return e.Type == types.UserLifecycleEventDisabled }) {
		t.Fatalf("the new user was never disabled; events = %+v", events)
	}

	// The AIW echo PUTs, including those after reactivation, change nothing and emit nothing.
	r.assertNoEffect(16, 20, 26, 29, 33, 46, 47, 49, 56, 61, 62, 64, 67, 69, 71)
}

func TestReplayPhase0Run001(t *testing.T) {
	testReplayPhase0Run001(t, newSCIMTest(t))
}

func TestReplayPhase0Run001Postgres(t *testing.T) {
	testReplayPhase0Run001(t, newPostgresSCIMTest(t))
}

func testReplayPhase0Run001(t *testing.T, s *scimTest) {
	t.Helper()

	// Run 001 used the catalog test app, which sends PATCH, against data seeded like the Phase 0 inventory.
	inv := seedPhase0Inventory(s, "okta-user-6", "okta-user-4")
	usersBefore := s.count(new(types.User), "")
	s.enable()

	r := replayFixtures(t, s, "run-001.json")

	owner := r.boundUser("f750689c-4994-41c7-afd0-81ae51fca0b1")
	basic := r.boundUser("add54f90-c78e-4df2-a945-64bd4defe307")
	newUser := r.boundUser("b46b3f1c-15f1-4548-b862-3598896ca9e1")
	if owner != inv.owner.ID || basic != inv.basic.ID {
		t.Fatalf("existing users bound to %d and %d, want %d and %d", owner, basic, inv.owner.ID, inv.basic.ID)
	}
	if got := s.count(new(types.User), ""); got != usersBefore+1 {
		t.Fatalf("got %d users, want %d", got, usersBefore+1)
	}
	assertNoDataLoss(t, s)

	// test bound by name to the existing group, then renamed, gained the new user, and lost the Basic user.
	r.assertGroup("507a7364-9515-4740-9b44-5aaa44f80af3", "okta/00gTestGroup00000001", "test-renamed", owner, newUser)

	// obot-scim-empty was created, deleted in the target, and pushed again. The second push bound the group the
	// first push created, which the delete had returned to unbound.
	first := r.retiredGroup("08319b99-7242-4c7d-9d0f-a94d82c05358")
	r.assertGroup("3c0becca-81df-40a4-a735-cb819a33ea5c", first, "obot-scim-empty")

	// The new user was deactivated and reactivated over PATCH, and renamed without a new identity.
	if u := s.user(newUser); u.DisabledAt != nil || u.Email != "user-7@example.invalid" {
		t.Fatalf("new user = %+v", u)
	}
	resp := s.do(http.MethodGet, "Users/"+r.ids["b46b3f1c-15f1-4548-b862-3598896ca9e1"], nil).expect(t, http.StatusOK)
	if resp.body["userName"] != "user-7@example.invalid" || resp.body["externalId"] != "okta-user-5" {
		t.Fatalf("renamed user = %v", resp.body)
	}

	// The redundant add after the push, the name reassertions, and Push now's authoritative replace of an
	// unchanged set change nothing and emit nothing.
	r.assertNoEffect(24, 30, 34, 37, 40, 41, 43)
}

// seedPhase0Inventory creates the Okta users and groups of the Phase 0 inventory. The owner and basic users
// get the native IDs the run's fixtures use for them.
func seedPhase0Inventory(s *scimTest, ownerNativeID, basicNativeID string) phase0Inventory {
	s.t.Helper()

	inv := phase0Inventory{
		owner:         s.seedUser(ownerNativeID, "owner@example.com", types2.RoleOwner),
		basic:         s.seedUser(basicNativeID, "basic@example.com", types2.RoleBasic),
		unprovisioned: s.seedUser("okta-user-unprovisioned", "unprovisioned@example.com", types2.RoleBasic),
	}
	s.seedGroup("okta/00gAssignedGroup0001", "obot-scim-assigned", inv.owner.ID, inv.basic.ID)
	s.seedGroup("okta/00gEveryoneGroup0001", "Everyone", inv.owner.ID, inv.basic.ID, inv.unprovisioned.ID)
	s.seedGroup("okta/00gTestGroup00000001", "test", inv.owner.ID, inv.basic.ID)
	return inv
}

func replayFixtures(t *testing.T, s *scimTest, name string) *replay {
	t.Helper()

	data, err := os.ReadFile(filepath.Join("testdata", "phase0", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []fixture
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}

	r := &replay{
		s:           s,
		ids:         map[string]string{},
		eventsAfter: map[int]int64{},
	}
	for _, f := range fixtures {
		// Local smoke checks are not Okta's traffic, and a 401 was a probe whose token is not recorded.
		if !strings.HasPrefix(f.UserAgent, fixtureUserAgent) || f.Status == http.StatusUnauthorized {
			continue
		}

		resource := r.remap(strings.TrimPrefix(f.Path, "/scim/v2/"))
		if f.Query != "" {
			resource += "?" + r.remap(f.Query)
		}
		var body any
		if len(f.RequestBody) > 0 && string(f.RequestBody) != "null" {
			body = r.remap(string(f.RequestBody))
		}

		resp := s.do(f.Method, resource, body)
		if f.Method == http.MethodPatch && strings.HasPrefix(resource, "Groups/") && f.Status == http.StatusOK {
			// The responder answered a group PATCH with the group, where this server answers with no body. The
			// group it reads afterwards must match the responder's answer.
			resp.expect(t, http.StatusNoContent)
			resp = s.do(http.MethodGet, resource, nil)
		}
		if resp.status != f.Status {
			t.Fatalf("%04d %s %s: got status %d, want %d: %v", f.Sequence, f.Method, f.Path, resp.status, f.Status, resp.body)
		}

		// Learn the IDs this server issued for the responder's.
		if fixtureID, _ := f.ResponseBody["id"].(string); fixtureID != "" && f.Method == http.MethodPost {
			r.ids[fixtureID] = resp.id()
		}

		if want, got := r.normalize(f.ResponseBody, false), r.normalize(resp.body, true); !jsonEqual(want, got) {
			t.Fatalf("%04d %s %s: response differs\nwant %s\n got %s", f.Sequence, f.Method, f.Path, mustJSON(want), mustJSON(got))
		}
		r.eventsAfter[f.Sequence] = s.count(new(types.UserLifecycleEvent), "")
	}
	return r
}

// remap replaces the responder's IDs and base URL in s with this server's.
func (r *replay) remap(s string) string {
	s = strings.ReplaceAll(s, fixtureBaseURL, BaseURL(testServerURL))
	for fixtureID, id := range r.ids {
		s = strings.ReplaceAll(s, fixtureID, id)
	}
	return s
}

// normalize reduces a response to the attributes that must match between the responder and this server, with
// this server's IDs mapped back to the responder's.
func (r *replay) normalize(body map[string]any, ours bool) any {
	if body == nil {
		return nil
	}

	if resources, ok := body["Resources"].([]any); ok {
		items := make([]any, 0, len(resources))
		for _, resource := range resources {
			m, _ := resource.(map[string]any)
			items = append(items, r.normalize(m, ours))
		}
		sort.Slice(items, func(i, j int) bool {
			return mustJSON(items[i]) < mustJSON(items[j])
		})
		return map[string]any{
			"totalResults": body["totalResults"],
			"Resources":    items,
		}
	}

	out := map[string]any{}
	for _, key := range []string{"userName", "externalId", "active", "displayName", "status", "scimType"} {
		if v, ok := body[key]; ok {
			out[key] = v
		}
	}
	if id, ok := body["id"].(string); ok {
		out["id"] = r.fixtureID(id, ours)
	}
	for _, key := range []string{"members", "groups"} {
		values, ok := body[key].([]any)
		if !ok {
			continue
		}
		ids := make([]string, 0, len(values))
		for _, value := range values {
			m, _ := value.(map[string]any)
			id, _ := m["value"].(string)
			ids = append(ids, r.fixtureID(id, ours))
		}
		sort.Strings(ids)
		out[key] = ids
	}
	return out
}

// fixtureID returns the responder's ID for one of this server's IDs.
func (r *replay) fixtureID(id string, ours bool) string {
	if !ours {
		return id
	}
	for fixtureID, ourID := range r.ids {
		if ourID == id {
			return fixtureID
		}
	}
	return id
}

// boundUser returns the Obot user bound to the SCIM user the responder knew by fixtureID.
func (r *replay) boundUser(fixtureID string) uint {
	r.s.t.Helper()

	id, ok := r.ids[fixtureID]
	if !ok {
		r.s.t.Fatalf("no SCIM user was created for %s", fixtureID)
	}
	binding := r.s.bindingByID(id)
	if binding.Retired() {
		r.s.t.Fatalf("SCIM user %s is retired", id)
	}
	return binding.UserID
}

// assertGroup checks the bound group the responder knew by fixtureID, and returns its Obot group ID. An empty
// groupID skips the ID check.
func (r *replay) assertGroup(fixtureID, groupID, displayName string, members ...uint) string {
	r.s.t.Helper()

	binding := r.groupBinding(fixtureID)
	if binding.Retired() {
		r.s.t.Fatalf("group %s is retired", fixtureID)
	}
	if groupID != "" && binding.GroupID != groupID {
		r.s.t.Fatalf("group %s is bound to %s, want %s", fixtureID, binding.GroupID, groupID)
	}

	var group types.Group
	if err := r.s.gorm().Where("id = ?", binding.GroupID).Take(&group).Error; err != nil {
		r.s.t.Fatal(err)
	}
	if group.Name != displayName {
		r.s.t.Fatalf("group %s is named %q, want %q", binding.GroupID, group.Name, displayName)
	}
	if got := r.s.memberships(binding.GroupID); !slices.Equal(got, sortedIDs(members...)) {
		r.s.t.Fatalf("group %s has members %v, want %v", binding.GroupID, got, sortedIDs(members...))
	}
	return binding.GroupID
}

// retiredGroup returns the Obot group ID of a group that was deleted in the target, checking that the group
// itself remains.
func (r *replay) retiredGroup(fixtureID string) string {
	r.s.t.Helper()

	binding := r.groupBinding(fixtureID)
	if !binding.Retired() {
		r.s.t.Fatalf("group %s is not retired", fixtureID)
	}
	if n := r.s.count(new(types.Group), "id = ?", binding.GroupID); n != 1 {
		r.s.t.Fatalf("the group of retired binding %s was removed", fixtureID)
	}
	return binding.GroupID
}

func (r *replay) groupBinding(fixtureID string) types.SCIMGroupBinding {
	r.s.t.Helper()

	id, ok := r.ids[fixtureID]
	if !ok {
		r.s.t.Fatalf("no SCIM group was created for %s", fixtureID)
	}
	var binding types.SCIMGroupBinding
	if err := r.s.gorm().Where("id = ?", id).Take(&binding).Error; err != nil {
		r.s.t.Fatal(err)
	}
	return binding
}

// assertNoEffect checks that the requests with the given sequence numbers recorded no lifecycle events.
func (r *replay) assertNoEffect(sequences ...int) {
	r.s.t.Helper()

	for _, seq := range sequences {
		before := int64(-1)
		for prev := seq - 1; prev > 0; prev-- {
			if n, ok := r.eventsAfter[prev]; ok {
				before = n
				break
			}
		}
		after, ok := r.eventsAfter[seq]
		if !ok || before < 0 {
			r.s.t.Fatalf("sequence %d was not replayed", seq)
		}
		if after != before {
			r.s.t.Errorf("sequence %d recorded %d events, want none", seq, after-before)
		}
	}
}

// assertNoDataLoss checks the data preservation invariants: no user is deleted and no identity removed.
func assertNoDataLoss(t *testing.T, s *scimTest) {
	t.Helper()

	if n := s.count(new(types.User), "deleted_at IS NOT NULL"); n != 0 {
		t.Fatalf("%d users were deleted", n)
	}
	var orphaned int64
	if err := s.gorm().Model(new(types.User)).
		Where("NOT EXISTS (SELECT 1 FROM identities WHERE identities.user_id = users.id)").
		Count(&orphaned).Error; err != nil {
		t.Fatal(err)
	}
	if orphaned != 0 {
		t.Fatalf("%d users lost their identities", orphaned)
	}
}

func sortedIDs(ids ...uint) []uint {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	if sorted == nil {
		sorted = []uint{}
	}
	return sorted
}

func jsonEqual(a, b any) bool {
	return mustJSON(a) == mustJSON(b)
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

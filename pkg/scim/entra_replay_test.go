package scim

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// The Entra fixtures were captured from real Entra SCIM payloads, with certain values redacted or modified.

const (
	// entraFixtureUserAgent identifies the requests that Entra sent, which carry no User-Agent of their own.
	entraFixtureUserAgent = "Microsoft Entra (Phase 0)"
)

// entraSeed is the legacy data of the Phase 0 development installation that run 002 started from.
type entraSeed struct {
	Users []struct {
		LegacyID   string `json:"legacyId"`
		ExternalID string `json:"externalId"`
	} `json:"users"`
	Groups []struct {
		LegacyID    string   `json:"legacyId"`
		ExternalID  string   `json:"externalId"`
		DisplayName string   `json:"displayName"`
		Members     []string `json:"members"`
	} `json:"groups"`
}

// entraExchanges selects the requests that Entra sent and the responder answered as itself. Faults that the responder
// injected are skipped.
func entraExchanges(f fixture) bool {
	return f.UserAgent == entraFixtureUserAgent && !f.Fault
}

// entraReplay returns the replay options of the Entra runs, with the answers in which this server intentionally
// differs from the responder: a user DELETE deletes the user, where the responder of run 003 answered 501, and every
// later request for the user is then a 404, which Entra treats as done, where the responder still had the user. Runs
// that continue one responder state share the options, which track the deleted users.
func entraReplay() replayOptions {
	deleted := map[string]bool{}
	return replayOptions{
		include: entraExchanges,
		expect: func(f fixture) (int, map[string]any) {
			if id, ok := strings.CutPrefix(f.Path, "/scim/v2/Users/"); ok {
				if deleted[id] {
					return http.StatusNotFound, map[string]any{
						"status": "404",
					}
				}
				if f.Method == http.MethodDelete {
					deleted[id] = true
					return http.StatusNoContent, nil
				}
			}
			return f.Status, f.ResponseBody
		},
	}
}

// seedEntraInventory creates the legacy users and groups of a seed, as directory synchronization left them. Each user
// signed in with a subject of its own, and its legacy user ID makes its email. The first user is the Owner. It returns
// the Obot user of each object ID.
func seedEntraInventory(s *scimTest, seed entraSeed) map[string]*types.User {
	s.t.Helper()

	users := make(map[string]*types.User, len(seed.Users))
	for i, u := range seed.Users {
		role := types2.RoleBasic
		if i == 0 {
			role = types2.RoleOwner
		}
		users[u.ExternalID] = s.seedEntraUser("sub-legacy-"+u.LegacyID, u.ExternalID, "legacy-"+u.LegacyID+"@example.com", role)
	}
	for _, g := range seed.Groups {
		memberIDs := make([]uint, 0, len(g.Members))
		for _, objectID := range g.Members {
			member, ok := users[objectID]
			if !ok {
				s.t.Fatalf("seed group %s lists %s, which is not a seed user", g.LegacyID, objectID)
			}
			memberIDs = append(memberIDs, member.ID)
		}
		s.seedProviderGroup(testEntraProviderName, g.LegacyID, g.DisplayName, memberIDs...)
	}
	return users
}

// entraFixture returns the exchange of a capture run with the given sequence number.
func entraFixture(t *testing.T, name string, sequence int) fixture {
	t.Helper()

	fixtures := readPhase0[[]fixture](t, name)
	i := slices.IndexFunc(fixtures, func(f fixture) bool { return f.Sequence == sequence })
	if i < 0 {
		t.Fatalf("%s has no exchange %d", name, sequence)
	}
	return fixtures[i]
}

// assertReadBackHolds checks that a user's read-back holds every value that the user PATCHes of a capture run add or
// replace, so that Entra, which compares the read-back with its source, has nothing to send again. It handles the
// operations that run 004 sends: an add or replace of a value at a path, and fails on any other.
func assertReadBackHolds(t *testing.T, r *replay, name, fixtureUserID string) {
	t.Helper()

	id, ok := r.ids[fixtureUserID]
	if !ok {
		t.Fatalf("no SCIM user was created for %s", fixtureUserID)
	}
	user := r.s.do(http.MethodGet, "Users/"+id, nil).expect(t, http.StatusOK).body
	var checked int
	for _, f := range readPhase0[[]fixture](t, name) {
		if f.Method != http.MethodPatch || f.Path != "/scim/v2/Users/"+fixtureUserID {
			continue
		}
		var patch struct {
			Operations []struct {
				Op    string `json:"op"`
				Path  string `json:"path"`
				Value any    `json:"value"`
			} `json:"Operations"`
		}
		if err := json.Unmarshal(f.RequestBody, &patch); err != nil {
			t.Fatal(err)
		}
		for _, op := range patch.Operations {
			if (!strings.EqualFold(op.Op, "add") && !strings.EqualFold(op.Op, "replace")) || op.Path == "" {
				t.Fatalf("%04d: assertReadBackHolds cannot check %s %q", f.Sequence, op.Op, op.Path)
			}
			if got := readBackValue(user, op.Path); !jsonEqual(got, op.Value) {
				t.Errorf("%04d %s = %v in the read-back, want %v", f.Sequence, op.Path, got, op.Value)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatalf("%s has no PATCH of user %s", name, fixtureUserID)
	}
}

// readBackValue returns the value of a PATCH path in a user resource, for the paths that run 004 sends: an enterprise
// extension attribute, a sub-attribute of the work address, or a top-level attribute.
func readBackValue(user map[string]any, path string) any {
	if attribute, ok := strings.CutPrefix(path, enterpriseUserSchema+":"); ok {
		extension, _ := user[enterpriseUserSchema].(map[string]any)
		return extension[attribute]
	}
	if attribute, ok := strings.CutPrefix(path, `addresses[type eq "work"].`); ok {
		addresses, _ := user["addresses"].([]any)
		for _, a := range addresses {
			if address, _ := a.(map[string]any); address["type"] == "work" {
				return address[attribute]
			}
		}
		return nil
	}
	return user[path]
}

// TestReplayEntraPhase0Run001DefaultMapping checks that run 001's first user create, under the default user mapping,
// which sends mailNickname as externalId, is refused with a detail that names the mapping to change.
func TestReplayEntraPhase0Run001DefaultMapping(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)

	for _, f := range readPhase0[[]fixture](t, "entra-run-001.json") {
		if f.Method != http.MethodPost || f.Path != "/scim/v2/Users" {
			continue
		}
		resp := s.do(f.Method, "Users", string(f.RequestBody)).expect(t, http.StatusBadRequest)
		if detail, _ := resp.body["detail"].(string); !strings.Contains(detail, "map objectId to externalId (the default maps mailNickname)") {
			t.Fatalf("detail = %q, want the mapping to change", detail)
		}
		if resp.body["scimType"] != scimTypeInvalidValue {
			t.Fatalf("scimType = %v, want %s", resp.body["scimType"], scimTypeInvalidValue)
		}
		if n := s.count(new(types.User), ""); n != 0 {
			t.Fatalf("the refused create made %d users", n)
		}
		return
	}
	t.Fatal("run 001 has no user create")
}

func TestReplayEntraPhase0Run001(t *testing.T) {
	testReplayEntraPhase0Run001(t, newSCIMTest(t))
}

func TestReplayEntraPhase0Run001Postgres(t *testing.T) {
	testReplayEntraPhase0Run001(t, newPostgresSCIMTest(t))
}

// testReplayEntraPhase0Run001 replays run 001 from the copy whose externalIds were rewritten to object IDs, as if the
// user mapping had mapped objectId: provisioning on demand, renames, a group unassigned and assigned again, blocks,
// soft deletes and restores, replays of on-demand changes, and a permanent delete.
func testReplayEntraPhase0Run001(t *testing.T, s *scimTest) {
	t.Helper()

	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)
	r := replayFixtures(t, s, "entra-run-001-objectid.json", entraReplay())

	// U1 and U3 were provisioned, and never signed in.
	u1 := r.boundUser("6cd5de02-fa58-424a-a3b2-6b4cce17f772")
	u3 := r.boundUser("a1df8984-d822-4ed9-860b-facd04f1a360")
	for _, id := range []uint{u1, u3} {
		if u := s.user(id); u.DisabledAt != nil || u.DeletedAt != nil {
			t.Fatalf("user %d = %+v, want active", id, u)
		}
	}

	// U4 was soft-deleted twice, restored once, then permanently deleted.
	u4Binding := s.bindingByID(r.ids["920f02ed-afc4-4ff6-9f63-65818e42a4b1"])
	if !u4Binding.Retired() {
		t.Fatal("the permanently deleted user is still bound")
	}
	if u := s.user(u4Binding.UserID); u.DeletedAt == nil {
		t.Fatalf("the permanently deleted user = %+v, want deleted", u)
	}

	// Each group's ID is its object ID's. G1 was renamed and renamed back. G2 was unassigned before enforcing, which
	// retired its binding, then assigned again, which bound the same group.
	r.assertGroup("1efc49a9-3adc-4998-b53b-704fef5e0205", testEntraGroupPrefix+"00000000-0000-4000-8000-000000000018", "Grant SCIM Test 1", u1)
	g2 := testEntraGroupPrefix + "00000000-0000-4000-8000-000000000026"
	if first := r.retiredGroup("48841468-44a9-4b4c-bb03-917c2fbc6dd3"); first != g2 {
		t.Fatalf("the unassigned G2 is %s, want %s", first, g2)
	}
	r.assertGroup("cf4dbdfb-2ef7-40db-b0a6-41b7eb8f1126", g2, "Grant SCIM Test 2", u3)

	// The redundant adds of initial cycles, and the replays of on-demand membership changes, change nothing.
	r.assertNoEffect(54, 55, 104, 105, 107)
}

func TestReplayEntraPhase0Runs002To004(t *testing.T) {
	testReplayEntraPhase0Runs002To004(t, newSCIMTest(t))
}

func TestReplayEntraPhase0Runs002To004Postgres(t *testing.T) {
	testReplayEntraPhase0Runs002To004(t, newPostgresSCIMTest(t))
}

// testReplayEntraPhase0Runs002To004 replays the runs against legacy data from directory synchronization: run 002 binds
// it, run 003 restarts after quarantine and deletes a new user, and run 004 sends the addresses and enterprise
// attributes that the read-backs lacked.
func testReplayEntraPhase0Runs002To004(t *testing.T, s *scimTest) {
	t.Helper()

	seed := readPhase0[entraSeed](t, "entra-run-002-seed.json")
	inventory := seedEntraInventory(s, seed)
	u1Legacy, u2Legacy, u3Legacy := inventory[seed.Users[0].ExternalID], inventory[seed.Users[1].ExternalID], inventory[seed.Users[2].ExternalID]
	g1, g2, allUsers := seed.Groups[0].LegacyID, seed.Groups[1].LegacyID, seed.Groups[2].LegacyID
	users := s.count(new(types.User), "")
	s.enableEntra(types.SCIMConnectionOriginMigrated)

	// The runs share their options, which track the users that were deleted.
	opts := entraReplay()
	r := replayFixtures(t, s, "entra-run-002.json", opts)

	// U1 and U3 bound their legacy users by object ID. U2 was never assigned.
	u1 := r.boundUser("8abbeddb-2e73-4947-ad53-c59486d18750")
	u3 := r.boundUser("39564c54-d69d-4267-ace0-2e34c224df25")
	if u1 != u1Legacy.ID || u3 != u3Legacy.ID {
		t.Fatalf("U1 and U3 bound users %d and %d, want %d and %d", u1, u3, u1Legacy.ID, u3Legacy.ID)
	}
	if binding, _ := s.gateway.SCIMUserBindingForUser(t.Context(), u2Legacy.ID); binding != nil {
		t.Fatal("the unassigned user was bound")
	}
	if got := s.count(new(types.User), ""); got != users {
		t.Fatalf("got %d users, want %d", got, users)
	}
	assertNoDataLoss(t, s)

	// The groups bound their legacy groups by externalId, keeping their IDs. Each POST's empty member list dropped
	// the legacy members, and Entra's Add restored them. All Users was never assigned, and is untouched.
	r.assertGroup("023eefe1-b6c9-440d-b6ad-5bc88732a04c", g1, "Grant SCIM Test 1", u1)
	r.assertGroup("0cef2426-094a-43e5-a265-84620c69d0bc", g2, "Grant SCIM Test 2", u3)
	if got := s.memberships(allUsers); !slices.Equal(got, sortedIDs(u1Legacy.ID, u2Legacy.ID, u3Legacy.ID)) {
		t.Fatalf("All Users members = %v, want its legacy members", got)
	}

	// The initial cycle's repeated adds change nothing.
	r.assertNoEffect(28, 29)

	// Run 003: the restart's initial cycle after quarantine, then U5, created and permanently deleted.
	r.run(t, "entra-run-003.json", opts)
	r.assertNoEffect(20, 21)
	u5Binding := s.bindingByID(r.ids["b07137d4-4499-4df0-a344-b9844b6fadc3"])
	if !u5Binding.Retired() {
		t.Fatal("the deleted user is still bound")
	}
	if u := s.user(u5Binding.UserID); u.DeletedAt == nil {
		t.Fatalf("the deleted user = %+v, want deleted", u)
	}
	s.do(http.MethodGet, "Users/"+u5Binding.ID, nil).expect(t, http.StatusNotFound)

	// Run 004: the PATCHes of the attributes that the read-backs lacked.
	r.run(t, "entra-run-004.json", opts)

	// The read-backs return everything that Entra sent, so Entra has nothing to send again.
	assertReadBackHolds(t, r, "entra-run-004.json", "39564c54-d69d-4267-ace0-2e34c224df25")
	assertReadBackHolds(t, r, "entra-run-004.json", "8abbeddb-2e73-4947-ad53-c59486d18750")

	// Sending the Restart's PATCH of U3 once more, as Entra would if the read-back lacked them, changes nothing.
	u3SCIM := r.ids["39564c54-d69d-4267-ace0-2e34c224df25"]
	restart := entraFixture(t, "entra-run-004.json", 20)
	if restart.Path != "/scim/v2/Users/39564c54-d69d-4267-ace0-2e34c224df25" {
		t.Fatalf("exchange 20 of run 004 is %s %s, want the Restart's PATCH of U3", restart.Method, restart.Path)
	}
	before := s.bindingByID(u3SCIM).Revision
	s.do(restart.Method, "Users/"+u3SCIM, r.remap(string(restart.RequestBody))).expect(t, http.StatusOK)
	if after := s.bindingByID(u3SCIM).Revision; after != before {
		t.Fatalf("a repeated PATCH changed U3's revision from %d to %d", before, after)
	}
}

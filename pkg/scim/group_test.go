package scim

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestGroupBinding(t *testing.T) {
	s := newSCIMTest(t)
	alice := s.seedUser("00u-alice", "alice@example.com", types2.RoleBasic)
	bob := s.seedUser("00u-bob", "bob@example.com", types2.RoleBasic)
	carol := s.seedUser("00u-carol", "carol@example.com", types2.RoleBasic)
	s.seedGroup("okta/00g-eng", "Engineering", alice.ID, carol.ID)
	s.seedGroup("okta/00g-other", "Other", alice.ID, bob.ID)
	s.seedGroup("okta/00g-dup-1", "Duplicate", alice.ID)
	s.seedGroup("okta/00g-dup-2", "duplicate ", bob.ID)
	s.enable()

	aliceID := s.do(http.MethodPost, "Users", scimUser("alice@example.com", "00u-alice")).expect(t, http.StatusCreated).id()
	bobID := s.do(http.MethodPost, "Users", scimUser("bob@example.com", "00u-bob")).expect(t, http.StatusCreated).id()
	eventsBefore := map[uint]int{
		alice.ID: len(s.events(alice.ID)),
		bob.ID:   len(s.events(bob.ID)),
		carol.ID: len(s.events(carol.ID)),
	}

	// Unbound groups are invisible, including to the displayName lookup Okta sends before pushing.
	if got := s.do(http.MethodGet, "Groups?filter="+url.QueryEscape(`displayName eq "Engineering"`), nil).expect(t, http.StatusOK).resources(); len(got) != 0 {
		t.Fatalf("unbound group listed: %v", got)
	}
	if total := s.do(http.MethodGet, "Groups?startIndex=1&count=100", nil).expect(t, http.StatusOK).body["totalResults"]; total != float64(0) {
		t.Fatalf("totalResults = %v, want 0", total)
	}

	// A push binds to the unbound group with the same trimmed, case-folded name, keeping its ID. Its members
	// replace the cached members: the stale member leaves through the normal path, and the remaining member
	// is not touched.
	eng := s.do(http.MethodPost, "Groups", scimGroup("  ENGINEERING ", aliceID, bobID)).expect(t, http.StatusCreated)
	if eng.body["displayName"] != "  ENGINEERING " {
		t.Errorf("displayName = %v", eng.body["displayName"])
	}
	if members := s.memberships("okta/00g-eng"); !slices.Equal(members, []uint{alice.ID, bob.ID}) {
		t.Fatalf("memberships = %v", members)
	}
	if n := s.count(new(types.Group), "id = ?", "okta/00g-eng"); n != 1 {
		t.Fatalf("the bound group's ID changed")
	}
	if got := s.events(alice.ID); len(got) != eventsBefore[alice.ID] {
		t.Errorf("the remaining member got events: %+v", got[eventsBefore[alice.ID]:])
	}
	if got := s.events(bob.ID); len(got) != eventsBefore[bob.ID]+1 || got[len(got)-1].GroupsRemoved {
		t.Errorf("the added member's events = %+v", got)
	}
	if got := s.events(carol.ID); len(got) != eventsBefore[carol.ID]+1 || !got[len(got)-1].GroupsRemoved {
		t.Errorf("the stale member's events = %+v", got)
	}

	// The unbound group is untouched: its memberships stay effective until it is bound.
	if members := s.memberships("okta/00g-other"); !slices.Equal(members, []uint{alice.ID, bob.ID}) {
		t.Fatalf("unbound memberships = %v", members)
	}

	found := s.do(http.MethodGet, "Groups?filter="+url.QueryEscape(`displayName eq "engineering"`), nil).expect(t, http.StatusOK).resources()
	if len(found) != 1 || found[0]["id"] != eng.id() {
		t.Fatalf("displayName lookup = %v", found)
	}

	// A push that duplicates a bound name is an Okta-side duplicate.
	if resp := s.do(http.MethodPost, "Groups", scimGroup("engineering")).expect(t, http.StatusConflict); resp.body["scimType"] != scimTypeUniqueness {
		t.Errorf("scimType = %v", resp.body["scimType"])
	}
	// More than one unbound candidate is ambiguous.
	s.do(http.MethodPost, "Groups", scimGroup("DUPLICATE")).expect(t, http.StatusConflict)

	// Without a candidate, a group is created with the provider's prefix.
	created := s.do(http.MethodPost, "Groups", scimGroup("New Group", aliceID)).expect(t, http.StatusCreated)
	if n := s.count(new(types.Group), "id = ?", "okta/"+created.id()); n != 1 {
		t.Fatalf("created group has no okta/<scim-id> row")
	}

	// Renames address the SCIM ID and keep the Obot group ID. A bound name conflicts; an unbound one does not.
	s.do(http.MethodPatch, "Groups/"+eng.id(), patchOp(map[string]any{
		"op": "replace",
		"value": map[string]any{
			"displayName": "new group",
			"id":          eng.id(),
		},
	})).expect(t, http.StatusConflict)
	s.do(http.MethodPatch, "Groups/"+eng.id(), patchOp(map[string]any{
		"op": "replace",
		"value": map[string]any{
			"displayName": "Other",
			"id":          eng.id(),
		},
	})).expect(t, http.StatusNoContent)
	if renamed := s.do(http.MethodGet, "Groups/"+eng.id(), nil).expect(t, http.StatusOK); renamed.body["displayName"] != "Other" || !slices.Equal(renamed.memberIDs(), []string{aliceID, bobID}) {
		t.Fatalf("renamed group = %v", renamed.body)
	}

	// An unknown member fails the whole request, atomically.
	resp := s.do(http.MethodPut, "Groups/"+eng.id(), scimGroup("Renamed Again", aliceID, "00000000-0000-0000-0000-000000000000")).expect(t, http.StatusBadRequest)
	if resp.body["scimType"] != scimTypeInvalidValue {
		t.Errorf("scimType = %v", resp.body["scimType"])
	}
	if got := s.do(http.MethodGet, "Groups/"+eng.id(), nil).expect(t, http.StatusOK); got.body["displayName"] != "Other" || !slices.Equal(got.memberIDs(), []string{aliceID, bobID}) {
		t.Fatalf("a failed PUT was partly applied: %v", got.body)
	}
	// A group is not a member.
	s.do(http.MethodPatch, "Groups/"+eng.id(), patchOp(map[string]any{
		"op":   "add",
		"path": "members",
		"value": []any{
			map[string]any{
				"value": created.id(),
			},
		},
	})).expect(t, http.StatusBadRequest)

	// Member values nested in an operation without a path count toward the limit.
	members := make([]any, 0, maxMemberValues+1)
	for i := range maxMemberValues + 1 {
		members = append(members, map[string]any{
			"value": fmt.Sprint(i),
		})
	}
	s.do(http.MethodPatch, "Groups/"+eng.id(), patchOp(map[string]any{
		"op": "add",
		"value": map[string]any{
			"members": members,
		},
	})).expect(t, http.StatusRequestEntityTooLarge)

	// Identical repeated PUTs change nothing and emit nothing.
	put := s.do(http.MethodGet, "Groups/"+eng.id(), nil).expect(t, http.StatusOK).body
	var binding types.SCIMGroupBinding
	if err := s.gorm().Where("id = ?", eng.id()).Take(&binding).Error; err != nil {
		t.Fatal(err)
	}
	eventCount := s.count(new(types.UserLifecycleEvent), "")
	for range 3 {
		s.do(http.MethodPut, "Groups/"+eng.id(), put).expect(t, http.StatusOK)
	}
	var after types.SCIMGroupBinding
	if err := s.gorm().Where("id = ?", eng.id()).Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Revision != binding.Revision || s.count(new(types.UserLifecycleEvent), "") != eventCount {
		t.Fatalf("identical PUTs had an effect: revision %d -> %d", binding.Revision, after.Revision)
	}

	// Members can be left out of the response.
	if got := s.do(http.MethodGet, "Groups/"+eng.id()+"?excludedAttributes=members", nil).expect(t, http.StatusOK); got.body["members"] != nil {
		t.Fatalf("members were returned: %v", got.body["members"])
	}

	// Delete returns the group to unbound: the row and its ID remain, and its memberships go.
	s.do(http.MethodDelete, "Groups/"+eng.id(), nil).expect(t, http.StatusNoContent)
	s.do(http.MethodGet, "Groups/"+eng.id(), nil).expect(t, http.StatusNotFound)
	s.do(http.MethodDelete, "Groups/"+eng.id(), nil).expect(t, http.StatusNotFound)
	if n := s.count(new(types.Group), "id = ?", "okta/00g-eng"); n != 1 {
		t.Fatal("the deleted group's row was removed")
	}
	if members := s.memberships("okta/00g-eng"); len(members) != 0 {
		t.Fatalf("the deleted group kept memberships %v", members)
	}
	// The deleted group was renamed to "Other", which an unbound group already has, so a push of that name is
	// now ambiguous.
	s.do(http.MethodPost, "Groups", scimGroup("Other", aliceID)).expect(t, http.StatusConflict)
}

func TestGroupRebindAfterDelete(t *testing.T) {
	s := newSCIMTest(t)
	alice := s.seedUser("00u-alice", "alice@example.com", types2.RoleBasic)
	s.seedGroup("okta/00g-team", "team", alice.ID)
	s.enable()

	aliceID := s.do(http.MethodPost, "Users", scimUser("alice@example.com", "00u-alice")).expect(t, http.StatusCreated).id()
	team := s.do(http.MethodPost, "Groups", scimGroup("team", aliceID)).expect(t, http.StatusCreated)
	s.do(http.MethodDelete, "Groups/"+team.id(), nil).expect(t, http.StatusNoContent)

	repushed := s.do(http.MethodPost, "Groups", scimGroup("Team", aliceID)).expect(t, http.StatusCreated)
	if repushed.id() == team.id() {
		t.Fatal("a retired SCIM ID was reused")
	}
	var binding types.SCIMGroupBinding
	if err := s.gorm().Where("id = ?", repushed.id()).Take(&binding).Error; err != nil {
		t.Fatal(err)
	}
	if binding.GroupID != "okta/00g-team" {
		t.Fatalf("re-push bound %q, want okta/00g-team", binding.GroupID)
	}
	if members := s.memberships("okta/00g-team"); !slices.Equal(members, []uint{alice.ID}) {
		t.Fatalf("memberships = %v", members)
	}
}

func TestDeleteSCIMManagedUser(t *testing.T) {
	s := newSCIMTest(t)
	alice := s.seedUser("00u-alice", "alice@example.com", types2.RoleBasic)
	s.seedUser("00u-owner", "owner@example.com", types2.RoleOwner)
	s.enable()

	aliceID := s.do(http.MethodPost, "Users", scimUser("alice@example.com", "00u-alice")).expect(t, http.StatusCreated).id()
	team := s.do(http.MethodPost, "Groups", scimGroup("team", aliceID)).expect(t, http.StatusCreated)

	// An active user cannot be deleted, nor can their identities be removed.
	err := s.gateway.DeleteUser(t.Context(), fmtID(alice.ID))
	if _, ok := errors.AsType[*gclient.SCIMManagedUserError](err); !ok {
		t.Fatalf("DeleteUser() error = %v, want SCIMManagedUserError", err)
	}
	if err := s.gateway.RemoveIdentity(t.Context(), &types.Identity{
		UserID: alice.ID,
	}); err == nil {
		t.Fatal("the identity of a SCIM-managed user was removed")
	}
	if u := s.user(alice.ID); u.DeletedAt != nil {
		t.Fatal("a refused deletion deleted the user")
	}

	// Once Okta deprovisions the user, deletion runs the normal path and retires the SCIM ID.
	s.do(http.MethodPatch, "Users/"+aliceID, patchOp(map[string]any{
		"op":    "replace",
		"path":  "active",
		"value": false,
	})).expect(t, http.StatusOK)
	if err := s.gateway.DeleteUser(t.Context(), fmtID(alice.ID)); err != nil {
		t.Fatalf("DeleteUser() error = %v", err)
	}
	s.do(http.MethodGet, "Users/"+aliceID, nil).expect(t, http.StatusNotFound)
	if got := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "alice@example.com"`), nil).expect(t, http.StatusOK).resources(); len(got) != 0 {
		t.Fatalf("a deleted user is still found: %v", got)
	}
	if err := s.gateway.RemoveIdentity(t.Context(), &types.Identity{
		UserID: alice.ID,
	}); err != nil {
		t.Fatalf("RemoveIdentity() error = %v", err)
	}
	if n := s.count(new(types.Identity), "user_id = ?", alice.ID); n != 0 {
		t.Fatalf("the deleted user still has %d identities", n)
	}

	// Okta may still list the deleted user in a group, and that value is ignored.
	s.do(http.MethodPut, "Groups/"+team.id(), scimGroup("team", aliceID)).expect(t, http.StatusOK)

	// Provisioning the same person again creates a new, empty account.
	again := s.do(http.MethodPost, "Users", scimUser("alice@example.com", "00u-alice")).expect(t, http.StatusCreated)
	if again.id() == aliceID {
		t.Fatal("a retired SCIM ID was reused")
	}
	binding := s.bindingByID(again.id())
	if binding.UserID == alice.ID {
		t.Fatal("the deleted user was restored")
	}
	if u := s.user(binding.UserID); u.DeletedAt != nil || u.Username != "00u-alice" {
		t.Fatalf("new account = %+v", u)
	}
}

func TestDeleteUserAfterIdentityCleanup(t *testing.T) {
	s := newSCIMTest(t)
	bob := s.seedUser("00u-bob", "bob@example.com", types2.RoleBasic)
	s.enable()

	// A deleted user whose identity still exists gets a new account relinked to that identity.
	bobID := s.do(http.MethodPost, "Users", scimUser("bob@example.com", "00u-bob")).expect(t, http.StatusCreated).id()
	s.do(http.MethodPut, "Users/"+bobID, withActive(scimUser("bob@example.com", "00u-bob"), false)).expect(t, http.StatusOK)
	if err := s.gateway.DeleteUser(t.Context(), fmtID(bob.ID)); err != nil {
		t.Fatal(err)
	}

	again := s.do(http.MethodPost, "Users", scimUser("bob@example.com", "00u-bob")).expect(t, http.StatusCreated)
	binding := s.bindingByID(again.id())
	if binding.UserID == bob.ID {
		t.Fatal("the deleted user was restored")
	}
	var identity types.Identity
	if err := s.gorm().Where("auth_provider_name = ? AND provider_user_id = ?", testOktaProviderName, "00u-bob").Take(&identity).Error; err != nil {
		t.Fatal(err)
	}
	if identity.UserID != binding.UserID {
		t.Fatalf("identity user = %d, want %d", identity.UserID, binding.UserID)
	}
}

func withActive(body map[string]any, active bool) map[string]any {
	body["active"] = active
	return body
}

func fmtID(id uint) string {
	return fmt.Sprint(id)
}

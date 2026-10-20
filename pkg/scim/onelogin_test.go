package scim

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// The tests in this file are based on real SCIM payloads captured from OneLogin's "SCIM Provisioner with SAML (SCIM v2
// Core)" connector, with the app set up as Obot requires: "externalId": "{$user.id}" in the SCIM JSON Template, the
// SCIM Username parameter on OneLogin ID, and both deprovisioning actions on Suspend.

const (
	testOneLoginProviderName = "onelogin-auth-provider"
	testOneLoginGroupPrefix  = "onelogin/"

	// Numeric OneLogin user IDs.
	oneLoginOwnerID         = "263801284"
	oneLoginBasicID         = "263801290"
	oneLoginUnprovisionedID = "263801300"
	oneLoginNewID           = "263801306"

	// oneLoginReusedID is the ID of a new person who re-uses a deleted person's username.
	oneLoginReusedID = "263883397"
	// oneLoginOtherID is the ID of a OneLogin user who has no account in Obot.
	oneLoginOtherID = "263882404"

	// oneLoginTestRoleID is the role that is pushed as obot-scim-test.
	oneLoginTestRoleID = "1113742"
)

// enableOneLogin creates a OneLogin SCIM connection.
func (s *scimTest) enableOneLogin() {
	s.t.Helper()
	s.enableProvider(testOneLoginProviderName, testOneLoginGroupPrefix, "https://acme.onelogin.com/oidc/2")
}

// oneLoginUser returns a user body as the Core connector's template renders it: the template's attributes, with
// userName and externalId both the numeric user ID, and active as a JSON boolean. It has no displayName, no groups,
// and no id or meta, even on a PUT.
func oneLoginUser(id, familyName string, active bool) map[string]any {
	return map[string]any{
		"schemas":    []string{userSchema},
		"userName":   id,
		"externalId": id,
		"active":     active,
		"name": map[string]any{
			"givenName":  "Given",
			"familyName": familyName,
		},
		"emails": []any{
			map[string]any{
				"primary": true,
				"type":    "work",
				"value":   "user-" + id + "@example.com",
			},
		},
	}
}

// oneLoginGroup returns a group create body, which OneLogin sends with no members key and then adds each member with
// its own PATCH.
func oneLoginGroup(displayName string) map[string]any {
	return map[string]any{
		"schemas":     []string{groupSchema},
		"displayName": displayName,
	}
}

// oneLoginMembers returns the PATCH that adds or removes one member, which OneLogin sends as a value list on members,
// never as a filter. It asks for no attributes, so Obot answers 204 with no body.
func oneLoginMembers(op, scimUserID string) map[string]any {
	return patchOp(map[string]any{
		"op":   op,
		"path": "members",
		"value": []any{
			map[string]any{
				"value": scimUserID,
			},
		},
	})
}

// oneLoginCreate sends a user's create task as OneLogin does: a userName eq lookup by the user's ID, which misses,
// then the create. It returns the user's SCIM ID.
func (s *scimTest) oneLoginCreate(id string) string {
	s.t.Helper()

	if found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "`+id+`"`), nil).expect(s.t, http.StatusOK).resources(); len(found) != 0 {
		s.t.Fatalf("the lookup of %s before its create found %v", id, found)
	}
	return s.do(http.MethodPost, "Users", oneLoginUser(id, "Family", true)).expect(s.t, http.StatusCreated).id()
}

func TestOneLoginProvisioning(t *testing.T) {
	testOneLoginProvisioning(t, newSCIMTest(t))
}

func TestOneLoginProvisioningPostgres(t *testing.T) {
	testOneLoginProvisioning(t, newPostgresSCIMTest(t))
}

func testOneLoginProvisioning(t *testing.T, s *scimTest) {
	t.Helper()

	// The installation used the OneLogin provider before SCIM, so its users and its role's group exist. The group's
	// cached members are stale: the Basic user has since left the role, and the new user has joined it.
	owner := s.seedProviderUser(testOneLoginProviderName, oneLoginOwnerID, "owner@example.com", types2.RoleOwner)
	basic := s.seedProviderUser(testOneLoginProviderName, oneLoginBasicID, "basic@example.com", types2.RoleBasic)
	unprovisioned := s.seedProviderUser(testOneLoginProviderName, oneLoginUnprovisionedID, "unprovisioned@example.com", types2.RoleBasic)
	legacyGroupID := testOneLoginGroupPrefix + oneLoginTestRoleID
	s.seedProviderGroup(testOneLoginProviderName, legacyGroupID, "obot-scim-test", owner.ID, basic.ID)
	s.enableOneLogin()

	// The connection test and Entitlements Refresh list the groups, and saving the Provisioning tab lists the users.
	// Neither sees the legacy data.
	if total := s.do(http.MethodGet, "Groups?count=100&startIndex=1", nil).expect(t, http.StatusOK).body["totalResults"]; total != float64(0) {
		t.Fatalf("the connection test's group list has %v groups, want 0", total)
	}
	if total := s.do(http.MethodGet, "Users?count=100&startIndex=1", nil).expect(t, http.StatusOK).body["totalResults"]; total != float64(0) {
		t.Fatalf("the user list has %v users, want 0", total)
	}

	// Granting access creates a user for each holder of the assigned role. The existing users are claimed by their
	// OneLogin user ID; the new user is created.
	basicSCIM := s.oneLoginCreate(oneLoginBasicID)
	newSCIM := s.oneLoginCreate(oneLoginNewID)
	ownerSCIM := s.oneLoginCreate(oneLoginOwnerID)
	if got := s.bindingByID(ownerSCIM).UserID; got != owner.ID {
		t.Fatalf("the Owner's create bound user %d, want %d", got, owner.ID)
	}
	if got := s.bindingByID(basicSCIM).UserID; got != basic.ID {
		t.Fatalf("the Basic user's create bound user %d, want %d", got, basic.ID)
	}
	newUser := s.bindingByID(newSCIM).UserID
	if newUser == owner.ID || newUser == basic.ID || newUser == unprovisioned.ID {
		t.Fatalf("the new user's create bound existing user %d", newUser)
	}
	if binding, _ := s.gateway.SCIMUserBindingForUser(t.Context(), unprovisioned.ID); binding != nil {
		t.Fatal("the user without access was bound")
	}

	// The tasks of the role's two holders each list the groups, find none, and push the role. The first push binds
	// the legacy group by name, keeping its ID, and drops both stale members.
	s.do(http.MethodGet, "Groups?count=100&startIndex=1", nil).expect(t, http.StatusOK)
	group := s.do(http.MethodPost, "Groups", oneLoginGroup("obot-scim-test")).expect(t, http.StatusCreated)
	if members := group.memberIDs(); len(members) != 0 {
		t.Fatalf("the bound group's members = %v, want none", members)
	}
	if members := s.memberships(legacyGroupID); len(members) != 0 {
		t.Fatalf("the bound group kept its stale members %v", members)
	}
	if n := s.count(new(types.Group), ""); n != 1 {
		t.Fatalf("got %d groups, want only the bound legacy group", n)
	}

	// The second push loses with 409, and its task finds the group by displayName and adds its member.
	if resp := s.do(http.MethodPost, "Groups", oneLoginGroup("obot-scim-test")).expect(t, http.StatusConflict); resp.body["scimType"] != scimTypeUniqueness {
		t.Fatalf("scimType = %v, want %s", resp.body["scimType"], scimTypeUniqueness)
	}
	found := s.do(http.MethodGet, "Groups?filter="+url.QueryEscape(`displayName eq "obot-scim-test"`), nil).expect(t, http.StatusOK).resources()
	if len(found) != 1 || found[0]["id"] != group.id() {
		t.Fatalf("the displayName lookup found %v, want %s", found, group.id())
	}
	s.do(http.MethodPatch, "Groups/"+group.id(), oneLoginMembers("add", newSCIM)).expect(t, http.StatusNoContent)
	s.do(http.MethodPatch, "Groups/"+group.id(), oneLoginMembers("add", ownerSCIM)).expect(t, http.StatusNoContent)
	if got, want := s.memberships(legacyGroupID), sortedIDs(owner.ID, newUser); !slices.Equal(got, want) {
		t.Fatalf("memberships = %v, want the role's holders %v", got, want)
	}

	// A profile change is a full PUT, then a GET.
	s.do(http.MethodPut, "Users/"+basicSCIM, oneLoginUser(oneLoginBasicID, "Changed", true)).expect(t, http.StatusOK)
	if name, _ := s.do(http.MethodGet, "Users/"+basicSCIM, nil).expect(t, http.StatusOK).body["name"].(map[string]any); name["familyName"] != "Changed" {
		t.Fatalf("name = %v, want the changed family name", name)
	}

	// Giving a user the role is a PUT, a PATCH that adds them, and a GET; removing it is the same with a value-list
	// remove.
	s.do(http.MethodPut, "Users/"+basicSCIM, oneLoginUser(oneLoginBasicID, "Changed", true)).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Groups/"+group.id(), oneLoginMembers("add", basicSCIM)).expect(t, http.StatusNoContent)
	if groups, _ := s.do(http.MethodGet, "Users/"+basicSCIM, nil).expect(t, http.StatusOK).body["groups"].([]any); len(groups) != 1 {
		t.Fatalf("the added member's groups = %v, want the role's group", groups)
	}
	s.do(http.MethodPut, "Users/"+basicSCIM, oneLoginUser(oneLoginBasicID, "Changed", true)).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Groups/"+group.id(), oneLoginMembers("remove", basicSCIM)).expect(t, http.StatusNoContent)
	if groups, _ := s.do(http.MethodGet, "Users/"+basicSCIM, nil).expect(t, http.StatusOK).body["groups"].([]any); len(groups) != 0 {
		t.Fatalf("the removed member's groups = %v, want none", groups)
	}
	if got, want := s.memberships(legacyGroupID), sortedIDs(owner.ID, newUser); !slices.Equal(got, want) {
		t.Fatalf("memberships = %v, want %v", got, want)
	}

	// Suspending a user is one PUT with active false, which keeps their memberships.
	s.do(http.MethodPut, "Users/"+ownerSCIM, oneLoginUser(oneLoginOwnerID, "Family", false)).expect(t, http.StatusOK)
	if u := s.user(owner.ID); u.DisabledAt == nil {
		t.Fatal("the suspended user is not disabled")
	}
	if got := s.memberships(legacyGroupID); !slices.Contains(got, owner.ID) {
		t.Fatalf("the suspended user left the group: %v", got)
	}

	// Unsuspending can send identical PUTs at once (two were seen), which all succeed.
	for _, status := range s.concurrently(func(int) (string, string, any) {
		return http.MethodPut, "Users/" + ownerSCIM, oneLoginUser(oneLoginOwnerID, "Family", true)
	}) {
		if status != http.StatusOK {
			t.Fatalf("a concurrent unsuspend PUT got %d, want %d", status, http.StatusOK)
		}
	}
	if u := s.user(owner.ID); u.DisabledAt != nil {
		t.Fatal("the unsuspended user is still disabled")
	}
	if active := s.do(http.MethodGet, "Users/"+ownerSCIM, nil).expect(t, http.StatusOK).body["active"]; active != true {
		t.Fatalf("active = %v, want true", active)
	}
	if got, want := s.memberships(legacyGroupID), sortedIDs(owner.ID, newUser); !slices.Equal(got, want) {
		t.Fatalf("memberships after unsuspending = %v, want %v", got, want)
	}
}

// TestOneLoginRemoveAndRestoreAccess replays a user losing app access and getting it back, with the removal action on
// Suspend. OneLogin finds the deactivated user by its lookup on the user's ID, and reactivates it.
func TestOneLoginRemoveAndRestoreAccess(t *testing.T) {
	s := newSCIMTest(t)
	s.enableOneLogin()

	scimID := s.oneLoginCreate(oneLoginNewID)
	userID := s.bindingByID(scimID).UserID
	group := s.do(http.MethodPost, "Groups", oneLoginGroup("obot-scim-test")).expect(t, http.StatusCreated)
	s.do(http.MethodPatch, "Groups/"+group.id(), oneLoginMembers("add", scimID)).expect(t, http.StatusNoContent)
	groupID := testOneLoginGroupPrefix + group.id()

	// Removing access is one PUT with active false, with no group traffic.
	s.do(http.MethodPut, "Users/"+scimID, oneLoginUser(oneLoginNewID, "Family", false)).expect(t, http.StatusOK)
	if u := s.user(userID); u.DisabledAt == nil {
		t.Fatal("the user whose access was removed is not disabled")
	}

	// Restoring access looks the user up by ID, finds the deactivated user, and reactivates it with a PUT.
	found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "`+oneLoginNewID+`"`), nil).expect(t, http.StatusOK).resources()
	if len(found) != 1 || found[0]["id"] != scimID || found[0]["active"] != false {
		t.Fatalf("the lookup found %v, want the deactivated user %s", found, scimID)
	}
	s.do(http.MethodPut, "Users/"+scimID, oneLoginUser(oneLoginNewID, "Family", true)).expect(t, http.StatusOK)
	restored := s.do(http.MethodGet, "Users/"+scimID, nil).expect(t, http.StatusOK)
	if restored.body["active"] != true {
		t.Fatalf("active = %v, want true", restored.body["active"])
	}
	if groups, _ := restored.body["groups"].([]any); len(groups) != 1 {
		t.Fatalf("the restored user's groups = %v, want the group they kept", groups)
	}
	if u := s.user(userID); u.DisabledAt != nil {
		t.Fatal("the restored user is still disabled")
	}
	if got := s.bindingByID(scimID).UserID; got != userID {
		t.Fatalf("the restored SCIM user is bound to %d, want the same user %d", got, userID)
	}
	if got := s.memberships(groupID); !slices.Equal(got, []uint{userID}) {
		t.Fatalf("memberships = %v, want the restored user", got)
	}

	// Deleting the user in OneLogin, with the deletion action on Suspend, deactivates it the same way. A new person who
	// re-uses the username has a new ID, so their lookup misses, and they get their own user.
	s.do(http.MethodPut, "Users/"+scimID, oneLoginUser(oneLoginNewID, "Family", false)).expect(t, http.StatusOK)
	reused := s.oneLoginCreate(oneLoginReusedID)
	if got := s.bindingByID(reused).UserID; got == userID {
		t.Fatal("a new person was given the deleted person's user")
	}
	if u := s.user(userID); u.DisabledAt == nil {
		t.Fatal("the deleted person's user was reactivated")
	}
}

// TestOneLoginErrorDetails checks that the failures OneLogin shows its admin say what to change in OneLogin.
func TestOneLoginErrorDetails(t *testing.T) {
	s := newSCIMTest(t)
	s.enableOneLogin()

	detail := func(resp testResponse) string {
		d, _ := resp.body["detail"].(string)
		return d
	}

	scimID := s.oneLoginCreate(oneLoginOwnerID)

	// With the action for deleted users and removed app access on Delete, OneLogin sends a DELETE.
	if got, want := detail(s.do(http.MethodDelete, "Users/"+scimID, nil).expect(t, http.StatusNotImplemented)),
		`Obot does not delete users. On the OneLogin app's Provisioning tab, set "When users are deleted in `+
			`OneLogin, or the user's app access is removed" to Suspend, then click Retry on this user.`; got != want {
		t.Errorf("user DELETE detail = %q, want %q", got, want)
	}
	if u := s.do(http.MethodGet, "Users/"+scimID, nil).expect(t, http.StatusOK); u.body["active"] != true {
		t.Errorf("the refused DELETE changed the user: %v", u.body)
	}

	// OneLogin is not expected to send this: it writes a user only to the account its lookup found, and a userName that
	// is not the user's own ID is refused before the externalId is compared. The PUT is sent directly, to check that a
	// changed externalId carries the connection's hint.
	if got, want := detail(s.do(http.MethodPut, "Users/"+scimID, oneLoginUser(oneLoginOtherID, "Family", true)).expect(t, http.StatusBadRequest)),
		"externalId identifies the user and cannot change. This OneLogin user's ID does not match the account the "+
			"request was sent to, so that account belongs to another OneLogin user."; got != want {
		t.Errorf("changed externalId detail = %q, want %q", got, want)
	}

	// An app set up without the template edit, or without SCIM Username on OneLogin ID, fails its first create with
	// the setting to change.
	noExternalID := oneLoginUser(oneLoginBasicID, "Family", true)
	delete(noExternalID, "externalId")
	if got, want := detail(s.do(http.MethodPost, "Users", noExternalID).expect(t, http.StatusBadRequest)),
		`externalId is required. Add "externalId": "{$user.id}" to the SCIM JSON Template on the OneLogin app's `+
			`Configuration tab.`; got != want {
		t.Errorf("missing externalId detail = %q, want %q", got, want)
	}
	byUsername := oneLoginUser(oneLoginBasicID, "Family", true)
	byUsername["userName"] = "jdoe"
	if got, want := detail(s.do(http.MethodPost, "Users", byUsername).expect(t, http.StatusBadRequest)),
		"userName must be the OneLogin user ID, the same as externalId. On the OneLogin app's Parameters tab, set "+
			"SCIM Username to OneLogin ID. OneLogin finds existing users by SCIM Username, and a username can be "+
			"re-used by another person."; got != want {
		t.Errorf("userName detail = %q, want %q", got, want)
	}
	delete(byUsername, "externalId")
	if got, want := detail(s.do(http.MethodPost, "Users", byUsername).expect(t, http.StatusBadRequest)),
		`externalId and userName must both be the numeric OneLogin user ID. Make the SCIM JSON Template on the `+
			`OneLogin app's Configuration tab send "externalId": "{$user.id}", and on the Parameters tab, set SCIM `+
			`Username to OneLogin ID.`; got != want {
		t.Errorf("missing externalId and userName detail = %q, want %q", got, want)
	}
	if n := s.count(new(types.SCIMUserBinding), ""); n != 1 {
		t.Errorf("got %d bindings, want only the first user's", n)
	}

	// The "SCIM2 PATCH for Groups" connector adds a member with a filtered path and no value.
	group := s.do(http.MethodPost, "Groups", oneLoginGroup("obot-scim-test")).expect(t, http.StatusCreated)
	if got, want := detail(s.do(http.MethodPatch, "Groups/"+group.id(), patchOp(map[string]any{
		"op":   "add",
		"path": `members[ value eq "` + scimID + `" ]`,
	})).expect(t, http.StatusBadRequest)), "add requires a value. This app uses a connector Obot does not support. "+
		"Create a OneLogin app with the 'SCIM Provisioner with SAML (SCIM v2 Core)' connector and grant users access "+
		"to it instead."; got != want {
		t.Errorf("valueless filtered add detail = %q, want %q", got, want)
	}

	// While another auth provider is configured, the connection's requests fail with its adapter's retry hint.
	s.env.name = "another-auth-provider"
	if got, want := detail(s.do(http.MethodGet, "Users", nil).expect(t, http.StatusServiceUnavailable)),
		"the auth provider of this SCIM connection is not configured. In OneLogin, click Retry on the user in the "+
			"app's Users tab, or run the connection test or entitlements refresh again."; got != want {
		t.Errorf("unconfigured auth provider detail = %q, want %q", got, want)
	}
}

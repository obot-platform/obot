package scim

import (
	"bytes"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"github.com/obot-platform/obot/pkg/system"
)

// The tests in this file follow the requests that the Microsoft Entra provisioning service sent in Phase 0, with the
// enterprise app's user mapping changed to map objectId to externalId, as Obot requires.

const (
	testEntraProviderName = "entra-auth-provider"
	testEntraGroupPrefix  = "entra/"
	testEntraTenantID     = "00000000-0000-4000-8000-0000000000aa"

	// Entra object IDs. Sign-in subjects are pairwise per application, and unrelated to them.
	entraOwnerOID         = "00000000-0000-4000-8000-000000000001"
	entraBasicOID         = "00000000-0000-4000-8000-000000000002"
	entraUnprovisionedOID = "00000000-0000-4000-8000-000000000003"
	entraNewOID           = "00000000-0000-4000-8000-000000000004"

	entraOwnerSub         = "sub-owner-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
	entraBasicSub         = "sub-basic-BBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBBB"
	entraUnprovisionedSub = "sub-unprovisioned-CCCCCCCCCCCCCCCCCCCCCCCCCC"
	entraNewSub           = "sub-new-DDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDDD"

	// Entra group object IDs.
	entraGroup1OID   = "00000000-0000-4000-8000-000000000011"
	entraGroup2OID   = "00000000-0000-4000-8000-000000000012"
	entraAllUsersOID = "00000000-0000-4000-8000-000000000013"
	entraDecoyOID    = "00000000-0000-4000-8000-000000000014"
)

// entraUser returns a user create body as Entra sends it with objectId mapped to externalId: the user principal name
// as userName, null sub-values, the enterprise extension, and no password.
func entraUser(userName, objectID string) map[string]any {
	return map[string]any{
		"schemas": []any{
			userSchema,
			enterpriseUserSchema,
		},
		"externalId":  objectID,
		"userName":    userName,
		"active":      true,
		"displayName": "Given Family",
		"name": map[string]any{
			"givenName":  "Given",
			"familyName": "Family",
			"formatted":  "Given Family",
		},
		"emails": []any{
			map[string]any{
				"primary": true,
				"type":    "work",
				"value":   userName,
			},
		},
		"addresses": []any{
			map[string]any{
				"type":          "work",
				"primary":       false,
				"country":       "US",
				"streetAddress": nil,
				"postalCode":    nil,
			},
		},
		enterpriseUserSchema: map[string]any{
			"department": "Engineering",
		},
		"meta": map[string]any{
			"resourceType": "User",
		},
	}
}

// entraGroup returns a group create body as Entra sends it: the group's object ID as externalId, and no members, which
// Entra adds one PATCH at a time.
func entraGroup(displayName, objectID string) map[string]any {
	return map[string]any{
		"schemas":     []any{groupSchema},
		"displayName": displayName,
		"externalId":  objectID,
		"members":     []any{},
		"meta": map[string]any{
			"resourceType": "Group",
		},
	}
}

// entraMembers returns the PATCH that adds or removes one member, as Entra sends it: a capitalized op on members, with a
// value list.
func entraMembers(op, scimUserID string) map[string]any {
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

// entraCreate sends a user's create as Entra does: a userName eq lookup, which misses, then the create. It returns the
// user's SCIM ID.
func (s *scimTest) entraCreate(userName, objectID string) string {
	s.t.Helper()

	if found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "`+userName+`"`), nil).expect(s.t, http.StatusOK).resources(); len(found) != 0 {
		s.t.Fatalf("the lookup of %s before its create found %v", userName, found)
	}
	return s.do(http.MethodPost, "Users", entraUser(userName, objectID)).expect(s.t, http.StatusCreated).id()
}

// entraGroupCreate sends a group's create as Entra does: a displayName eq lookup without members, which misses, then
// the create. It returns the group's SCIM ID.
func (s *scimTest) entraGroupCreate(displayName, objectID string) string {
	s.t.Helper()

	query := "excludedAttributes=members&filter=" + url.QueryEscape(`displayName eq "`+displayName+`"`)
	if found := s.do(http.MethodGet, "Groups?"+query, nil).expect(s.t, http.StatusOK).resources(); len(found) != 0 {
		s.t.Fatalf("the lookup of %s before its create found %v", displayName, found)
	}
	return s.do(http.MethodPost, "Groups", entraGroup(displayName, objectID)).expect(s.t, http.StatusCreated).id()
}

// entraProbe sends the request that starts every Entra provisioning cycle, a lookup of a random userName.
func (s *scimTest) entraProbe() testResponse {
	s.t.Helper()
	return s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "3f6a0f7e-5d1b-4f43-9a4e-0c1b2a3d4e5f"`), nil)
}

// pendingIdentities returns the number of identities that SCIM created for the object ID, which wait for a sign-in.
func (s *scimTest) pendingIdentities(objectID string) int64 {
	s.t.Helper()
	return s.count(new(types.Identity), "auth_provider_name = ? AND hashed_provider_user_id = ?",
		testEntraProviderName, hash.String(adapter.PendingProviderUserID(objectID)))
}

// enableEntra creates the SCIM connection of the Entra auth provider, whose issuer is its tenant ID, once the backfill
// of hashed group lookup IDs has run, as it has on any running installation.
func (s *scimTest) enableEntra(origin types.SCIMConnectionOrigin) {
	s.t.Helper()

	if err := s.gateway.BackfillIdentityGroupLookupIDHashes(s.t.Context()); err != nil {
		s.t.Fatalf("failed to backfill: %v", err)
	}
	s.enableProviderWithOrigin(testEntraProviderName, testEntraGroupPrefix, testEntraTenantID, origin)
}

// seedEntraUser creates an Entra user through sign-in, as the Entra auth provider signs users in: the identity is
// keyed by the OIDC subject, which is also the username, and stores the object ID as the group lookup ID.
func (s *scimTest) seedEntraUser(sub, objectID, email string, role types2.Role) *types.User {
	s.t.Helper()

	user, err := s.signInEntra(sub, objectID, email, role)
	if err != nil {
		s.t.Fatalf("failed to seed Entra user %s: %v", objectID, err)
	}
	return user
}

// signInEntra signs a person in through the Entra auth provider, which supplies the object ID from
// /obot-get-user-info.
func (s *scimTest) signInEntra(sub, objectID, email string, role types2.Role) (*types.User, error) {
	s.t.Helper()

	return s.gateway.EnsureIdentityWithRole(s.t.Context(), &types.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      testEntraProviderName,
		ProviderUsername:      sub,
		ProviderUserID:        sub,
		ProviderGroupLookupID: objectID,
		Email:                 email,
	}, "", role, gclient.UserLimit{
		Unlimited: true,
	})
}

func TestEntraSCIMFirst(t *testing.T) {
	testEntraSCIMFirst(t, newSCIMTest(t))
}

func TestEntraSCIMFirstPostgres(t *testing.T) {
	testEntraSCIMFirst(t, newPostgresSCIMTest(t))
}

func testEntraSCIMFirst(t *testing.T, s *scimTest) {
	t.Helper()

	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)

	// Test Connection and every cycle start with the probe, which finds nothing.
	if found := s.entraProbe().expect(t, http.StatusOK).resources(); len(found) != 0 {
		t.Fatalf("the probe found %v", found)
	}

	// The Owner and a Basic user are provisioned before either signs in. Their identities wait for a sign-in.
	ownerSCIM := s.entraCreate("owner@example.com", entraOwnerOID)
	basicSCIM := s.entraCreate("basic@example.com", entraBasicOID)
	ownerID := s.bindingByID(ownerSCIM).UserID
	basicID := s.bindingByID(basicSCIM).UserID
	for _, oid := range []string{entraOwnerOID, entraBasicOID} {
		if n := s.pendingIdentities(oid); n != 1 {
			t.Fatalf("got %d pending identities for %s, want 1", n, oid)
		}
	}
	if u := s.user(basicID); u.Username != entraBasicOID || u.DisabledAt != nil {
		t.Fatalf("provisioned user = %+v, want the object ID as username, and enabled", u)
	}
	users := s.count(new(types.User), "")

	// Each first sign-in claims the identity that SCIM created, keyed by its subject from then on, and takes the
	// subject as username, as every Entra user that signs in has.
	owner, err := s.signInEntra(entraOwnerSub, entraOwnerOID, "owner@example.com", types2.RoleOwner)
	if err != nil || owner.ID != ownerID {
		t.Fatalf("the Owner's first sign-in = %+v, %v, want user %d", owner, err, ownerID)
	}
	basic, err := s.signInEntra(entraBasicSub, entraBasicOID, "basic@example.com", types2.RoleUnknown)
	if err != nil || basic.ID != basicID {
		t.Fatalf("the Basic user's first sign-in = %+v, %v, want user %d", basic, err, basicID)
	}
	if u := s.user(basicID); u.Username != entraBasicSub {
		t.Fatalf("username after the claim = %q, want the subject", u.Username)
	}
	for _, oid := range []string{entraOwnerOID, entraBasicOID} {
		if n := s.pendingIdentities(oid); n != 0 {
			t.Fatalf("got %d pending identities for %s after the claim, want none", n, oid)
		}
	}
	if n := s.count(new(types.Identity), "auth_provider_name = ? AND hashed_provider_user_id = ? AND user_id = ?",
		testEntraProviderName, hash.String(entraBasicSub), basicID); n != 1 {
		t.Fatalf("got %d identities keyed by the subject, want 1", n)
	}
	if got := s.count(new(types.User), ""); got != users {
		t.Fatalf("got %d users after the sign-ins, want %d", got, users)
	}

	// The claim changed nothing that SCIM reads back, so Entra has nothing to correct.
	resp := s.do(http.MethodGet, "Users/"+basicSCIM, nil).expect(t, http.StatusOK)
	if resp.body["userName"] != "basic@example.com" || resp.body["externalId"] != entraBasicOID {
		t.Fatalf("user after the claim = %v", resp.body)
	}

	// Once enforced, provisioned users still sign in, and a person Entra never provisioned cannot.
	s.enforce(ownerID)
	if again, err := s.signInEntra(entraBasicSub, entraBasicOID, "basic@example.com", types2.RoleUnknown); err != nil || again.ID != basicID {
		t.Fatalf("a sign-in after enforcing = %+v, %v, want user %d", again, err, basicID)
	}
	if _, err := s.signInEntra(entraUnprovisionedSub, entraUnprovisionedOID, "unprovisioned@example.com", types2.RoleUnknown); err == nil {
		t.Fatal("an unprovisioned person signed in after enforcing")
	}

	// A person provisioned after enforcing signs in for the first time to the provisioned account.
	newSCIM := s.entraCreate("new@example.com", entraNewOID)
	newID := s.bindingByID(newSCIM).UserID
	if newUser, err := s.signInEntra(entraNewSub, entraNewOID, "new@example.com", types2.RoleUnknown); err != nil || newUser.ID != newID {
		t.Fatalf("the first sign-in after enforcing = %+v, %v, want user %d", newUser, err, newID)
	}
}

func TestEntraMigration(t *testing.T) {
	testEntraMigration(t, newSCIMTest(t))
}

func TestEntraMigrationPostgres(t *testing.T) {
	testEntraMigration(t, newPostgresSCIMTest(t))
}

func testEntraMigration(t *testing.T, s *scimTest) {
	t.Helper()

	// The installation synchronized its Entra directory before SCIM, as in Phase 0 run 002: U1 and U3 are assigned to
	// the app, U2 never is, and All Users is never assigned. G1 has a role assignment, and was renamed in Entra after
	// the last sign-in that cached it. Another group, never assigned, has the name G1 has now: names are not unique in
	// Entra, so only the externalId tells them apart.
	u1 := s.seedEntraUser(entraOwnerSub, entraOwnerOID, "owner@example.com", types2.RoleOwner)
	u2 := s.seedEntraUser(entraUnprovisionedSub, entraUnprovisionedOID, "unprovisioned@example.com", types2.RoleBasic)
	u3 := s.seedEntraUser(entraBasicSub, entraBasicOID, "basic@example.com", types2.RoleBasic)
	g1 := testEntraGroupPrefix + entraGroup1OID
	g2 := testEntraGroupPrefix + entraGroup2OID
	all := testEntraGroupPrefix + entraAllUsersOID
	decoy := testEntraGroupPrefix + entraDecoyOID
	s.seedProviderGroup(testEntraProviderName, g1, "Old Team Name", u1.ID)
	s.seedProviderGroup(testEntraProviderName, g2, "Grant SCIM Test 2", u3.ID)
	s.seedProviderGroup(testEntraProviderName, all, "All Users", u1.ID, u2.ID, u3.ID)
	s.seedProviderGroup(testEntraProviderName, decoy, "Grant SCIM Test 1", u2.ID)
	if _, err := s.gateway.CreateGroupRoleAssignment(t.Context(), g1, types2.RoleAdmin, ""); err != nil {
		t.Fatal(err)
	}
	users := s.count(new(types.User), "")
	s.enableEntra(types.SCIMConnectionOriginMigrated)

	// Users bind by object ID, and keep their accounts, identities, and usernames.
	u1SCIM := s.entraCreate("owner@example.com", entraOwnerOID)
	u3SCIM := s.entraCreate("basic@example.com", strings.ToUpper(entraBasicOID))
	if got := s.bindingByID(u1SCIM).UserID; got != u1.ID {
		t.Fatalf("U1 bound user %d, want %d", got, u1.ID)
	}
	if got := s.bindingByID(u3SCIM).UserID; got != u3.ID {
		t.Fatalf("U3 bound user %d, want %d", got, u3.ID)
	}
	if got := s.count(new(types.User), ""); got != users {
		t.Fatalf("got %d users, want %d", got, users)
	}
	if n := s.pendingIdentities(entraOwnerOID) + s.pendingIdentities(entraBasicOID); n != 0 {
		t.Fatalf("binding existing users created %d pending identities", n)
	}
	if u := s.user(u3.ID); u.Username != entraBasicSub {
		t.Fatalf("U3's username = %q, want the subject", u.Username)
	}
	if binding, _ := s.gateway.SCIMUserBindingForUser(t.Context(), u2.ID); binding != nil {
		t.Fatal("the unassigned user was bound")
	}

	// Each group POST binds the legacy group by externalId, keeping its ID and its role assignment, and takes the
	// pushed name. The empty member list drops the legacy members until Entra's Add arrives.
	g1SCIM := s.entraGroupCreate("Grant SCIM Test 1", entraGroup1OID)
	g2SCIM := s.entraGroupCreate("Grant SCIM Test 2", entraGroup2OID)
	if got := s.groupBindingByID(g1SCIM).GroupID; got != g1 {
		t.Fatalf("G1 bound %s, want %s", got, g1)
	}
	if got := s.groupBindingByID(g2SCIM).GroupID; got != g2 {
		t.Fatalf("G2 bound %s, want %s", got, g2)
	}
	for _, id := range []string{g1, g2} {
		if members := s.memberships(id); len(members) != 0 {
			t.Fatalf("%s kept its legacy members %v until Entra added them", id, members)
		}
	}
	if n := s.count(new(types.GroupRoleAssignment), "group_name = ?", g1); n != 1 {
		t.Fatalf("G1 has %d role assignments, want 1", n)
	}
	if n := s.count(new(types.Group), ""); n != 4 {
		t.Fatalf("got %d groups, want the 4 legacy groups", n)
	}
	g1Resp := s.do(http.MethodGet, "Groups/"+g1SCIM+"?excludedAttributes=members", nil).expect(t, http.StatusOK)
	if g1Resp.body["externalId"] != entraGroup1OID || g1Resp.body["displayName"] != "Grant SCIM Test 1" {
		t.Fatalf("G1 = %v, want its externalId echoed and the pushed name", g1Resp.body)
	}

	// Entra adds each member with its own PATCH, which restores them.
	s.do(http.MethodPatch, "Groups/"+g1SCIM, entraMembers("Add", u1SCIM)).expect(t, http.StatusNoContent)
	s.do(http.MethodPatch, "Groups/"+g2SCIM, entraMembers("Add", u3SCIM)).expect(t, http.StatusNoContent)
	if got := s.memberships(g1); !slices.Equal(got, sortedIDs(u1.ID)) {
		t.Fatalf("G1 members = %v, want U1", got)
	}
	if got := s.memberships(g2); !slices.Equal(got, sortedIDs(u3.ID)) {
		t.Fatalf("G2 members = %v, want U3", got)
	}

	// The unassigned groups stay untouched, and invisible, including the one with G1's name.
	if got := s.memberships(all); !slices.Equal(got, sortedIDs(u1.ID, u2.ID, u3.ID)) {
		t.Fatalf("All Users members = %v, want its legacy members", got)
	}
	if got := s.memberships(decoy); !slices.Equal(got, sortedIDs(u2.ID)) {
		t.Fatalf("the group with G1's name has members %v, want its legacy member", got)
	}
	if total := s.do(http.MethodGet, "Groups", nil).expect(t, http.StatusOK).body["totalResults"]; total != float64(2) {
		t.Fatalf("totalResults = %v, want the 2 pushed groups", total)
	}

	// A group the admin then matches by externalId is found by it.
	found := s.do(http.MethodGet, "Groups?filter="+url.QueryEscape(`externalId eq "`+entraGroup2OID+`"`), nil).expect(t, http.StatusOK).resources()
	if len(found) != 1 || found[0]["id"] != g2SCIM {
		t.Fatalf("externalId eq found %v, want G2", found)
	}

	// The bound users still sign in to the same accounts, without a new identity.
	identities := s.count(new(types.Identity), "")
	if user, err := s.signInEntra(entraOwnerSub, entraOwnerOID, "owner@example.com", types2.RoleUnknown); err != nil || user.ID != u1.ID {
		t.Fatalf("U1's sign-in = %+v, %v, want user %d", user, err, u1.ID)
	}
	if got := s.count(new(types.Identity), ""); got != identities {
		t.Fatalf("got %d identities after a sign-in, want %d", got, identities)
	}
}

// TestEntraDefaultMappingIsRefused checks that the default user mapping, which sends mailNickname as externalId, is
// refused on the first create with a detail that names the mapping to change. Entra shows the detail verbatim.
func TestEntraDefaultMappingIsRefused(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)

	resp := s.do(http.MethodPost, "Users", entraUser("user@example.com", "jdoe")).expect(t, http.StatusBadRequest)
	detail, _ := resp.body["detail"].(string)
	if !strings.Contains(detail, "map objectId to externalId (the default maps mailNickname)") {
		t.Fatalf("detail = %q, want the mapping to change", detail)
	}
	// The portal shows the body as written, so the path to the mapping is not escaped.
	if !bytes.Contains(resp.raw, []byte("Provisioning > Attribute mapping > Provision Microsoft Entra ID Users")) {
		t.Fatalf("body = %s, want the mapping's path written as it is", resp.raw)
	}
	if n := s.count(new(types.User), ""); n != 0 {
		t.Fatalf("the refused create made %d users", n)
	}

	// A group without its object ID is refused too.
	resp = s.do(http.MethodPost, "Groups", entraGroup("team", "team")).expect(t, http.StatusBadRequest)
	if detail, _ := resp.body["detail"].(string); !strings.Contains(detail, "map objectId to externalId") {
		t.Fatalf("detail = %q, want the mapping to change", detail)
	}
}

// TestEntraSoftDeleteRestoreAndDelete follows a user through a soft delete in Entra, a restore, another soft delete,
// and the permanent delete, as Phase 0 run 001 captured them.
func TestEntraSoftDeleteRestoreAndDelete(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)

	userSCIM := s.entraCreate("user@example.com", entraNewOID)
	userID := s.bindingByID(userSCIM).UserID
	groupSCIM := s.entraGroupCreate("team", entraGroup1OID)
	groupID := testEntraGroupPrefix + entraGroup1OID
	s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Add", userSCIM)).expect(t, http.StatusNoContent)

	// A soft delete deactivates the user, prefixes the object ID's hex to the userName after a collision lookup, and
	// removes the user from the group.
	mangled := strings.ReplaceAll(entraNewOID, "-", "") + "user@example.com"
	softDelete := func() {
		t.Helper()
		if found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "`+mangled+`"`), nil).expect(t, http.StatusOK).resources(); len(found) != 0 {
			t.Fatalf("the collision lookup found %v", found)
		}
		s.do(http.MethodPatch, "Users/"+userSCIM, patchOp(
			map[string]any{
				"op":    "Replace",
				"path":  "active",
				"value": "False",
			},
			map[string]any{
				"op":    "Replace",
				"path":  "userName",
				"value": mangled,
			},
		)).expect(t, http.StatusOK)
		s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Remove", userSCIM)).expect(t, http.StatusNoContent)
		if u := s.user(userID); u.DisabledAt == nil {
			t.Fatal("the soft-deleted user is not disabled")
		}
		if members := s.memberships(groupID); len(members) != 0 {
			t.Fatalf("group members after the soft delete = %v", members)
		}
	}
	softDelete()
	if got := s.do(http.MethodGet, "Users/"+userSCIM, nil).expect(t, http.StatusOK).body; got["userName"] != mangled || got["active"] != false {
		t.Fatalf("soft-deleted user = %v", got)
	}

	// A restore reverses all of it, on the same account.
	s.do(http.MethodPatch, "Users/"+userSCIM, patchOp(
		map[string]any{
			"op":    "Replace",
			"path":  "active",
			"value": "True",
		},
		map[string]any{
			"op":    "Replace",
			"path":  "userName",
			"value": "user@example.com",
		},
	)).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Add", userSCIM)).expect(t, http.StatusNoContent)
	if u := s.user(userID); u.DisabledAt != nil {
		t.Fatal("the restored user is still disabled")
	}
	if got := s.do(http.MethodGet, "Users/"+userSCIM, nil).expect(t, http.StatusOK).body; got["userName"] != "user@example.com" || got["active"] != true {
		t.Fatalf("restored user = %v", got)
	}
	if got := s.memberships(groupID); !slices.Equal(got, sortedIDs(userID)) {
		t.Fatalf("group members after the restore = %v", got)
	}

	// Another soft delete, then the permanent delete, which deletes the user.
	softDelete()
	s.do(http.MethodDelete, "Users/"+userSCIM, nil).expect(t, http.StatusNoContent)
	if u := s.user(userID); u.DeletedAt == nil {
		t.Fatal("the permanently deleted user was not deleted")
	}
	s.do(http.MethodGet, "Users/"+userSCIM, nil).expect(t, http.StatusNotFound)
	s.do(http.MethodDelete, "Users/"+userSCIM, nil).expect(t, http.StatusNotFound)
	if found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "`+mangled+`"`), nil).expect(t, http.StatusOK).resources(); len(found) != 0 {
		t.Fatalf("the deleted user is still listed: %v", found)
	}

	// Entra never reuses object IDs, but provisioning the same one again starts a new account.
	again := s.entraCreate("user@example.com", entraNewOID)
	if got := s.bindingByID(again).UserID; got == userID {
		t.Fatal("the deleted user was restored")
	}
}

// TestEntraGroupUnassignAndReassign checks that unassigning a group, which Entra sends as a DELETE, and assigning it
// again bind the same Obot group. Before enforcing, the group keeps its grants; after, the DELETE removes them.
func TestEntraGroupUnassignAndReassign(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)
	groupID := testEntraGroupPrefix + entraGroup1OID

	ownerSCIM := s.entraCreate("owner@example.com", entraOwnerOID)
	owner, err := s.signInEntra(entraOwnerSub, entraOwnerOID, "owner@example.com", types2.RoleOwner)
	if err != nil {
		t.Fatal(err)
	}

	first := s.entraGroupCreate("team", entraGroup1OID)
	s.do(http.MethodPatch, "Groups/"+first, entraMembers("Add", ownerSCIM)).expect(t, http.StatusNoContent)
	if _, err := s.gateway.CreateGroupRoleAssignment(t.Context(), groupID, types2.RoleAdmin, ""); err != nil {
		t.Fatal(err)
	}

	// Before enforcing, the DELETE retires the binding and keeps the group and its grants, and the same externalId
	// binds the same group again.
	s.do(http.MethodDelete, "Groups/"+first, nil).expect(t, http.StatusNoContent)
	if n := s.count(new(types.Group), "id = ?", groupID); n != 1 {
		t.Fatal("unassigning the group before enforcing deleted it")
	}
	second := s.entraGroupCreate("team", entraGroup1OID)
	if second == first {
		t.Fatal("a retired SCIM ID was reused")
	}
	if got := s.groupBindingByID(second).GroupID; got != groupID {
		t.Fatalf("the reassigned group bound %s, want %s", got, groupID)
	}
	if n := s.count(new(types.GroupRoleAssignment), "group_name = ?", groupID); n != 1 {
		t.Fatalf("the reassigned group has %d role assignments, want 1", n)
	}
	s.do(http.MethodPatch, "Groups/"+second, entraMembers("Add", ownerSCIM)).expect(t, http.StatusNoContent)

	// Once enforced, the DELETE deletes the group and its grants. Assigning it again recreates the same group ID,
	// without them.
	s.enforce(owner.ID)
	s.do(http.MethodDelete, "Groups/"+second, nil).expect(t, http.StatusNoContent)
	if n := s.count(new(types.Group), "id = ?", groupID); n != 0 {
		t.Fatal("unassigning the group after enforcing kept it")
	}
	if n := s.count(new(types.GroupRoleAssignment), "group_name = ?", groupID); n != 0 {
		t.Fatalf("the deleted group kept %d role assignments", n)
	}
	// The group's ID is its object ID's, so until its subjects are removed from access policies, assigning it again
	// is a conflict, which Entra retries in a later cycle.
	resp := s.do(http.MethodPost, "Groups", entraGroup("team", entraGroup1OID)).expect(t, http.StatusConflict)
	if detail, _ := resp.body["detail"].(string); !strings.Contains(detail, "retry later") {
		t.Fatalf("detail = %v, want a retry", resp.body["detail"])
	}
	if err := s.gateway.CompleteSCIMGroupSubjectCleanup(t.Context(), groupID); err != nil {
		t.Fatal(err)
	}
	third := s.entraGroupCreate("team", entraGroup1OID)
	if got := s.groupBindingByID(third).GroupID; got != groupID {
		t.Fatalf("the group assigned again is %s, want %s", got, groupID)
	}
	if n := s.count(new(types.GroupRoleAssignment), "group_name = ?", groupID); n != 0 {
		t.Fatalf("the recreated group has %d role assignments, want none", n)
	}
}

// TestEntraRepeatedMemberChanges checks that the repeated Add and Remove PATCHes that Entra sends on initial cycles,
// restarts, and replays of provision on demand all succeed and change nothing more.
func TestEntraRepeatedMemberChanges(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)
	groupID := testEntraGroupPrefix + entraGroup1OID

	memberSCIM := s.entraCreate("member@example.com", entraBasicOID)
	otherSCIM := s.entraCreate("other@example.com", entraNewOID)
	memberID := s.bindingByID(memberSCIM).UserID
	groupSCIM := s.entraGroupCreate("team", entraGroup1OID)

	for range 2 {
		s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Add", memberSCIM)).expect(t, http.StatusNoContent)
	}
	if got := s.memberships(groupID); !slices.Equal(got, sortedIDs(memberID)) {
		t.Fatalf("members after repeated adds = %v", got)
	}
	events := s.count(new(types.UserLifecycleEvent), "")

	// Removing a non-member, a member twice, and an ID Obot never issued all answer 204.
	s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Remove", otherSCIM)).expect(t, http.StatusNoContent)
	if got := s.count(new(types.UserLifecycleEvent), ""); got != events {
		t.Fatalf("removing a non-member recorded %d events", got-events)
	}
	for range 2 {
		s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Remove", memberSCIM)).expect(t, http.StatusNoContent)
	}
	s.do(http.MethodPatch, "Groups/"+groupSCIM, entraMembers("Remove", "8eaf7175-12c1-4f88-8ddb-dd6b9cb24928")).expect(t, http.StatusNoContent)
	if members := s.memberships(groupID); len(members) != 0 {
		t.Fatalf("members after repeated removes = %v", members)
	}
}

// TestEntraGroupRename checks a rename, which Entra sends as a replace of displayName on the same SCIM ID after a
// displayName eq lookup of the new name. Bound names are unique, so a rename to another group's name is a conflict.
func TestEntraGroupRename(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)

	first := s.entraGroupCreate("first", entraGroup1OID)
	s.entraGroupCreate("second", entraGroup2OID)
	rename := func(name string) testResponse {
		return s.do(http.MethodPatch, "Groups/"+first, patchOp(map[string]any{
			"op":    "Replace",
			"path":  "displayName",
			"value": name,
		}))
	}

	query := "excludedAttributes=members&filter=" + url.QueryEscape(`displayName eq "second"`)
	if found := s.do(http.MethodGet, "Groups?"+query, nil).expect(t, http.StatusOK).resources(); len(found) != 1 {
		t.Fatalf("the collision lookup found %v, want the other group", found)
	}
	if resp := rename("second").expect(t, http.StatusConflict); resp.body["scimType"] != scimTypeUniqueness {
		t.Fatalf("scimType = %v, want %s", resp.body["scimType"], scimTypeUniqueness)
	}

	rename("renamed").expect(t, http.StatusNoContent)
	if got := s.groupBindingByID(first).GroupID; got != testEntraGroupPrefix+entraGroup1OID {
		t.Fatalf("the renamed group is %s, want its ID kept", got)
	}
	var group types.Group
	if err := s.gorm().Where("id = ?", testEntraGroupPrefix+entraGroup1OID).Take(&group).Error; err != nil {
		t.Fatal(err)
	}
	if group.Name != "renamed" {
		t.Fatalf("group name = %q, want renamed", group.Name)
	}
}

// TestEntraProbe checks the answers to the probe that starts every cycle. While the provider is not the configured
// one, it is a 503 whose detail tells the admin how to recover from the quarantine that two such cycles cause.
func TestEntraProbe(t *testing.T) {
	s := newSCIMTest(t)
	s.enableEntra(types.SCIMConnectionOriginSCIMFirst)

	if found := s.entraProbe().expect(t, http.StatusOK).resources(); len(found) != 0 {
		t.Fatalf("the probe found %v", found)
	}

	// Another auth provider is configured, then none is.
	for _, name := range []string{"google-auth-provider", ""} {
		s.env.name = name
		resp := s.entraProbe().expect(t, http.StatusServiceUnavailable)
		if detail, _ := resp.body["detail"].(string); !strings.Contains(detail, "select Restart provisioning") {
			t.Fatalf("detail with %q configured = %q, want the Restart provisioning hint", name, detail)
		}
	}
}

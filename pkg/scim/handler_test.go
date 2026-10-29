package scim

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
)

func TestAvailabilityAndAuthentication(t *testing.T) {
	s := newSCIMTest(t)

	// Before any connection exists, the endpoint is unavailable, and says so before authenticating.
	resp := s.request(http.MethodGet, s.path("Users"), "", nil).expect(t, http.StatusServiceUnavailable)
	if resp.header.Get("Retry-After") == "" {
		t.Fatal("503 without Retry-After")
	}
	if resp.body["status"] != "503" {
		t.Fatalf("status is %#v, want the string \"503\"", resp.body["status"])
	}

	s.enable()

	tests := []struct {
		name   string
		path   string
		token  string
		status int
	}{
		{
			name:   "no token",
			path:   s.path("Users"),
			status: http.StatusUnauthorized,
		},
		{
			name:   "wrong token",
			path:   s.path("Users"),
			token:  "obot_scim_wrong",
			status: http.StatusUnauthorized,
		},
		{
			name:   "a path that names a connection, which the base URL does not",
			path:   PathPrefix + "00000000-0000-0000-0000-000000000000/Users",
			token:  s.token,
			status: http.StatusNotFound,
		},
		{
			name:   "valid token",
			path:   s.path("Users"),
			token:  s.token,
			status: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.request(http.MethodGet, tt.path, tt.token, nil).expect(t, tt.status)
			if tt.status == http.StatusUnauthorized && resp.header.Get("WWW-Authenticate") == "" {
				t.Fatal("401 without WWW-Authenticate")
			}
		})
	}

	// While the connection's auth provider is not the configured one, only an authenticated caller learns why.
	s.env.name = "google-auth-provider"
	s.request(http.MethodGet, s.path("Users"), "", nil).expect(t, http.StatusUnauthorized)
	s.do(http.MethodGet, "Users", nil).expect(t, http.StatusServiceUnavailable)
	s.env.name = ""
	s.do(http.MethodGet, "Users", nil).expect(t, http.StatusServiceUnavailable)
	s.env.name = testOktaProviderName

	// A rotated token keeps working until it is revoked.
	previous := s.token
	_, rotated, err := s.gateway.RotateSCIMConnectionToken(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatalf("failed to rotate token: %v", err)
	}
	s.request(http.MethodGet, s.path("Users"), previous, nil).expect(t, http.StatusOK)
	s.request(http.MethodGet, s.path("Users"), rotated, nil).expect(t, http.StatusOK)
	if err := s.gateway.RevokePreviousSCIMConnectionToken(t.Context(), s.conn.ID); err != nil {
		t.Fatalf("failed to revoke previous token: %v", err)
	}
	s.request(http.MethodGet, s.path("Users"), previous, nil).expect(t, http.StatusUnauthorized)
	s.request(http.MethodGet, s.path("Users"), rotated, nil).expect(t, http.StatusOK)

	// Only a verifier of the token is stored.
	stored, err := s.gateway.SCIMConnection(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TokenVerifier == rotated || stored.TokenVerifier != hash.String(rotated) {
		t.Fatal("the stored token verifier is not a hash of the token")
	}

	// A second connection is refused.
	if _, _, err := s.gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      testOktaProviderName,
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	}); err == nil {
		t.Fatal("a second SCIM connection was created")
	} else if _, ok := errors.AsType[*gclient.SCIMConnectionExistsError](err); !ok {
		t.Fatalf("creating a second SCIM connection failed with %v, want a SCIMConnectionExistsError", err)
	}
}

func TestDiscoveryAndUnsupportedOperations(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	spc := s.do(http.MethodGet, "ServiceProviderConfig", nil).expect(t, http.StatusOK)
	for _, feature := range []string{"bulk", "sort", "etag", "changePassword"} {
		if supported := spc.body[feature].(map[string]any)["supported"]; supported != false {
			t.Errorf("%s supported = %v, want false", feature, supported)
		}
	}
	if spc.body["patch"].(map[string]any)["supported"] != true {
		t.Error("patch is not reported as supported")
	}

	if got := len(s.do(http.MethodGet, "Schemas", nil).expect(t, http.StatusOK).resources()); got != 3 {
		t.Errorf("got %d schemas, want 3", got)
	}
	s.do(http.MethodGet, "Schemas/"+userSchema, nil).expect(t, http.StatusOK)
	s.do(http.MethodGet, "Schemas/"+enterpriseUserSchema, nil).expect(t, http.StatusOK)
	if got := len(s.do(http.MethodGet, "ResourceTypes", nil).expect(t, http.StatusOK).resources()); got != 2 {
		t.Errorf("got %d resource types, want 2", got)
	}
	s.do(http.MethodGet, "ResourceTypes/Group", nil).expect(t, http.StatusOK)

	// The User resource type lists the enterprise extension as optional, and the Group resource type has none.
	userType := s.do(http.MethodGet, "ResourceTypes/User", nil).expect(t, http.StatusOK).body
	wantExtensions := []any{
		map[string]any{
			"schema":   enterpriseUserSchema,
			"required": false,
		},
	}
	if got := userType["schemaExtensions"]; !reflect.DeepEqual(got, wantExtensions) {
		t.Errorf("User schemaExtensions = %v, want %v", got, wantExtensions)
	}
	if got, ok := s.do(http.MethodGet, "ResourceTypes/Group", nil).expect(t, http.StatusOK).body["schemaExtensions"]; ok {
		t.Errorf("Group schemaExtensions = %v, want none", got)
	}

	// Deleting a user is an operation the RFC defines that is not supported, and other methods are not defined.
	s.do(http.MethodDelete, "Users/anything", nil).expect(t, http.StatusNotImplemented)
	resp := s.do(http.MethodPost, "Users/anything", map[string]any{}).expect(t, http.StatusMethodNotAllowed)
	if allow := resp.header.Get("Allow"); allow != "GET, PUT, PATCH" {
		t.Errorf("Allow = %q", allow)
	}
	s.do(http.MethodPost, "Bulk", map[string]any{}).expect(t, http.StatusNotImplemented)
	s.do(http.MethodGet, "Me", nil).expect(t, http.StatusNotImplemented)
	s.do(http.MethodGet, "Unknown", nil).expect(t, http.StatusNotFound)

	filtered := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`emails co "x"`), nil).expect(t, http.StatusBadRequest)
	if filtered.body["scimType"] != scimTypeInvalidFilter {
		t.Errorf("scimType = %v, want %s", filtered.body["scimType"], scimTypeInvalidFilter)
	}

	s.do(http.MethodPost, "Users", "not json").expect(t, http.StatusBadRequest)

	// A body with anything after its object is not valid JSON, and nothing of it is applied.
	for _, body := range []string{
		`{"userName":"one@example.com","externalId":"00u-one"}{"userName":"two@example.com","externalId":"00u-two"}`,
		`{"userName":"one@example.com","externalId":"00u-one"} garbage`,
	} {
		resp := s.do(http.MethodPost, "Users", body).expect(t, http.StatusBadRequest)
		if resp.body["scimType"] != scimTypeInvalidSyntax {
			t.Errorf("scimType = %v, want %s", resp.body["scimType"], scimTypeInvalidSyntax)
		}
	}
	if n := s.count(new(types.SCIMUserBinding), ""); n != 0 {
		t.Fatalf("a body with trailing data provisioned %d users", n)
	}
	s.do(http.MethodPost, "Users", `{"userName":"one@example.com","externalId":"00u-one"}`+"\n").expect(t, http.StatusCreated)
}

func TestUserBinding(t *testing.T) {
	s := newSCIMTest(t)

	existing := s.seedUser("00u-existing", "old@example.com", types2.RoleOwner)
	unprovisioned := s.seedUser("00u-unprovisioned", "unprovisioned@example.com", types2.RoleBasic)
	local := s.seedProviderUser(system.LocalAuthProvider, "new@example.com", "new@example.com", types2.RoleBasic)
	usersBefore := s.count(new(types.User), "")

	s.enable()

	// An existing user is bound by the native ID in externalId, and keeps its ID and role.
	body := scimUser("owner@example.com", "00u-existing")
	created := s.do(http.MethodPost, "Users", body).expect(t, http.StatusCreated)
	if created.header.Get("Location") != testServerURL+s.path("Users/"+created.id()) {
		t.Errorf("Location = %q", created.header.Get("Location"))
	}
	binding, err := s.gateway.SCIMUserBindingForUser(t.Context(), existing.ID)
	if err != nil || binding == nil || binding.ID != created.id() {
		t.Fatalf("existing user was not bound: %v %v", binding, err)
	}
	bound := s.user(existing.ID)
	if bound.Email != "owner@example.com" || bound.DisplayName != "Given Family" || bound.Role != types2.RoleOwner {
		t.Errorf("bound user = %+v", bound)
	}
	if created.body["password"] != nil {
		t.Error("the password was returned")
	}

	// A duplicate POST is a conflict, and the first user is still found by its userName.
	dup := s.do(http.MethodPost, "Users", body).expect(t, http.StatusConflict)
	if dup.body["scimType"] != scimTypeUniqueness {
		t.Errorf("scimType = %v", dup.body["scimType"])
	}
	s.do(http.MethodPost, "Users", scimUser("OWNER@example.com", "00u-other")).expect(t, http.StatusConflict)
	found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "Owner@Example.com"`), nil).expect(t, http.StatusOK)
	if got := found.resources(); len(got) != 1 || got[0]["id"] != created.id() {
		t.Fatalf("userName lookup = %v", got)
	}

	// Email similarity never binds: the local user with the same email stays unbound, and a new user is created.
	newUser := s.do(http.MethodPost, "Users", scimUser("new@example.com", "00u-new")).expect(t, http.StatusCreated)
	if binding, err := s.gateway.SCIMUserBindingForUser(t.Context(), local.ID); err != nil || binding != nil {
		t.Fatalf("the local user was bound: %v %v", binding, err)
	}
	if got := s.count(new(types.User), ""); got != usersBefore+1 {
		t.Fatalf("got %d users, want %d", got, usersBefore+1)
	}
	newBinding := s.bindingByID(newUser.id())
	createdUser := s.user(newBinding.UserID)
	if createdUser.Username != "00u-new" || createdUser.Email != "new@example.com" || createdUser.Role != types2.RoleBasic {
		t.Errorf("created user = %+v", createdUser)
	}
	if n := s.count(new(types.Identity), "auth_provider_name = ? AND hashed_provider_user_id = ? AND user_id = ?", testOktaProviderName, hash.String("00u-new"), createdUser.ID); n != 1 {
		t.Errorf("got %d identities for the created user, want 1", n)
	}
	if events := s.events(createdUser.ID); len(events) != 1 || events[0].Type != types.UserLifecycleEventReconcile {
		t.Errorf("created user events = %+v", events)
	}

	// A user that SCIM never provisioned is invisible.
	if got := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "unprovisioned@example.com"`), nil).expect(t, http.StatusOK).resources(); len(got) != 0 {
		t.Fatalf("unprovisioned user listed: %v", got)
	}
	if total := s.do(http.MethodGet, "Users", nil).expect(t, http.StatusOK).body["totalResults"]; total != float64(2) {
		t.Fatalf("totalResults = %v, want 2", total)
	}

	// A user disabled for never having been provisioned, when the bound Owner enforced SCIM, is re-enabled when it is.
	s.enforce(existing.ID)
	if u := s.user(unprovisioned.ID); u.DisabledReason != types.UserDisabledReasonSCIMUnprovisioned {
		t.Fatalf("unprovisioned user after enforcing = %+v, want disabled as unprovisioned", u)
	}
	s.do(http.MethodPost, "Users", scimUser("unprovisioned@example.com", "00u-unprovisioned")).expect(t, http.StatusCreated)
	if u := s.user(unprovisioned.ID); u.DisabledAt != nil {
		t.Errorf("unprovisioned user is still disabled: %+v", u)
	}

	// Reserved and malformed native IDs are refused.
	s.do(http.MethodPost, "Users", scimUser("reserved@example.com", system.BootstrapName)).expect(t, http.StatusBadRequest)
	s.do(http.MethodPost, "Users", scimUser("spaces@example.com", "00u with spaces")).expect(t, http.StatusBadRequest)

	// A create without the native ID is refused.
	missing := scimUser("missing@example.com", "")
	delete(missing, "externalId")
	if resp := s.do(http.MethodPost, "Users", missing).expect(t, http.StatusBadRequest); resp.body["scimType"] != scimTypeInvalidValue {
		t.Errorf("scimType = %v", resp.body["scimType"])
	}
}

func TestUserWireSemantics(t *testing.T) {
	s := newSCIMTest(t)
	existing := s.seedUser("00u-user", "user@example.com", types2.RoleBasic)
	s.seedGroup("okta/00g-team", "team", existing.ID)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated)
	group := s.do(http.MethodPost, "Groups", scimGroup("team", user.id())).expect(t, http.StatusCreated)

	// The user's groups are read-only and reported on the user.
	got := s.do(http.MethodGet, "Users/"+user.id(), nil).expect(t, http.StatusOK)
	groups, _ := got.body["groups"].([]any)
	if len(groups) != 1 || groups[0].(map[string]any)["value"] != group.id() {
		t.Fatalf("groups = %v", got.body["groups"])
	}

	// A profile PUT that echoes id, meta, and groups (including a stale group) leaves memberships alone.
	put := got.body
	put["name"] = map[string]any{
		"givenName":  "Given",
		"familyName": "Updated",
	}
	put["groups"] = []any{}
	s.do(http.MethodPut, "Users/"+user.id(), put).expect(t, http.StatusOK)
	if members := s.memberships("okta/00g-team"); !slices.Equal(members, []uint{existing.ID}) {
		t.Fatalf("memberships = %v", members)
	}

	// Repeated identical PUTs have one effect.
	before := s.bindingByID(user.id()).Revision
	eventsBefore := len(s.events(existing.ID))
	for range 3 {
		s.do(http.MethodPut, "Users/"+user.id(), put).expect(t, http.StatusOK)
	}
	if after := s.bindingByID(user.id()).Revision; after != before {
		t.Fatalf("revision changed from %d to %d on identical PUTs", before, after)
	}
	if got := len(s.events(existing.ID)); got != eventsBefore {
		t.Fatalf("identical PUTs recorded %d events", got-eventsBefore)
	}

	tests := []struct {
		name       string
		method     string
		body       func() map[string]any
		wantActive bool
	}{
		{
			name:   "deactivate over PUT",
			method: http.MethodPut,
			body: func() map[string]any {
				b := scimUser("user@example.com", "00u-user")
				b["active"] = false
				return b
			},
			wantActive: false,
		},
		{
			name:   "PUT without active keeps the state",
			method: http.MethodPut,
			body: func() map[string]any {
				b := scimUser("user@example.com", "00u-user")
				delete(b, "active")
				return b
			},
			wantActive: false,
		},
		{
			name:   "reactivate over PUT",
			method: http.MethodPut,
			body: func() map[string]any {
				return scimUser("user@example.com", "00u-user")
			},
			wantActive: true,
		},
		{
			name:   "deactivate over pathless PATCH",
			method: http.MethodPatch,
			body: func() map[string]any {
				return patchOp(map[string]any{
					"op": "replace",
					"value": map[string]any{
						"active": false,
					},
				})
			},
			wantActive: false,
		},
		{
			name:   "reactivate over PATCH with a path",
			method: http.MethodPatch,
			body: func() map[string]any {
				return patchOp(map[string]any{
					"op":    "replace",
					"path":  "active",
					"value": true,
				})
			},
			wantActive: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.do(tt.method, "Users/"+user.id(), tt.body()).expect(t, http.StatusOK)
			if resp.body["active"] != tt.wantActive {
				t.Fatalf("active = %v, want %v", resp.body["active"], tt.wantActive)
			}

			u := s.user(existing.ID)
			if (u.DisabledAt == nil) != tt.wantActive {
				t.Fatalf("user disabled at %v, want active %v", u.DisabledAt, tt.wantActive)
			}
			if !tt.wantActive && u.DisabledReason != types.UserDisabledReasonSCIMInactive {
				t.Fatalf("disabled reason = %q", u.DisabledReason)
			}
			// Deactivation never removes memberships.
			if members := s.memberships("okta/00g-team"); !slices.Equal(members, []uint{existing.ID}) {
				t.Fatalf("memberships = %v", members)
			}
		})
	}

	// userName changes independently of email, and lookups are case-insensitive.
	rename := scimUser("renamed@example.com", "00u-user")
	s.do(http.MethodPut, "Users/"+user.id(), rename).expect(t, http.StatusOK)
	if got := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "RENAMED@example.com"`), nil).expect(t, http.StatusOK).resources(); len(got) != 1 {
		t.Fatalf("renamed user not found: %v", got)
	}

	// The native ID cannot change, and there is no upsert.
	changed := scimUser("renamed@example.com", "00u-someone-else")
	if resp := s.do(http.MethodPut, "Users/"+user.id(), changed).expect(t, http.StatusBadRequest); resp.body["scimType"] != scimTypeMutability {
		t.Errorf("scimType = %v", resp.body["scimType"])
	}
	s.do(http.MethodPut, "Users/00000000-0000-0000-0000-000000000000", rename).expect(t, http.StatusNotFound)

	// Okta follows RFC 7644, so a replace whose filter matches nothing fails instead of creating the value.
	if resp := s.do(http.MethodPatch, "Users/"+user.id(), patchOp(map[string]any{
		"op":    "replace",
		"path":  `emails[type eq "home"].value`,
		"value": "home@example.com",
	})).expect(t, http.StatusBadRequest); resp.body["scimType"] != scimTypeNoTarget {
		t.Errorf("scimType = %v, want %s", resp.body["scimType"], scimTypeNoTarget)
	}

	// PATCH rejects read-only targets and validates the result before committing anything. Restating the user's
	// groups unchanged is not a change.
	current := s.do(http.MethodGet, "Users/"+user.id(), nil).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Users/"+user.id(), patchOp(map[string]any{
		"op": "replace",
		"value": map[string]any{
			"groups":   current.body["groups"],
			"nickName": "Nick",
		},
	})).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Users/"+user.id(), patchOp(map[string]any{
		"op":    "replace",
		"path":  "groups",
		"value": []any{},
	})).expect(t, http.StatusBadRequest)
	// Adding a group the user is already in changes nothing, but adding one they are not in is refused.
	s.do(http.MethodPatch, "Users/"+user.id(), patchOp(map[string]any{
		"op":   "add",
		"path": "groups",
		"value": []any{
			map[string]any{
				"value": group.id(),
			},
		},
	})).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Users/"+user.id(), patchOp(map[string]any{
		"op":   "add",
		"path": "groups",
		"value": []any{
			map[string]any{
				"value": "00000000-0000-0000-0000-000000000000",
			},
		},
	})).expect(t, http.StatusBadRequest)
	if resp := s.do(http.MethodPatch, "Users/"+user.id(), patchOp(map[string]any{
		"op":   "remove",
		"path": "externalId",
	})).expect(t, http.StatusBadRequest); resp.body["scimType"] != scimTypeMutability {
		t.Errorf("removing externalId: scimType = %v, want %s", resp.body["scimType"], scimTypeMutability)
	}
	s.do(http.MethodPatch, "Users/"+user.id(), patchOp(
		map[string]any{
			"op":    "replace",
			"path":  "displayName",
			"value": "Should Not Stick",
		},
		map[string]any{
			"op":   "remove",
			"path": "userName",
		},
	)).expect(t, http.StatusBadRequest)
	if name := s.do(http.MethodGet, "Users/"+user.id(), nil).expect(t, http.StatusOK).body["displayName"]; name != "Given Family" {
		t.Fatalf("a failed PATCH was partly applied: displayName = %v", name)
	}
}

func TestUserPagination(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	const total = 250
	for i := range total {
		s.do(http.MethodPost, "Users", scimUser(fmt.Sprintf("user%03d@example.com", i), fmt.Sprintf("00u-%03d", i))).expect(t, http.StatusCreated)
	}

	var seen []string
	for _, page := range []struct {
		startIndex int
		want       int
	}{
		{
			startIndex: 1,
			want:       100,
		},
		{
			startIndex: 101,
			want:       100,
		},
		{
			startIndex: 201,
			want:       50,
		},
		{
			startIndex: 251,
			want:       0,
		},
	} {
		resp := s.do(http.MethodGet, fmt.Sprintf("Users?startIndex=%d&count=100", page.startIndex), nil).expect(t, http.StatusOK)
		if resp.body["totalResults"] != float64(total) || resp.body["startIndex"] != float64(page.startIndex) {
			t.Fatalf("page %d: %v", page.startIndex, resp.body)
		}
		resources := resp.resources()
		if len(resources) != page.want || resp.body["itemsPerPage"] != float64(page.want) {
			t.Fatalf("page %d has %d items, want %d", page.startIndex, len(resources), page.want)
		}
		for _, r := range resources {
			seen = append(seen, r["id"].(string))
		}
	}
	slices.Sort(seen)
	if len(slices.Compact(seen)) != total {
		t.Fatalf("pages returned %d distinct users, want %d", len(seen), total)
	}

	empty := s.do(http.MethodGet, "Users?count=0", nil).expect(t, http.StatusOK)
	if empty.body["totalResults"] != float64(total) {
		t.Fatalf("totalResults = %v", empty.body["totalResults"])
	}
	if resources, ok := empty.body["Resources"].([]any); !ok || len(resources) != 0 {
		t.Fatalf("Resources = %#v, want []", empty.body["Resources"])
	}

	projected := s.do(http.MethodGet, "Users?count=1&attributes=userName", nil).expect(t, http.StatusOK).resources()[0]
	if projected["userName"] == nil || projected["id"] == nil || projected["emails"] != nil {
		t.Fatalf("projected user = %v", projected)
	}
}

// bindingByID returns a user binding, which is still encrypted.
func (s *scimTest) bindingByID(id string) types.SCIMUserBinding {
	s.t.Helper()

	var binding types.SCIMUserBinding
	if err := s.gorm().Where("id = ?", id).Take(&binding).Error; err != nil {
		s.t.Fatalf("failed to get binding %s: %v", id, err)
	}
	return binding
}

// groupBindingByID returns a group binding.
func (s *scimTest) groupBindingByID(id string) types.SCIMGroupBinding {
	s.t.Helper()

	var binding types.SCIMGroupBinding
	if err := s.gorm().Where("id = ?", id).Take(&binding).Error; err != nil {
		s.t.Fatalf("failed to get group binding %s: %v", id, err)
	}
	return binding
}

func TestEmptyFilterValuesMatchNothing(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated)
	s.do(http.MethodPost, "Groups", scimGroup("team", user.id())).expect(t, http.StatusCreated)

	for _, resource := range []string{
		"Users?filter=" + url.QueryEscape(`userName eq ""`),
		"Users?filter=" + url.QueryEscape(`id eq ""`),
		"Groups?filter=" + url.QueryEscape(`displayName eq ""`),
		"Groups?filter=" + url.QueryEscape(`id eq ""`),
		"Users?filter=" + url.QueryEscape(`externalId eq ""`),
		"Groups?filter=" + url.QueryEscape(`externalId eq ""`),
	} {
		resp := s.do(http.MethodGet, resource, nil).expect(t, http.StatusOK)
		if got := resp.resources(); len(got) != 0 || resp.body["totalResults"] != float64(0) {
			t.Errorf("GET %s = %v, want no resources", resource, resp.body)
		}
	}
}

func TestWriteResponsesAreProjected(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated).id()
	group := s.do(http.MethodPost, "Groups", scimGroup("team", user)).expect(t, http.StatusCreated).id()

	tests := []struct {
		name     string
		method   string
		resource string
		body     map[string]any
		status   int
		want     []string
	}{
		{
			name:     "create a user",
			method:   http.MethodPost,
			resource: "Users?attributes=userName",
			body:     scimUser("other@example.com", "00u-other"),
			status:   http.StatusCreated,
			want:     []string{"id", "schemas", "userName"},
		},
		{
			name:     "replace a user",
			method:   http.MethodPut,
			resource: "Users/" + user + "?attributes=active,name.givenName",
			body:     scimUser("user@example.com", "00u-user"),
			status:   http.StatusOK,
			want:     []string{"active", "id", "name", "schemas"},
		},
		{
			name:     "patch a user",
			method:   http.MethodPatch,
			resource: "Users/" + user + "?excludedAttributes=emails,groups,meta,name",
			body: patchOp(map[string]any{
				"op":    "replace",
				"path":  "nickName",
				"value": "Nick",
			}),
			status: http.StatusOK,
			want:   []string{"active", "displayName", "externalId", "id", "locale", "nickName", "schemas", "userName"},
		},
		{
			name:     "create a group",
			method:   http.MethodPost,
			resource: "Groups?excludedAttributes=members",
			body:     scimGroup("other team", user),
			status:   http.StatusCreated,
			want:     []string{"displayName", "id", "meta", "schemas"},
		},
		{
			name:     "replace a group",
			method:   http.MethodPut,
			resource: "Groups/" + group + "?attributes=displayName",
			body:     scimGroup("team", user),
			status:   http.StatusOK,
			want:     []string{"displayName", "id", "schemas"},
		},
		{
			name:     "patch a group with attributes",
			method:   http.MethodPatch,
			resource: "Groups/" + group + "?attributes=displayName,members",
			body: patchOp(map[string]any{
				"op":    "replace",
				"path":  "displayName",
				"value": "renamed team",
			}),
			status: http.StatusOK,
			want:   []string{"displayName", "id", "members", "schemas"},
		},
		{
			name:     "patch a group without attributes",
			method:   http.MethodPatch,
			resource: "Groups/" + group + "?excludedAttributes=members",
			body: patchOp(map[string]any{
				"op":    "replace",
				"path":  "displayName",
				"value": "team",
			}),
			status: http.StatusNoContent,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.do(tt.method, tt.resource, tt.body).expect(t, tt.status)
			if got := slices.Sorted(maps.Keys(resp.body)); !slices.Equal(got, tt.want) {
				t.Fatalf("attributes = %v, want %v", got, tt.want)
			}
		})
	}

	// An invalid projection is refused before anything is written, and so is one that both selects and excludes
	// attributes, which RFC 7644 section 3.9 makes mutually exclusive.
	s.do(http.MethodPost, "Users?attributes="+url.QueryEscape("not an attribute"), scimUser("third@example.com", "00u-third")).expect(t, http.StatusBadRequest)
	s.do(http.MethodPost, "Users?attributes=userName&excludedAttributes=emails", scimUser("third@example.com", "00u-third")).expect(t, http.StatusBadRequest)
	s.do(http.MethodGet, "Users?attributes=userName&excludedAttributes=emails", nil).expect(t, http.StatusBadRequest)
	if n := s.count(new(types.SCIMUserBinding), ""); n != 2 {
		t.Fatalf("got %d users, want 2", n)
	}
}

// TestUserNamesAreComparedAsPrepared checks that userNames that the PRECIS rules prepare alike are the same userName,
// as RFC 7644 section 5 requires.
func TestUserNamesAreComparedAsPrepared(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	created := s.do(http.MethodPost, "Users", scimUser("jos\u00e9@example.com", "00u-jose")).expect(t, http.StatusCreated)
	for i, userName := range []string{
		// Decomposed.
		"jose\u0301@example.com",
		"JOS\u00c9@example.com",
		// Full-width.
		"\uff4a\uff4f\uff53\u00e9@example.com",
	} {
		if resp := s.do(http.MethodPost, "Users", scimUser(userName, fmt.Sprintf("00u-%d", i))).expect(t, http.StatusConflict); resp.body["scimType"] != scimTypeUniqueness {
			t.Errorf("POST %q: scimType = %v, want %s", userName, resp.body["scimType"], scimTypeUniqueness)
		}
		found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "`+userName+`"`), nil).expect(t, http.StatusOK).resources()
		if len(found) != 1 || found[0]["id"] != created.id() {
			t.Errorf("userName eq %q found %v", userName, found)
		}
	}

	// A userName that the rules do not allow is refused, and matches nothing.
	const invalid = "jos\u00e9\u0007@example.com"
	if resp := s.do(http.MethodPost, "Users", scimUser(invalid, "00u-invalid")).expect(t, http.StatusBadRequest); resp.body["scimType"] != scimTypeInvalidValue {
		t.Errorf("scimType = %v, want %s", resp.body["scimType"], scimTypeInvalidValue)
	}
	if found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`userName eq "jos\u00e9\u0007@example.com"`), nil).expect(t, http.StatusOK).resources(); len(found) != 0 {
		t.Errorf("userName eq %q found %v", invalid, found)
	}
}

// TestSchemasDescribeWhatIsEnforced checks the characteristics of attributes that the server enforces beyond the RFC's
// own schemas.
func TestSchemasDescribeWhatIsEnforced(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	attribute := func(attributes any, name string) map[string]any {
		t.Helper()
		list, _ := attributes.([]any)
		for _, a := range list {
			if m, _ := a.(map[string]any); m["name"] == name {
				return m
			}
		}
		t.Fatalf("no attribute %q in %v", name, attributes)
		return nil
	}

	group := s.do(http.MethodGet, "Schemas/"+groupSchema, nil).expect(t, http.StatusOK).body
	if displayName := attribute(group["attributes"], "displayName"); displayName["uniqueness"] != uniquenessServer || displayName["required"] != true {
		t.Errorf("displayName = %v, want required and unique", displayName)
	}
	members := attribute(group["attributes"], "members")
	if value := attribute(members["subAttributes"], "value"); value["required"] != true {
		t.Errorf("members.value = %v, want required", value)
	}
	if ref := attribute(members["subAttributes"], "$ref"); ref["caseExact"] != true {
		t.Errorf("members.$ref = %v, want case exact, as every reference is", ref)
	}

	user := s.do(http.MethodGet, "Schemas/"+userSchema, nil).expect(t, http.StatusOK).body
	if profileURL := attribute(user["attributes"], "profileUrl"); profileURL["caseExact"] != true {
		t.Errorf("profileUrl = %v, want case exact, as every reference is", profileURL)
	}
	for _, a := range user["attributes"].([]any) {
		if a.(map[string]any)["name"] == "password" {
			t.Error("the ignored password attribute is advertised")
		}
	}
}

func TestAttributesNamingNoKnownAttributeReturnOnlyTheIDAndSchemas(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated).id()
	s.do(http.MethodPost, "Groups", scimGroup("team", user)).expect(t, http.StatusCreated)

	got := s.do(http.MethodGet, "Users/"+user+"?attributes=nosuch", nil).expect(t, http.StatusOK)
	if keys := slices.Sorted(maps.Keys(got.body)); !slices.Equal(keys, []string{"id", "schemas"}) {
		t.Errorf("GET /Users/{id}?attributes=nosuch returned %v", keys)
	}
	for _, group := range s.do(http.MethodGet, "Groups?attributes=nosuch", nil).expect(t, http.StatusOK).resources() {
		if keys := slices.Sorted(maps.Keys(group)); !slices.Equal(keys, []string{"id", "schemas"}) {
			t.Errorf("GET /Groups?attributes=nosuch returned %v", keys)
		}
	}
}

func TestUsersHaveAtMostOnePrimaryValue(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	twoPrimaries := []any{
		map[string]any{
			"value":   "work@example.com",
			"type":    "work",
			"primary": true,
		},
		map[string]any{
			"value":   "home@example.com",
			"type":    "home",
			"primary": true,
		},
	}
	body := scimUser("user@example.com", "00u-user")
	body["emails"] = twoPrimaries
	if resp := s.do(http.MethodPost, "Users", body).expect(t, http.StatusBadRequest); resp.body["scimType"] != scimTypeInvalidValue {
		t.Errorf("POST scimType = %v, want %s", resp.body["scimType"], scimTypeInvalidValue)
	}

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated).id()
	s.do(http.MethodPut, "Users/"+user, body).expect(t, http.StatusBadRequest)
	s.do(http.MethodPatch, "Users/"+user, patchOp(map[string]any{
		"op":    "replace",
		"path":  "emails",
		"value": twoPrimaries,
	})).expect(t, http.StatusBadRequest)
}

// TestUserAddressesAndEnterpriseExtension checks that addresses and the enterprise extension are stored and returned,
// so that an identity provider that compares its source with what it reads back, as Entra does, sees them unchanged.
func TestUserAddressesAndEnterpriseExtension(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	// The body has the shapes Entra sent in Phase 0: null sub-values, a false primary, and both schemas listed.
	body := scimUser("user@example.com", "00u-user")
	body["schemas"] = []string{userSchema, enterpriseUserSchema}
	body["addresses"] = []any{
		map[string]any{
			"country":       "US",
			"formatted":     "Building 1",
			"locality":      "Redmond",
			"postalCode":    nil,
			"primary":       false,
			"region":        "WA",
			"streetAddress": nil,
			"type":          "work",
		},
	}
	body[enterpriseUserSchema] = map[string]any{
		"department":     "Engineering",
		"employeeNumber": "42",
		"costCenter":     nil,
		"manager": map[string]any{
			"value":       "manager-1",
			"displayName": "The Boss",
		},
	}

	wantAddresses := []any{
		map[string]any{
			"country":   "US",
			"formatted": "Building 1",
			"locality":  "Redmond",
			"region":    "WA",
			"type":      "work",
		},
	}
	wantEnterprise := map[string]any{
		"department":     "Engineering",
		"employeeNumber": "42",
		// The manager's displayName is read-only, so it is not stored.
		"manager": map[string]any{
			"value": "manager-1",
		},
	}
	wantSchemas := []any{userSchema, enterpriseUserSchema}
	check := func(what string, got map[string]any, wantAddresses []any, wantEnterprise map[string]any, wantSchemas []any) {
		t.Helper()
		if !reflect.DeepEqual(got["addresses"], wantAddresses) {
			t.Errorf("%s: addresses = %#v, want %#v", what, got["addresses"], wantAddresses)
		}
		if !reflect.DeepEqual(got[enterpriseUserSchema], wantEnterprise) {
			t.Errorf("%s: %s = %#v, want %#v", what, enterpriseUserSchema, got[enterpriseUserSchema], wantEnterprise)
		}
		if !reflect.DeepEqual(got["schemas"], wantSchemas) {
			t.Errorf("%s: schemas = %#v, want %#v", what, got["schemas"], wantSchemas)
		}
	}

	created := s.do(http.MethodPost, "Users", body).expect(t, http.StatusCreated)
	user := created.id()
	check("POST", created.body, wantAddresses, wantEnterprise, wantSchemas)
	check("GET", s.do(http.MethodGet, "Users/"+user, nil).expect(t, http.StatusOK).body, wantAddresses, wantEnterprise, wantSchemas)

	// The operations Entra sent in Phase 0 run 004 for a user whose read-back lacked them.
	run004 := patchOp(
		map[string]any{
			"op":    "Add",
			"path":  `addresses[type eq "work"].formatted`,
			"value": "Building 2",
		},
		map[string]any{
			"op":    "Add",
			"path":  `addresses[type eq "work"].locality`,
			"value": "Seattle",
		},
		map[string]any{
			"op":    "Add",
			"path":  "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber",
			"value": "43",
		},
		map[string]any{
			"op":    "Add",
			"path":  "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department",
			"value": "Sales",
		},
		map[string]any{
			"op":    "Replace",
			"path":  "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager",
			"value": "manager-2",
		},
	)
	wantAddresses = []any{
		map[string]any{
			"country":   "US",
			"formatted": "Building 2",
			"locality":  "Seattle",
			"region":    "WA",
			"type":      "work",
		},
	}
	wantEnterprise = map[string]any{
		"department":     "Sales",
		"employeeNumber": "43",
		"manager": map[string]any{
			"value": "manager-2",
		},
	}
	check("PATCH", s.do(http.MethodPatch, "Users/"+user, run004).expect(t, http.StatusOK).body, wantAddresses, wantEnterprise, wantSchemas)
	check("GET after PATCH", s.do(http.MethodGet, "Users/"+user, nil).expect(t, http.StatusOK).body, wantAddresses, wantEnterprise, wantSchemas)

	// Repeating the PATCH changes nothing.
	before := s.bindingByID(user).Revision
	s.do(http.MethodPatch, "Users/"+user, run004).expect(t, http.StatusOK)
	if after := s.bindingByID(user).Revision; after != before {
		t.Errorf("revision changed from %d to %d on a repeated PATCH", before, after)
	}

	// The extension's attributes and addresses can be projected.
	projected := s.do(http.MethodGet, "Users/"+user+"?attributes="+url.QueryEscape("urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department,addresses.locality"), nil).expect(t, http.StatusOK).body
	wantProjected := map[string]any{
		"id":      user,
		"schemas": wantSchemas,
		"addresses": []any{
			map[string]any{
				"locality": "Seattle",
			},
		},
		enterpriseUserSchema: map[string]any{
			"department": "Sales",
		},
	}
	if !reflect.DeepEqual(projected, wantProjected) {
		t.Errorf("projected = %#v, want %#v", projected, wantProjected)
	}
	projected = s.do(http.MethodGet, "Users/"+user+"?attributes="+url.QueryEscape(enterpriseUserSchema), nil).expect(t, http.StatusOK).body
	if !reflect.DeepEqual(projected[enterpriseUserSchema], wantEnterprise) {
		t.Errorf("projected %s = %#v, want %#v", enterpriseUserSchema, projected[enterpriseUserSchema], wantEnterprise)
	}
	excluded := s.do(http.MethodGet, "Users/"+user+"?excludedAttributes="+url.QueryEscape("urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager,addresses"), nil).expect(t, http.StatusOK).body
	wantExcluded := map[string]any{
		"department":     "Sales",
		"employeeNumber": "43",
	}
	if _, ok := excluded["addresses"]; ok || !reflect.DeepEqual(excluded[enterpriseUserSchema], wantExcluded) {
		t.Errorf("excluded = %#v, want no addresses and %s %#v", excluded, enterpriseUserSchema, wantExcluded)
	}
	excluded = s.do(http.MethodGet, "Users/"+user+"?excludedAttributes="+url.QueryEscape(enterpriseUserSchema), nil).expect(t, http.StatusOK).body
	if _, ok := excluded[enterpriseUserSchema]; ok {
		t.Errorf("excluded %s = %#v, want none", enterpriseUserSchema, excluded[enterpriseUserSchema])
	}

	// A null manager ID clears the stored manager.
	clearManager := patchOp(
		map[string]any{
			"op":    "Replace",
			"path":  "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:manager",
			"value": map[string]any{"value": nil},
		},
	)
	wantEnterprise = map[string]any{
		"department":     "Sales",
		"employeeNumber": "43",
	}
	check("PATCH with a null manager ID", s.do(http.MethodPatch, "Users/"+user, clearManager).expect(t, http.StatusOK).body, wantAddresses, wantEnterprise, wantSchemas)
	check("GET after clearing the manager", s.do(http.MethodGet, "Users/"+user, nil).expect(t, http.StatusOK).body, wantAddresses, wantEnterprise, wantSchemas)

	// A PUT without them removes them, and the extension is no longer listed in schemas.
	got := s.do(http.MethodPut, "Users/"+user, scimUser("user@example.com", "00u-user")).expect(t, http.StatusOK).body
	if _, ok := got["addresses"]; ok {
		t.Errorf("PUT: addresses = %#v, want none", got["addresses"])
	}
	if _, ok := got[enterpriseUserSchema]; ok {
		t.Errorf("PUT: %s = %#v, want none", enterpriseUserSchema, got[enterpriseUserSchema])
	}
	if want := []any{userSchema}; !reflect.DeepEqual(got["schemas"], want) {
		t.Errorf("PUT: schemas = %#v, want %#v", got["schemas"], want)
	}

	// A user can have at most one primary address.
	twoPrimaries := scimUser("other@example.com", "00u-other")
	twoPrimaries["addresses"] = []any{
		map[string]any{
			"type":    "work",
			"primary": true,
		},
		map[string]any{
			"type":    "home",
			"primary": true,
		},
	}
	s.do(http.MethodPost, "Users", twoPrimaries).expect(t, http.StatusBadRequest)

	// An extension value that is not an object is refused.
	notAnObject := scimUser("other@example.com", "00u-other")
	notAnObject[enterpriseUserSchema] = "Sales"
	s.do(http.MethodPost, "Users", notAnObject).expect(t, http.StatusBadRequest)
}

// TestProjectionsListOnlyTheExtensionsTheyReturn checks that schemas lists a schema extension only while the response
// holds its object, as RFC 7643 section 3 requires.
func TestProjectionsListOnlyTheExtensionsTheyReturn(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	body := scimUser("user@example.com", "00u-user")
	body[enterpriseUserSchema] = map[string]any{
		"department": "Engineering",
	}
	user := s.do(http.MethodPost, "Users", body).expect(t, http.StatusCreated).id()

	tests := []struct {
		name        string
		query       string
		wantSchemas []any
	}{
		{
			name:        "the whole resource",
			wantSchemas: []any{userSchema, enterpriseUserSchema},
		},
		{
			name:        "the extension excluded",
			query:       "excludedAttributes=" + url.QueryEscape(enterpriseUserSchema),
			wantSchemas: []any{userSchema},
		},
		{
			name:        "the extension's only attribute excluded",
			query:       "excludedAttributes=" + url.QueryEscape(enterpriseUserSchema+":department"),
			wantSchemas: []any{userSchema},
		},
		{
			name:        "only core attributes selected",
			query:       "attributes=userName",
			wantSchemas: []any{userSchema},
		},
		{
			name:        "an extension attribute selected",
			query:       "attributes=" + url.QueryEscape(enterpriseUserSchema+":department"),
			wantSchemas: []any{userSchema, enterpriseUserSchema},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.do(http.MethodGet, "Users/"+user+"?"+tt.query, nil).expect(t, http.StatusOK).body
			if !reflect.DeepEqual(got["schemas"], tt.wantSchemas) {
				t.Fatalf("schemas = %#v, want %#v", got["schemas"], tt.wantSchemas)
			}
		})
	}
}

// TestProjectionsReturnNothingEmpty checks that a projection leaves out the values that lack the sub-attributes it
// selects or that it excludes all of, rather than returning them empty.
func TestProjectionsReturnNothingEmpty(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	body := scimUser("user@example.com", "00u-user")
	body["addresses"] = []any{
		map[string]any{
			"type": "work",
		},
		map[string]any{
			"type":    "home",
			"country": "US",
		},
	}
	body[enterpriseUserSchema] = map[string]any{
		"manager": map[string]any{
			"$ref": "https://example.com/scim/Users/manager-1",
		},
	}
	user := s.do(http.MethodPost, "Users", body).expect(t, http.StatusCreated).id()

	tests := []struct {
		name  string
		query string
		want  map[string]any
	}{
		{
			name:  "a sub-attribute that no value has",
			query: "attributes=" + url.QueryEscape(enterpriseUserSchema+":manager.value,name.middleName"),
			want: map[string]any{
				"id":      user,
				"schemas": []any{userSchema},
			},
		},
		{
			name:  "sub-attributes of the same extension attribute that no value has",
			query: "attributes=" + url.QueryEscape(enterpriseUserSchema+":manager.value,"+enterpriseUserSchema+":manager.displayName"),
			want: map[string]any{
				"id":      user,
				"schemas": []any{userSchema},
			},
		},
		{
			name:  "a sub-attribute that only some values have",
			query: "attributes=addresses.country",
			want: map[string]any{
				"id":      user,
				"schemas": []any{userSchema},
				"addresses": []any{
					map[string]any{
						"country": "US",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.do(http.MethodGet, "Users/"+user+"?"+tt.query, nil).expect(t, http.StatusOK).body
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("projected = %#v, want %#v", got, tt.want)
			}
		})
	}

	// Excluding all the sub-attributes that some values have leaves them out too.
	got := s.do(http.MethodGet, "Users/"+user+"?excludedAttributes="+url.QueryEscape("addresses.type,"+enterpriseUserSchema+":manager.$ref"), nil).expect(t, http.StatusOK).body
	wantAddresses := []any{
		map[string]any{
			"country": "US",
		},
	}
	if !reflect.DeepEqual(got["addresses"], wantAddresses) {
		t.Errorf("addresses = %#v, want %#v", got["addresses"], wantAddresses)
	}
	if _, ok := got[enterpriseUserSchema]; ok {
		t.Errorf("%s = %#v, want none", enterpriseUserSchema, got[enterpriseUserSchema])
	}
	if want := []any{userSchema}; !reflect.DeepEqual(got["schemas"], want) {
		t.Errorf("schemas = %#v, want %#v", got["schemas"], want)
	}
}

func TestExternalIDFilters(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("user@example.com", "00u-user")).expect(t, http.StatusCreated).id()
	s.do(http.MethodPost, "Users", scimUser("other@example.com", "00u-other")).expect(t, http.StatusCreated)

	tests := []struct {
		name    string
		filter  string
		wantIDs []string
	}{
		{
			name:    "matches the user's externalId",
			filter:  `externalId eq "00u-user"`,
			wantIDs: []string{user},
		},
		{
			name:   "matches only the externalId as it was sent, which is case exact",
			filter: `externalId eq " 00u-user "`,
		},
		{
			name:   "matches no other externalId",
			filter: `externalId eq "00u-nobody"`,
		},
		{
			name:   "matches nothing for an externalId the adapter refuses",
			filter: `externalId eq "not an id"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(tt.filter), nil).expect(t, http.StatusOK)
			var ids []string
			for _, r := range resp.resources() {
				ids = append(ids, r["id"].(string))
			}
			if !slices.Equal(ids, tt.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, tt.wantIDs)
			}
		})
	}

	// An inactive user is still found by its externalId.
	s.do(http.MethodPatch, "Users/"+user, patchOp(map[string]any{
		"op":    "replace",
		"path":  "active",
		"value": false,
	})).expect(t, http.StatusOK)
	if found := s.do(http.MethodGet, "Users?filter="+url.QueryEscape(`externalId eq "00u-user"`), nil).expect(t, http.StatusOK).resources(); len(found) != 1 {
		t.Fatalf("an inactive user's externalId found %v, want the user", found)
	}

	// Okta groups bind by name, and their externalId is kept and returned as it was sent, and matched exactly.
	group := scimGroup("team", user)
	group["externalId"] = "00g-team"
	created := s.do(http.MethodPost, "Groups", group).expect(t, http.StatusCreated)
	if created.body["externalId"] != "00g-team" {
		t.Fatalf("externalId = %v, want 00g-team", created.body["externalId"])
	}
	findGroups := func(externalID string) []map[string]any {
		t.Helper()
		return s.do(http.MethodGet, "Groups?filter="+url.QueryEscape(`externalId eq "`+externalID+`"`), nil).expect(t, http.StatusOK).resources()
	}
	if found := findGroups("00g-team"); len(found) != 1 || found[0]["id"] != created.id() {
		t.Fatalf("externalId eq found %v, want the group", found)
	}
	if found := findGroups("00G-TEAM"); len(found) != 0 {
		t.Fatalf("externalId eq in another case found %v, want none", found)
	}

	// It is readWrite, so a PATCH changes it, and a PUT that omits it keeps it.
	s.do(http.MethodPatch, "Groups/"+created.id(), patchOp(map[string]any{
		"op":    "replace",
		"path":  "externalId",
		"value": "00g-renamed",
	})).expect(t, http.StatusNoContent)
	s.do(http.MethodPut, "Groups/"+created.id(), scimGroup("team", user)).expect(t, http.StatusOK)
	if got := s.do(http.MethodGet, "Groups/"+created.id(), nil).expect(t, http.StatusOK).body["externalId"]; got != "00g-renamed" {
		t.Fatalf("externalId = %v, want 00g-renamed", got)
	}
	if found := findGroups("00g-team"); len(found) != 0 {
		t.Fatalf("the old externalId found %v, want none", found)
	}

	s.do(http.MethodPatch, "Groups/"+created.id(), patchOp(map[string]any{
		"op":   "remove",
		"path": "externalId",
	})).expect(t, http.StatusNoContent)
	if got, ok := s.do(http.MethodGet, "Groups/"+created.id(), nil).expect(t, http.StatusOK).body["externalId"]; ok {
		t.Fatalf("externalId = %v, want none", got)
	}
}

func TestBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		serverURL string
		want      string
	}{
		{
			name:      "a server URL without a trailing slash",
			serverURL: "https://obot.example.com",
			want:      "https://obot.example.com/scim/v2",
		},
		{
			name:      "a server URL with a path and trailing slashes",
			serverURL: "https://example.com/obot//",
			want:      "https://example.com/obot/scim/v2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BaseURL(tt.serverURL); got != tt.want {
				t.Fatalf("BaseURL(%q) = %q, want %q", tt.serverURL, got, tt.want)
			}
		})
	}
}

func TestPathsBelowTheBaseURL(t *testing.T) {
	tests := []struct {
		name         string
		path         string
		wantSCIM     bool
		wantSegments []string
	}{
		{
			name:         "a resource",
			path:         "/scim/v2/Users/abc",
			wantSCIM:     true,
			wantSegments: []string{"Users", "abc"},
		},
		{
			name:         "doubled and trailing slashes",
			path:         "/scim/v2//Users/",
			wantSCIM:     true,
			wantSegments: []string{"Users"},
		},
		{
			name:     "the base URL",
			path:     "/scim/v2",
			wantSCIM: true,
		},
		{
			name:     "the SCIM root",
			path:     "/scim/v2/",
			wantSCIM: true,
		},
		{
			name: "another path that begins the same",
			path: "/scim/v2x/Users",
		},
		{
			name: "an API path",
			path: "/api/me",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSCIMPath(tt.path); got != tt.wantSCIM {
				t.Fatalf("IsSCIMPath(%q) = %v, want %v", tt.path, got, tt.wantSCIM)
			}
			if got := splitPath(tt.path); !slices.Equal(got, tt.wantSegments) {
				t.Fatalf("splitPath(%q) = %q, want %q", tt.path, got, tt.wantSegments)
			}
		})
	}
}

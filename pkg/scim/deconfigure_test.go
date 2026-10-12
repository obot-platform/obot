package scim

import (
	"net/http"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
)

// TestDeconfigureDeletesSCIMData checks that deconfiguring the auth provider of a SCIM connection deletes the
// connection with its SCIM data and the provider's groups, keeps its users, with the disabled ones disabled, and that
// configuring the provider again starts SCIM over, binding the users that the identity provider pushes again to the
// accounts they had.
func TestDeconfigureDeletesSCIMData(t *testing.T) {
	s := newSCIMTest(t)
	existing := s.seedUser("00u-existing", "existing@example.com", types2.RoleBasic)
	s.seedGroup("okta/00g-team", "team", existing.ID)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("existing@example.com", "00u-existing")).expect(t, http.StatusCreated)
	group := s.do(http.MethodPost, "Groups", scimGroup("team", user.id())).expect(t, http.StatusCreated)
	s.do(http.MethodPost, "Groups", scimGroup("created", user.id())).expect(t, http.StatusCreated)
	deactivated := s.do(http.MethodPost, "Users", scimUser("gone@example.com", "00u-gone")).expect(t, http.StatusCreated)
	s.do(http.MethodPatch, "Users/"+deactivated.id(), patchOp(map[string]any{
		"op":    "replace",
		"path":  "active",
		"value": false,
	})).expect(t, http.StatusOK)
	deactivatedUserID := s.bindingByID(deactivated.id()).UserID
	if got := s.user(deactivatedUserID); got.DisabledAt == nil {
		t.Fatal("the deactivated user is not disabled")
	}
	identities := s.count(new(types.Identity), "")

	// Deconfiguring: the credential goes, then the connection with its data.
	s.env.name = "entra-auth-provider"
	deleted, err := s.gateway.DeleteAuthProviderSCIMConnection(t.Context(), gclient.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      testOktaProviderName,
	})
	if err != nil || deleted == nil || deleted.ConnectionID != s.conn.ID {
		t.Fatalf("DeleteAuthProviderSCIMConnection() = %+v, %v", deleted, err)
	}

	// The identity provider's requests fail from then on, and record nothing.
	s.do(http.MethodPatch, "Groups/"+group.id(), patchOp(map[string]any{
		"op":    "remove",
		"path":  `members[value eq "` + user.id() + `"]`,
		"value": nil,
	})).expect(t, http.StatusServiceUnavailable)

	for _, tt := range []struct {
		name  string
		model any
		query string
		args  []any
		want  int64
	}{
		{
			name:  "connections",
			model: new(types.SCIMConnection),
		},
		{
			name:  "user bindings",
			model: new(types.SCIMUserBinding),
		},
		{
			name:  "group bindings",
			model: new(types.SCIMGroupBinding),
		},
		{
			name:  "request failures",
			model: new(types.SCIMRequestFailure),
		},
		{
			name:  "groups",
			model: new(types.Group),
		},
		{
			name:  "memberships",
			model: new(types.GroupMemberships),
		},
		{
			name:  "live users",
			model: new(types.User),
			query: "id IN ? AND deleted_at IS NULL",
			args:  []any{[]uint{existing.ID, deactivatedUserID}},
			want:  2,
		},
		{
			name:  "disabled users, who stay disabled",
			model: new(types.User),
			query: "id = ? AND disabled_at IS NOT NULL",
			args:  []any{deactivatedUserID},
			want:  1,
		},
		{
			name:  "identities",
			model: new(types.Identity),
			want:  identities,
		},
	} {
		if got := s.count(tt.model, tt.query, tt.args...); got != tt.want {
			t.Errorf("%s after deconfiguring = %d, want %d", tt.name, got, tt.want)
		}
	}

	// Configuring the provider again starts over, with a new connection and token at the same base URL. The old token
	// is refused.
	old := s.conn
	oldToken := s.token
	s.env.name = testOktaProviderName
	conn, token, err := s.gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      testOktaProviderName,
		GroupIDPrefix:         "okta/",
		Issuer:                "https://example.okta.com",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
		IssueToken:            true,
		RequireNoGroupData:    true,
	})
	if err != nil {
		t.Fatalf("failed to set SCIM up again: %v", err)
	}
	if conn.ID == old.ID {
		t.Fatal("the new connection has the old connection's ID")
	}
	s.conn, s.token = conn, token
	s.request(http.MethodGet, s.path("Users"), oldToken, nil).expect(t, http.StatusUnauthorized)

	// The identity provider pushes its users again, who keep their accounts.
	again := s.do(http.MethodPost, "Users", scimUser("existing@example.com", "00u-existing")).expect(t, http.StatusCreated)
	if again.id() == user.id() {
		t.Fatal("the user was bound with the old SCIM ID")
	}
	if got := s.bindingByID(again.id()).UserID; got != existing.ID {
		t.Fatalf("the pushed user was bound to user %d, want %d", got, existing.ID)
	}
	s.do(http.MethodGet, "Users/"+user.id(), nil).expect(t, http.StatusNotFound)
	s.do(http.MethodPost, "Groups", scimGroup("team", again.id())).expect(t, http.StatusCreated)
}

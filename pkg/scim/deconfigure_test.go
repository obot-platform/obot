package scim

import (
	"errors"
	"net/http"
	"slices"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
)

// TestDeconfigureAndReconfigurePreservesSCIMData checks that switching away from the auth provider of a SCIM
// connection pauses SCIM without losing anything, and that switching back resumes it with the same bindings and
// groups.
func TestDeconfigureAndReconfigurePreservesSCIMData(t *testing.T) {
	s := newSCIMTest(t)
	existing := s.seedUser("00u-existing", "existing@example.com", types2.RoleBasic)
	s.seedGroup("okta/00g-team", "team", existing.ID)
	s.enable()

	user := s.do(http.MethodPost, "Users", scimUser("existing@example.com", "00u-existing")).expect(t, http.StatusCreated)
	group := s.do(http.MethodPost, "Groups", scimGroup("team", user.id())).expect(t, http.StatusCreated)
	created := s.do(http.MethodPost, "Groups", scimGroup("created", user.id())).expect(t, http.StatusCreated)

	var createdGroupID string
	if err := s.gorm().Model(new(types.SCIMGroupBinding)).Where("id = ?", created.id()).Pluck("group_id", &createdGroupID).Error; err != nil {
		t.Fatal(err)
	}

	// Deconfiguring: the auth provider no longer serves sign-ins, and its cleanup keeps the group data.
	s.env.name = "entra-auth-provider"
	if err := s.gateway.DeleteAuthProviderGroupData(t.Context(), system.DefaultNamespace, testOktaProviderName, "okta/"); !errors.Is(err, gclient.ErrSCIMManagedGroupData) {
		t.Fatalf("cleanup of the SCIM provider = %v, want ErrSCIMManagedGroupData", err)
	}

	// While deconfigured, authenticated requests answer 503, which Okta records as failed tasks, and only an
	// authenticated caller learns why.
	paused := s.do(http.MethodPatch, "Groups/"+group.id(), patchOp(map[string]any{
		"op":    "remove",
		"path":  `members[value eq "` + user.id() + `"]`,
		"value": nil,
	})).expect(t, http.StatusServiceUnavailable)
	if paused.header.Get("Retry-After") == "" {
		t.Fatal("503 without Retry-After")
	}
	s.request(http.MethodGet, s.path("Users"), "", nil).expect(t, http.StatusUnauthorized)
	if got := s.memberships("okta/00g-team"); !slices.Equal(got, []uint{existing.ID}) {
		t.Fatalf("a request while deconfigured changed memberships: %v", got)
	}

	// Reconfiguring resumes SCIM with the same bindings and groups, and the retried task applies.
	s.env.name = testOktaProviderName
	s.do(http.MethodGet, "Users/"+user.id(), nil).expect(t, http.StatusOK)
	s.do(http.MethodGet, "Groups/"+group.id(), nil).expect(t, http.StatusOK)
	s.do(http.MethodGet, "Groups/"+created.id(), nil).expect(t, http.StatusOK)
	s.do(http.MethodPatch, "Groups/"+group.id(), patchOp(map[string]any{
		"op":    "remove",
		"path":  `members[value eq "` + user.id() + `"]`,
		"value": nil,
	})).expect(t, http.StatusNoContent)
	if got := s.memberships("okta/00g-team"); len(got) != 0 {
		t.Fatalf("the retried task did not apply: %v", got)
	}
	if got := s.memberships(createdGroupID); !slices.Equal(got, []uint{existing.ID}) {
		t.Fatalf("created group memberships = %v", got)
	}
	if n := s.count(new(types.Group), "id IN ?", []string{"okta/00g-team", createdGroupID}); n != 2 {
		t.Fatalf("got %d of the groups after reconfiguring, want 2", n)
	}
}

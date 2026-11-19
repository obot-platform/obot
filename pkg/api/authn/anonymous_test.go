package authn

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/obot-platform/obot/pkg/api/authz"
)

// authz grants unauthenticated callers access to /mcp-connect (so the handler can start the
// login flow) based solely on membership in authz.UnauthenticatedGroup. If Anonymous stops
// setting that group, those callers get denied instead of prompted to log in. authz cannot
// import authn (cycle), so this contract is checked here.
func TestAnonymousIsInUnauthenticatedGroup(t *testing.T) {
	resp, ok, err := Anonymous{}.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/mcp-connect/ms1test", nil))
	if err != nil || !ok {
		t.Fatalf("AuthenticateRequest() = %v, %v, want true, nil", ok, err)
	}
	if !slices.Contains(resp.User.GetGroups(), authz.UnauthenticatedGroup) {
		t.Fatalf("anonymous groups = %v, want %q", resp.User.GetGroups(), authz.UnauthenticatedGroup)
	}
}

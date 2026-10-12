package scim

import (
	"net/http"
	"testing"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func TestRequestActivity(t *testing.T) {
	s := newSCIMTest(t)
	s.enable()

	// Requests that do not authenticate are never recorded, so anyone who knows the base URL cannot fill the
	// failure log.
	s.request(http.MethodGet, s.path("Users"), "", nil).expect(t, http.StatusUnauthorized)
	s.request(http.MethodGet, s.path("Users"), "obot_scim_wrong", nil).expect(t, http.StatusUnauthorized)
	conn, err := s.gateway.SCIMConnection(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if conn.LastRequestAt != nil || s.count(new(types.SCIMRequestFailure), "connection_id = ?", s.conn.ID) != 0 {
		t.Fatal("an unauthenticated request was recorded")
	}

	s.do(http.MethodGet, "Users", nil).expect(t, http.StatusOK)
	conn, err = s.gateway.SCIMConnection(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if conn.LastRequestAt == nil || conn.LastSuccessAt == nil {
		t.Fatalf("a successful request was not recorded: %+v", conn)
	}

	// A failed request is recorded with its SCIM error, and so is a request refused while the auth provider is
	// not configured.
	s.do(http.MethodPost, "Groups", scimGroup("")).expect(t, http.StatusBadRequest)
	s.env.name = ""
	s.do(http.MethodGet, "Users", nil).expect(t, http.StatusServiceUnavailable)

	failures, _, err := s.gateway.SCIMRequestFailurePage(t.Context(), s.conn.ID, gclient.SCIMPage{
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 2 {
		t.Fatalf("failures = %+v", failures)
	}
	unavailable, invalid := failures[0], failures[1]
	if unavailable.Status != http.StatusServiceUnavailable || unavailable.Method != http.MethodGet || unavailable.Resource != "Users" {
		t.Errorf("unavailable failure = %+v", unavailable)
	}
	if invalid.Status != http.StatusBadRequest || invalid.Method != http.MethodPost || invalid.Resource != "Groups" ||
		invalid.SCIMType != scimTypeInvalidValue || invalid.Detail == "" {
		t.Errorf("invalid failure = %+v", invalid)
	}
}

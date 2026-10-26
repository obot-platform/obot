package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/entitlements"
	"github.com/obot-platform/obot/pkg/proxy"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type testEntitlements types.Entitlements

func (e testEntitlements) Entitlements(context.Context) (types.Entitlements, error) {
	return types.Entitlements(e), nil
}

func TestEntitlementGateRestriction(t *testing.T) {
	member := &kuser.DefaultInfo{Groups: []string{types.GroupAuthenticated}}
	admin := &kuser.DefaultInfo{Groups: []string{types.GroupAuthenticated, types.GroupAdmin}}
	owner := &kuser.DefaultInfo{Groups: []string{types.GroupAuthenticated, types.GroupOwner}}
	adminKey := &kuser.DefaultInfo{
		Groups: []string{types.GroupAuthenticated, types.GroupMCP},
		Extra: map[string][]string{
			"obot_groups": {types.GroupAdmin, types.GroupBasic},
		},
	}
	memberKey := &kuser.DefaultInfo{
		Groups: []string{types.GroupAuthenticated, types.GroupMCP},
		Extra: map[string][]string{
			"obot_groups": {types.GroupBasic},
		},
	}
	device := &kuser.DefaultInfo{
		Groups: []string{types.GroupAuthenticated, types.GroupDeviceScans},
		Extra: map[string][]string{
			"device_id": {"device1"},
		},
	}

	for _, tt := range []struct {
		name         string
		restrictions bool
		restricted   bool
		method       string
		path         string
		pattern      string
		user         kuser.Info
		wantRefused  bool
	}{
		{
			name:         "a member is refused while restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodGet,
			path:         "/api/mcp-servers",
			pattern:      "GET /api/mcp-servers",
			user:         member,
			wantRefused:  true,
		},
		{
			name:         "gated paths are refused for members while restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodPost,
			path:         "/mcp-connect/vmcp1",
			pattern:      "/mcp-connect/{mcp_id}",
			user:         member,
			wantRefused:  true,
		},
		{
			name:         "admins aren't restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodGet,
			path:         "/api/mcp-servers",
			pattern:      "GET /api/mcp-servers",
			user:         admin,
		},
		{
			name:         "owners aren't restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodPost,
			path:         "/mcp-connect/vmcp1",
			pattern:      "/mcp-connect/{mcp_id}",
			user:         owner,
		},
		{
			name:         "an admin's scoped API key isn't restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodPost,
			path:         "/mcp-connect/vmcp1",
			pattern:      "/mcp-connect/{mcp_id}",
			user:         adminKey,
		},
		{
			name:         "a member's scoped API key is refused while restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodPost,
			path:         "/mcp-connect/vmcp1",
			pattern:      "/mcp-connect/{mcp_id}",
			user:         memberKey,
			wantRefused:  true,
		},
		{
			name:         "a member's profile is refused while restricted, so the UI signs them out",
			restrictions: true,
			restricted:   true,
			method:       http.MethodGet,
			path:         "/api/me",
			pattern:      "GET /api/me",
			user:         member,
			wantRefused:  true,
		},
		{
			name:         "devices keep enforcing policy",
			restrictions: true,
			restricted:   true,
			method:       http.MethodPost,
			path:         "/api/enforcement/decisions",
			pattern:      "POST /api/enforcement/decisions",
			user:         member,
		},
		{
			name:         "devices keep uploading scans",
			restrictions: true,
			restricted:   true,
			method:       http.MethodPost,
			path:         "/api/devices/scans",
			pattern:      "POST /api/devices/scans",
			user:         device,
		},
		{
			name:         "a member's pages are refused while restricted",
			restrictions: true,
			restricted:   true,
			method:       http.MethodGet,
			path:         "/admin",
			pattern:      "/",
			user:         member,
			wantRefused:  true,
		},
		{
			name:         "static assets load",
			restrictions: true,
			restricted:   true,
			method:       http.MethodGet,
			path:         "/_app/immutable/entry/app.js",
			pattern:      "/",
			user:         member,
		},
		{
			name:         "members can sign out",
			restrictions: true,
			restricted:   true,
			method:       http.MethodGet,
			path:         "/oauth2/sign_out",
			pattern:      "/oauth2/",
			user:         member,
		},
		{
			name:         "a member is allowed while not restricted",
			restrictions: true,
			method:       http.MethodGet,
			path:         "/api/mcp-servers",
			pattern:      "GET /api/mcp-servers",
			user:         member,
		},
		{
			name:       "without billing nothing is restricted",
			restricted: true,
			method:     http.MethodGet,
			path:       "/api/mcp-servers",
			pattern:    "GET /api/mcp-servers",
			user:       member,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				storageClient: fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
				entitlements:  testEntitlements{Restricted: tt.restricted},
				restrictions:  tt.restrictions,
				gatedPaths:    gatedPaths(),
			}
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil)
			req.Pattern = tt.pattern

			err := s.entitlementGate(req, tt.user)
			var httpErr *types.ErrHTTP
			refused := errors.As(err, &httpErr) && httpErr.Code == http.StatusPaymentRequired
			if refused != tt.wantRefused {
				t.Fatalf("entitlementGate() error = %v, want refused %v", err, tt.wantRefused)
			}
			if !refused && err != nil {
				t.Fatalf("entitlementGate() error = %v", err)
			}
		})
	}
}

func TestSignOutRestricted(t *testing.T) {
	const pageAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"

	for _, tt := range []struct {
		name         string
		path         string
		accept       string
		wantAnswered bool
		wantLocation string
	}{
		{
			name:         "a page load goes to the login page, which says why",
			path:         "/mcp-servers",
			accept:       pageAccept,
			wantAnswered: true,
			wantLocation: restrictedLoginPath,
		},
		{
			name:   "the login page isn't sent there again",
			path:   restrictedLoginPath,
			accept: pageAccept,
		},
		{
			name: "an API call is refused as usual",
			path: "/api/mcp-servers",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.accept != "" {
				req.Header.Set("Accept", tt.accept)
			}
			rec := httptest.NewRecorder()

			if answered := signOutRestricted(rec, req); answered != tt.wantAnswered {
				t.Errorf("signOutRestricted() = %v, want %v", answered, tt.wantAnswered)
			}
			if location := rec.Header().Get("Location"); location != tt.wantLocation {
				t.Errorf("Location = %q, want %q", location, tt.wantLocation)
			}

			var signedOut, told bool
			for _, cookie := range rec.Result().Cookies() {
				if cookie.Name == proxy.ObotAccessTokenCookie && cookie.MaxAge < 0 {
					signedOut = true
				}
				if cookie.Name == restrictedCookie && cookie.Value == "true" && cookie.MaxAge > 0 {
					told = true
				}
			}
			if !signedOut || !told {
				t.Errorf("session cookie cleared = %v, restricted cookie set = %v, want both", signedOut, told)
			}
		})
	}
}

func TestAnswerRefusedMCPCall(t *testing.T) {
	const initialize = `{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}`
	for _, tt := range []struct {
		name         string
		method       string
		path         string
		body         string
		wantAnswered bool
	}{
		{
			name:         "a call is answered with the restriction",
			method:       http.MethodPost,
			path:         "/mcp-connect/vmcp1",
			body:         initialize,
			wantAnswered: true,
		},
		{
			name:   "a notification has nothing to answer",
			method: http.MethodPost,
			path:   "/mcp-connect/vmcp1",
			body:   `{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		},
		{
			name:   "a stream isn't a call",
			method: http.MethodGet,
			path:   "/mcp-connect/vmcp1",
		},
		{
			name:   "other paths aren't MCP",
			method: http.MethodPost,
			path:   "/api/vmcp-instances",
			body:   initialize,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rw := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, strings.NewReader(tt.body))

			if answered := answerRefusedMCPCall(rw, req, errRestricted); answered != tt.wantAnswered {
				t.Fatalf("answerRefusedMCPCall() = %v, want %v", answered, tt.wantAnswered)
			}
			if !tt.wantAnswered {
				return
			}
			var response struct {
				ID    int `json:"id"`
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rw.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.ID != 7 || response.Error.Message != entitlements.RestrictedMessage {
				t.Errorf("response = %s, want the restriction answering call 7", rw.Body.String())
			}
		})
	}
}

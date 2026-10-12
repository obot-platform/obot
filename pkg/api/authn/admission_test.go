package authn

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api/authz"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/user"
)

type stubAuthenticator struct {
	resp *authenticator.Response
	ok   bool
	err  error
}

func (s stubAuthenticator) AuthenticateRequest(*http.Request) (*authenticator.Response, bool, error) {
	return s.resp, s.ok, s.err
}

func principalWith(uid string, groups []string, extra map[string][]string) *authenticator.Response {
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   uid,
			UID:    uid,
			Groups: groups,
			Extra:  extra,
		},
	}
}

func TestAdmissionCheck(t *testing.T) {
	for _, tt := range []struct {
		name       string
		resp       *authenticator.Response
		wantAdmit  bool
		wantDenied types2.UserStatus
		wantLookup bool
	}{
		{
			name: "active user",
			resp: principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, map[string][]string{
				principal.UserStatusExtra: {string(types2.UserStatusActive)},
			}),
			wantAdmit: true,
		},
		{
			name: "disabled user",
			resp: principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, map[string][]string{
				principal.UserStatusExtra: {string(types2.UserStatusDisabled)},
			}),
			wantDenied: types2.UserStatusDisabled,
		},
		{
			name: "deleted or missing user",
			resp: principalWith("7", []string{types2.GroupAPI, types2.GroupAuthenticated}, map[string][]string{
				principal.UserStatusExtra: {string(types2.UserStatusDeleted)},
			}),
			wantDenied: types2.UserStatusDeleted,
		},
		{
			name: "hosted agent of an active owner",
			resp: principalWith("hosted-agent:hai1abc", []string{types2.GroupAuthenticated, types2.GroupLLM}, map[string][]string{
				principal.HostedAgentOwnerExtra: {"7"},
				principal.UserStatusExtra:       {string(types2.UserStatusActive)},
			}),
			wantAdmit: true,
		},
		{
			name: "hosted agent of a disabled owner",
			resp: principalWith("hosted-agent:hai1abc", []string{types2.GroupAuthenticated, types2.GroupLLM}, map[string][]string{
				principal.HostedAgentOwnerExtra: {"7"},
				principal.UserStatusExtra:       {string(types2.UserStatusDisabled)},
			}),
			wantDenied: types2.UserStatusDisabled,
		},
		{
			name:       "user principal whose authenticator reported no status",
			resp:       principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, map[string][]string{}),
			wantLookup: true,
		},
		{
			name:       "hosted agent whose authenticator reported no status",
			resp:       principalWith("hosted-agent:hai1abc", []string{types2.GroupAuthenticated}, map[string][]string{principal.HostedAgentOwnerExtra: {"7"}}),
			wantLookup: true,
		},
		{
			name:       "user API key with device scans but no device",
			resp:       principalWith("7", []string{types2.GroupDeviceScans, types2.GroupAuthenticated}, map[string][]string{}),
			wantLookup: true,
		},
		{
			name: "status recorded more than once",
			resp: principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, map[string][]string{
				principal.UserStatusExtra: {string(types2.UserStatusActive), string(types2.UserStatusDisabled)},
			}),
			wantLookup: true,
		},
		{
			name:      "anonymous",
			resp:      principalWith("anonymous", []string{authz.UnauthenticatedGroup}, nil),
			wantAdmit: true,
		},
		{
			name:      "metrics",
			resp:      principalWith("metrics", []string{authz.MetricsGroup}, nil),
			wantAdmit: true,
		},
		{
			name:      "tunnel",
			resp:      principalWith("12345", []string{types2.GroupTunnel}, nil),
			wantAdmit: true,
		},
		{
			name:      "tunnel bridge",
			resp:      principalWith("bridge", []string{types2.GroupTunnelBridge}, nil),
			wantAdmit: true,
		},
		{
			name:      "tunnel peer",
			resp:      principalWith("peer", []string{types2.GroupTunnelPeer}, nil),
			wantAdmit: true,
		},
		{
			name:      "device enrollment",
			resp:      principalWith("enroll", []string{types2.GroupDeviceEnroll, types2.GroupAuthenticated}, nil),
			wantAdmit: true,
		},
		{
			name: "device",
			resp: principalWith("device", []string{types2.GroupDeviceScans, types2.GroupAuthenticated}, map[string][]string{
				"device_id": {"device-1"},
			}),
			wantAdmit: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			check := NewAdmissionCheck(stubAuthenticator{
				resp: tt.resp,
				ok:   true,
			})

			resp, ok, err := check.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/api/me", nil))

			switch {
			case tt.wantAdmit:
				if err != nil || !ok || resp != tt.resp {
					t.Fatalf("got %v, %v, %v; want the principal admitted", resp, ok, err)
				}
			case tt.wantDenied != "":
				denied, isDenied := errors.AsType[*gclient.UserAccessDeniedError](err)
				if !isDenied || denied.Status != tt.wantDenied || ok || resp != nil {
					t.Fatalf("got %v, %v, %v; want the principal denied as %s", resp, ok, err, tt.wantDenied)
				}
				if denied.UserID != 7 {
					t.Errorf("denied user ID = %d, want 7", denied.UserID)
				}
			case tt.wantLookup:
				if _, isLookup := errors.AsType[*gclient.UserAccessLookupError](err); !isLookup || ok || resp != nil {
					t.Fatalf("got %v, %v, %v; want the principal refused as unverifiable", resp, ok, err)
				}
			}
		})
	}
}

func TestAdmissionCheckPassesThroughUnauthenticatedResults(t *testing.T) {
	failure := errors.New("authenticator failed")

	for _, tt := range []struct {
		name    string
		next    stubAuthenticator
		wantErr error
	}{
		{
			name: "declined",
			next: stubAuthenticator{},
		},
		{
			name: "failed",
			next: stubAuthenticator{
				err: failure,
			},
			wantErr: failure,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp, ok, err := NewAdmissionCheck(tt.next).AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/api/me", nil))
			if resp != nil || ok || !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("got %v, %v, %v; want nil, false, %v", resp, ok, err, tt.wantErr)
			}
		})
	}
}

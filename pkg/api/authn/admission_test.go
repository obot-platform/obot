package authn

import (
	"context"
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

type restriction bool

type failingRestriction struct{}

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
			name:       "user principal whose authenticator reported no status",
			resp:       principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, map[string][]string{}),
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
			}, nil)

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
			resp, ok, err := NewAdmissionCheck(tt.next, nil).AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/api/me", nil))
			if resp != nil || ok || !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("got %v, %v, %v; want nil, false, %v", resp, ok, err, tt.wantErr)
			}
		})
	}
}

func (r restriction) Restricted(context.Context) (bool, error) {
	return bool(r), nil
}

func TestAdmissionCheckRestriction(t *testing.T) {
	active := map[string][]string{
		principal.UserStatusExtra: {string(types2.UserStatusActive)},
	}
	member := principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, active)

	for _, tt := range []struct {
		name        string
		resp        *authenticator.Response
		restriction Restriction
		wantRefused bool
	}{
		{
			name:        "a member while restricted",
			resp:        member,
			restriction: restriction(true),
			wantRefused: true,
		},
		{
			name:        "a member while not restricted",
			resp:        member,
			restriction: restriction(false),
		},
		{
			name: "a member without billing",
			resp: member,
		},
		{
			name:        "an admin while restricted",
			resp:        principalWith("7", []string{types2.GroupAdmin, types2.GroupAuthenticated}, active),
			restriction: restriction(true),
		},
		{
			name: "an owner's scoped API key while restricted",
			resp: principalWith("7", []string{types2.GroupAPI, types2.GroupAuthenticated}, map[string][]string{
				principal.UserStatusExtra: {string(types2.UserStatusActive)},
				"obot_groups":             types2.RoleOwner.RoleGroups(),
			}),
			restriction: restriction(true),
		},
		{
			name: "a device while restricted",
			resp: principalWith("device", []string{types2.GroupDeviceScans, types2.GroupAuthenticated}, map[string][]string{
				"device_id": {"device-1"},
			}),
			restriction: restriction(true),
			wantRefused: true,
		},
		{
			name:        "a device enrolling while restricted",
			resp:        principalWith("enroll", []string{types2.GroupDeviceEnroll, types2.GroupAuthenticated}, nil),
			restriction: restriction(true),
			wantRefused: true,
		},
		{
			name:        "SCIM while restricted",
			resp:        principalWith("scim", []string{types2.GroupSCIM}, nil),
			restriction: restriction(true),
		},
		{
			name:        "anonymous while restricted",
			resp:        principalWith("anonymous", []string{authz.UnauthenticatedGroup}, nil),
			restriction: restriction(true),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			check := NewAdmissionCheck(stubAuthenticator{
				resp: tt.resp,
				ok:   true,
			}, tt.restriction)
			_, ok, err := check.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/api/me", nil))
			if tt.wantRefused {
				if ok || !errors.Is(err, ErrRestricted) {
					t.Fatalf("AuthenticateRequest() = %v, %v; want ErrRestricted", ok, err)
				}
			} else if !ok || err != nil {
				t.Fatalf("AuthenticateRequest() = %v, %v; want admitted", ok, err)
			}
		})
	}
}

func (failingRestriction) Restricted(context.Context) (bool, error) {
	return false, errors.New("storage unavailable")
}

func TestAdmissionCheckRefusesAsUnavailableWhenRestrictionCantBeRead(t *testing.T) {
	check := NewAdmissionCheck(stubAuthenticator{
		resp: principalWith("7", []string{types2.GroupBasic, types2.GroupAuthenticated}, map[string][]string{
			principal.UserStatusExtra: {string(types2.UserStatusActive)},
		}),
		ok: true,
	}, failingRestriction{})

	_, ok, err := check.AuthenticateRequest(httptest.NewRequest(http.MethodGet, "/api/me", nil))
	if lookup, isLookup := errors.AsType[*gclient.UserAccessLookupError](err); ok || !isLookup || lookup.UserID != 7 {
		t.Fatalf("AuthenticateRequest() = %v, %v; want a lookup error for user 7", ok, err)
	}
}

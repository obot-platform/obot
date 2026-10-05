package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestUserChangesThatSCIMManagesAreConflicts(t *testing.T) {
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gatewayClient := client.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	conn, _, err := gatewayClient.CreateSCIMConnection(t.Context(), client.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "okta-auth-provider",
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	})
	if err != nil {
		t.Fatal(err)
	}
	provisioned, err := gatewayClient.CreateSCIMUser(t.Context(), conn, client.SCIMUserInput{
		UserName:   "alice@example.com",
		ExternalID: "00u-alice",
	}, client.SCIMUserCreateOptions{
		UserLimit: client.UserLimit{
			Unlimited: true,
		},
		DefaultRole: types2.RoleBasic,
	})
	if err != nil {
		t.Fatal(err)
	}
	userID := fmt.Sprint(provisioned.UserID)

	request := func(method string, body any) api.Context {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(method, "/api/users/"+userID, bytes.NewReader(data))
		req.SetPathValue("user_id", userID)
		return api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        req,
			Storage:        clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
			GatewayClient:  gatewayClient,
			User: &user.DefaultInfo{
				Name:   "owner",
				UID:    "1000",
				Groups: types2.RoleOwner.Groups(),
			},
		}
	}

	// The identity provider manages the user, so changing what it manages conflicts with it, as deleting the user
	// while it still provisions them does.
	for _, tt := range []struct {
		name   string
		handle func(api.Context) error
		method string
		body   any
	}{
		{
			name:   "renaming the user",
			handle: (&Server{}).updateUser,
			method: http.MethodPatch,
			body: types.User{
				Username: "renamed",
			},
		},
		{
			name:   "deleting the user",
			handle: (&Server{}).deleteUser,
			method: http.MethodDelete,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var httpErr *types2.ErrHTTP
			if err := tt.handle(request(tt.method, tt.body)); !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
				t.Fatalf("%s = %v, want a conflict", tt.name, err)
			}
		})
	}
}

func TestEnableUser(t *testing.T) {
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gatewayClient := client.New(t.Context(), db, nil, nil, nil, []string{"00u-explicit@example.com"}, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gatewayClient.Close() })
	okta := client.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}
	conn, _, err := gatewayClient.CreateSCIMConnection(t.Context(), client.CreateSCIMConnectionOptions{
		AuthProviderNamespace: okta.Namespace,
		AuthProviderName:      okta.Name,
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The identity provider provisions users with various roles, deactivated.
	deactivate := func(nativeID string, role types2.Role) uint {
		t.Helper()
		provisioned, err := gatewayClient.CreateSCIMUser(t.Context(), conn, client.SCIMUserInput{
			UserName:   nativeID + "@example.com",
			ExternalID: nativeID,
			Active:     new(false),
			Profile: types.SCIMUserProfile{
				Emails: []types.SCIMMultiValue{
					{
						Value:   nativeID + "@example.com",
						Primary: true,
					},
				},
			},
		}, client.SCIMUserCreateOptions{
			UserLimit: client.UserLimit{
				Unlimited: true,
			},
			DefaultRole: role,
		})
		if err != nil {
			t.Fatal(err)
		}
		return provisioned.UserID
	}
	users := map[string]uint{
		"basic":         deactivate("00u-basic", types2.RoleBasic),
		"owner":         deactivate("00u-owner", types2.RoleOwner),
		"auditor":       deactivate("00u-auditor", types2.RoleBasic|types2.RoleAuditor),
		"impersonator":  deactivate("00u-impersonator", types2.RoleAdmin|types2.RoleUserImpersonation),
		"group owner":   deactivate("00u-group", types2.RoleBasic),
		"explicit":      deactivate("00u-explicit", types2.RoleBasic),
		"owner by role": deactivate("00u-owner2", types2.RoleOwner),
	}
	// A group of another provider, which outlives Okta's deconfiguration, grants the Owner role.
	if _, err := gatewayClient.CreateGroupRoleAssignment(t.Context(), "github/owners", types2.RoleOwner, ""); err != nil {
		t.Fatal(err)
	}
	if err := storageServices.DB.DB.Create(&types.GroupMemberships{
		UserID:  users["group owner"],
		GroupID: "github/owners",
	}).Error; err != nil {
		t.Fatal(err)
	}

	enable := func(userID string, role types2.Role) (*httptest.ResponseRecorder, error) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/users/"+userID+"/enable", nil)
		req.SetPathValue("user_id", userID)
		rec := httptest.NewRecorder()
		return rec, (&Server{}).enableUser(api.Context{
			ResponseWriter: rec,
			Request:        req,
			Storage:        clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
			GatewayClient:  gatewayClient,
			User: &user.DefaultInfo{
				Name:   "requester",
				UID:    "1000",
				Groups: role.Groups(),
			},
		})
	}
	status := func(err error) int {
		t.Helper()
		if err == nil {
			return http.StatusOK
		}
		var httpErr *types2.ErrHTTP
		if !errors.As(err, &httpErr) {
			t.Fatalf("enableUser() = %v, want an HTTP error", err)
		}
		return httpErr.Code
	}

	// While SCIM manages a user, the identity provider decides.
	if _, err := enable(fmt.Sprint(users["basic"]), types2.RoleAdmin); status(err) != http.StatusConflict {
		t.Fatalf("enableUser() of a user SCIM manages = %v, want a conflict", err)
	}

	// Once their auth provider is deconfigured, an administrator can enable them, but only an Owner can enable a user
	// with the Owner, auditor, or user impersonation role, however they get it.
	if _, err := gatewayClient.DeleteAuthProviderSCIMConnection(t.Context(), okta); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name       string
		user       string
		requester  types2.Role
		wantStatus int
	}{
		{
			name:       "an Admin enabling an Owner",
			user:       "owner",
			requester:  types2.RoleAdmin,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "an Admin enabling an auditor",
			user:       "auditor",
			requester:  types2.RoleAdmin,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "an Admin enabling a user with the user impersonation role",
			user:       "impersonator",
			requester:  types2.RoleAdmin,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "an Admin enabling a user whose group grants the Owner role",
			user:       "group owner",
			requester:  types2.RoleAdmin,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "an Admin enabling a user whose email is an explicit Owner",
			user:       "explicit",
			requester:  types2.RoleAdmin,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "an Owner enabling an Owner",
			user:       "owner by role",
			requester:  types2.RoleOwner,
			wantStatus: http.StatusOK,
		},
		{
			name:       "an Admin enabling a Basic user",
			user:       "basic",
			requester:  types2.RoleAdmin,
			wantStatus: http.StatusOK,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rec, err := enable(fmt.Sprint(users[tt.user]), tt.requester)
			if got := status(err); got != tt.wantStatus {
				t.Fatalf("enableUser() = %v, want status %d", err, tt.wantStatus)
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var enabled types2.User
			if err := json.Unmarshal(rec.Body.Bytes(), &enabled); err != nil {
				t.Fatal(err)
			}
			if enabled.Status != types2.UserStatusActive || enabled.DisabledReason != "" || enabled.ManagementSource == types2.UserManagementSourceSCIM {
				t.Fatalf("enabled user = %+v, want an active user that SCIM does not manage", enabled)
			}
		})
	}

	if _, err := enable("4242", types2.RoleAdmin); status(err) != http.StatusNotFound {
		t.Fatalf("enableUser() of an unknown user = %v, want not found", err)
	}
}

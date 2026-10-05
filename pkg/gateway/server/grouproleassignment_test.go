package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestCreateGroupRoleAssignmentRefusesAMissingSCIMGroup(t *testing.T) {
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
	if _, _, err := gatewayClient.CreateSCIMConnection(t.Context(), client.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "okta-auth-provider",
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	}); err != nil {
		t.Fatal(err)
	}

	body, err := json.Marshal(types2.GroupRoleAssignment{
		GroupName: "okta/00g-missing",
		Role:      types2.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = (&Server{}).createGroupRoleAssignment(api.Context{
		ResponseWriter: httptest.NewRecorder(),
		Request:        httptest.NewRequest(http.MethodPost, "/api/group-role-assignments", bytes.NewReader(body)),
		Storage:        clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Name:   "owner",
			UID:    "1",
			Groups: types2.RoleOwner.Groups(),
		},
	})
	if err == nil || !strings.Contains(err.Error(), "okta/00g-missing") || !strings.Contains(err.Error(), "push the group") {
		t.Fatalf("createGroupRoleAssignment() = %v, want a refusal of the missing group", err)
	}

	assignments, err := gatewayClient.ListGroupRoleAssignments(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(assignments) != 0 {
		t.Fatalf("a refused role assignment was saved: %+v", assignments)
	}
}

func TestCreateGroupRoleAssignmentRefusesADuplicate(t *testing.T) {
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

	create := func() error {
		t.Helper()
		body, err := json.Marshal(types2.GroupRoleAssignment{
			GroupName: "github/engineering",
			Role:      types2.RoleAdmin,
		})
		if err != nil {
			t.Fatal(err)
		}
		return (&Server{}).createGroupRoleAssignment(api.Context{
			ResponseWriter: httptest.NewRecorder(),
			Request:        httptest.NewRequest(http.MethodPost, "/api/group-role-assignments", bytes.NewReader(body)),
			Storage:        clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
			GatewayClient:  gatewayClient,
			User: &user.DefaultInfo{
				Name:   "owner",
				UID:    "1",
				Groups: types2.RoleOwner.Groups(),
			},
		})
	}

	if err := create(); err != nil {
		t.Fatalf("createGroupRoleAssignment() = %v", err)
	}
	// SQLite reports the duplicate by its message, which IsUniqueViolation recognizes.
	var httpErr *types2.ErrHTTP
	if err := create(); !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict || !strings.Contains(httpErr.Message, "already exists") {
		t.Fatalf("a duplicate createGroupRoleAssignment() = %v, want a conflict", err)
	}
}

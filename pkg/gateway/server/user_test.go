package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/server/dispatcher"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetCurrentUserContinuesWhenAuthProviderURLUnavailable(t *testing.T) {
	gatewayClient := newGatewayTestClient(t)
	emailVerified := true
	gatewayUser, err := gatewayClient.EnsureIdentityWithRole(t.Context(), &gwtypes.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "generic-oauth-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "studio-sub",
		ProviderIssuer:        "https://studio.example.com/api/auth",
		ProviderEmailVerified: &emailVerified,
	}, "", apitypes.RoleBasic)
	if err != nil {
		t.Fatal(err)
	}

	kclient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.AuthProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "generic-oauth-auth-provider",
			Namespace: system.DefaultNamespace,
		},
		Status: v1.AuthProviderStatus{
			MissingConfigurationParameters: []string{"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_CLIENT_ID"},
		},
	}).Build()
	s := &Server{
		dispatcher: dispatcher.New(nil, kclient, gatewayClient, nil, "", "", ""),
	}

	rec := httptest.NewRecorder()
	err = s.getCurrentUser(api.Context{
		ResponseWriter: rec,
		Request:        httptest.NewRequest(http.MethodGet, "/api/me", nil),
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Name: gatewayUser.Username,
			UID:  "1",
			Extra: map[string][]string{
				"auth_provider_name":      {"generic-oauth-auth-provider"},
				"auth_provider_namespace": {system.DefaultNamespace},
				"auth_provider_groups":    {},
			},
			Groups: apitypes.RoleBasic.Groups(),
		},
	})
	if err != nil {
		t.Fatalf("expected /api/me to ignore unavailable provider URL, got %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestGetCurrentUserIncludesRequestTimeAdminUplift(t *testing.T) {
	gatewayClient := newGatewayTestClient(t)
	gatewayUser, err := gatewayClient.EnsureIdentityWithRole(t.Context(), &gwtypes.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "generic-oauth-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "studio-sub",
		ProviderIssuer:        "https://studio.example.com/api/auth",
	}, "", apitypes.RoleBasic)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	err = (&Server{}).getCurrentUser(api.Context{
		ResponseWriter: rec,
		Request:        httptest.NewRequest(http.MethodGet, "/api/me", nil),
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			Name: gatewayUser.Username,
			UID:  fmt.Sprintf("%d", gatewayUser.ID),
			Extra: map[string][]string{
				"auth_provider_groups": {},
			},
			Groups: append(apitypes.RoleBasic.Groups(), apitypes.GroupAdmin),
		},
	})
	if err != nil {
		t.Fatalf("expected /api/me to preserve request-time admin uplift, got %v", err)
	}

	var body apitypes.User
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !contains(body.Groups, apitypes.GroupAdmin) {
		t.Fatalf("expected response groups to include request-time admin, got %#v", body.Groups)
	}
	if contains(body.Groups, apitypes.GroupOwner) {
		t.Fatalf("expected response groups not to include owner, got %#v", body.Groups)
	}
	if body.EffectiveRole.HasRole(apitypes.RoleOwner) {
		t.Fatalf("expected response effective role not to include owner, got %v", body.EffectiveRole)
	}
}

func newGatewayTestClient(t *testing.T) *gatewayclient.Client {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		t.Fatalf("failed to auto-migrate gateway db: %v", err)
	}

	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(&v1.UserDefaultRoleSetting{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.DefaultRoleSettingName,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.UserDefaultRoleSettingSpec{
			Role: apitypes.RoleBasic,
		},
	}).Build()
	return gatewayclient.New(context.Background(), db, storageClient, nil, nil, nil, time.Hour, 100, 1)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

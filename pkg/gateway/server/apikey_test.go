package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newAPIKeyServerTestClient(t *testing.T) (*gatewayclient.Client, context.Context) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		cancel()
		t.Fatalf("failed to create storage services: %v", err)
	}

	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		cancel()
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := db.AutoMigrate(); err != nil {
		cancel()
		t.Fatalf("failed to auto-migrate: %v", err)
	}

	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.DefaultNamespace,
				Name:      system.DefaultRoleSettingName,
			},
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: apitypes.RoleBasic,
			},
		}).
		Build()

	c := gatewayclient.New(ctx, db, storageClient, nil, nil, nil, time.Hour, 100, 0)
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
	})

	return c, ctx
}

func createAPIKeyTestUser(t *testing.T, c *gatewayclient.Client) *gatewaytypes.User {
	t.Helper()

	u, err := c.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		Email:                 "api-key-user@example.com",
		AuthProviderName:      "github-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "api-key-user",
		ProviderUserID:        "api-key-user-provider-id",
	}, "", apitypes.RoleBasic)
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	return u
}

func TestCreateAPIKeyAllowsAuditOnlyKey(t *testing.T) {
	gatewayClient, _ := newAPIKeyServerTestClient(t)
	u := createAPIKeyTestUser(t, gatewayClient)

	req := httptest.NewRequest(http.MethodPost, "/api/api-keys", bytes.NewBufferString(`{"name":"audit","canAppendAuditLogs":true}`))
	rec := httptest.NewRecorder()

	err := (&Server{}).createAPIKey(api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  gatewayClient,
		User: &user.DefaultInfo{
			UID: fmt.Sprint(u.ID),
		},
	})
	if err != nil {
		t.Fatalf("createAPIKey returned error: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var response gatewaytypes.APIKeyCreateResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.Key == "" {
		t.Fatal("expected plaintext key in create response")
	}
	if response.CanAccessSkills {
		t.Fatal("expected audit-only key not to grant skills access")
	}
	if !response.CanAppendAuditLogs {
		t.Fatal("expected audit append access to be enabled")
	}
	if len(response.MCPServerIDs) != 0 {
		t.Fatalf("expected audit-only key to have no MCP server scopes, got %v", response.MCPServerIDs)
	}
}

func TestAPIKeyAuthenticatorExposesAuditAppendExtra(t *testing.T) {
	gatewayClient, ctx := newAPIKeyServerTestClient(t)
	u := createAPIKeyTestUser(t, gatewayClient)

	created, err := gatewayClient.CreateAPIKey(ctx, u.ID, "audit", "audit append key", nil, nil, false, true)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	resp, ok, err := NewAPIKeyAuthenticator(gatewayClient).AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected API key authentication to succeed")
	}
	if got := resp.User.GetGroups(); len(got) != 1 || got[0] != apitypes.GroupAPIKey {
		t.Fatalf("expected only API key group, got %v", got)
	}
	if got := resp.User.GetExtra()[apitypes.APIKeySkillsAccessExtraKey]; len(got) != 1 || got[0] != "false" {
		t.Fatalf("expected skills access extra to be false, got %v", got)
	}
	if got := resp.User.GetExtra()[apitypes.APIKeyAuditLogsAppendExtraKey]; len(got) != 1 || got[0] != "true" {
		t.Fatalf("expected audit append extra to be true, got %v", got)
	}
}

func TestInspectAPIKeySelf(t *testing.T) {
	gatewayClient, ctx := newAPIKeyServerTestClient(t)
	u := createAPIKeyTestUser(t, gatewayClient)
	expiresAt := time.Now().UTC().Add(time.Hour)

	created, err := gatewayClient.CreateAPIKey(ctx, u.ID, "audit", "audit append key", &expiresAt, []string{"server-a"}, true, true)
	if err != nil {
		t.Fatalf("failed to create API key: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/api-keys-self", nil)
	req.Header.Set("Authorization", "Bearer "+created.Key)

	authResp, ok, err := NewAPIKeyAuthenticator(gatewayClient).AuthenticateRequest(req)
	if err != nil {
		t.Fatalf("AuthenticateRequest returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected API key authentication to succeed")
	}

	rec := httptest.NewRecorder()
	err = (&Server{}).inspectAPIKeySelf(api.Context{
		ResponseWriter: rec,
		Request:        req,
		GatewayClient:  gatewayClient,
		User:           authResp.User,
	})
	if err != nil {
		t.Fatalf("inspectAPIKeySelf returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var response gatewaytypes.APIKeySelfInspectionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if response.ID != created.ID {
		t.Fatalf("expected key ID %d, got %d", created.ID, response.ID)
	}
	if response.UserID != u.ID {
		t.Fatalf("expected user ID %d, got %d", u.ID, response.UserID)
	}
	if !response.CanAccessSkills {
		t.Fatal("expected skills access to be enabled")
	}
	if !response.CanAppendAuditLogs {
		t.Fatal("expected audit append access to be enabled")
	}
	if len(response.MCPServerIDs) != 1 || response.MCPServerIDs[0] != "server-a" {
		t.Fatalf("expected server scope [server-a], got %v", response.MCPServerIDs)
	}
	if response.ExpiresAt == nil {
		t.Fatal("expected expiration to be returned")
	}
	if response.Identity.Subject != fmt.Sprint(u.ID) {
		t.Fatalf("expected subject %d, got %q", u.ID, response.Identity.Subject)
	}
	if response.Identity.Username != u.Username {
		t.Fatalf("expected username %q, got %q", u.Username, response.Identity.Username)
	}
	if response.Identity.Email != u.Email {
		t.Fatalf("expected email %q, got %q", u.Email, response.Identity.Email)
	}
}

func TestInspectAPIKeySelfRejectsExpiredOrDeletedKeys(t *testing.T) {
	tests := []struct {
		name  string
		setup func(context.Context, *gatewayclient.Client, uint) (string, error)
	}{
		{
			name: "expired",
			setup: func(ctx context.Context, c *gatewayclient.Client, userID uint) (string, error) {
				expiresAt := time.Now().UTC().Add(-time.Hour)
				created, err := c.CreateAPIKey(ctx, userID, "expired", "expired key", &expiresAt, nil, false, true)
				if err != nil {
					return "", err
				}
				return created.Key, nil
			},
		},
		{
			name: "deleted",
			setup: func(ctx context.Context, c *gatewayclient.Client, userID uint) (string, error) {
				created, err := c.CreateAPIKey(ctx, userID, "deleted", "deleted key", nil, nil, false, true)
				if err != nil {
					return "", err
				}
				if err := c.DeleteAPIKey(ctx, userID, created.ID); err != nil {
					return "", err
				}
				return created.Key, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gatewayClient, ctx := newAPIKeyServerTestClient(t)
			u := createAPIKeyTestUser(t, gatewayClient)

			key, err := tt.setup(ctx, gatewayClient, u.ID)
			if err != nil {
				t.Fatalf("failed to set up key: %v", err)
			}

			req := httptest.NewRequest(http.MethodGet, "/api/api-keys-self", nil)
			req.Header.Set("Authorization", "Bearer "+key)

			err = (&Server{}).inspectAPIKeySelf(api.Context{
				ResponseWriter: httptest.NewRecorder(),
				Request:        req,
				GatewayClient:  gatewayClient,
				User: &user.DefaultInfo{
					UID:    fmt.Sprint(u.ID),
					Groups: []string{apitypes.GroupAPIKey},
				},
			})

			var errHTTP *apitypes.ErrHTTP
			if !errors.As(err, &errHTTP) {
				t.Fatalf("expected HTTP error, got %v", err)
			}
			if errHTTP.Code != http.StatusUnauthorized {
				t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, errHTTP.Code)
			}
		})
	}
}

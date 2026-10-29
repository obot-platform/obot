package setup

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	"github.com/obot-platform/obot/pkg/scim/adapter/adaptertest"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTestGateway returns a gateway client over a new in-memory database.
func newTestGateway(t *testing.T) *gclient.Client {
	t.Helper()

	gateway, _ := newTestGatewayWithDB(t, nil)
	return gateway
}

// newTestGatewayWithDB returns a gateway client over a new in-memory database, which writes controller objects to
// storage, and the database.
func newTestGatewayWithDB(t *testing.T, storage kclient.Client) (*gclient.Client, *gorm.DB) {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gateway := gclient.New(t.Context(), database, storage, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway, services.DB.DB
}

func TestSCIMFirstConnectionRefusedWhileACleanupIsPending(t *testing.T) {
	provider := &v1.AuthProvider{
		Name:      "okta-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "okta/",
			},
		},
	}

	for _, tc := range []struct {
		name    string
		cleanup *v1.AuthProviderCleanup
	}{
		{
			name: "cleanup of the auth provider",
			cleanup: &v1.AuthProviderCleanup{
				Name:      "auth-provider-cleanup-okta-auth-provider",
				Namespace: system.DefaultNamespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: provider.Name,
					GroupIDPrefix:    "okta/",
				},
			},
		},
		{
			name: "cleanup of another auth provider with the same group ID prefix",
			cleanup: &v1.AuthProviderCleanup{
				Name:      "auth-provider-cleanup-custom-okta",
				Namespace: system.DefaultNamespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: "custom-okta",
					GroupIDPrefix:    "okta/",
					Ready:            true,
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			gateway := newTestGateway(t)
			storage := fake.NewClientBuilder().
				WithScheme(storagescheme.Scheme).
				WithObjects(provider.DeepCopy(), tc.cleanup).
				Build()

			_, err := EnsureSCIMFirstConnection(ctx, storage, gateway, *provider, map[string]string{
				"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL": "https://example.okta.com",
			})
			var pending *CleanupPendingError
			if !errors.As(err, &pending) || pending.CleanupName != tc.cleanup.Name {
				t.Fatalf("EnsureSCIMFirstConnection() error = %v, want the pending cleanup %q", err, tc.cleanup.Name)
			}

			conns, err := gateway.SCIMConnections(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(conns) != 0 {
				t.Fatalf("a connection was created while a cleanup was pending: %+v", conns)
			}
		})
	}
}

func TestSCIMFirstConnectionWaitsForTheBackfillOfGroupLookupIDHashes(t *testing.T) {
	provider := registerLookupProvider(t)
	ctx := t.Context()
	gateway := newTestGateway(t)
	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(provider.DeepCopy()).
		Build()
	config := map[string]string{
		"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL": "https://example.okta.com",
	}

	_, err := EnsureSCIMFirstConnection(ctx, storage, gateway, *provider, config)
	var ineligible *IneligibleError
	if !errors.As(err, &ineligible) || !strings.Contains(ineligible.Message, "still preparing identities") {
		t.Fatalf("EnsureSCIMFirstConnection() error = %v, want the backfill to be awaited", err)
	}

	if err := gateway.BackfillIdentityGroupLookupIDHashes(ctx); err != nil {
		t.Fatal(err)
	}
	conn, err := EnsureSCIMFirstConnection(ctx, storage, gateway, *provider, config)
	if err != nil || conn == nil {
		t.Fatalf("EnsureSCIMFirstConnection() = %v, %v, want a connection", conn, err)
	}
}

func TestSCIMFirstConnectionWaitsForUsersWithoutAGroupLookupID(t *testing.T) {
	provider := registerLookupProvider(t)
	ctx := t.Context()
	gateway, db := newTestGatewayWithDB(t, nil)
	if err := gateway.BackfillIdentityGroupLookupIDHashes(ctx); err != nil {
		t.Fatal(err)
	}

	// A user who has not signed in since Obot started storing group lookup IDs, and is disabled.
	now := time.Now()
	user := &types.User{
		Username:       "pat",
		HashedUsername: hash.String("pat"),
		Email:          "pat@example.com",
		HashedEmail:    hash.String("pat@example.com"),
		DisabledAt:     &now,
		DisabledReason: types.UserDisabledReasonSCIMInactive,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&types.Identity{
		AuthProviderNamespace: provider.Namespace,
		AuthProviderName:      provider.Name,
		ProviderUserID:        "sub-pat",
		HashedProviderUserID:  hash.String("sub-pat"),
		UserID:                user.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}

	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(provider.DeepCopy()).
		Build()
	_, err := EnsureSCIMFirstConnection(ctx, storage, gateway, *provider, map[string]string{
		"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL": "https://example.okta.com",
	})
	var ineligible *IneligibleError
	if !errors.As(err, &ineligible) {
		t.Fatalf("EnsureSCIMFirstConnection() error = %v, want the users without a group lookup ID", err)
	}
	for _, want := range []string{"pat@example.com", "sign in to Obot in a browser", "Disabled users among them: 1"} {
		if !strings.Contains(ineligible.Message, want) {
			t.Errorf("message %q does not contain %q", ineligible.Message, want)
		}
	}
}

func TestMissingGroupLookupIDBlockersNameAtMostTwentyUsers(t *testing.T) {
	users := make([]gclient.SCIMUserWithoutGroupLookupID, 0, 22)
	for i := range 22 {
		users = append(users, gclient.SCIMUserWithoutGroupLookupID{
			UserID: uint(i + 1),
			Name:   fmt.Sprintf("user-%d@example.com", i+1),
		})
	}

	blockers := missingGroupLookupIDBlockers(t.Context(), "Entra", users)
	if len(blockers) != 1 {
		t.Fatalf("got %d blockers, want 1 without disabled users: %v", len(blockers), blockers)
	}
	for _, want := range []string{"these users of Entra", "(22)", "user-20@example.com, and 2 more"} {
		if !strings.Contains(blockers[0], want) {
			t.Errorf("blocker %q does not contain %q", blockers[0], want)
		}
	}
	if strings.Contains(blockers[0], "user-21@example.com") {
		t.Errorf("blocker %q names more than twenty users", blockers[0])
	}
}

// registerLookupProvider registers an adapter that matches users by group lookup ID for the auth provider it returns.
func registerLookupProvider(t *testing.T) *v1.AuthProvider {
	t.Helper()

	adapter.RegisterForTest(t, "lookup-auth-provider", adaptertest.Adapter{
		Users: adapter.UserKeyGroupLookupID,
	})
	return &v1.AuthProvider{
		Name:      "lookup-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "lookup/",
			},
		},
	}
}

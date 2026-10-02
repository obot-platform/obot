package setup

import (
	"errors"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
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
	gateway := gclient.New(t.Context(), database, storage, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
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

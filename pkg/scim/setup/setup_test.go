package setup

import (
	"errors"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newTestGateway returns a gateway client over a new in-memory database.
func newTestGateway(t *testing.T) *gclient.Client {
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
	gateway := gclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() { _ = gateway.Close() })
	return gateway
}

func TestCreateConnectionRecomputesTheAuthProviderStatus(t *testing.T) {
	ctx := t.Context()
	gateway := newTestGateway(t)

	// The provider's credential no longer holds the directory parameters, so it reads as unconfigured until a SCIM
	// connection relaxes them.
	provider := &v1.AuthProvider{
		Name:      "okta-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				CommonProviderMetadata: clienttypes.CommonProviderMetadata{
					RequiredConfigurationParameters: []clienttypes.ProviderConfigurationParameter{
						{
							Name: "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL",
						},
						{
							Name: "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
						},
						{
							Name: "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
						},
					},
				},
				GroupIDPrefix: "okta/",
			},
		},
		Status: v1.AuthProviderStatus{
			MissingConfigurationParameters: []string{
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID",
				"OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY",
			},
		},
	}
	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.AuthProvider{}).
		WithObjects(provider).
		Build()
	if err := gateway.UpsertCredential(ctx, types.Credential{
		Context: provider.Name,
		Name:    provider.Name,
		Secrets: map[string]string{
			"OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL": "https://example.okta.com/",
		},
	}); err != nil {
		t.Fatal(err)
	}
	licenseProvider, err := license.NewProvider(ctx, nil, license.Config{})
	if err != nil {
		t.Fatal(err)
	}

	conn, token, err := CreateConnection(ctx, storage, gateway, licenseProvider, Options{
		AuthProviderName: provider.Name,
		Origin:           types.SCIMConnectionOriginSCIMFirst,
	})
	if err != nil {
		t.Fatalf("failed to create the connection: %v", err)
	}
	if token != "" || conn.HasToken() {
		t.Fatalf("a connection created without a token has one: %+v", conn)
	}
	if conn.Issuer != "https://example.okta.com" || conn.GroupIDPrefix != "okta/" || conn.AdapterType != "okta" {
		t.Fatalf("connection = %+v", conn)
	}

	var got v1.AuthProvider
	if err := storage.Get(ctx, kclient.ObjectKeyFromObject(provider), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Status.Configured || len(got.Status.MissingConfigurationParameters) != 0 {
		t.Fatalf("status after creating the connection = %+v, want configured", got.Status)
	}

	// An auth provider without an adapter cannot have a connection.
	other := &v1.AuthProvider{
		Name:      "github-auth-provider",
		Namespace: system.DefaultNamespace,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: clienttypes.AuthProviderManifest{
				GroupIDPrefix: "github/",
			},
		},
	}
	if err := storage.Create(ctx, other); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CreateConnection(ctx, storage, gateway, licenseProvider, Options{
		AuthProviderName: other.Name,
		Origin:           types.SCIMConnectionOriginSCIMFirst,
	}); err == nil {
		t.Fatal("a connection was created for an auth provider without a SCIM adapter")
	}
}

func TestCreateConnectionRefusesWhileACleanupIsPending(t *testing.T) {
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
				WithStatusSubresource(&v1.AuthProvider{}).
				WithObjects(provider.DeepCopy(), tc.cleanup).
				Build()
			licenseProvider, err := license.NewProvider(ctx, nil, license.Config{})
			if err != nil {
				t.Fatal(err)
			}

			_, _, err = CreateConnection(ctx, storage, gateway, licenseProvider, Options{
				AuthProviderName: provider.Name,
				Origin:           types.SCIMConnectionOriginSCIMFirst,
				Issuer:           "https://example.okta.com",
			})
			var pending *CleanupPendingError
			if !errors.As(err, &pending) || pending.CleanupName != tc.cleanup.Name {
				t.Fatalf("CreateConnection() error = %v, want the pending cleanup %q", err, tc.cleanup.Name)
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

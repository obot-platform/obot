package dispatcher

import (
	"context"
	"slices"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	oktaProviderName  = "okta-auth-provider"
	oktaIssuer        = "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
	oktaServiceClient = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID"
	oktaServiceKey    = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY"
)

// The dispatcher's configured-provider check and the daemon start use the effective parameters: a provider without a
// SCIM connection synchronizes its directory and needs the directory parameters, and one with a connection does not.
func TestSCIMConnectionsRelaxTheDirectoryParameters(t *testing.T) {
	oidcOnly := map[string]string{
		oktaIssuer: "https://example.okta.com",
	}
	withDirectory := map[string]string{
		oktaIssuer:        "https://example.okta.com",
		oktaServiceClient: "client",
		oktaServiceKey:    "key",
	}

	tests := []struct {
		name           string
		credential     map[string]string
		context        string
		connect        bool
		wantConfigured bool
		wantMissing    []string
	}{
		{
			name:           "directory synchronization with the directory parameters",
			credential:     withDirectory,
			context:        oktaProviderName,
			wantConfigured: true,
		},
		{
			name:        "no connection and no directory parameters",
			credential:  oidcOnly,
			context:     oktaProviderName,
			wantMissing: []string{oktaServiceClient, oktaServiceKey},
		},
		{
			name:           "a SCIM connection without the directory parameters",
			credential:     oidcOnly,
			context:        oktaProviderName,
			connect:        true,
			wantConfigured: true,
		},
		{
			name:           "a SCIM connection whose credential still holds the directory parameters",
			credential:     withDirectory,
			context:        oktaProviderName,
			connect:        true,
			wantConfigured: true,
		},
		{
			name:        "a staged replacement without the directory parameters or a connection",
			credential:  oidcOnly,
			context:     system.ReplacementAuthProviderCredentialContext,
			wantMissing: []string{oktaServiceClient, oktaServiceKey},
		},
		{
			name:       "a staged replacement with its SCIM connection",
			credential: oidcOnly,
			context:    system.ReplacementAuthProviderCredentialContext,
			connect:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)

			services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
			if err != nil {
				t.Fatal(err)
			}
			db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(); err != nil {
				t.Fatal(err)
			}

			provider := &v1.AuthProvider{
				Namespace: system.DefaultNamespace,
				Name:      oktaProviderName,
				Spec: v1.AuthProviderSpec{
					AuthProviderManifest: apitypes.AuthProviderManifest{
						CommonProviderMetadata: apitypes.CommonProviderMetadata{
							Name: "Okta",
							RequiredConfigurationParameters: []apitypes.ProviderConfigurationParameter{
								{
									Name: oktaIssuer,
								},
								{
									Name: oktaServiceClient,
								},
								{
									Name: oktaServiceKey,
								},
							},
						},
						GroupIDPrefix: "okta/",
					},
				},
			}
			storageClient := fake.NewClientBuilder().
				WithScheme(storagescheme.Scheme).
				WithObjects(provider).
				WithIndex(&v1.AuthProvider{}, "status.configured", func(o kclient.Object) []string {
					return []string{o.(*v1.AuthProvider).Get("status.configured")}
				}).
				Build()
			gatewayClient := client.New(ctx, db, storageClient, nil, nil, nil, nil, time.Hour, 1, 90, 90, 90, true)
			t.Cleanup(func() { _ = gatewayClient.Close() })

			if err := gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
				Context: tt.context,
				Name:    oktaProviderName,
				Secrets: tt.credential,
			}); err != nil {
				t.Fatal(err)
			}
			if tt.connect {
				if _, _, err := gatewayClient.CreateSCIMConnection(ctx, client.CreateSCIMConnectionOptions{
					AuthProviderNamespace: system.DefaultNamespace,
					AuthProviderName:      oktaProviderName,
					GroupIDPrefix:         "okta/",
					Origin:                gatewaytypes.SCIMConnectionOriginSCIMFirst,
				}); err != nil {
					t.Fatal(err)
				}
			}

			d := &Dispatcher{client: storageClient, gatewayClient: gatewayClient}

			configured, err := d.GetConfiguredAuthProvider(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got := configured == oktaProviderName; got != tt.wantConfigured {
				t.Errorf("configured provider = %q, want configured %v", configured, tt.wantConfigured)
			}

			missing, err := d.missingDaemonParameters(ctx, *provider, tt.credential)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(missing, tt.wantMissing) {
				t.Errorf("daemon start misses %v, want %v", missing, tt.wantMissing)
			}
		})
	}
}

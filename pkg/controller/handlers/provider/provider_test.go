package provider

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	authproviderconfig "github.com/obot-platform/obot/pkg/authprovider"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	k8stypes "k8s.io/apimachinery/pkg/types"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func TestReadLocalProviderRegistryFromSubdirectories(t *testing.T) {
	dir := t.TempDir()
	modelProvidersDir := filepath.Join(dir, modelProvidersRegistryDir)
	authProvidersDir := filepath.Join(dir, authProvidersRegistryDir)
	if err := os.MkdirAll(modelProvidersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(authProvidersDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(modelProvidersDir, "openai-model-provider.yaml"), []byte(`name: OpenAI
command: bin/openai-model-provider
dialect: OpenAIResponses
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelProvidersDir, "ignored.json"), []byte(`{"name":"ignored"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(authProvidersDir, "github-auth-provider.yaml"), []byte(`name: GitHub
command: bin/github-auth-provider
groupIDPrefix: github/
`), 0o644); err != nil {
		t.Fatal(err)
	}

	objs, err := readRegistry(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 provider objects, got %d", len(objs))
	}

	var foundModel, foundAuth bool
	for _, obj := range objs {
		switch provider := obj.(type) {
		case *v1.ModelProvider:
			foundModel = true
			if provider.Name != "openai-model-provider" {
				t.Fatalf("expected model provider name openai-model-provider, got %q", provider.Name)
			}
			if provider.Spec.Name != "OpenAI" {
				t.Fatalf("expected model provider display name OpenAI, got %q", provider.Spec.Name)
			}
			if provider.Spec.Command != filepath.Join(dir, "bin/openai-model-provider") {
				t.Fatalf("expected model provider command %q, got %q", filepath.Join(dir, "bin/openai-model-provider"), provider.Spec.Command)
			}
			if provider.Spec.Dialect != "OpenAIResponses" {
				t.Fatalf("expected model provider dialect OpenAIResponses, got %q", provider.Spec.Dialect)
			}
		case *v1.AuthProvider:
			foundAuth = true
			if provider.Name != "github-auth-provider" {
				t.Fatalf("expected auth provider name github-auth-provider, got %q", provider.Name)
			}
			if provider.Spec.Name != "GitHub" {
				t.Fatalf("expected auth provider display name GitHub, got %q", provider.Spec.Name)
			}
			if provider.Spec.Command != filepath.Join(dir, "bin/github-auth-provider") {
				t.Fatalf("expected auth provider command %q, got %q", filepath.Join(dir, "bin/github-auth-provider"), provider.Spec.Command)
			}
			if provider.Spec.GroupIDPrefix != "github/" {
				t.Fatalf("expected auth provider group ID prefix github/, got %q", provider.Spec.GroupIDPrefix)
			}
		default:
			t.Fatalf("unexpected object type %T", obj)
		}
	}
	if !foundModel || !foundAuth {
		t.Fatalf("expected both model and auth providers, foundModel=%v foundAuth=%v", foundModel, foundAuth)
	}
}

func TestAppendProvidersSkipsInvalidGroupIDPrefix(t *testing.T) {
	providers := []providerFromFile[types.AuthProviderManifest]{
		{
			Name: "valid",
			Manifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Command: "bin/valid",
				},
				GroupIDPrefix: "valid/",
			},
		},
		{
			Name: "invalid",
			Manifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Command: "bin/invalid",
				},
				GroupIDPrefix: "invalid%/",
			},
		},
	}

	objects := appendProviders("/registry", providers, nil)
	if len(objects) != 1 {
		t.Fatalf("providers = %d, want 1", len(objects))
	}
	provider, ok := objects[0].(*v1.AuthProvider)
	if !ok {
		t.Fatalf("provider type = %T, want *v1.AuthProvider", objects[0])
	}
	if provider.Name != "valid" || provider.Spec.GroupIDPrefix != "valid/" {
		t.Fatalf("provider = %#v, want valid provider", provider)
	}
}

func TestValidateUniqueAuthProviderGroupIDPrefixes(t *testing.T) {
	tests := []struct {
		name      string
		objects   []kclient.Object
		wantError bool
	}{
		{
			name: "unique prefixes",
			objects: []kclient.Object{
				&v1.AuthProvider{
					Name: "entra",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "okta",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "okta/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "local",
				},
				&v1.ModelProvider{
					Name: "model",
				},
			},
		},
		{
			name: "duplicate prefixes",
			objects: []kclient.Object{
				&v1.AuthProvider{
					Name: "first",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "second",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
			},
			wantError: true,
		},
		{
			name: "case variants are duplicate prefixes",
			objects: []kclient.Object{
				&v1.AuthProvider{
					Name: "first",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "Entra/",
						},
					},
				},
				&v1.AuthProvider{
					Name: "second",
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: types.AuthProviderManifest{
							GroupIDPrefix: "entra/",
						},
					},
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateUniqueAuthProviderGroupIDPrefixes(tt.objects)
			if tt.wantError && err == nil {
				t.Fatal("expected an error")
			}
			if !tt.wantError && err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestModelDialectPrefersMetadataDialect(t *testing.T) {
	for _, tc := range []struct {
		name     string
		metadata map[string]string
		fallback string
		want     string
	}{
		{
			name:     "metadata dialect wins",
			metadata: map[string]string{"dialect": "AnthropicMessages"},
			fallback: "OpenAIResponses",
			want:     "AnthropicMessages",
		},
		{
			name:     "empty metadata dialect falls back",
			metadata: map[string]string{"dialect": ""},
			fallback: "OpenAIResponses",
			want:     "OpenAIResponses",
		},
		{
			name:     "missing metadata dialect falls back",
			metadata: map[string]string{"usage": "llm"},
			fallback: "OpenAIResponses",
			want:     "OpenAIResponses",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := modelDialect(tc.metadata, tc.fallback); got != tc.want {
				t.Fatalf("modelDialect() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSetAuthProviderConfiguredStatusAcknowledgesConfiguration(t *testing.T) {
	gatewayClient := newProviderTestGatewayClient(t)
	licenseProvider, err := license.NewProvider(t.Context(), nil, license.Config{})
	if err != nil {
		t.Fatal(err)
	}
	authProvider := &v1.AuthProvider{
		Name:       "entra-auth-provider",
		Namespace:  system.DefaultNamespace,
		UID:        k8stypes.UID("provider-uid"),
		Generation: 3,
		Annotations: map[string]string{
			v1.AuthProviderSyncAnnotation: "revision-one",
		},
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					Command: "provider-command",
					RequiredConfigurationParameters: []types.ProviderConfigurationParameter{
						{
							Name: "CLIENT_SECRET",
						},
					},
				},
			},
		},
	}
	credentialEnvironment := map[string]string{
		"CLIENT_SECRET": "client-secret",
	}
	if err := gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: authProvider.Name,
		Name:    authProvider.Name,
		Secrets: credentialEnvironment,
	}); err != nil {
		t.Fatal(err)
	}

	if err := SetAuthProviderConfiguredStatus(t.Context(), gatewayClient, licenseProvider, authProvider); err != nil {
		t.Fatal(err)
	}
	wantHash, err := authproviderconfig.DaemonConfigurationHash(*authProvider, credentialEnvironment)
	if err != nil {
		t.Fatal(err)
	}
	if !authProvider.Status.Configured {
		t.Fatal("auth provider was not configured")
	}
	if authProvider.Status.DaemonConfigurationHash != wantHash {
		t.Fatalf("configuration hash = %q, want %q", authProvider.Status.DaemonConfigurationHash, wantHash)
	}
	if authProvider.Status.ObservedSyncRevision != "revision-one" {
		t.Fatalf("observed sync revision = %q, want revision-one", authProvider.Status.ObservedSyncRevision)
	}
	if authProvider.Status.ObservedGeneration != authProvider.Generation {
		t.Fatalf("observed generation = %d, want %d", authProvider.Status.ObservedGeneration, authProvider.Generation)
	}

	authProvider.Annotations[v1.AuthProviderSyncAnnotation] = "revision-two"
	if err := SetAuthProviderConfiguredStatus(t.Context(), gatewayClient, licenseProvider, authProvider); err != nil {
		t.Fatal(err)
	}
	if authProvider.Status.DaemonConfigurationHash != wantHash {
		t.Fatalf("configuration hash changed to %q, want %q", authProvider.Status.DaemonConfigurationHash, wantHash)
	}
	if authProvider.Status.ObservedSyncRevision != "revision-two" {
		t.Fatalf("observed sync revision = %q, want revision-two", authProvider.Status.ObservedSyncRevision)
	}

	unchangedStatus := *authProvider.Status.DeepCopy()
	if err := SetAuthProviderConfiguredStatus(t.Context(), gatewayClient, licenseProvider, authProvider); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(authProvider.Status, unchangedStatus) {
		t.Fatalf("unchanged reconciliation modified status: got %#v, want %#v", authProvider.Status, unchangedStatus)
	}

	if _, err := gatewayClient.DeleteCredential(t.Context(), authProvider.Name, authProvider.Name); err != nil {
		t.Fatal(err)
	}
	if err := SetAuthProviderConfiguredStatus(t.Context(), gatewayClient, licenseProvider, authProvider); err != nil {
		t.Fatal(err)
	}
	if authProvider.Status.Configured {
		t.Fatal("auth provider remained configured after credential deletion")
	}
	if len(authProvider.Status.MissingConfigurationParameters) != 1 || authProvider.Status.MissingConfigurationParameters[0] != "CLIENT_SECRET" {
		t.Fatalf("missing configuration parameters = %#v, want CLIENT_SECRET", authProvider.Status.MissingConfigurationParameters)
	}
	if authProvider.Status.ObservedSyncRevision != "revision-two" {
		t.Fatalf("observed sync revision = %q, want revision-two", authProvider.Status.ObservedSyncRevision)
	}
}

func newProviderTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	gatewayClient := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		if err := gatewayClient.Close(); err != nil {
			t.Errorf("close gateway client: %v", err)
		}
	})
	return gatewayClient
}

package mcpwebhookvalidation

import (
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureSystemServerCopiesStaticConfigurationCredential(t *testing.T) {
	gatewayClient := newWebhookValidationTestGatewayClient(t)
	validation := &v1.MCPWebhookValidation{
		Name:      "validation-1",
		Namespace: "default",
		Spec: v1.MCPWebhookValidationSpec{
			Manifest: types.MCPWebhookValidationManifest{
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Enabled: new(true),
					Runtime: types.RuntimeContainerized,
					ContainerizedConfig: &types.ContainerizedRuntimeConfig{
						Image: "example/image:latest",
					},
					Env: []types.MCPEnv{{
						Key:             "TOKEN",
						Sensitive:       true,
						Required:        true,
						ValueConfigured: true,
					}},
				},
			},
		},
	}
	require.NoError(t, gatewayClient.UpsertCredential(t.Context(), gatewaytypes.Credential{
		Context: system.MCPWebhookValidationCredentialContext,
		Name:    validation.Name,
		Secrets: map[string]string{"secret": "webhook-secret"},
	}))
	require.NoError(t, mcp.StoreStaticCredentialSecrets(t.Context(), gatewayClient, system.MCPWebhookValidationCredentialContext, validation.Name, map[string]string{
		"TOKEN": "static-token",
	}))

	storageClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	err := (&Handler{gatewayClient: gatewayClient}).EnsureSystemServer(router.Request{
		Client:    storageClient,
		Ctx:       t.Context(),
		Object:    validation,
		Namespace: validation.Namespace,
		Name:      validation.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)
	require.True(t, validation.Status.Configured)

	derivedName := system.SystemMCPServerPrefix + validation.Name
	derivedCredential, err := gatewayClient.RevealCredential(t.Context(), []string{derivedName}, derivedName)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"WEBHOOK_SECRET": "webhook-secret"}, derivedCredential.Secrets)

	derivedStatic, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, derivedName, derivedName)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "static-token"}, derivedStatic)
}

func TestMigrateStaticConfiguration(t *testing.T) {
	gatewayClient := newWebhookValidationTestGatewayClient(t)
	validation := &v1.MCPWebhookValidation{
		Name:      "validation-1",
		Namespace: "default",
		Spec: v1.MCPWebhookValidationSpec{
			Manifest: types.MCPWebhookValidationManifest{
				SystemMCPServerManifest: &types.SystemMCPServerManifest{
					Env: []types.MCPEnv{{
						Key:       "TOKEN",
						Value:     "legacy-token",
						Sensitive: true,
					}},
				},
			},
		},
	}
	storageClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	require.NoError(t, storageClient.Create(t.Context(), validation))

	err := (&Handler{gatewayClient: gatewayClient}).MigrateStaticConfiguration(router.Request{
		Client:    storageClient,
		Ctx:       t.Context(),
		Object:    validation,
		Namespace: validation.Namespace,
		Name:      validation.Name,
	}, &router.ResponseWrapper{})
	require.NoError(t, err)
	require.Empty(t, validation.Spec.Manifest.SystemMCPServerManifest.Env[0].Value)
	require.True(t, validation.Spec.Manifest.SystemMCPServerManifest.Env[0].ValueConfigured)

	staticSecrets, err := mcp.StaticCredentialSecrets(t.Context(), gatewayClient, system.MCPWebhookValidationCredentialContext, validation.Name)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"TOKEN": "legacy-token"}, staticSecrets)

	var stored v1.MCPWebhookValidation
	require.NoError(t, storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(validation), &stored))
	require.Empty(t, stored.Spec.Manifest.SystemMCPServerManifest.Env[0].Value)
	require.True(t, stored.Spec.Manifest.SystemMCPServerManifest.Env[0].ValueConfigured)
}

func TestDesiredSystemServer_CopiesProvidedManifest(t *testing.T) {
	validation := &v1.MCPWebhookValidation{}
	validation.Name = "validation-1"
	validation.Namespace = "default"
	validation.Spec.Manifest.URL = "https://ignored.example.com/webhook"
	validation.Spec.Manifest.SystemMCPServerManifest = &types.SystemMCPServerManifest{
		Name:             "custom-validator",
		ShortDescription: "Custom validation server",
		Enabled:          new(true),
		Runtime:          types.RuntimeContainerized,
		ContainerizedConfig: &types.ContainerizedRuntimeConfig{
			Image: "example/image:latest",
			Port:  9999,
			Path:  "/custom",
		},
		Env: []types.MCPEnv{{
			Key: "CUSTOM", Value: "1",
		}},
	}

	server := desiredSystemServer(validation, "ignored-image")

	if server.Spec.Manifest.Name != "custom-validator" {
		t.Fatalf("expected manifest name to be copied, got %q", server.Spec.Manifest.Name)
	}
	if server.Spec.Manifest.ContainerizedConfig == nil || server.Spec.Manifest.ContainerizedConfig.Image != "example/image:latest" {
		t.Fatalf("expected containerized config image to be copied, got %#v", server.Spec.Manifest.ContainerizedConfig)
	}
	if len(server.Spec.Manifest.Env) != 1 || server.Spec.Manifest.Env[0].Key != "CUSTOM" {
		t.Fatalf("expected env to be copied, got %#v", server.Spec.Manifest.Env)
	}
	if server.Spec.WebhookValidationName != validation.Name {
		t.Fatalf("expected webhook validation name %q, got %q", validation.Name, server.Spec.WebhookValidationName)
	}

	validation.Spec.Manifest.SystemMCPServerManifest.Name = "mutated"
	if server.Spec.Manifest.Name != "custom-validator" {
		t.Fatalf("expected copied manifest to be independent after mutation, got %q", server.Spec.Manifest.Name)
	}
	if server.Spec.Manifest.Env[0].Key == "WEBHOOK_URL" {
		t.Fatalf("expected provided manifest to be used instead of derived webhook env, got %#v", server.Spec.Manifest.Env)
	}
}

func newWebhookValidationTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, database.AutoMigrate())
	client := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

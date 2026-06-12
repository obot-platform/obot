package provider

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestAuthProviderEnvIDNoopWhenEnvAbsent(t *testing.T) {
	providerID, configured := authProviderEnvID(func(string) (string, bool) {
		return "", false
	})
	if configured {
		t.Fatalf("expected env bootstrap to be disabled")
	}
	if providerID != "" {
		t.Fatalf("expected no provider id, got %q", providerID)
	}
}

func TestAuthProviderEnvIDFallsBackToGenericOAuthForLegacyEnv(t *testing.T) {
	providerID, configured := authProviderEnvID(func(key string) (string, bool) {
		if key == genericOAuthIssuerEnvVar {
			return "https://studio.example.com/api/auth", true
		}
		return "", false
	})
	if !configured {
		t.Fatalf("expected legacy generic OAuth env to enable auth provider bootstrap")
	}
	if providerID != genericOAuthAuthProviderName {
		t.Fatalf("provider id = %q, want %q", providerID, genericOAuthAuthProviderName)
	}
}

func TestAuthProviderEnvSecretsRejectsMissingRequiredConfig(t *testing.T) {
	_, err := authProviderEnvSecrets(genericAuthProviderValue(), func(key string) (string, bool) {
		values := map[string]string{
			authProviderIDEnvVar:     genericOAuthAuthProviderName,
			genericOAuthIssuerEnvVar: "https://studio.example.com/api/auth",
		}
		value, ok := values[key]
		return value, ok
	})
	if err == nil {
		t.Fatal("expected partial auth provider env config to fail")
	}
}

func TestEnsureAuthProviderEnvCredentialCreatesCredentialAndSyncsProvider(t *testing.T) {
	ctx := context.Background()
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(genericAuthProvider()).Build()
	gatewayClient := newProviderTestGatewayClient(t, storageClient)

	setGenericOAuthEnv(t)
	handler := &Handler{gatewayClient: gatewayClient}
	if err := handler.EnsureAuthProviderEnvCredential(ctx, storageClient); err != nil {
		t.Fatal(err)
	}

	cred, err := gatewayClient.RevealCredential(ctx, []string{genericOAuthAuthProviderName, system.GenericAuthProviderCredentialContext}, genericOAuthAuthProviderName)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		genericOAuthProviderNameEnvVar:      "Studio",
		genericOAuthIssuerEnvVar:            "http://host.docker.internal:5173/api/auth",
		genericOAuthClientIDEnvVar:          "obot-client",
		genericOAuthClientSecretEnvVar:      "obot-secret",
		genericOAuthScopeEnvVar:             "openid profile email",
		authProviderEmailDomainsEnv:         "*",
		genericOAuthTrustEmailLinkingEnvVar: "true",
	} {
		if got := cred.Secrets[key]; got != want {
			t.Fatalf("credential secret %s = %q, want %q", key, got, want)
		}
	}
	decodedCookieSecret, err := base64.StdEncoding.DecodeString(cred.Secrets[authProviderCookieSecretEnv])
	if err != nil {
		t.Fatalf("cookie secret is not base64: %v", err)
	}
	if len(decodedCookieSecret) != 32 {
		t.Fatalf("cookie secret length = %d, want 32", len(decodedCookieSecret))
	}

	var authProvider v1.AuthProvider
	if err := storageClient.Get(ctx, objectKey(genericOAuthAuthProviderName), &authProvider); err != nil {
		t.Fatal(err)
	}
	if authProvider.Annotations[v1.AuthProviderSyncAnnotation] == "" {
		t.Fatalf("expected auth provider sync annotation to be toggled")
	}
}

func TestEnsureAuthProviderEnvCredentialWaitsForProviderRegistration(t *testing.T) {
	ctx := context.Background()
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	gatewayClient := newProviderTestGatewayClient(t, storageClient)

	setGenericOAuthEnv(t)
	go func() {
		time.Sleep(50 * time.Millisecond)
		if err := storageClient.Create(ctx, genericAuthProvider()); err != nil {
			t.Errorf("failed to create generic auth provider: %v", err)
		}
	}()
	handler := &Handler{gatewayClient: gatewayClient}
	if err := handler.EnsureAuthProviderEnvCredential(ctx, storageClient); err != nil {
		t.Fatal(err)
	}

	var authProvider v1.AuthProvider
	if err := storageClient.Get(ctx, objectKey(genericOAuthAuthProviderName), &authProvider); err != nil {
		t.Fatal(err)
	}
	cred, err := gatewayClient.RevealCredential(ctx, []string{genericOAuthAuthProviderName}, genericOAuthAuthProviderName)
	if err != nil {
		t.Fatal(err)
	}
	if got := cred.Secrets[genericOAuthClientIDEnvVar]; got != "obot-client" {
		t.Fatalf("client id = %q, want env value", got)
	}
}

func TestEnsureAuthProviderEnvCredentialSupportsAnyRegistryBackedProvider(t *testing.T) {
	ctx := context.Background()
	const providerName = "example-auth-provider"
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(testAuthProvider(providerName, []string{
		"EXAMPLE_CLIENT_ID",
		"EXAMPLE_CLIENT_SECRET",
		authProviderCookieSecretEnv,
	}, []string{
		"EXAMPLE_ALLOWED_ORG",
	})).Build()
	gatewayClient := newProviderTestGatewayClient(t, storageClient)

	t.Setenv(authProviderIDEnvVar, providerName)
	t.Setenv("EXAMPLE_CLIENT_ID", "example-client")
	t.Setenv("EXAMPLE_CLIENT_SECRET", "example-secret")
	t.Setenv("EXAMPLE_ALLOWED_ORG", "engineering")

	handler := &Handler{gatewayClient: gatewayClient}
	if err := handler.EnsureAuthProviderEnvCredential(ctx, storageClient); err != nil {
		t.Fatal(err)
	}

	cred, err := gatewayClient.RevealCredential(ctx, []string{providerName, system.GenericAuthProviderCredentialContext}, providerName)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"EXAMPLE_CLIENT_ID":     "example-client",
		"EXAMPLE_CLIENT_SECRET": "example-secret",
		"EXAMPLE_ALLOWED_ORG":   "engineering",
	} {
		if got := cred.Secrets[key]; got != want {
			t.Fatalf("credential secret %s = %q, want %q", key, got, want)
		}
	}
	decodedCookieSecret, err := base64.StdEncoding.DecodeString(cred.Secrets[authProviderCookieSecretEnv])
	if err != nil {
		t.Fatalf("cookie secret is not base64: %v", err)
	}
	if len(decodedCookieSecret) != 32 {
		t.Fatalf("cookie secret length = %d, want 32", len(decodedCookieSecret))
	}
}

func TestEnsureAuthProviderEnvCredentialPreservesCookieSecret(t *testing.T) {
	ctx := context.Background()
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(genericAuthProvider()).Build()
	gatewayClient := newProviderTestGatewayClient(t, storageClient)
	const cookieSecret = "12345678901234567890123456789012"
	encodedCookieSecret := base64.StdEncoding.EncodeToString([]byte(cookieSecret))
	if err := gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: genericOAuthAuthProviderName,
		Name:    genericOAuthAuthProviderName,
		Secrets: map[string]string{
			authProviderCookieSecretEnv:    encodedCookieSecret,
			genericOAuthIssuerEnvVar:       "http://old.example.com/api/auth",
			genericOAuthClientIDEnvVar:     "old-client",
			genericOAuthClientSecretEnvVar: "old-secret",
		},
	}); err != nil {
		t.Fatal(err)
	}

	setGenericOAuthEnv(t)
	handler := &Handler{gatewayClient: gatewayClient}
	if err := handler.EnsureAuthProviderEnvCredential(ctx, storageClient); err != nil {
		t.Fatal(err)
	}

	cred, err := gatewayClient.RevealCredential(ctx, []string{genericOAuthAuthProviderName}, genericOAuthAuthProviderName)
	if err != nil {
		t.Fatal(err)
	}
	if got := cred.Secrets[authProviderCookieSecretEnv]; got != encodedCookieSecret {
		t.Fatalf("cookie secret rotated: got %q, want %q", got, encodedCookieSecret)
	}
	if got := cred.Secrets[genericOAuthClientIDEnvVar]; got != "obot-client" {
		t.Fatalf("client id = %q, want updated env value", got)
	}
}

func setGenericOAuthEnv(t *testing.T) {
	t.Helper()
	t.Setenv(authProviderIDEnvVar, genericOAuthAuthProviderName)
	t.Setenv(genericOAuthProviderNameEnvVar, "Studio")
	t.Setenv(genericOAuthIssuerEnvVar, "http://host.docker.internal:5173/api/auth")
	t.Setenv(genericOAuthClientIDEnvVar, "obot-client")
	t.Setenv(genericOAuthClientSecretEnvVar, "obot-secret")
	t.Setenv(genericOAuthScopeEnvVar, "openid profile email")
	t.Setenv(authProviderEmailDomainsEnv, "*")
	t.Setenv(genericOAuthTrustEmailLinkingEnvVar, "true")
}

func genericAuthProvider() *v1.AuthProvider {
	provider := genericAuthProviderValue()
	return &provider
}

func genericAuthProviderValue() v1.AuthProvider {
	return testAuthProviderValue(genericOAuthAuthProviderName, []string{
		genericOAuthProviderNameEnvVar,
		genericOAuthIssuerEnvVar,
		genericOAuthClientIDEnvVar,
		genericOAuthClientSecretEnvVar,
		authProviderCookieSecretEnv,
		authProviderEmailDomainsEnv,
		genericOAuthTrustEmailLinkingEnvVar,
	}, []string{
		genericOAuthScopeEnvVar,
		authProviderPostgresDSNEnv,
		authProviderRefreshPeriodEnv,
		authProviderLoggingEnv,
	})
}

func testAuthProvider(name string, required []string, optional []string) *v1.AuthProvider {
	provider := testAuthProviderValue(name, required, optional)
	return &provider
}

func testAuthProviderValue(name string, required []string, optional []string) v1.AuthProvider {
	return v1.AuthProvider{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: system.DefaultNamespace,
			Name:      name,
		},
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: types.AuthProviderManifest{
				CommonProviderMetadata: types.CommonProviderMetadata{
					RequiredConfigurationParameters: providerConfigurationParameters(required),
					OptionalConfigurationParameters: providerConfigurationParameters(optional),
				},
			},
		},
	}
}

func providerConfigurationParameters(names []string) []types.ProviderConfigurationParameter {
	params := make([]types.ProviderConfigurationParameter, 0, len(names))
	for _, name := range names {
		params = append(params, types.ProviderConfigurationParameter{Name: name})
	}
	return params
}

func objectKey(name string) kclient.ObjectKey {
	return kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: name}
}

func newProviderTestGatewayClient(t *testing.T, storageClient kclient.Client) *gateway.Client {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
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
	t.Cleanup(func() {
		_ = db.Close()
	})

	return gateway.New(context.Background(), db, storageClient, nil, nil, nil, time.Hour, 1000, 90)
}

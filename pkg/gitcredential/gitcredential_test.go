package gitcredential

import (
	"context"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNormalizeHost(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    string
		wantErr string
	}{
		{name: "hostname", value: " GitHub.COM ", want: "github.com"},
		{name: "host with port", value: "Git.Example.com:8443", want: "git.example.com:8443"},
		{name: "scheme", value: "https://github.com", wantErr: "must not include"},
		{name: "path", value: "github.com/org", wantErr: "must not include"},
		{name: "empty", wantErr: "required"},
		{name: "invalid port", value: "github.com:99999", wantErr: "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeHost(tt.value)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateSourceURL(t *testing.T) {
	require.NoError(t, validateSourceURL("https://github.com/obot-platform/obot", "github.com"))
	require.NoError(t, validateSourceURL("https://git.example.com:8443/team/repo.git", "git.example.com:8443"))
	assert.ErrorContains(t, validateSourceURL("https://gitlab.com/team/repo", "github.com"), "cannot be used")
	assert.ErrorContains(t, validateSourceURL("http://github.com/obot-platform/obot", "github.com"), "must use HTTPS")
	assert.ErrorContains(t, validateSourceURL("https://example.com/catalog.yaml", "example.com"), "not a Git repository")
}

func TestConfigured(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)

	configured, err := Configured(ctx, gatewayClient, "missing")
	require.NoError(t, err)
	assert.False(t, configured)

	require.NoError(t, gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: credentialContext,
		Name:    "empty",
	}))
	configured, err = Configured(ctx, gatewayClient, "empty")
	require.NoError(t, err)
	assert.False(t, configured)

	require.NoError(t, Store(ctx, gatewayClient, "configured", "shared-token"))
	configured, err = Configured(ctx, gatewayClient, "configured")
	require.NoError(t, err)
	assert.True(t, configured)

	canceledCtx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = Configured(canceledCtx, gatewayClient, "configured")
	require.ErrorIs(t, err, context.Canceled)
}

func TestResolveSharedCredential(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	repositoryClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "gc1-test", Namespace: system.DefaultNamespace},
		Spec:       v1.GitCredentialSpec{DisplayName: "Shared GitHub", Host: "github.com"},
	}).Build()

	require.NoError(t, Store(ctx, gatewayClient, "gc1-test", "shared-token"))

	for _, sourceURL := range []string{
		"https://github.com/obot-platform/skills",
		"https://github.com/obot-platform/catalog.git",
	} {
		token, err := Resolve(ctx, repositoryClient, gatewayClient, system.DefaultNamespace, "gc1-test", sourceURL)
		require.NoError(t, err)
		assert.Equal(t, "shared-token", token)
	}

	_, err := Resolve(ctx, repositoryClient, gatewayClient, system.DefaultNamespace, "gc1-test", "https://gitlab.com/obot-platform/catalog")
	assert.ErrorContains(t, err, "cannot be used")
}

func TestResolveRejectsCredentialWithoutToken(t *testing.T) {
	ctx := context.Background()
	gatewayClient := newTestGatewayClient(t)
	repositoryClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "gc1-empty", Namespace: system.DefaultNamespace},
		Spec:       v1.GitCredentialSpec{DisplayName: "Empty GitHub", Host: "github.com"},
	}).Build()
	require.NoError(t, gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
		Context: credentialContext,
		Name:    "gc1-empty",
	}))

	_, err := Resolve(ctx, repositoryClient, gatewayClient, system.DefaultNamespace, "gc1-empty", "https://github.com/obot-platform/skills")
	require.ErrorContains(t, err, "no token configured")
}

func TestResolveOrReveal(t *testing.T) {
	ctx := t.Context()
	storageClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.GitCredential{
		ObjectMeta: metav1.ObjectMeta{Name: "gc1-test", Namespace: system.DefaultNamespace},
		Spec:       v1.GitCredentialSpec{DisplayName: "Shared GitHub", Host: "github.com"},
	}).Build()
	const sourceURL = "https://github.com/obot-platform/skills"

	t.Run("returns legacy token", func(t *testing.T) {
		gatewayClient := newTestGatewayClient(t)
		require.NoError(t, gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: "repo1",
			Name:    "legacy-tool",
			Secrets: map[string]string{sourceURL: "legacy-token"},
		}))

		token, err := ResolveOrReveal(ctx, storageClient, gatewayClient, system.DefaultNamespace, "", sourceURL, "repo1", "legacy-tool")
		require.NoError(t, err)
		assert.Equal(t, "legacy-token", token)
	})

	t.Run("missing legacy token falls back without authentication", func(t *testing.T) {
		token, err := ResolveOrReveal(ctx, storageClient, newTestGatewayClient(t), system.DefaultNamespace, "", sourceURL, "repo1", "legacy-tool")
		require.NoError(t, err)
		assert.Empty(t, token)
	})

	t.Run("legacy backend failure falls back without authentication", func(t *testing.T) {
		gatewayClient := newTestGatewayClient(t)
		require.NoError(t, gatewayClient.Close())

		token, err := ResolveOrReveal(ctx, storageClient, gatewayClient, system.DefaultNamespace, "", sourceURL, "repo1", "legacy-tool")
		require.NoError(t, err)
		assert.Empty(t, token)
	})

	t.Run("shared credential backend failure remains fatal", func(t *testing.T) {
		gatewayClient := newTestGatewayClient(t)
		require.NoError(t, gatewayClient.Close())

		_, err := ResolveOrReveal(ctx, storageClient, gatewayClient, system.DefaultNamespace, "gc1-test", sourceURL, "repo1", "legacy-tool")
		require.Error(t, err)
	})
}

func newTestGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	client := gatewayclient.New(t.Context(), db, nil, nil, nil, nil, nil, time.Hour, 10, 90, 90, true)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

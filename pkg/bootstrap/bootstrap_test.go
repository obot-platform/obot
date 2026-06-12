package bootstrap

import (
	"context"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type staticAuthProviderGetter string

func (s staticAuthProviderGetter) GetConfiguredAuthProvider(context.Context) (string, error) {
	return string(s), nil
}

func newBootstrapTestClient(t *testing.T) (*client.Client, context.Context) {
	t.Helper()

	ctx, cancel := context.WithCancel(t.Context())

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
				Role: types2.RoleBasic,
			},
		}).
		Build()

	c := client.New(ctx, db, storageClient, nil, nil, nil, time.Hour, 1, 90)
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
	})

	return c, ctx
}

func ensureOwner(t *testing.T, c *client.Client, username, email, authProviderName string) {
	t.Helper()

	if _, err := c.EnsureIdentityWithRole(t.Context(), &gwtypes.Identity{
		Email:                 email,
		AuthProviderName:      authProviderName,
		AuthProviderNamespace: "default",
		ProviderUsername:      username,
		ProviderUserID:        username,
	}, "", types2.RoleOwner); err != nil {
		t.Fatalf("failed to ensure owner identity: %v", err)
	}
}

func TestBootstrapEnabledDependsOnConfiguredProviderOwner(t *testing.T) {
	c, ctx := newBootstrapTestClient(t)

	ensureOwner(t, c, "old-owner", "old-owner@example.com", "old-auth-provider")

	b := &Bootstrap{
		authEnabled:        true,
		gatewayClient:      c,
		authProviderGetter: staticAuthProviderGetter(""),
	}
	enabled, err := b.Enabled(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected bootstrap enabled when no auth provider is configured")
	}

	b.authProviderGetter = staticAuthProviderGetter("new-auth-provider")
	enabled, err = b.Enabled(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected bootstrap enabled when no owner belongs to the configured auth provider")
	}

	ensureOwner(t, c, "new-owner", "new-owner@example.com", "new-auth-provider")
	enabled, err = b.Enabled(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if enabled {
		t.Fatal("expected bootstrap disabled once an owner belongs to the configured auth provider")
	}

	b.forceEnableBootstrap = true
	enabled, err = b.Enabled(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !enabled {
		t.Fatal("expected bootstrap enabled when force-enabled")
	}

	setupEnabled, err := b.SetupEnabled(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if setupEnabled {
		t.Fatal("expected setup disabled when a configured auth provider owner exists, even with bootstrap force-enabled")
	}
}

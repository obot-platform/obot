package bootstrap

import (
	"context"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gwtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type staticAuthProviderGetter string

func (s staticAuthProviderGetter) GetConfiguredAuthProvider(context.Context) (string, error) {
	return string(s), nil
}

func newBootstrapTestClient(t *testing.T) (*client.Client, context.Context) {
	t.Helper()

	c, _, ctx := newBootstrapTestClientWithDB(t)
	return c, ctx
}

func newBootstrapTestClientWithDB(t *testing.T) (*client.Client, *gorm.DB, context.Context) {
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
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultRoleSettingName,
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: types2.RoleBasic,
			},
		}).
		Build()

	c := client.New(ctx, db, storageClient, nil, nil, nil, nil, time.Hour, 1, 90, 90, 90, true)
	t.Cleanup(func() {
		cancel()
		_ = c.Close()
	})

	return c, services.DB.DB, ctx
}

func ensureOwner(t *testing.T, c *client.Client, username, email, authProviderName string) {
	t.Helper()

	if _, err := c.EnsureIdentityWithRole(t.Context(), &gwtypes.Identity{
		Email:                 email,
		AuthProviderName:      authProviderName,
		AuthProviderNamespace: "default",
		ProviderUsername:      username,
		ProviderUserID:        username,
	}, "", types2.RoleOwner, client.UserLimit{Unlimited: true}); err != nil {
		t.Fatalf("failed to ensure owner identity: %v", err)
	}
}

// deactivateThroughSCIM has the identity provider of provider deactivate the user with the native ID nativeID and the
// email email through SCIM, as it does in production, setting up the provider's SCIM connection first if it has none.
// A user who signed in with that ID is bound to their account, and disabled.
func deactivateThroughSCIM(t *testing.T, c *client.Client, provider client.AuthProviderRef, nativeID, email string) error {
	t.Helper()

	conn, err := c.SCIMConnectionForAuthProvider(t.Context(), provider.Namespace, provider.Name)
	if err != nil {
		return err
	}
	if conn == nil {
		if conn, _, err = c.CreateSCIMConnection(t.Context(), client.CreateSCIMConnectionOptions{
			AuthProviderNamespace: provider.Namespace,
			AuthProviderName:      provider.Name,
			GroupIDPrefix:         "okta/",
			Origin:                gwtypes.SCIMConnectionOriginSCIMFirst,
		}); err != nil {
			return err
		}
	}
	_, err = c.CreateSCIMUser(t.Context(), conn, client.SCIMUserInput{
		UserName:   email,
		ExternalID: nativeID,
		Active:     new(false),
		Profile: gwtypes.SCIMUserProfile{
			Emails: []gwtypes.SCIMMultiValue{
				{
					Value:   email,
					Primary: true,
				},
			},
		},
	}, client.SCIMUserCreateOptions{
		UserLimit: client.UserLimit{
			Unlimited: true,
		},
		DefaultRole: types2.RoleBasic,
	})
	return err
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

func TestDisabledBootstrapIsOffWhileAuthenticationRemainsEnabled(t *testing.T) {
	b := &Bootstrap{
		authEnabled:          true,
		forceEnableBootstrap: true,
		disabled:             true,
	}

	if enabled, err := b.Enabled(t.Context()); err != nil {
		t.Fatalf("checking bootstrap enabled: %v", err)
	} else if enabled {
		t.Fatal("disabled bootstrap reported enabled")
	}
	if enabled, err := b.SetupEnabled(t.Context()); err != nil {
		t.Fatalf("checking bootstrap setup enabled: %v", err)
	} else if enabled {
		t.Fatal("disabled bootstrap setup reported enabled")
	}
}

func TestBootstrapStaysEnabledUntilAnOwnerSignsIn(t *testing.T) {
	c, db, ctx := newBootstrapTestClientWithDB(t)
	provider := client.AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "okta-auth-provider",
	}

	b := &Bootstrap{
		authEnabled:        true,
		gatewayClient:      c,
		authProviderGetter: staticAuthProviderGetter(provider.Name),
	}
	assertEnabled := func(want bool, why string) {
		t.Helper()
		enabled, err := b.Enabled(ctx)
		if err != nil {
			t.Fatalf("failed to check bootstrap: %v", err)
		}
		if enabled != want {
			t.Fatalf("bootstrap enabled = %v, want %v %s", enabled, want, why)
		}
	}

	// An Owner whose identity was created before they ever signed in, as SCIM provisioning does.
	owner := &gwtypes.User{
		Username:       "00u-owner",
		HashedUsername: "owner-hash",
		Email:          "owner@example.com",
		HashedEmail:    "owner-email-hash",
		Role:           types2.RoleOwner | types2.RoleAuditor,
	}
	if err := db.Create(owner).Error; err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	if err := db.Create(&gwtypes.Identity{
		AuthProviderName:      provider.Name,
		AuthProviderNamespace: provider.Namespace,
		ProviderUserID:        "00u-owner",
		HashedProviderUserID:  hash.String("00u-owner"),
		UserID:                owner.ID,
	}).Error; err != nil {
		t.Fatalf("failed to create owner identity: %v", err)
	}
	assertEnabled(true, "while the only owner has never signed in")

	if err := db.Model(new(gwtypes.Identity)).Where("user_id = ?", owner.ID).UpdateColumn("first_sign_in_at", time.Now()).Error; err != nil {
		t.Fatalf("failed to record sign-in: %v", err)
	}
	assertEnabled(false, "once an owner has signed in")

	// The identity provider can reactivate a disabled owner, so disabling every owner does not reopen bootstrap.
	if err := deactivateThroughSCIM(t, c, provider, "00u-owner", owner.Email); err != nil {
		t.Fatalf("failed to disable owner: %v", err)
	}
	assertEnabled(false, "while the only owner who signed in is disabled")

	if err := db.Model(owner).UpdateColumn("deleted_at", time.Now()).Error; err != nil {
		t.Fatalf("failed to delete owner: %v", err)
	}
	assertEnabled(true, "once the only owner who signed in is deleted")
}

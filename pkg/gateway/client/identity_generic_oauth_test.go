package client

import (
	"context"
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenericOAuthLinksByEmailWhenIssuerTrustedAndEmailVerified(t *testing.T) {
	c := newGenericOAuthTestClient(t, "https://issuer.example.com/", "true")
	ctx := context.Background()

	existing, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "google-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice",
		ProviderUserID:        "google-alice",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	emailVerified := true
	actual, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "iss:https://issuer.example.com/\x00sub:alice",
		ProviderIssuer:        "https://issuer.example.com/",
		ProviderEmailVerified: &emailVerified,
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	if actual.ID != existing.ID {
		t.Fatalf("expected trusted generic OAuth identity to link to existing user %d, got %d", existing.ID, actual.ID)
	}
}

func TestGenericOAuthDoesNotLinkWhenEmailVerifiedFalse(t *testing.T) {
	c := newGenericOAuthTestClient(t, "https://issuer.example.com/", "true")
	ctx := context.Background()

	existing, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "google-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice",
		ProviderUserID:        "google-alice",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	emailVerified := false
	actual, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "iss:https://issuer.example.com/\x00sub:alice",
		ProviderIssuer:        "https://issuer.example.com/",
		ProviderEmailVerified: &emailVerified,
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	if actual.ID == existing.ID {
		t.Fatalf("expected email_verified=false generic OAuth identity not to link to existing user %d", existing.ID)
	}
}

func TestGenericOAuthLinksWhenEmailVerifiedAbsentAndTrustEnabled(t *testing.T) {
	c := newGenericOAuthTestClient(t, "https://issuer.example.com/", "true")
	ctx := context.Background()

	existing, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "google-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice",
		ProviderUserID:        "google-alice",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	actual, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "iss:https://issuer.example.com/\x00sub:alice",
		ProviderIssuer:        "https://issuer.example.com/",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	if actual.ID != existing.ID {
		t.Fatalf("expected trusted generic OAuth identity with absent email_verified to link to existing user %d, got %d", existing.ID, actual.ID)
	}
}

func TestGenericOAuthDoesNotLinkWhenTrustDisabled(t *testing.T) {
	c := newGenericOAuthTestClient(t, "https://issuer.example.com/", "false")
	ctx := context.Background()

	existing, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "google-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice",
		ProviderUserID:        "google-alice",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	emailVerified := true
	actual, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "iss:https://issuer.example.com/\x00sub:alice",
		ProviderIssuer:        "https://issuer.example.com/",
		ProviderEmailVerified: &emailVerified,
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	if actual.ID == existing.ID {
		t.Fatalf("expected trust-disabled generic OAuth identity not to link to existing user %d", existing.ID)
	}
}

func TestGenericOAuthDoesNotLinkWhenIssuerChanges(t *testing.T) {
	c := newGenericOAuthTestClient(t, "https://issuer.example.com/", "true")
	ctx := context.Background()

	existing, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      "google-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice",
		ProviderUserID:        "google-alice",
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	emailVerified := true
	actual, err := c.EnsureIdentity(ctx, &types.Identity{
		Email:                 "alice@example.com",
		AuthProviderName:      genericOAuthAuthProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		ProviderUsername:      "alice@example.com",
		ProviderUserID:        "iss:https://other-issuer.example.com/\x00sub:alice",
		ProviderIssuer:        "https://other-issuer.example.com/",
		ProviderEmailVerified: &emailVerified,
	}, "")
	if err != nil {
		t.Fatal(err)
	}

	if actual.ID == existing.ID {
		t.Fatalf("expected issuer-mismatched generic OAuth identity not to link to existing user %d", existing.ID)
	}
}

func newGenericOAuthTestClient(t *testing.T, issuer, trust string) *Client {
	t.Helper()

	c := newTestClient(t)
	c.storageClient = fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
		&v1.AuthProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      genericOAuthAuthProviderName,
				Namespace: system.DefaultNamespace,
			},
		},
		&v1.UserDefaultRoleSetting{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.DefaultRoleSettingName,
				Namespace: system.DefaultNamespace,
			},
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: types2.RoleBasic,
			},
		},
	).Build()
	if c.emailsWithExplicitRoles == nil {
		c.emailsWithExplicitRoles = map[string]types2.Role{}
	}

	if err := c.UpsertCredential(context.Background(), types.Credential{
		Context: genericOAuthAuthProviderName,
		Name:    genericOAuthAuthProviderName,
		Secrets: map[string]string{
			"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_ISSUER":              issuer,
			"OBOT_GENERIC_OAUTH_AUTH_PROVIDER_TRUST_EMAIL_LINKING": trust,
		},
	}); err != nil {
		t.Fatal(err)
	}

	return c
}

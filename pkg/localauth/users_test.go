package localauth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newUsersTestProvider(t *testing.T) (*Provider, *client.Client) {
	t.Helper()

	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatalf("creating storage services: %v", err)
	}
	gatewayDB, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("creating gateway database: %v", err)
	}
	if err := gatewayDB.AutoMigrate(); err != nil {
		t.Fatalf("migrating gateway database: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	storageClient := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	gatewayClient := client.New(ctx, gatewayDB, storageClient, nil, nil, nil, nil, time.Hour, 1, 90, 90, 90, true)
	t.Cleanup(func() {
		cancel()
		_ = gatewayClient.Close()
	})

	return &Provider{gatewayClient: gatewayClient}, gatewayClient
}

func TestEnsureInitialOwnerCreatesAndRotatesPendingAccount(t *testing.T) {
	provider, gatewayClient := newUsersTestProvider(t)
	firstToken := "first-high-entropy-owner-setup-token"
	firstExpiry := time.Now().Add(time.Hour)
	if err := provider.EnsureInitialOwner(t.Context(), "Owner@Example.com", firstToken, firstExpiry); err != nil {
		t.Fatalf("ensuring initial owner: %v", err)
	}

	credential, err := gatewayClient.RevealCredential(t.Context(), []string{ProviderName, system.GenericAuthProviderCredentialContext}, ProviderName)
	if err != nil {
		t.Fatalf("reading configured local provider: %v", err)
	}
	if got := credential.Secrets[EmailDomainsEnvVar]; got != "example.com" {
		t.Fatalf("configured email domains = %q, want example.com", got)
	}

	user, err := gatewayClient.LocalAuthUserByEmail(t.Context(), "owner@example.com")
	if err != nil {
		t.Fatalf("reading initial owner: %v", err)
	}
	if user.SetupTokenHash != hash.String(firstToken) || !user.RequirePasswordChange {
		t.Fatalf("initial owner pending state = hash %q, required %v", user.SetupTokenHash, user.RequirePasswordChange)
	}

	oldSessionID := hash.String("old-setup-session")
	if _, err := gatewayClient.ActivateLocalAuthUser(t.Context(), user.SetupTokenHash, oldSessionID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating first setup token: %v", err)
	}
	secondToken := "second-high-entropy-owner-setup-token"
	if err := provider.EnsureInitialOwner(t.Context(), "owner@example.com", secondToken, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatalf("rotating initial owner setup token: %v", err)
	}
	if _, _, err := gatewayClient.LocalAuthSession(t.Context(), oldSessionID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("old setup session after rotation error = %v, want record not found", err)
	}
	if _, err := gatewayClient.ActivateLocalAuthUser(t.Context(), hash.String(secondToken), hash.String("new-setup-session"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("activating rotated setup token: %v", err)
	}
}

func TestEnsureInitialOwnerDoesNotRearmExistingAccount(t *testing.T) {
	provider, gatewayClient := newUsersTestProvider(t)
	if err := gatewayClient.UpsertCredential(t.Context(), types.Credential{
		Context: ProviderName,
		Name:    ProviderName,
		Secrets: map[string]string{EmailDomainsEnvVar: "example.com"},
	}); err != nil {
		t.Fatalf("configuring local provider: %v", err)
	}
	passwordHash, err := HashPassword("an-existing-secure-password")
	if err != nil {
		t.Fatalf("hashing existing password: %v", err)
	}
	user, err := gatewayClient.CreateLocalAuthUser(t.Context(), "owner@example.com", passwordHash, false)
	if err != nil {
		t.Fatalf("creating existing user: %v", err)
	}

	if err := provider.EnsureInitialOwner(t.Context(), "owner@example.com", "replacement-high-entropy-setup-token", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("ensuring existing owner: %v", err)
	}
	got, err := gatewayClient.LocalAuthUserByID(t.Context(), user.ID)
	if err != nil {
		t.Fatalf("reading existing user: %v", err)
	}
	if got.SetupTokenHash != "" || got.RequirePasswordChange || got.PasswordHash != passwordHash {
		t.Fatalf("existing account was rearmed or reset: %+v", got)
	}
}

func TestEnsureInitialOwnerDoesNotReviveExpiredUnchangedToken(t *testing.T) {
	provider, gatewayClient := newUsersTestProvider(t)
	setupToken := "an-expired-high-entropy-owner-setup-token"
	expiredAt := time.Now().Add(-time.Hour)
	if err := provider.EnsureInitialOwner(t.Context(), "owner@example.com", setupToken, expiredAt); err != nil {
		t.Fatalf("creating expired pending owner: %v", err)
	}

	if err := provider.EnsureInitialOwner(t.Context(), "owner@example.com", setupToken, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("reconciling unchanged setup token: %v", err)
	}
	user, err := gatewayClient.LocalAuthUserByEmail(t.Context(), "owner@example.com")
	if err != nil {
		t.Fatalf("reading pending owner: %v", err)
	}
	if user.SetupTokenExpiresAt == nil || !user.SetupTokenExpiresAt.Equal(expiredAt) {
		t.Fatalf("setup token expiration = %v, want unchanged %v", user.SetupTokenExpiresAt, expiredAt)
	}
	if _, err := gatewayClient.ActivateLocalAuthUser(t.Context(), hash.String(setupToken), hash.String("expired-session"), time.Now().Add(time.Hour)); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("activating expired unchanged token error = %v, want record not found", err)
	}
}

func TestEnsureInitialOwnerRejectsDisallowedConfiguredDomain(t *testing.T) {
	provider, gatewayClient := newUsersTestProvider(t)
	if err := gatewayClient.UpsertCredential(t.Context(), types.Credential{
		Context: ProviderName,
		Name:    ProviderName,
		Secrets: map[string]string{EmailDomainsEnvVar: "other.example"},
	}); err != nil {
		t.Fatalf("configuring local provider: %v", err)
	}

	err := provider.EnsureInitialOwner(t.Context(), "owner@example.com", "a-high-entropy-owner-setup-token", time.Now().Add(time.Hour))
	var invalid InvalidUserError
	if !errors.As(err, &invalid) {
		t.Fatalf("error = %v, want InvalidUserError", err)
	}
}

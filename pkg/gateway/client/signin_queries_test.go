package client

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
)

// statementCounter records the SQL statements that a client runs while measuring.
type statementCounter struct {
	counting   atomic.Bool
	mu         sync.Mutex
	statements []string
}

// newStatementCounter registers a counter with the client's database. Each test client has its own database, so the
// counter is never removed.
func newStatementCounter(t *testing.T, c *Client) *statementCounter {
	t.Helper()

	counter := new(statementCounter)
	record := func(db *gorm.DB) {
		if !counter.counting.Load() {
			return
		}
		counter.mu.Lock()
		defer counter.mu.Unlock()
		counter.statements = append(counter.statements, db.Statement.SQL.String())
	}
	const name = "test:count_statements"
	callbacks := c.db.WithContext(t.Context()).Callback()
	for _, register := range []func() error{
		func() error { return callbacks.Query().After("gorm:query").Register(name, record) },
		func() error { return callbacks.Row().After("gorm:row").Register(name, record) },
		func() error { return callbacks.Raw().After("gorm:raw").Register(name, record) },
		func() error { return callbacks.Create().After("gorm:create").Register(name, record) },
		func() error { return callbacks.Update().After("gorm:update").Register(name, record) },
		func() error { return callbacks.Delete().After("gorm:delete").Register(name, record) },
	} {
		if err := register(); err != nil {
			t.Fatal(err)
		}
	}
	return counter
}

// measure returns the statements that f runs.
func (s *statementCounter) measure(f func()) []string {
	s.mu.Lock()
	s.statements = nil
	s.mu.Unlock()

	s.counting.Store(true)
	f()
	s.counting.Store(false)

	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statements
}

func TestSteadySignInsRunNoQueryForSCIM(t *testing.T) {
	testSteadySignInsRunNoQueryForSCIM(t, newLifecycleTestClient(t))
}

func TestSteadySignInsRunNoQueryForSCIMOnPostgres(t *testing.T) {
	testSteadySignInsRunNoQueryForSCIM(t, newPostgresLifecycleTestClient(t))
}

// testSteadySignInsRunNoQueryForSCIM checks that a sign-in that changes nothing, which every cookie-authenticated
// request makes, reads the identity and the user and nothing else, whether or not SCIM manages the provider.
func testSteadySignInsRunNoQueryForSCIM(t *testing.T, c *Client) {
	t.Helper()
	ctx := t.Context()

	other := AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "google-auth-provider",
	}
	conn, _ := createTestSCIMConnection(t, c, false)
	provisionTestSCIMUser(t, c, conn, "00u-provisioned", "provisioned@example.com")
	setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)
	counter := newStatementCounter(t, c)

	for _, tt := range []struct {
		name     string
		identity func() *types.Identity
	}{
		{
			name: "a provider without SCIM",
			identity: func() *types.Identity {
				return &types.Identity{
					AuthProviderNamespace: other.Namespace,
					AuthProviderName:      other.Name,
					ProviderUsername:      "person",
					ProviderUserID:        "google-1",
					Email:                 "person@example.com",
				}
			},
		},
		{
			name: "an enforced SCIM connection, and a profile that differs from SCIM's",
			identity: func() *types.Identity {
				return signInIdentity("00u-provisioned", "different@example.com")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			signIn := func() {
				t.Helper()
				id := tt.identity()
				if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					// As EnsureIdentity calls it.
					_, err := c.ensureIdentity(ctx, tx, id, "", apitypes.RoleUnknown, UserLimit{
						Unlimited: true,
					})
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}

			// The first sign-ins create or correct the identity and the user, and record the sign-in and the day's
			// activity.
			signIn()
			signIn()
			if statements := counter.measure(signIn); len(statements) != 2 {
				t.Fatalf("a steady sign-in ran %d statements, want 2, the identity and the user:\n%s", len(statements), strings.Join(statements, "\n"))
			}
		})
	}
}

func TestRevealAuthProviderCredentialReadsTheSCIMConnectionInTheSameQuery(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	if err := c.UpsertCredential(ctx, types.Credential{
		Context: lifecycleTestProvider.Name,
		Name:    lifecycleTestProvider.Name,
		Secrets: map[string]string{
			"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID": "client",
		},
	}); err != nil {
		t.Fatal(err)
	}
	contexts := []string{lifecycleTestProvider.Name, system.GenericAuthProviderCredentialContext}

	credential, adapterType, err := c.RevealAuthProviderCredential(ctx, contexts, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name)
	if err != nil {
		t.Fatal(err)
	}
	if adapterType != "" || credential.Secrets["OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID"] != "client" {
		t.Fatalf("without a connection, RevealAuthProviderCredential() = %+v, %q", credential, adapterType)
	}

	createTestSCIMConnection(t, c, false)
	counter := newStatementCounter(t, c)
	statements := counter.measure(func() {
		credential, adapterType, err = c.RevealAuthProviderCredential(ctx, contexts, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name)
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapterType != "okta" || credential.Secrets["OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID"] != "client" {
		t.Fatalf("with a connection, RevealAuthProviderCredential() = %+v, %q", credential, adapterType)
	}
	if len(statements) != 1 {
		t.Fatalf("RevealAuthProviderCredential() ran %d statements, want 1:\n%s", len(statements), strings.Join(statements, "\n"))
	}

	if _, _, err := c.RevealAuthProviderCredential(ctx, contexts, lifecycleTestProvider.Namespace, "unknown"); !errorsAsCredentialNotFound(err) {
		t.Fatalf("RevealAuthProviderCredential() for an unknown provider error = %v", err)
	}

	// The first context that has the credential wins, and a later one is read in the same query.
	if err := c.UpsertCredential(ctx, types.Credential{
		Context: system.GenericAuthProviderCredentialContext,
		Name:    lifecycleTestProvider.Name,
		Secrets: map[string]string{
			"OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID": "generic",
		},
	}); err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		contexts []string
		want     string
	}{
		{
			contexts: contexts,
			want:     "client",
		},
		{
			contexts: []string{"missing", system.GenericAuthProviderCredentialContext},
			want:     "generic",
		},
	} {
		statements := counter.measure(func() {
			credential, _, err = c.RevealAuthProviderCredential(ctx, tt.contexts, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name)
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := credential.Secrets["OBOT_OKTA_AUTH_PROVIDER_CLIENT_ID"]; got != tt.want || len(statements) != 1 {
			t.Fatalf("RevealAuthProviderCredential(%v) = %q in %d statements, want %q in 1", tt.contexts, got, len(statements), tt.want)
		}
	}
}

func errorsAsCredentialNotFound(err error) bool {
	_, ok := err.(CredentialNotFoundError)
	return ok
}

// TestProfileRefreshRunsNoQueryForSCIM checks that GET /api/me reads only the identity when the profile needs no
// refresh, whether or not SCIM manages the provider, and never asks a SCIM-managed provider for the profile.
func TestProfileRefreshRunsNoQueryForSCIM(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := accesstoken.ContextWithAccessToken(t.Context(), "access-token")
	createTestSCIMConnection(t, c, false)
	counter := newStatementCounter(t, c)

	for _, provider := range []AuthProviderRef{
		lifecycleTestLocalProvider,
		lifecycleTestProvider,
	} {
		t.Run(provider.Name, func(t *testing.T) {
			user := createLifecycleTestUser(t, c, "person-"+provider.Name, provider)
			if err := c.db.WithContext(ctx).Model(new(types.Identity)).Where("user_id = ?", user.ID).
				UpdateColumn("icon_last_checked", time.Now()).Error; err != nil {
				t.Fatal(err)
			}
			user.DisplayName = "Person"

			var err error
			statements := counter.measure(func() {
				// The provider URL answers nothing, so asking the provider for the profile would fail.
				err = c.UpdateProfileIfNeeded(ctx, user, provider.Name, provider.Namespace, "http://127.0.0.1:1")
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(statements) != 1 {
				t.Fatalf("a profile check ran %d statements, want 1, the identity:\n%s", len(statements), strings.Join(statements, "\n"))
			}
		})
	}
}

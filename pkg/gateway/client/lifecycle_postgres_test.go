package client

import (
	"os"
	"strings"
	"testing"
	"uuid"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newPostgresLifecycleTestClient returns a client whose gateway database is a fresh PostgreSQL schema, where
// sessions of auth providers other than local auth can be deleted.
func newPostgresLifecycleTestClient(t *testing.T) *Client {
	t.Helper()

	dsn := os.Getenv(postgresUserLimitTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}

	admin, err := storageservices.New(storageservices.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("failed to open PostgreSQL admin connection: %v", err)
	}
	schema := "obot_lifecycle_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if err := admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		_ = admin.DB.SQLDB.Close()
		t.Fatalf("failed to create schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("failed to drop schema: %v", err)
		}
		_ = admin.DB.SQLDB.Close()
	})

	scoped, err := storageservices.New(storageservices.Config{DSN: postgresUserLimitTestDSN(t, dsn, schema)})
	if err != nil {
		t.Fatalf("failed to open scoped PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() { _ = scoped.DB.SQLDB.Close() })

	database, err := gatewaydb.New(scoped.DB.DB, scoped.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway database: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway database: %v", err)
	}

	return &Client{
		db:            database,
		storageClient: fake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build(),
	}
}

func TestDisabledUsersRefreshTokensAreDeletedWhenTheirSessionsCannotBe(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	ctx := t.Context()
	// The sessions table of the user's auth provider lacks the columns that deleting a user's sessions matches, so the
	// deletion fails.
	if err := c.storageClient.Create(ctx, &v1.AuthProvider{
		Namespace: lifecycleTestProvider.Namespace,
		Name:      lifecycleTestProvider.Name,
		Spec: v1.AuthProviderSpec{
			AuthProviderManifest: apitypes.AuthProviderManifest{
				PostgresTablePrefix: "broken_",
			},
		},
	}); err != nil {
		t.Fatalf("failed to create auth provider: %v", err)
	}
	if err := c.db.WithContext(ctx).Exec("CREATE TABLE broken_sessions (key TEXT)").Error; err != nil {
		t.Fatalf("failed to create sessions table: %v", err)
	}
	user := createLifecycleTestUser(t, c, "zoe", lifecycleTestProvider)
	if err := c.storageClient.Create(ctx, &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      "zoe-token",
		Spec: v1.OAuthTokenSpec{
			ClientID: "client",
			UserID:   user.ID,
		},
	}); err != nil {
		t.Fatalf("failed to create OAuth token: %v", err)
	}

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	assertOAuthTokenExists(t, c, "zoe-token", false)

	// The event stays undelivered, so ending the sessions is retried.
	events := lifecycleEvents(t, c, user.ID)
	if len(events) != 1 || events[0].DeliveredAt != nil || events[0].Attempts != 1 || !strings.Contains(events[0].LastError, "failed to end the sessions") {
		t.Fatalf("lifecycle events after delivery = %+v, want one undelivered event that failed to end sessions", events)
	}
}

func TestDisabledEventIsDeliveredWhenTheAuthProviderNoLongerExists(t *testing.T) {
	c := newPostgresLifecycleTestClient(t)
	ctx := t.Context()
	// The user's auth provider no longer exists, so it has no sessions table to delete their sessions from.
	user := createLifecycleTestUser(t, c, "zoe", lifecycleTestProvider)
	if err := c.storageClient.Create(ctx, &v1.OAuthToken{
		Namespace: system.DefaultNamespace,
		Name:      "zoe-token",
		Spec: v1.OAuthTokenSpec{
			ClientID: "client",
			UserID:   user.ID,
		},
	}); err != nil {
		t.Fatalf("failed to create OAuth token: %v", err)
	}

	if _, err := disableUser(t, c, lifecycleTestProvider, user.ID, types.UserDisabledReasonSCIMInactive); err != nil {
		t.Fatalf("failed to disable user: %v", err)
	}
	if err := c.deliverUserLifecycleEvents(ctx); err != nil {
		t.Fatalf("failed to deliver lifecycle events: %v", err)
	}

	assertOAuthTokenExists(t, c, "zoe-token", false)
	events := lifecycleEvents(t, c, user.ID)
	if len(events) != 1 || events[0].DeliveredAt == nil || events[0].LastError != "" {
		t.Fatalf("lifecycle events after delivery = %+v, want one delivered event", events)
	}
}

package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const postgresUserLimitTestDSNEnv = "OBOT_TEST_POSTGRES_DSN"

func TestEnsureIdentityWithRoleEnforcesUserLimitAcrossPostgresClients(t *testing.T) {
	postgresDSN := os.Getenv(postgresUserLimitTestDSNEnv)
	if postgresDSN == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}

	adminServices, err := storageservices.New(storageservices.Config{DSN: postgresDSN})
	if err != nil {
		t.Fatalf("opening PostgreSQL admin connection: %v", err)
	}

	schema := "obot_user_limit_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := adminServices.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		_ = adminServices.DB.SQLDB.Close()
		t.Fatalf("creating isolated PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		if err := adminServices.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("dropping isolated PostgreSQL schema: %v", err)
		}
		if err := adminServices.DB.SQLDB.Close(); err != nil {
			t.Errorf("closing PostgreSQL admin connection: %v", err)
		}
	})

	scopedDSN := postgresUserLimitTestDSN(t, postgresDSN, schema)
	dbA := newPostgresUserLimitTestDB(t, scopedDSN)
	dbB := newPostgresUserLimitTestDB(t, scopedDSN)
	if err := dbA.WithContext(t.Context()).AutoMigrate(&gatewaytypes.User{}, &gatewaytypes.Identity{}); err != nil {
		t.Fatalf("migrating PostgreSQL user-limit tables: %v", err)
	}

	const maximum = 1
	userLimit := UserLimit{Maximum: maximum}
	clientA := newPostgresUserLimitTestClient(dbA)
	clientB := newPostgresUserLimitTestClient(dbB)

	// Hold the production advisory lock so both independent gateway clients have
	// to reach and wait on it before either can perform the count-and-create.
	lockTx := adminServices.DB.DB.WithContext(t.Context()).Begin()
	if lockTx.Error != nil {
		t.Fatalf("starting advisory-lock transaction: %v", lockTx.Error)
	}
	lockHeld := true
	defer func() {
		if lockHeld {
			_ = lockTx.Rollback().Error
		}
	}()
	if err := lockTx.Exec("SELECT pg_advisory_xact_lock(?)", userCreationAdvisoryLockID).Error; err != nil {
		t.Fatalf("holding user-creation advisory lock: %v", err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	start := make(chan struct{})
	results := make(chan error, 2)
	for i, c := range []*Client{clientA, clientB} {
		go func(i int, c *Client) {
			<-start
			username := fmt.Sprintf("postgres-concurrent-user-%d", i)
			_, err := ensureUserLimitTestIdentity(ctx, c, username, username+"@example.com", userLimit)
			results <- err
		}(i, c)
	}
	close(start)

	if err := waitForPostgresUserLimitLockWaiters(ctx, adminServices.DB.DB, 2); err != nil {
		t.Fatalf("waiting for both gateway clients to contend on the advisory lock: %v", err)
	}

	if err := lockTx.Commit().Error; err != nil {
		t.Fatalf("releasing user-creation advisory lock: %v", err)
	}
	lockHeld = false

	var (
		succeeded int
		rejected  int
		rejectErr error
	)
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				succeeded++
				continue
			}

			var httpErr *apitypes.ErrHTTP
			if errors.As(err, &httpErr) && httpErr.Code == http.StatusForbidden {
				rejected++
				rejectErr = err
				continue
			}
			t.Fatalf("concurrent PostgreSQL user creation returned unexpected error: %v", err)
		case <-ctx.Done():
			t.Fatalf("concurrent PostgreSQL user creation did not finish: %v", ctx.Err())
		}
	}

	if succeeded != maximum {
		t.Fatalf("successful concurrent PostgreSQL creations = %d, want %d", succeeded, maximum)
	}
	if rejected != 1 {
		t.Fatalf("rejected concurrent PostgreSQL creations = %d, want 1", rejected)
	}
	requireIdentityUserLimitForbiddenError(t, rejectErr)
	if got := countIdentityUserLimitTestUsers(t, clientA, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}
	if got := countIdentityUserLimitTestIdentities(t, clientA); got != maximum {
		t.Fatalf("identities = %d, want %d; rejected identity was not rolled back", got, maximum)
	}
}

func postgresUserLimitTestDSN(t *testing.T, dsn, schema string) string {
	t.Helper()

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parsing %s: %v", postgresUserLimitTestDSNEnv, err)
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		t.Fatalf("%s must use a postgres:// or postgresql:// URL", postgresUserLimitTestDSNEnv)
	}

	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	return u.String()
}

func newPostgresUserLimitTestDB(t *testing.T, dsn string) *gatewaydb.DB {
	t.Helper()

	services, err := storageservices.New(storageservices.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("opening scoped PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() {
		if err := services.DB.SQLDB.Close(); err != nil {
			t.Errorf("closing scoped PostgreSQL connection: %v", err)
		}
	})

	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, false)
	if err != nil {
		t.Fatalf("creating gateway PostgreSQL database: %v", err)
	}
	return database
}

func newPostgresUserLimitTestClient(database *gatewaydb.DB) *Client {
	storageClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.DefaultNamespace,
				Name:      system.DefaultRoleSettingName,
			},
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: apitypes.RoleBasic,
			},
		}).
		Build()

	return &Client{
		db:            database,
		storageClient: storageClient,
	}
}

func waitForPostgresUserLimitLockWaiters(ctx context.Context, db *gorm.DB, want int64) error {
	const pollInterval = 10 * time.Millisecond

	lockClassID := uint64(userCreationAdvisoryLockID) >> 32
	lockObjectID := uint64(userCreationAdvisoryLockID) & 0xffffffff
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		var waiters int64
		if err := db.WithContext(ctx).Raw(`
			SELECT COUNT(*)
			FROM pg_locks
			WHERE locktype = 'advisory'
			  AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
			  AND classid = CAST(? AS oid)
			  AND objid = CAST(? AS oid)
			  AND objsubid = 1
			  AND NOT granted
		`, lockClassID, lockObjectID).Scan(&waiters).Error; err != nil {
			return err
		}
		if waiters == want {
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("found %d of %d lock waiters: %w", waiters, want, ctx.Err())
		case <-ticker.C:
		}
	}
}

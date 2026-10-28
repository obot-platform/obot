package llmengine

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Set OBOT_TEST_POSTGRES_DSN to a Postgres database the test may create databases next to.
func TestDerivePostgresDSNAndPostgresStores(t *testing.T) {
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	derived, err := DerivePostgresDSN(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(derived, "_llm_gateway") {
		t.Fatalf("unexpected derived DSN %q", derived)
	}
	// Deriving again finds the existing database.
	if again, err := DerivePostgresDSN(ctx, dsn); err != nil || again != derived {
		t.Fatalf("second derive: %q, %v", again, err)
	}

	e := New()
	if err := e.Start(ctx, Options{DataDir: t.TempDir(), PostgresDSN: derived}); err != nil {
		t.Fatal(err)
	}
	defer e.Shutdown()
	if code, body := do(t, e, "GET", "/api/providers", ""); code != 200 {
		t.Fatalf("GET /api/providers: %d %s", code, body)
	}
}

// Replicas starting together derive the gateway's database at the same time; every one must
// end up with it, whichever error the losing CREATE DATABASE reports.
func TestDerivePostgresDSNConcurrently(t *testing.T) {
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(context.Background())
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	for round := range 5 {
		// A fresh Obot database each round, so its gateway database doesn't exist yet.
		name := fmt.Sprintf("race_%d_%d", time.Now().UnixNano(), round)
		for _, db := range []string{name, name + "_llm_gateway"} {
			t.Cleanup(func() {
				_, _ = admin.Exec(context.Background(), "DROP DATABASE IF EXISTS "+pgx.Identifier{db}.Sanitize())
			})
		}
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		u.Path = "/" + name
		base := u.String()

		var wg sync.WaitGroup
		errs := make(chan error, 8)
		for range 8 {
			wg.Go(func() {
				if _, err := DerivePostgresDSN(ctx, base); err != nil {
					errs <- err
				}
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatalf("round %d: %v", round, err)
		}
	}
}

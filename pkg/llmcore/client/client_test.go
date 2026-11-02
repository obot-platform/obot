package client_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"github.com/maximhq/bifrost/core/schemas"
	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/llmcore/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStoreTransactions(t *testing.T) {
	for _, backend := range []string{"sqlite", "postgres"} {
		t.Run(backend, func(t *testing.T) {
			var dialector gorm.Dialector
			if backend == "sqlite" {
				dialector = sqlite.Open(filepath.Join(t.TempDir(), "store.db"))
			} else {
				dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
				}
				dialector = postgres.Open(dsn)
			}
			db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Discard})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			originalSQL := sqlDB
			t.Cleanup(func() { _ = originalSQL.Close() })
			if backend == "sqlite" {
				sqlDB.SetMaxOpenConns(1)
			} else {
				schema := "llmcore_" + uuid.NewString()[:8]
				require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
				// The test has a private pool and schema; production uses Obot's normal schema.
				parsed, err := url.Parse(os.Getenv("OBOT_TEST_POSTGRES_DSN"))
				require.NoError(t, err)
				query := parsed.Query()
				query.Set("search_path", schema)
				parsed.RawQuery = query.Encode()
				isolated, err := gorm.Open(postgres.Open(parsed.String()), &gorm.Config{Logger: logger.Discard})
				require.NoError(t, err)
				isolatedSQL, err := isolated.DB()
				require.NoError(t, err)
				t.Cleanup(func() { _ = isolatedSQL.Close() })
				original := db
				db = isolated
				sqlDB = isolatedSQL
				defer func() { _ = original.Exec("DROP SCHEMA " + schema + " CASCADE").Error }()
			}
			migrations, err := gatewaydb.New(db, sqlDB, true)
			require.NoError(t, err)
			require.NoError(t, migrations.AutoMigrate())
			store := client.New(db)
			const writers = 8
			var wg sync.WaitGroup
			errs := make(chan error, writers)
			for i := range writers {
				wg.Go(func() {
					errs <- store.Mutate(context.Background(), func(cfg *types.Config) error { cfg.Settings[fmt.Sprint(i)] = []byte(`true`); return nil })
				})
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				require.NoError(t, err)
			}
			cfg, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Len(t, cfg.Settings, writers)
			require.EqualValues(t, writers, cfg.Revision)
			err = store.Mutate(t.Context(), func(cfg *types.Config) error { delete(cfg.Settings, "0"); return fmt.Errorf("reject change") })
			require.ErrorContains(t, err, "reject change")
			after, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, cfg, after)
			require.NoError(t, migrations.AutoMigrate())
			after, err = store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, cfg, after, "rerunning migrations must preserve configuration and revision")
			if backend == "postgres" {
				injected := false
				var writeErr error
				require.NoError(t, db.Callback().Query().After("gorm:query").Register("test_snapshot", func(tx *gorm.DB) {
					if tx.Statement.Table != "llm_core_revision" || injected {
						return
					}
					injected = true
					writeErr = store.Mutate(t.Context(), func(latest *types.Config) error { latest.Settings["concurrent"] = []byte(`true`); return nil })
				}))
				snapshot, err := store.Load(t.Context())
				require.NoError(t, db.Callback().Query().Remove("test_snapshot"))
				require.NoError(t, err)
				require.True(t, injected)
				require.NoError(t, writeErr)
				require.Equal(t, cfg, snapshot, "one load must not mix pre-commit revision with post-commit settings")
				latest, err := store.Load(t.Context())
				require.NoError(t, err)
				require.Equal(t, cfg.Revision+1, latest.Revision)
				require.Contains(t, latest.Settings, "concurrent")
			}
			testInvalidKeyMutations(t, store)
		})
	}
}

// testInvalidKeyMutations exercises validation and rollback on both database backends.
func testInvalidKeyMutations(t *testing.T, store *client.Client) {
	t.Helper()
	require.NoError(t, store.PutProvider(t.Context(), "openai", schemas.ProviderConfig{}))
	require.NoError(t, store.PutKey(t.Context(), "openai", schemas.Key{
		ID:    "existing-key",
		Name:  "existing",
		Value: *schemas.NewSecretVar("existing-secret"),
	}))
	require.NoError(t, store.PutRule(t.Context(), configstoreTables.TableRoutingRule{
		ID:    "existing-rule",
		Scope: "global",
	}))
	for _, tc := range []struct {
		name      string
		change    func(*types.Config)
		errorText string
	}{
		{
			name: "empty key ID",
			change: func(cfg *types.Config) {
				cfg.Keys[schemas.OpenAI] = append(cfg.Keys[schemas.OpenAI], schemas.Key{Name: "invalid"})
			},
			errorText: "a key needs an ID",
		},
		{
			name: "removed provider with retained keys",
			change: func(cfg *types.Config) {
				delete(cfg.Providers, schemas.OpenAI)
			},
			errorText: "provider \"openai\" not found",
		},
		{
			name: "key for unknown provider",
			change: func(cfg *types.Config) {
				cfg.Keys[schemas.Anthropic] = []schemas.Key{{ID: "orphan-key"}}
			},
			errorText: "provider \"anthropic\" not found",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := store.Load(t.Context())
			require.NoError(t, err)
			err = store.Mutate(t.Context(), func(cfg *types.Config) error {
				cfg.Settings["must-not-commit"] = json.RawMessage(`true`)
				tc.change(cfg)
				return nil
			})
			require.ErrorContains(t, err, tc.errorText)
			after, err := store.Load(t.Context())
			require.NoError(t, err)
			require.Equal(t, before, after, "rejected mutation must preserve all configuration and the revision")
		})
	}
}

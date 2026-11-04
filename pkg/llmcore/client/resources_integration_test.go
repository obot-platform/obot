package client_test

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"k8s.io/client-go/util/retry"

	"github.com/google/uuid"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/obot-platform/obot/pkg/llmcore/client"
	"github.com/obot-platform/obot/pkg/storage"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Exercise the real resource API, including its optimistic locking and watch stream.
func TestResourceAPI(t *testing.T) {
	testResourceAPI(t, "sqlite://file:"+filepath.Join(t.TempDir(), "resources.db")+"?_journal=WAL&_busy_timeout=30000")
}

func testResourceAPI(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	api, _, db, _, err := storage.Start(ctx, sservices.Config{DSN: dsn, StorageToken: "test"})
	require.NoError(t, err)
	defer db.SQLDB.Close()
	store := newStore(t, api)
	watch, err := api.Watch(ctx, &v1.LLMGatewayProviderList{}, kclient.InNamespace(system.DefaultNamespace))
	require.NoError(t, err)
	defer watch.Stop()
	require.NoError(t, store.CreateProvider(ctx, "openai", client.Provider{}))
	select {
	case event := <-watch.ResultChan():
		require.NotNil(t, event.Object)
	case <-time.After(10 * time.Second):
		t.Fatal("resource watch missed provider creation")
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"one", "two"} {
		wg.Go(func() {
			errs <- retry.RetryOnConflict(retry.DefaultRetry, func() error { return store.PutKey(ctx, "openai", schemas.Key{ID: id}) })
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	cfg, err := store.Load(ctx)
	require.NoError(t, err)
	require.Len(t, cfg.Config.Keys["openai"], 2)
	require.NoError(t, store.DeleteProvider(ctx, "openai"))
	require.NoError(t, store.CreateProvider(ctx, "openai", client.Provider{}))
	cfg, err = store.Load(ctx)
	require.NoError(t, err)
	require.Empty(t, cfg.Config.Keys["openai"])
}

func TestResourceAPIPostgres(t *testing.T) {
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	admin, err := sservices.New(sservices.Config{DSN: dsn})
	require.NoError(t, err)
	name := "llm_resources_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, admin.DB.DB.Exec("CREATE SCHEMA "+name).Error)
	defer admin.DB.SQLDB.Close()
	defer func() { require.NoError(t, admin.DB.DB.Exec("DROP SCHEMA "+name+" CASCADE").Error) }()
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	testResourceAPI(t, u.String())
}

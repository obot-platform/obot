package client

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

// newPostgresAuditBodyTestClients returns clients that share one fresh PostgreSQL schema but hold
// separate connection pools and in-memory state, as separate Obot replicas do.
func newPostgresAuditBodyTestClients(t *testing.T, replicas int) []*Client {
	t.Helper()

	dsn := os.Getenv(postgresUserLimitTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresUserLimitTestDSNEnv)
	}

	admin, err := storageservices.New(storageservices.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("failed to open PostgreSQL admin connection: %v", err)
	}
	schema := "obot_audit_body_" + strings.ReplaceAll(uuid.New().String(), "-", "")
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

	scopedDSN := postgresUserLimitTestDSN(t, dsn, schema)
	clients := make([]*Client, replicas)
	for i := range clients {
		scoped, err := storageservices.New(storageservices.Config{DSN: scopedDSN})
		if err != nil {
			t.Fatalf("failed to open scoped PostgreSQL connection: %v", err)
		}
		t.Cleanup(func() { _ = scoped.DB.SQLDB.Close() })
		database, err := gatewaydb.New(scoped.DB.DB, scoped.DB.SQLDB, true)
		if err != nil {
			t.Fatalf("failed to create gateway database: %v", err)
		}
		if i == 0 {
			if err := database.AutoMigrate(); err != nil {
				t.Fatalf("failed to migrate gateway database: %v", err)
			}
		}
		clients[i] = &Client{
			db:                      database,
			auditLogDeleteBatchSize: 100,
		}
	}
	return clients
}

func TestPostgresReplicasRacingToCreateTheHashKeyAgreeOnOne(t *testing.T) {
	clients := newPostgresAuditBodyTestClients(t, 8)

	keys := make([][]byte, len(clients))
	errs := make([]error, len(clients))
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Go(func() { keys[i], errs[i] = c.auditBodyHashKey(t.Context()) })
	}
	wg.Wait()

	for i := range clients {
		if errs[i] != nil {
			t.Fatalf("replica %d failed to load the hash key: %v", i, errs[i])
		}
		if !bytes.Equal(keys[i], keys[0]) {
			t.Fatalf("replica %d uses a different hash key than replica 0", i)
		}
	}
}

func TestPostgresConcurrentWritersStoreOneSharedBody(t *testing.T) {
	clients := newPostgresAuditBodyTestClients(t, 8)
	result := testListResult("concurrent", 16384)

	errs := make([]error, len(clients))
	var wg sync.WaitGroup
	for i, c := range clients {
		wg.Go(func() {
			entries := listCallEntries(i+1, "tools/list", testListResponse(i+1, result))
			for j := range entries {
				entries[j].NormalizeMCPFields()
				if errs[i] = c.encryptMCPAuditLog(t.Context(), &entries[j]); errs[i] != nil {
					return
				}
			}
			errs[i] = c.insertMCPAuditLogs(t.Context(), entries)
		})
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("replica %d failed to write: %v", i, err)
		}
	}

	if got := countAuditBodies(t, clients[0]); got != 1 {
		t.Fatalf("stored %d shared bodies, want 1", got)
	}
	logs := readMCPAuditLogs(t, clients[0])
	if len(logs) != len(clients) {
		t.Fatalf("read %d rows, want %d", len(logs), len(clients))
	}
	for _, log := range logs {
		if len(log.UnavailablePayloads) != 0 || !strings.Contains(string(log.MCP().ResponseBody), result) {
			t.Fatalf("row %d did not read its shared body: unavailable=%v", log.ID, log.UnavailablePayloads)
		}
	}
}

// staleAuditBodyTime is old enough for retention with a one-day period to delete a shared body.
func staleAuditBodyTime() time.Time {
	return time.Now().UTC().AddDate(0, 0, -30)
}

// A writer that references a body old enough to delete refreshes it under a row lock. Cleanup that
// arrives meanwhile must wait, see the refreshed time, and leave the body alone.
func TestPostgresCleanupSkipsBodyRefreshedByUncommittedWriter(t *testing.T) {
	clients := newPostgresAuditBodyTestClients(t, 2)
	writer, cleaner := clients[0], clients[1]
	ctx := t.Context()
	response := testListResponse(1, testListResult("locked", 4096))
	storeMCPAuditEntries(t, writer, listCallEntries(1, "tools/list", response)...)
	setAuditBodyLastReferenced(t, writer, staleAuditBodyTime())
	body := loadAuditBody(t, writer)

	now := time.Now().UTC()
	tx := writer.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if err := storeAuditBodies(tx, []auditBodyWrite{{body: types.MCPAuditLogBody{Hash: body.Hash, Body: body.Body, CreatedAt: now, LastReferencedAt: now}}}); err != nil {
		t.Fatalf("refresh shared body: %v", err)
	}

	cleaned := make(chan error, 1)
	go func() { cleaned <- cleaner.deleteUnreferencedMCPAuditLogBodies(ctx, now, 1) }()
	select {
	case err := <-cleaned:
		t.Fatalf("cleanup did not wait for the writer holding the body's row lock (err=%v)", err)
	case <-time.After(500 * time.Millisecond):
	}

	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-cleaned; err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if got := countAuditBodies(t, writer); got != 1 {
		t.Fatalf("cleanup deleted a body that a writer had just refreshed: %d remain", got)
	}
}

// When cleanup deletes a stale body first, a writer referencing it must wait and then store the
// body again rather than commit a row that points at nothing.
func TestPostgresWriterRestoresBodyDeletedByUncommittedCleanup(t *testing.T) {
	clients := newPostgresAuditBodyTestClients(t, 2)
	writer, cleaner := clients[0], clients[1]
	ctx := t.Context()
	result := testListResult("deleted", 4096)
	storeMCPAuditEntries(t, writer, listCallEntries(1, "tools/list", testListResponse(1, result))...)
	setAuditBodyLastReferenced(t, writer, staleAuditBodyTime())

	var deleted int64
	tx := cleaner.db.WithContext(ctx).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer tx.Rollback()
	if result := tx.Where("1 = 1").Delete(&types.MCPAuditLogBody{}); result.Error != nil {
		t.Fatal(result.Error)
	} else {
		deleted = result.RowsAffected
	}
	if deleted != 1 {
		t.Fatalf("cleanup transaction deleted %d bodies, want 1", deleted)
	}

	second := testListResponse(2, result)
	written := make(chan error, 1)
	go func() {
		entries := listCallEntries(2, "tools/list", second)
		for i := range entries {
			entries[i].NormalizeMCPFields()
			if err := writer.encryptMCPAuditLog(ctx, &entries[i]); err != nil {
				written <- err
				return
			}
		}
		written <- writer.insertMCPAuditLogs(ctx, entries)
	}()
	select {
	case err := <-written:
		t.Fatalf("writer did not wait for the cleanup holding the body's row lock (err=%v)", err)
	case <-time.After(500 * time.Millisecond):
	}

	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if err := <-written; err != nil {
		t.Fatalf("writer: %v", err)
	}

	if got := countAuditBodies(t, writer); got != 1 {
		t.Fatalf("writer did not restore the deleted body: %d stored", got)
	}
	logs := readMCPAuditLogs(t, writer)
	for _, log := range logs {
		if len(log.UnavailablePayloads) != 0 {
			t.Fatalf("row %d references a body that is gone", log.ID)
		}
	}
	if !bytes.Equal(logs[len(logs)-1].MCP().ResponseBody, second) {
		t.Fatal("row written during cleanup did not read back")
	}
}

// Writers and cleanup run freely against a body that is always old enough to delete. However
// they interleave, no committed row may be left without its body.
func TestPostgresWritersRacingCleanupNeverLoseABody(t *testing.T) {
	const (
		writers = 4
		rounds  = 25
	)
	clients := newPostgresAuditBodyTestClients(t, writers+1)
	cleaner := clients[writers]
	ctx := t.Context()
	result := testListResult("raced", 4096)

	for round := range rounds {
		// Forget what each replica knows so every write decides from the database.
		for _, c := range clients {
			c.auditBodies.known = nil
		}
		if err := cleaner.db.WithContext(ctx).Session(&gorm.Session{AllowGlobalUpdate: true}).
			Model(&types.MCPAuditLogBody{}).Update("last_referenced_at", staleAuditBodyTime()).Error; err != nil {
			t.Fatal(err)
		}

		errs := make([]error, writers+1)
		var wg sync.WaitGroup
		for i, c := range clients[:writers] {
			wg.Go(func() {
				id := round*writers + i + 1
				entries := listCallEntries(id, "tools/list", testListResponse(id, result))
				for j := range entries {
					entries[j].NormalizeMCPFields()
					if errs[i] = c.encryptMCPAuditLog(ctx, &entries[j]); errs[i] != nil {
						return
					}
				}
				errs[i] = c.insertMCPAuditLogs(ctx, entries)
			})
		}
		wg.Go(func() { errs[writers] = cleaner.deleteUnreferencedMCPAuditLogBodies(ctx, time.Now().UTC(), 1) })
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("round %d: participant %d failed: %v", round, i, err)
			}
		}
	}

	logs := readMCPAuditLogs(t, cleaner)
	if len(logs) != writers*rounds {
		t.Fatalf("read %d rows, want %d", len(logs), writers*rounds)
	}
	for _, log := range logs {
		if len(log.UnavailablePayloads) != 0 {
			t.Fatalf("row %d was committed without its shared body", log.ID)
		}
	}
}

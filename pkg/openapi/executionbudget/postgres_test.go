package executionbudget

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	"gorm.io/gorm"
)

func TestOpenAPIBudgetPostgresLifecycle(t *testing.T) {
	db := isolatedPostgres(t)
	config := DefaultConfig()
	config.PlatformMaxInFlight = 2
	config.DefaultPoolMaxInFlight = 2
	config.DedicatedPoolMaxInFlight = 2
	config.DefaultOperationInFlight = 1
	config.AdmissionInterval = time.Millisecond
	manager, err := NewManager(db, config)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ResolveBudgetIdentity([]CatalogReference{{SourceURL: "https://api.example.test/v1", EntryKey: "api"}}, nil, "GET /pets")
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	first, err := manager.Acquire(ctx, AcquireRequest{Identity: identity, Deadline: time.Now().Add(3 * time.Second), ObotInstanceID: "obot-1"})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := manager.enqueue(ctx, AcquireRequest{
		Identity: identity, Deadline: time.Now().Add(3 * time.Second), ObotInstanceID: "obot-1",
	}, "request-waiting", "token-waiting", time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if waiting.State != types.OpenAPIRequestStateWaiting {
		t.Fatalf("expected waiting request, got %s", waiting.State)
	}
	if _, err = manager.enqueue(ctx, AcquireRequest{
		Identity: identity, Deadline: time.Now().Add(3 * time.Second),
	}, "request-waiting", "token", time.Now().Add(time.Second)); !errors.Is(err, ErrRequestIDConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err = manager.CancelWaiting(ctx, "request-waiting", "wrong"); !errors.Is(err, ErrInvalidReleaseToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
	if err = manager.CancelWaiting(ctx, "request-waiting", "token-waiting"); err != nil {
		t.Fatal(err)
	}
	if err = manager.CancelWaiting(ctx, "request-waiting", "token-waiting"); err != nil {
		t.Fatal(err)
	}
	if err = manager.Release(ctx, first.Context.LeaseID, first.ReleaseToken, OutcomeTransportUnknown); !errors.Is(err, ErrUncertainOutcome) {
		t.Fatalf("uncertain transport must not immediately release: %v", err)
	}
	if err = manager.Release(ctx, first.Context.LeaseID, first.ReleaseToken, OutcomeSnapshotRequired); err != nil {
		t.Fatalf("snapshot miss must retain the active lease: %v", err)
	}
	var retainedRequest types.OpenAPIExecutionRequest
	if err = db.Where("request_id = ?", first.Context.LeaseID).Take(&retainedRequest).Error; err != nil {
		t.Fatal(err)
	}
	if retainedRequest.State != types.OpenAPIRequestStateActive {
		t.Fatalf("snapshot miss changed lease state: %s", retainedRequest.State)
	}
	if err = manager.Release(ctx, first.Context.LeaseID, "wrong", OutcomeBusinessResult); !errors.Is(err, ErrInvalidReleaseToken) {
		t.Fatalf("expected release-token rejection, got %v", err)
	}
	if err = manager.Release(ctx, first.Context.LeaseID, first.ReleaseToken, OutcomeBusinessResult); err != nil {
		t.Fatal(err)
	}
	if err = manager.Release(ctx, first.Context.LeaseID, first.ReleaseToken, OutcomeBusinessResult); err != nil {
		t.Fatal(err)
	}

	if err = db.Model(&types.OpenAPIExecutionRequest{}).Where("request_id = ?", first.Context.LeaseID).
		Updates(map[string]any{"wait_deadline": time.Now().Add(-2 * time.Second), "deadline": time.Now().Add(-2 * time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	active, err := manager.Acquire(ctx, AcquireRequest{Identity: identity, Deadline: time.Now().Add(2 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&types.OpenAPIExecutionRequest{}).Where("request_id = ?", active.Context.LeaseID).
		Updates(map[string]any{"wait_deadline": time.Now().Add(-2 * time.Second), "deadline": time.Now().Add(-2 * time.Second)}).Error; err != nil {
		t.Fatal(err)
	}
	reclaimed, err := manager.ReclaimExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed != 1 {
		t.Fatalf("expected one grace reclaim, got %d", reclaimed)
	}
	var operationStates int64
	if err = db.Model(&types.OpenAPIBudgetState{}).Where("scope = ?", types.OpenAPIBudgetScopeOperation).Count(&operationStates).Error; err != nil {
		t.Fatal(err)
	}
	if operationStates != 0 {
		t.Fatalf("idle operation runtime state was not deleted: %d", operationStates)
	}
}

func TestOpenAPIBudgetPostgresFairnessAndCrossOperationLimits(t *testing.T) {
	db := isolatedPostgres(t)
	config := DefaultConfig()
	config.PlatformMaxInFlight = 1
	config.PlatformMaxWaiting = 3
	config.DefaultPoolMaxInFlight = 1
	config.DedicatedPoolMaxInFlight = 1
	config.DefaultOperationInFlight = 1
	config.DefaultOperationWaiting = 2
	manager, err := NewManager(db, config)
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := budgetIdentityForPath(t, "first")
	secondIdentity := budgetIdentityForPath(t, "second")
	ctx := context.Background()
	firstRequest := enqueueOperation(t, manager, firstIdentity, "request-1", "token-1")
	secondRequest := enqueueOperation(t, manager, secondIdentity, "request-2", "token-2")
	thirdRequest := enqueueOperation(t, manager, secondIdentity, "request-3", "token-3")
	if _, err = manager.enqueue(ctx, AcquireRequest{Identity: secondIdentity, Deadline: time.Now().Add(3 * time.Second)}, "request-4", "token-4", time.Now().Add(time.Second)); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("expected operation queue full, got %v", err)
	}
	if _, err = manager.AdmitOne(ctx); err != nil {
		t.Fatal(err)
	}
	var activeFirst types.OpenAPIExecutionRequest
	if err = db.Where("request_id = ?", firstRequest).Take(&activeFirst).Error; err != nil {
		t.Fatal(err)
	}
	if activeFirst.State != types.OpenAPIRequestStateActive {
		t.Fatalf("oldest operation was not admitted first: %#v", activeFirst)
	}
	admitted, err := manager.AdmitOne(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if admitted {
		t.Fatal("second operation unexpectedly exceeded platform in-flight limit")
	}
	var waitingSecond types.OpenAPIExecutionRequest
	if err = db.Where("request_id = ?", secondRequest).Take(&waitingSecond).Error; err != nil {
		t.Fatal(err)
	}
	if waitingSecond.State != types.OpenAPIRequestStateWaiting {
		t.Fatalf("second operation did not remain queued: %#v", waitingSecond)
	}
	if err = db.Model(&types.OpenAPIExecutionRequest{}).Where("request_id = ?", firstRequest).
		Updates(map[string]any{"state": types.OpenAPIRequestStateCompleted, "finished_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err = manager.ReconcileAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.AdmitOne(ctx); err != nil {
		t.Fatal(err)
	}
	var activeSecond types.OpenAPIExecutionRequest
	if err = db.Where("request_id = ?", secondRequest).Take(&activeSecond).Error; err != nil {
		t.Fatal(err)
	}
	if activeSecond.State != types.OpenAPIRequestStateActive {
		t.Fatalf("released platform slot did not admit the next operation: %#v", activeSecond)
	}
	if err = db.Model(&types.OpenAPIExecutionRequest{}).Where("request_id = ?", secondRequest).
		Updates(map[string]any{"state": types.OpenAPIRequestStateCompleted, "finished_at": time.Now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err = manager.ReconcileAll(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = manager.AdmitOne(ctx); err != nil {
		t.Fatal(err)
	}
	var activeThird types.OpenAPIExecutionRequest
	if err = db.Where("request_id = ?", thirdRequest).Take(&activeThird).Error; err != nil {
		t.Fatal(err)
	}
	if activeThird.State != types.OpenAPIRequestStateActive {
		t.Fatalf("released platform slot did not admit the next ticket: %#v", activeThird)
	}
}

func TestOpenAPIBudgetPostgresConcurrentAcquireRespectsLimit(t *testing.T) {
	db := isolatedPostgres(t)
	config := DefaultConfig()
	config.PlatformMaxInFlight = 2
	config.DefaultPoolMaxInFlight = 2
	config.DedicatedPoolMaxInFlight = 2
	config.DefaultOperationInFlight = 2
	config.DefaultOperationWaiting = 8
	config.AdmissionInterval = time.Millisecond
	manager, err := NewManager(db, config)
	if err != nil {
		t.Fatal(err)
	}
	identity := budgetIdentityForPath(t, "concurrent")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	const callers = 8
	results := make(chan error, callers)
	var started sync.WaitGroup
	started.Add(callers)
	for index := range callers {
		go func(index int) {
			started.Done()
			acquisition, err := manager.Acquire(ctx, AcquireRequest{Identity: identity, Deadline: time.Now().Add(2 * time.Second), ObotInstanceID: fmt.Sprintf("obot-%d", index%2)})
			if err == nil {
				defer func() {
					_ = manager.Release(context.WithoutCancel(ctx), acquisition.Context.LeaseID, acquisition.ReleaseToken, OutcomeBusinessResult)
				}()
			}
			results <- err
		}(index)
	}
	started.Wait()
	var succeeded, queuedOut int
	for range callers {
		if err := <-results; err == nil {
			succeeded++
		} else if errors.Is(err, ErrWaitDeadlineExceeded) || errors.Is(err, ErrQueueFull) {
			queuedOut++
		} else {
			t.Fatalf("unexpected acquisition error: %v", err)
		}
	}
	if succeeded < 2 || succeeded+queuedOut != callers {
		t.Fatalf("unexpected acquisition distribution: succeeded=%d queuedOut=%d", succeeded, queuedOut)
	}
}

func isolatedPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("OBOT_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("OBOT_TEST_POSTGRES_DSN is not set")
	}
	admin, err := storageservices.New(storageservices.Config{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	schema := "openapi_budget_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err = admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		_ = admin.DB.SQLDB.Close()
	})
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	services, err := storageservices.New(storageservices.Config{DSN: parsed.String()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = services.DB.SQLDB.Close() })
	if err = gatewaydb.AutoMigrateOpenAPIBudgets(services.DB.DB); err != nil {
		t.Fatal(err)
	}
	return services.DB.DB
}

func budgetIdentityForPath(t *testing.T, path string) BudgetIdentity {
	t.Helper()
	identity, err := ResolveBudgetIdentity([]CatalogReference{{SourceURL: "https://api.example.test/" + path, EntryKey: "api"}}, nil, "GET /"+path)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func enqueueOperation(t *testing.T, manager *Manager, identity BudgetIdentity, requestID, token string) string {
	t.Helper()
	request, err := manager.enqueue(context.Background(), AcquireRequest{
		Identity: identity, Deadline: time.Now().Add(3 * time.Second), ObotInstanceID: "obot-test",
	}, requestID, token, time.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	return request.RequestID
}

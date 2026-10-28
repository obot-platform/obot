package client

import (
	"errors"
	"os"
	"testing"
	"time"
)

// Replicas take turns: while one holds the LLM gateway lock, another waits, and it gives up
// with errLLMGatewayLockBusy once the wait runs out. Set OBOT_TEST_POSTGRES_DSN to run it.
func TestLLMGatewayLockAcrossPostgresClients(t *testing.T) {
	dsn := os.Getenv(postgresUserLimitTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL", postgresUserLimitTestDSNEnv)
	}
	a := &Client{db: newPostgresUserLimitTestDB(t, dsn)}
	b := &Client{db: newPostgresUserLimitTestDB(t, dsn)}

	held, release := make(chan struct{}), make(chan struct{})
	aDone := make(chan error, 1)
	go func() {
		aDone <- a.WithLLMGatewayLock(t.Context(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held

	// While a holds the lock, b waits.
	bRan := make(chan time.Time, 1)
	bDone := make(chan error, 1)
	go func() {
		bDone <- b.WithLLMGatewayLock(t.Context(), func() error {
			bRan <- time.Now()
			return nil
		})
	}()
	time.Sleep(500 * time.Millisecond)
	released := time.Now()
	close(release)
	if err := <-aDone; err != nil {
		t.Fatal(err)
	}
	if err := <-bDone; err != nil {
		t.Fatal(err)
	}
	if ran := <-bRan; ran.Before(released) {
		t.Fatalf("b ran at %s, before a released the lock at %s", ran, released)
	}

	// b gives up when a holds the lock for longer than b may wait.
	old := llmGatewayLockWait
	llmGatewayLockWait = 300 * time.Millisecond
	defer func() { llmGatewayLockWait = old }()
	held, release = make(chan struct{}), make(chan struct{})
	go func() {
		aDone <- a.WithLLMGatewayLock(t.Context(), func() error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	err := b.WithLLMGatewayLock(t.Context(), func() error {
		t.Error("b ran while a held the lock")
		return nil
	})
	close(release)
	if !errors.Is(err, errLLMGatewayLockBusy) {
		t.Fatalf("err = %v, want errLLMGatewayLockBusy", err)
	}
	if err := <-aDone; err != nil {
		t.Fatal(err)
	}
}

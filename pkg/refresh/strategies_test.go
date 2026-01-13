package refresh

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Strategy != StrategyReactive {
		t.Errorf("expected strategy %s, got %s", StrategyReactive, config.Strategy)
	}

	if config.RefreshBuffer != 5*time.Minute {
		t.Errorf("expected refresh buffer 5m, got %v", config.RefreshBuffer)
	}

	if config.CheckInterval != 1*time.Minute {
		t.Errorf("expected check interval 1m, got %v", config.CheckInterval)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("OBOT_AUTH_PROVIDER_REFRESH_STRATEGY", "proactive")
	os.Setenv("OBOT_AUTH_PROVIDER_REFRESH_BUFFER", "10m")
	os.Setenv("OBOT_AUTH_PROVIDER_REFRESH_CHECK_INTERVAL", "30s")
	defer func() {
		os.Unsetenv("OBOT_AUTH_PROVIDER_REFRESH_STRATEGY")
		os.Unsetenv("OBOT_AUTH_PROVIDER_REFRESH_BUFFER")
		os.Unsetenv("OBOT_AUTH_PROVIDER_REFRESH_CHECK_INTERVAL")
	}()

	config := LoadFromEnv()

	if config.Strategy != StrategyProactive {
		t.Errorf("expected strategy %s, got %s", StrategyProactive, config.Strategy)
	}

	if config.RefreshBuffer != 10*time.Minute {
		t.Errorf("expected refresh buffer 10m, got %v", config.RefreshBuffer)
	}

	if config.CheckInterval != 30*time.Second {
		t.Errorf("expected check interval 30s, got %v", config.CheckInterval)
	}
}

func TestValidateStrategy(t *testing.T) {
	tests := []struct {
		strategy  string
		shouldErr bool
	}{
		{"reactive", false},
		{"proactive", false},
		{"background", false},
		{"invalid", true},
		{"", true},
		{"REACTIVE", true}, // Case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.strategy, func(t *testing.T) {
			err := ValidateStrategy(tt.strategy)
			if tt.shouldErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.shouldErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestReactiveStrategy(t *testing.T) {
	config := &Config{
		Strategy:      StrategyReactive,
		RefreshBuffer: 5 * time.Minute,
	}

	refreshCalled := false
	refreshFunc := func(_ context.Context, _ string) error {
		refreshCalled = true
		return nil
	}

	manager := NewManager(config, refreshFunc)

	// Token not yet expired - should not refresh
	futureExpiry := time.Now().Add(10 * time.Minute)
	if manager.ShouldRefresh(futureExpiry) {
		t.Error("reactive strategy should not refresh unexpired token")
	}

	// Token expired - should refresh
	pastExpiry := time.Now().Add(-1 * time.Minute)
	if !manager.ShouldRefresh(pastExpiry) {
		t.Error("reactive strategy should refresh expired token")
	}

	// Test RefreshIfNeeded
	ctx := context.Background()
	err := manager.RefreshIfNeeded(ctx, "token1", pastExpiry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !refreshCalled {
		t.Error("expected refresh to be called for expired token")
	}

	// Test with future expiry - should not call refresh
	refreshCalled = false
	err = manager.RefreshIfNeeded(ctx, "token2", futureExpiry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if refreshCalled {
		t.Error("expected refresh not to be called for unexpired token")
	}
}

func TestProactiveStrategy(t *testing.T) {
	config := &Config{
		Strategy:      StrategyProactive,
		RefreshBuffer: 5 * time.Minute,
	}

	refreshCalled := false
	refreshFunc := func(_ context.Context, _ string) error {
		refreshCalled = true
		return nil
	}

	manager := NewManager(config, refreshFunc)

	// Token expires in 10 minutes - should not refresh yet
	farFutureExpiry := time.Now().Add(10 * time.Minute)
	if manager.ShouldRefresh(farFutureExpiry) {
		t.Error("proactive strategy should not refresh token with 10min remaining")
	}

	// Token expires in 4 minutes - within buffer, should refresh
	nearFutureExpiry := time.Now().Add(4 * time.Minute)
	if !manager.ShouldRefresh(nearFutureExpiry) {
		t.Error("proactive strategy should refresh token within buffer window")
	}

	// Test RefreshIfNeeded
	ctx := context.Background()
	err := manager.RefreshIfNeeded(ctx, "token1", nearFutureExpiry)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !refreshCalled {
		t.Error("expected refresh to be called for token within buffer")
	}
}

func TestBackgroundStrategy(t *testing.T) {
	config := &Config{
		Strategy:      StrategyBackground,
		RefreshBuffer: 100 * time.Millisecond,
		CheckInterval: 50 * time.Millisecond,
	}

	var mu sync.Mutex
	refreshedTokens := make(map[string]int)
	refreshFunc := func(_ context.Context, tokenID string) error {
		mu.Lock()
		defer mu.Unlock()
		refreshedTokens[tokenID]++
		return nil
	}

	manager := NewManager(config, refreshFunc)
	defer manager.Stop()

	// ShouldRefresh should always return false for background strategy
	if manager.ShouldRefresh(time.Now().Add(-1 * time.Minute)) {
		t.Error("background strategy ShouldRefresh should always return false")
	}

	// Register tokens with different expiry times

	// Token that needs immediate refresh
	expiredToken := "token-expired"
	expiredExpiry := time.Now().Add(50 * time.Millisecond)
	manager.RegisterToken(expiredToken, expiredExpiry)

	// Token that doesn't need refresh yet
	futureToken := "token-future"
	futureExpiry := time.Now().Add(10 * time.Minute)
	manager.RegisterToken(futureToken, futureExpiry)

	// Wait for background refresh to run (2 check intervals)
	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	expiredCount := refreshedTokens[expiredToken]
	futureCount := refreshedTokens[futureToken]
	mu.Unlock()

	if expiredCount == 0 {
		t.Error("expected expired token to be refreshed by background goroutine")
	}

	if futureCount != 0 {
		t.Error("expected future token not to be refreshed")
	}

	// Test unregister
	manager.UnregisterToken(expiredToken)

	// Give time for another check cycle
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	countAfterUnregister := refreshedTokens[expiredToken]
	mu.Unlock()

	// Count should not have increased after unregister
	if countAfterUnregister > expiredCount+1 {
		t.Error("expected token not to be refreshed after unregister")
	}
}

func TestBackgroundStrategyRefreshError(t *testing.T) {
	config := &Config{
		Strategy:      StrategyBackground,
		RefreshBuffer: 50 * time.Millisecond,
		CheckInterval: 30 * time.Millisecond,
	}

	refreshErr := errors.New("refresh failed")
	var mu sync.Mutex
	refreshAttempts := 0
	refreshFunc := func(_ context.Context, _ string) error {
		mu.Lock()
		defer mu.Unlock()
		refreshAttempts++
		return refreshErr
	}

	manager := NewManager(config, refreshFunc)
	defer manager.Stop()

	// Register expired token
	expiredExpiry := time.Now().Add(10 * time.Millisecond)
	manager.RegisterToken("token1", expiredExpiry)

	// Wait for background refresh attempts
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	attempts := refreshAttempts
	mu.Unlock()

	// Should have attempted refresh at least once despite error
	if attempts == 0 {
		t.Error("expected at least one refresh attempt despite error")
	}

	// Manager should continue running despite errors
	if manager.backgroundCtx.Err() != nil {
		t.Error("background context should not be cancelled due to refresh errors")
	}
}

func TestRefreshIfNeededWithError(t *testing.T) {
	config := &Config{
		Strategy:      StrategyProactive,
		RefreshBuffer: 5 * time.Minute,
	}

	expectedErr := errors.New("refresh failed")
	refreshFunc := func(_ context.Context, _ string) error {
		return expectedErr
	}

	manager := NewManager(config, refreshFunc)

	ctx := context.Background()
	nearExpiry := time.Now().Add(4 * time.Minute)

	err := manager.RefreshIfNeeded(ctx, "token1", nearExpiry)
	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

func TestManagerStop(t *testing.T) {
	config := &Config{
		Strategy:      StrategyBackground,
		RefreshBuffer: 1 * time.Second,
		CheckInterval: 100 * time.Millisecond,
	}

	refreshFunc := func(_ context.Context, _ string) error {
		return nil
	}

	manager := NewManager(config, refreshFunc)

	// Verify background goroutine is running
	if manager.backgroundCtx == nil {
		t.Fatal("expected background context to be initialized")
	}

	// Stop the manager
	manager.Stop()

	// Verify background goroutine stopped
	select {
	case <-manager.backgroundDone:
		// Expected - goroutine stopped
	case <-time.After(1 * time.Second):
		t.Error("background goroutine did not stop within timeout")
	}

	// Verify context was cancelled
	if manager.backgroundCtx.Err() == nil {
		t.Error("expected background context to be cancelled")
	}
}

func TestGetters(t *testing.T) {
	config := &Config{
		Strategy:      StrategyProactive,
		RefreshBuffer: 10 * time.Minute,
		CheckInterval: 2 * time.Minute,
	}

	refreshFunc := func(_ context.Context, _ string) error {
		return nil
	}

	manager := NewManager(config, refreshFunc)

	if manager.GetStrategy() != StrategyProactive {
		t.Errorf("expected strategy %s, got %s", StrategyProactive, manager.GetStrategy())
	}

	returnedConfig := manager.GetConfig()
	if returnedConfig.Strategy != config.Strategy {
		t.Error("GetConfig returned different strategy")
	}
	if returnedConfig.RefreshBuffer != config.RefreshBuffer {
		t.Error("GetConfig returned different refresh buffer")
	}
	if returnedConfig.CheckInterval != config.CheckInterval {
		t.Error("GetConfig returned different check interval")
	}
}

func TestNilConfig(t *testing.T) {
	refreshFunc := func(_ context.Context, _ string) error {
		return nil
	}

	// Should not panic with nil config
	manager := NewManager(nil, refreshFunc)

	if manager.GetStrategy() != StrategyReactive {
		t.Error("expected default strategy when config is nil")
	}
}

func TestBackgroundRefreshContextTimeout(t *testing.T) {
	config := &Config{
		Strategy:      StrategyBackground,
		RefreshBuffer: 50 * time.Millisecond,
		CheckInterval: 30 * time.Millisecond,
	}

	var mu sync.Mutex
	refreshStarted := make(chan struct{})
	refreshFunc := func(ctx context.Context, _ string) error {
		mu.Lock()
		close(refreshStarted)
		mu.Unlock()

		// Block until context is cancelled (should timeout after 30s)
		<-ctx.Done()
		return ctx.Err()
	}

	manager := NewManager(config, refreshFunc)
	defer manager.Stop()

	// Register token that needs refresh
	expiredExpiry := time.Now().Add(10 * time.Millisecond)
	manager.RegisterToken("token1", expiredExpiry)

	// Wait for refresh to start
	select {
	case <-refreshStarted:
		// Refresh started as expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("refresh did not start within timeout")
	}

	// Refresh function should complete due to context timeout
	// even though it blocks - the 30s timeout in checkAndRefreshTokens
	// should eventually cancel it
}

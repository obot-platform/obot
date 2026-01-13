package refresh

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// Strategy defines the token refresh approach
type Strategy string

const (
	// StrategyReactive refreshes tokens only when they expire (default)
	StrategyReactive Strategy = "reactive"

	// StrategyProactive refreshes tokens before they expire (reduces user-visible delays)
	StrategyProactive Strategy = "proactive"

	// StrategyBackground refreshes tokens asynchronously in the background
	StrategyBackground Strategy = "background"
)

// Config holds token refresh strategy configuration
type Config struct {
	// Strategy determines when tokens are refreshed
	Strategy Strategy

	// RefreshBuffer is the duration before expiry to trigger proactive refresh
	// Only used for StrategyProactive and StrategyBackground
	// Default: 5 minutes
	RefreshBuffer time.Duration

	// CheckInterval is how often to check for tokens needing refresh in background mode
	// Only used for StrategyBackground
	// Default: 1 minute
	CheckInterval time.Duration
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		Strategy:      StrategyReactive,
		RefreshBuffer: 5 * time.Minute,
		CheckInterval: 1 * time.Minute,
	}
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() *Config {
	config := DefaultConfig()

	if val := os.Getenv("OBOT_AUTH_PROVIDER_REFRESH_STRATEGY"); val != "" {
		switch Strategy(val) {
		case StrategyReactive, StrategyProactive, StrategyBackground:
			config.Strategy = Strategy(val)
		}
	}

	if val := os.Getenv("OBOT_AUTH_PROVIDER_REFRESH_BUFFER"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.RefreshBuffer = duration
		}
	}

	if val := os.Getenv("OBOT_AUTH_PROVIDER_REFRESH_CHECK_INTERVAL"); val != "" {
		if duration, err := time.ParseDuration(val); err == nil {
			config.CheckInterval = duration
		}
	}

	return config
}

// Func is the function signature for token refresh operations
type Func func(ctx context.Context, tokenID string) error

// Manager manages token refresh strategies
type Manager struct {
	config      *Config
	refreshFunc Func

	// For background refresh
	mu             sync.RWMutex
	tokens         map[string]time.Time // tokenID -> expiryTime
	backgroundCtx  context.Context
	backgroundStop context.CancelFunc
	backgroundDone chan struct{}
}

// NewManager creates a new refresh strategy manager
func NewManager(config *Config, refreshFunc Func) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	m := &Manager{
		config:      config,
		refreshFunc: refreshFunc,
		tokens:      make(map[string]time.Time),
	}

	// Start background refresh if configured
	if config.Strategy == StrategyBackground {
		m.startBackgroundRefresh()
	}

	return m
}

// ShouldRefresh determines if a token should be refreshed based on the configured strategy
func (m *Manager) ShouldRefresh(expiryTime time.Time) bool {
	switch m.config.Strategy {
	case StrategyReactive:
		// Only refresh when token has expired
		return time.Now().After(expiryTime)

	case StrategyProactive:
		// Refresh before expiry (within buffer window)
		return time.Now().Add(m.config.RefreshBuffer).After(expiryTime)

	case StrategyBackground:
		// Background strategy handles refresh separately
		// Always return false here as refresh happens asynchronously
		return false

	default:
		// Default to reactive
		return time.Now().After(expiryTime)
	}
}

// RefreshIfNeeded checks if a token needs refresh and refreshes it if necessary
func (m *Manager) RefreshIfNeeded(ctx context.Context, tokenID string, expiryTime time.Time) error {
	// For background strategy, register the token and return
	if m.config.Strategy == StrategyBackground {
		m.RegisterToken(tokenID, expiryTime)
		return nil
	}

	// For reactive and proactive strategies, check and refresh synchronously
	if m.ShouldRefresh(expiryTime) {
		return m.refreshFunc(ctx, tokenID)
	}

	return nil
}

// RegisterToken registers a token for background refresh monitoring
func (m *Manager) RegisterToken(tokenID string, expiryTime time.Time) {
	if m.config.Strategy != StrategyBackground {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.tokens[tokenID] = expiryTime
}

// UnregisterToken removes a token from background refresh monitoring
func (m *Manager) UnregisterToken(tokenID string) {
	if m.config.Strategy != StrategyBackground {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.tokens, tokenID)
}

// startBackgroundRefresh starts the background refresh goroutine
func (m *Manager) startBackgroundRefresh() {
	m.backgroundCtx, m.backgroundStop = context.WithCancel(context.Background())
	m.backgroundDone = make(chan struct{})

	go m.backgroundRefreshLoop()
}

// backgroundRefreshLoop runs the background refresh check loop
func (m *Manager) backgroundRefreshLoop() {
	defer close(m.backgroundDone)

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.backgroundCtx.Done():
			return

		case <-ticker.C:
			m.checkAndRefreshTokens()
		}
	}
}

// checkAndRefreshTokens checks all registered tokens and refreshes those that need it
func (m *Manager) checkAndRefreshTokens() {
	m.mu.RLock()
	tokensToRefresh := make([]string, 0)
	now := time.Now()
	refreshThreshold := now.Add(m.config.RefreshBuffer)

	for tokenID, expiryTime := range m.tokens {
		if refreshThreshold.After(expiryTime) {
			tokensToRefresh = append(tokensToRefresh, tokenID)
		}
	}
	m.mu.RUnlock()

	// Refresh tokens outside of lock
	for _, tokenID := range tokensToRefresh {
		// Create a timeout context for each refresh
		ctx, cancel := context.WithTimeout(m.backgroundCtx, 30*time.Second)

		if err := m.refreshFunc(ctx, tokenID); err != nil {
			// Log error but continue with other tokens
			// In production, this should use proper logging
			fmt.Fprintf(os.Stderr, "background refresh failed for token %s: %v\n", tokenID, err)
		}

		cancel()
	}
}

// Stop stops the background refresh goroutine if running
func (m *Manager) Stop() {
	if m.backgroundStop != nil {
		m.backgroundStop()
		<-m.backgroundDone
	}
}

// GetStrategy returns the current refresh strategy
func (m *Manager) GetStrategy() Strategy {
	return m.config.Strategy
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// ValidateStrategy checks if a strategy string is valid
func ValidateStrategy(s string) error {
	strategy := Strategy(s)
	switch strategy {
	case StrategyReactive, StrategyProactive, StrategyBackground:
		return nil
	default:
		return fmt.Errorf("invalid refresh strategy '%s': must be one of 'reactive', 'proactive', or 'background'", s)
	}
}

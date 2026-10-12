package billing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

const (
	// EntitlementsPropertyKey is the database property key the last good
	// entitlements response is persisted under.
	EntitlementsPropertyKey = "obot-billing-entitlements"

	entitlementsPath = "/v1/entitlements"

	defaultPollInterval   = 5 * time.Minute
	defaultRequestTimeout = 15 * time.Second
	maxResponseBytes      = 1 << 20
)

var (
	// ErrNotConfigured indicates the billing settings are not both set.
	ErrNotConfigured = errors.New("billing is not configured")
)

// Store persists the last good entitlements response so a restart during a
// billing outage does not lose them.
type Store interface {
	GetProperty(context.Context, string) (gatewaytypes.Property, error)
	SetProperty(context.Context, string, string) (gatewaytypes.Property, error)
}

// Client reads this environment's entitlements from the billing service.
type Client struct {
	baseURL      *url.URL
	key          string
	httpClient   *http.Client
	store        Store
	pollInterval time.Duration

	lock         sync.RWMutex
	entitlements *Entitlements

	refresh singleflight.Group
}

// New creates a billing client and seeds it with the last entitlements this
// installation saw, so limits survive a restart the billing service is down for.
func New(ctx context.Context, config Config, store Store) (*Client, error) {
	if !config.Configured() {
		return nil, ErrNotConfigured
	}

	baseURL, err := url.Parse(strings.TrimSuffix(strings.TrimSpace(config.BillingURL), "/"))
	if err != nil {
		return nil, fmt.Errorf("failed to parse billing URL: %w", err)
	}
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, fmt.Errorf("billing URL must be http or https, got %q", baseURL.Scheme)
	}

	c := &Client{
		baseURL:      baseURL,
		key:          strings.TrimSpace(config.BillingKey),
		httpClient:   &http.Client{Timeout: defaultRequestTimeout},
		store:        store,
		pollInterval: defaultPollInterval,
	}

	if err := c.load(ctx); err != nil {
		slog.Warn("failed to load persisted billing entitlements", "error", err)
	}

	return c, nil
}

// Entitlements returns the entitlements this environment last read successfully,
// or nil when it has never read any.
func (c *Client) Entitlements() *Entitlements {
	if c == nil {
		return nil
	}

	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.entitlements
}

// Refresh reads entitlements from the billing service and stores them. A failed
// read leaves the last good entitlements in place and is not a downgrade.
func (c *Client) Refresh(ctx context.Context) error {
	if c == nil {
		return ErrNotConfigured
	}

	_, err, _ := c.refresh.Do("refresh", func() (any, error) {
		entitlements, err := c.fetch(ctx)
		if err != nil {
			return nil, err
		}

		c.lock.Lock()
		c.entitlements = entitlements
		c.lock.Unlock()

		return nil, c.save(ctx, entitlements)
	})
	return err
}

// Poll refreshes entitlements on an interval. It is started on the leader, so a
// subscription change reaches the installation without anyone visiting the
// License page.
func (c *Client) Poll(ctx context.Context) {
	if c == nil {
		return
	}

	if err := c.Refresh(ctx); err != nil {
		slog.Warn("failed to read billing entitlements", "error", err)
	}

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.Refresh(ctx); err != nil {
				slog.Warn("failed to read billing entitlements", "error", err)
				continue
			}
			slog.Debug("billing entitlements updated", "entitlements", c.Entitlements())
		}
	}
}

func (c *Client) fetch(ctx context.Context) (*Entitlements, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL.JoinPath(entitlementsPath).String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build billing entitlements request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.key)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to read billing entitlements: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("billing entitlements request failed with status %d", resp.StatusCode)
	}

	var entitlements Entitlements
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&entitlements); err != nil {
		return nil, fmt.Errorf("failed to decode billing entitlements: %w", err)
	}

	return &entitlements, nil
}

func (c *Client) load(ctx context.Context) error {
	if c.store == nil {
		return nil
	}

	property, err := c.store.GetProperty(ctx, EntitlementsPropertyKey)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to read persisted billing entitlements: %w", err)
	}
	if strings.TrimSpace(property.Value) == "" {
		return nil
	}

	var entitlements Entitlements
	if err := json.Unmarshal([]byte(property.Value), &entitlements); err != nil {
		return fmt.Errorf("failed to decode persisted billing entitlements: %w", err)
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	c.entitlements = &entitlements
	return nil
}

func (c *Client) save(ctx context.Context, entitlements *Entitlements) error {
	if c.store == nil {
		return nil
	}

	encoded, err := json.Marshal(entitlements)
	if err != nil {
		return fmt.Errorf("failed to encode billing entitlements: %w", err)
	}
	if _, err := c.store.SetProperty(ctx, EntitlementsPropertyKey, string(encoded)); err != nil {
		return fmt.Errorf("failed to persist billing entitlements: %w", err)
	}
	return nil
}

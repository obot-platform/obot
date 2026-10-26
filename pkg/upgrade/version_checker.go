package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/version"
)

const (
	upgradeCheckEndpoint     = "check-upgrade"
	upgradeCheckInitialDelay = 30 * time.Minute
	upgradeCheckInterval     = 24 * time.Hour
	upgradeCheckTimeout      = 5 * time.Second
)

type Status struct {
	UpgradeAvailable bool
	LatestVersion    string
}

type VersionCheckerOptions struct {
	GatewayClient      *client.Client
	DisableUpdateCheck bool
}

type VersionChecker struct {
	httpClient       *http.Client
	installationID   string
	currentVersion   string
	upgradeServerURL string
	checkInterval    time.Duration
	requestTimeout   time.Duration

	statusLock sync.RWMutex
	status     Status

	done chan struct{}
}

type upgradeCheckResponse struct {
	UpgradeAvailable bool   `json:"upgradeAvailable"`
	LatestVersion    string `json:"latestVersion"`
	CurrentVersion   string `json:"currentVersion"`
}

func NewVersionChecker(ctx context.Context, opts VersionCheckerOptions) (*VersionChecker, error) {
	checker := &VersionChecker{
		httpClient:       http.DefaultClient,
		currentVersion:   version.Get().String(),
		upgradeServerURL: EndpointURL(ServerBaseURL(), upgradeCheckEndpoint),
		checkInterval:    upgradeCheckInterval,
		requestTimeout:   upgradeCheckTimeout,
		done:             make(chan struct{}),
	}
	if err := checker.start(ctx, opts.GatewayClient, opts.DisableUpdateCheck, os.Getenv("OBOT_FORCE_UPGRADE_CHECK") == "true", upgradeCheckInitialDelay); err != nil {
		return nil, err
	}
	return checker, nil
}

func (c *VersionChecker) start(ctx context.Context, gatewayClient propertyClient, disableUpdateCheck, forceUpdateCheck bool, initialDelay time.Duration) error {
	c.currentVersion = normalizeVersion(c.currentVersion)

	// Don't start the upgrade check if explicitly disabled or if this is a development version.
	if disableUpdateCheck || (version.IsDev(c.currentVersion) && !forceUpdateCheck) {
		close(c.done)
		return nil
	}

	installationID, err := GetInstallationID(ctx, gatewayClient)
	if err != nil {
		return err
	}
	c.installationID = installationID

	go c.run(ctx, initialDelay)
	return nil
}

func normalizeVersion(value string) string {
	value, _, _ = strings.Cut(value, "+")
	value, _, _ = strings.Cut(value, "-")
	return value
}

func (c *VersionChecker) Status() Status {
	c.statusLock.RLock()
	defer c.statusLock.RUnlock()
	return c.status
}

func (c *VersionChecker) run(ctx context.Context, initialDelay time.Duration) {
	defer close(c.done)

	// Delay the first check so that short-lived instances and frequent restarts don't each trigger a check.
	timer := time.NewTimer(initialDelay)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Debug("upgrade check context cancelled, exiting")
			return
		case <-timer.C:
		}

		if err := c.checkForUpgrade(ctx); err != nil {
			slog.Debug("failed to check for server upgrade", "error", err)
		}
		timer.Reset(c.checkInterval)
	}
}

func (c *VersionChecker) checkForUpgrade(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.upgradeServerURL, nil)
	if err != nil {
		return err
	}

	query := req.URL.Query()
	query.Set("uid", c.installationID)
	query.Set("current-version", c.currentVersion)
	req.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var status upgradeCheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return err
	}

	c.statusLock.Lock()
	c.status = Status{
		UpgradeAvailable: status.UpgradeAvailable,
		LatestVersion:    status.LatestVersion,
	}
	c.statusLock.Unlock()

	return nil
}

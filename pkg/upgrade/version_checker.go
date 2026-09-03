package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/version"
)

const (
	upgradeCheckEndpoint = "check-upgrade"
	upgradeCheckInterval = 24 * time.Hour
	upgradeCheckTimeout  = 5 * time.Second
)

type Status struct {
	UpgradeAvailable bool
	LatestVersion    string
}

type VersionCheckerOptions struct {
	GatewayClient      *client.Client
	LicenseProvider    *license.Provider
	Engine             string
	DisableUpdateCheck bool
}

type entitlementProvider interface {
	Entitlements(context.Context) ([]string, error)
}

type VersionChecker struct {
	licenseProvider  entitlementProvider
	httpClient       *http.Client
	engine           string
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
		licenseProvider:  opts.LicenseProvider,
		httpClient:       http.DefaultClient,
		engine:           opts.Engine,
		currentVersion:   version.Get().String(),
		upgradeServerURL: EndpointURL(ServerBaseURL(), upgradeCheckEndpoint),
		checkInterval:    upgradeCheckInterval,
		requestTimeout:   upgradeCheckTimeout,
		done:             make(chan struct{}),
	}
	if err := checker.start(ctx, opts.GatewayClient, opts.DisableUpdateCheck, os.Getenv("OBOT_FORCE_UPGRADE_CHECK") == "true"); err != nil {
		return nil, err
	}
	return checker, nil
}

func (c *VersionChecker) start(ctx context.Context, gatewayClient propertyClient, disableUpdateCheck, forceUpdateCheck bool) error {
	c.currentVersion = normalizeVersion(c.currentVersion)

	// Don't start the upgrade check if explicitly disabled or if this is a development version.
	if disableUpdateCheck || (strings.HasPrefix(c.currentVersion, "v0.0.0") && !forceUpdateCheck) {
		close(c.done)
		return nil
	}

	installationID, err := GetInstallationID(ctx, gatewayClient)
	if err != nil {
		return err
	}
	c.installationID = installationID

	go c.run(ctx)
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

func (c *VersionChecker) run(ctx context.Context) {
	defer close(c.done)

	timer := time.NewTimer(c.checkInterval)
	defer timer.Stop()

	for {
		distribution := clienttypes.ProductTelemetryDistributionUnregistered
		entitlements, err := c.licenseProvider.Entitlements(ctx)
		switch {
		case err != nil:
			slog.Debug("failed to refresh license state for upgrade check", "error", err)
		case slices.Contains(entitlements, license.CloudEntitlement):
			distribution = clienttypes.ProductTelemetryDistributionCloud
		case slices.Contains(entitlements, license.EnterpriseEntitlement):
			distribution = clienttypes.ProductTelemetryDistributionEnterprise
		case slices.Contains(entitlements, license.CommunityEntitlement):
			distribution = clienttypes.ProductTelemetryDistributionRegistered
		}
		if err := c.checkForUpgrade(ctx, distribution); err != nil {
			slog.Debug("failed to check for server upgrade", "error", err)
		}

		select {
		case <-ctx.Done():
			slog.Debug("upgrade check context cancelled, exiting")
			return
		case <-timer.C:
			timer.Reset(c.checkInterval)
		}
	}
}

func (c *VersionChecker) checkForUpgrade(ctx context.Context, distribution clienttypes.ProductTelemetryDistribution) error {
	ctx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.upgradeServerURL, nil)
	if err != nil {
		return err
	}

	query := req.URL.Query()
	query.Set("uid", c.installationID)
	query.Set("engine", c.engine)
	query.Set("distribution", string(distribution))
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

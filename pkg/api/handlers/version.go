package handlers

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"strings"

	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/mcptester"
	"github.com/obot-platform/obot/pkg/storage"
	"github.com/obot-platform/obot/pkg/upgrade"
	"github.com/obot-platform/obot/pkg/version"
)

const (
	SessionStoreDB     SessionStore = "db"
	SessionStoreCookie SessionStore = "cookie"
)

type SessionStore string

type UpgradeStatusReader interface {
	Status() upgrade.Status
}

type VersionHandlerOptions struct {
	ProviderConfiguration     mcptester.ProviderConfigurationResolver
	ModelProxyURL             *url.URL
	ModelProxySettings        mcptester.ModelProxySettingsReader
	GatewayClient             *client.Client
	StorageClient             storage.Client
	LicenseProvider           *license.Provider
	BillingProvider           *billing.Provider // nil when the environment isn't billed
	EntitlementProvider       client.EntitlementProvider
	PostgresDSN               string
	Engine                    string
	MCPNetworkPolicyEnabled   bool
	MCPDefaultDenyAllEgress   bool
	AuthEnabled               bool
	ClaudeEnvironmentsEnabled bool
	HideK8sDetails            bool
	UpgradeStatusReader       UpgradeStatusReader
}

type VersionHandler struct {
	VersionHandlerOptions

	sessionStore SessionStore
}

func sessionStoreFromPostgresDSN(postgresDSN string) SessionStore {
	if postgresDSN != "" {
		return SessionStoreDB
	}
	return SessionStoreCookie
}

func NewVersionHandler(opts VersionHandlerOptions) *VersionHandler {
	return &VersionHandler{
		VersionHandlerOptions: opts,
		sessionStore:          sessionStoreFromPostgresDSN(opts.PostgresDSN),
	}
}

func (v *VersionHandler) GetVersion(req api.Context) error {
	response, err := v.getVersionResponse(req.Context())
	if err != nil {
		return err
	}
	return req.Write(response)
}

func (v *VersionHandler) getVersionResponse(ctx context.Context) (map[string]any, error) {
	engine := v.Engine
	if mcp.IsKubernetesBackend(engine) {
		engine = mcp.RuntimeBackendKubernetes
	}

	upgradeStatus := v.upgradeStatus()

	granted, err := v.EntitlementProvider.Entitlements(ctx)
	if err != nil {
		return nil, err
	}
	used, err := license.Usage(ctx, v.GatewayClient, v.StorageClient)
	if err != nil {
		return nil, err
	}
	violations, err := license.Violations(ctx, v.StorageClient, granted, used)
	if err != nil {
		return nil, err
	}
	if v.BillingProvider != nil {
		restricted, err := v.BillingProvider.Restricted(ctx)
		if err != nil {
			return nil, err
		}
		if restricted {
			violations = append(violations, license.Violation{Type: "restricted"})
		}
	}

	values := map[string]any{
		"upgradeAvailable": upgradeStatus.UpgradeAvailable,
		"latestVersion":    upgradeStatus.LatestVersion,
		"obot":             version.Get().String(),
		"authEnabled":      v.AuthEnabled,
		"sessionStore":     v.sessionStore,
		"enterprise":       granted.Enterprise,
		"community":        granted.Community,
		"cloud":            granted.Cloud,
		// A license can grant cloud too, so the UI is told when the environment is billed.
		"billingEnabled":               v.BillingProvider != nil,
		"userCount":                    used.Users,
		"deviceCount":                  used.Devices,
		"engine":                       engine,
		"mcpNetworkPolicyEnabled":      v.MCPNetworkPolicyEnabled,
		"mcpDefaultDenyAllEgress":      v.MCPDefaultDenyAllEgress,
		"licenseEntitlementViolations": violations,
	}
	for key, value := range v.featureValues() {
		values[key] = value
	}

	if granted.Users != nil {
		values["userLimit"] = *granted.Users
	}
	if granted.Devices != nil {
		values["deviceLimit"] = *granted.Devices
	}

	if versions := os.Getenv("OBOT_SERVER_VERSIONS"); versions != "" {
		for pair := range strings.SplitSeq(versions, ",") {
			key, value, ok := strings.Cut(pair, "=")
			if !ok {
				continue
			}
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}

	// Model configuration can be changing independently of the rest of the app.
	// Keep the version response available, but never interpret lookup failure as
	// permission to use the external model service.
	availability, availabilityErr := mcptester.ResolveModelProxyAvailability(ctx, v.ModelProxyURL, v.ProviderConfiguration, v.ModelProxySettings)
	if availabilityErr != nil {
		slog.Warn("model provider availability unresolved", "error", availabilityErr)
	}

	hasValidLicense, err := v.LicenseProvider.HasValidLicense(ctx)
	if err != nil {
		return nil, err
	}

	// Set after OBOT_SERVER_VERSIONS so capability values cannot be overridden.
	values["hasModelProvider"] = availability.HasModelProvider

	values["hasValidLicense"] = hasValidLicense
	values["mcpTesterModelProxyAvailable"] = availabilityErr == nil && availability.Enabled && hasValidLicense

	return values, nil
}

func (v *VersionHandler) upgradeStatus() upgrade.Status {
	if v.UpgradeStatusReader == nil {
		return upgrade.Status{}
	}
	return v.UpgradeStatusReader.Status()
}

func (v *VersionHandler) featureValues() map[string]bool {
	return map[string]bool{
		"claudeEnvironmentsEnabled": v.ClaudeEnvironmentsEnabled,
		"hideK8sDetails":            v.HideK8sDetails,
	}
}

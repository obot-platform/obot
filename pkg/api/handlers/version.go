package handlers

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/billing"
	"github.com/obot-platform/obot/pkg/entitlements"
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
	ProviderConfiguration mcptester.ProviderConfigurationResolver
	ModelProxyURL         *url.URL
	ModelProxySettings    mcptester.ModelProxySettingsReader
	GatewayClient         *client.Client
	StorageClient         storage.Client
	// Entitlements reads the license's or, with billing, the plan's entitlements. LicenseProvider is the Keygen
	// license, which reports hasValidLicense. Billing is nil without billing.
	Entitlements              entitlements.Reader
	LicenseProvider           *license.Provider
	Billing                   *billing.Subscription
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

	granted, usage, err := v.entitlements(ctx)
	if err != nil {
		return nil, err
	}

	userCount, err := v.GatewayClient.UserCount(ctx)
	if err != nil {
		return nil, err
	}

	deviceCount, err := v.GatewayClient.DeviceCount(ctx)
	if err != nil {
		return nil, err
	}

	// Users and devices are counted now, since their changes don't trigger billing's recount.
	var counted types.EntitlementsUsage
	if usage != nil {
		counted = *usage
	}
	counted.Users, counted.Devices = userCount, deviceCount
	violations, err := v.violations(ctx, granted, counted)
	if err != nil {
		return nil, err
	}

	values := map[string]any{
		"upgradeAvailable":             upgradeStatus.UpgradeAvailable,
		"latestVersion":                upgradeStatus.LatestVersion,
		"obot":                         version.Get().String(),
		"authEnabled":                  v.AuthEnabled,
		"sessionStore":                 v.sessionStore,
		"enterprise":                   granted.Enterprise,
		"community":                    granted.Community,
		"licenseEntitlements":          license.Codes(granted),
		"userCount":                    userCount,
		"deviceCount":                  deviceCount,
		"engine":                       engine,
		"mcpNetworkPolicyEnabled":      v.MCPNetworkPolicyEnabled,
		"mcpDefaultDenyAllEgress":      v.MCPDefaultDenyAllEgress,
		"licenseEntitlementViolations": violations,
		"missingLicenseEntitlements":   missingEntitlements(violations),
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
	if v.Billing != nil {
		values["billingEnabled"] = true
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

// entitlements returns the environment's entitlements. With billing, it also returns the leader's last count, from the
// same read of the BillingSubscription.
func (v *VersionHandler) entitlements(ctx context.Context) (types.Entitlements, *types.EntitlementsUsage, error) {
	if v.Billing == nil {
		granted, err := v.Entitlements.Entitlements(ctx)
		return granted, nil, err
	}
	subscription, err := v.Billing.Read(ctx)
	if err != nil {
		return types.Entitlements{}, nil, err
	}
	return billing.EntitlementsOf(subscription.Status), &subscription.Status.Usage, nil
}

// violations returns the configured auth providers missing an entitlement, the limits usage is over, and a
// restriction.
func (v *VersionHandler) violations(ctx context.Context, granted types.Entitlements, usage types.EntitlementsUsage) ([]entitlements.Violation, error) {
	violations, err := entitlements.ProviderViolations(ctx, v.StorageClient, granted)
	if err != nil {
		return nil, err
	}
	violations = append(violations, entitlements.LimitViolations(granted, usage)...)
	if granted.Restricted {
		violations = append(violations, entitlements.Violation{
			Type:    "restricted",
			Message: entitlements.RestrictedMessage,
		})
	}
	return violations, nil
}

func missingEntitlements(violations []entitlements.Violation) []string {
	seen := make(map[string]struct{})
	for _, violation := range violations {
		for _, entitlement := range violation.MissingEntitlements {
			seen[entitlement] = struct{}{}
		}
	}
	missing := make([]string, 0, len(seen))
	for entitlement := range seen {
		missing = append(missing, entitlement)
	}
	slices.Sort(missing)
	return missing
}

package producttelemetry

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"
	"uuid"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/license"
	"github.com/obot-platform/obot/pkg/mcp"
	storagev1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/upgrade"
	"github.com/obot-platform/obot/pkg/version"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type requestGatewayClient interface {
	GetOrCreateProperty(context.Context, string, string) (gatewaytypes.Property, error)
	UserCount(context.Context) (int64, error)
	ActiveUserCountByDate(context.Context, time.Time, time.Time) (int64, error)
	MCPToolCallCount(context.Context, time.Time, time.Time) (int64, error)
	LLMAuditLogCount(context.Context, time.Time, time.Time) (int64, error)
	DeviceScanCount(context.Context, time.Time, time.Time) (int64, error)
	EnforcementDecisionCount(context.Context, time.Time, time.Time) (int64, error)
}

type licenseEntitlementProvider interface {
	Entitlements(context.Context) ([]string, error)
}

// buildRequest constructs a telemetry payload, failing when required installation metadata cannot be loaded.
// Usage metrics are collected on a best-effort basis.
func buildRequest(ctx context.Context, gatewayClient requestGatewayClient, storageClient kclient.Reader, licenseProvider licenseEntitlementProvider, defaultMCPCatalogPath, engine string) (clienttypes.ProductTelemetryRequest, error) {
	installationID, err := upgrade.GetInstallationID(ctx, gatewayClient)
	if err != nil {
		return clienttypes.ProductTelemetryRequest{}, fmt.Errorf("get installation ID: %w", err)
	}

	licenseMachineID, err := gatewayClient.GetOrCreateProperty(ctx, license.LicenseMachineIDPropertyKey, uuid.New().String())
	if err != nil {
		return clienttypes.ProductTelemetryRequest{}, fmt.Errorf("get license machine ID: %w", err)
	}

	entitlements, err := licenseProvider.Entitlements(ctx)
	if err != nil {
		return clienttypes.ProductTelemetryRequest{}, fmt.Errorf("get product telemetry distribution: %w", err)
	}
	distribution := license.GetDistributionFromEntitlements(entitlements)

	reportedAt := time.Now().UTC()
	dayEnd := reportedAt.Truncate(24 * time.Hour)
	metrics := collectMetrics(ctx, gatewayClient, storageClient, defaultMCPCatalogPath, dayEnd.Add(-24*time.Hour), dayEnd)

	return clienttypes.ProductTelemetryRequest{
		InstallationID:   installationID,
		LicenseMachineID: licenseMachineID.Value,
		ReportedAt:       reportedAt,
		Distribution:     distribution,
		Engine:           normalizeEngine(engine),
		CurrentVersion:   version.Get().String(),
		Metrics:          &metrics,
	}, nil
}

func normalizeEngine(engine string) string {
	if mcp.IsKubernetesBackend(engine) {
		return mcp.RuntimeBackendKubernetes
	}
	return engine
}

// collectMetrics gathers usage and inventory metrics, leaving fields nil when their source is unavailable.
func collectMetrics(ctx context.Context, gatewayClient requestGatewayClient, storageClient kclient.Reader, defaultMCPCatalogPath string, dayStart, dayEnd time.Time) clienttypes.ProductTelemetryMetrics {
	var metrics clienttypes.ProductTelemetryMetrics

	if count, err := gatewayClient.UserCount(ctx); err != nil {
		logMetricError("total users", err)
	} else {
		metrics.TotalUsers = &count
	}

	if count, err := gatewayClient.ActiveUserCountByDate(ctx, dayStart, dayEnd); err != nil {
		logMetricError("active users", err)
	} else {
		metrics.ActiveUsers = &count
	}

	if count, err := gatewayClient.MCPToolCallCount(ctx, dayStart, dayEnd); err != nil {
		logMetricError("MCP tool calls", err)
	} else {
		metrics.MCPToolCallCount = &count
	}

	if count, err := gatewayClient.LLMAuditLogCount(ctx, dayStart, dayEnd); err != nil {
		logMetricError("LLM audit logs", err)
	} else {
		metrics.LLMAuditLogCount = &count
	}

	if count, err := gatewayClient.DeviceScanCount(ctx, dayStart, dayEnd); err != nil {
		logMetricError("Sentry scans", err)
	} else {
		metrics.SentryScanCount = &count
	}

	if count, err := gatewayClient.EnforcementDecisionCount(ctx, dayStart, dayEnd); err != nil {
		logMetricError("Sentry enforcement events", err)
	} else {
		metrics.SentryEnforcementEventCount = &count
	}

	var servers storagev1.MCPServerList
	if err := storageClient.List(ctx, &servers, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		logMetricError("deployed MCP servers", err)
	} else {
		deploymentCounts := make(map[string]int64)
		var count int64
		for _, server := range servers.Items {
			if !server.DeletionTimestamp.IsZero() || server.Spec.Template {
				continue
			}
			count++
			if server.Spec.MCPServerCatalogEntryName != "" {
				deploymentCounts[server.Spec.MCPServerCatalogEntryName]++
			}
		}
		metrics.DeployedMCPServers = &count

		builtIns, customCount, err := collectMCPEntryMetrics(ctx, storageClient, defaultMCPCatalogPath, deploymentCounts)
		if err != nil {
			logMetricError("MCP server catalog entries", err)
		} else {
			metrics.BuiltInMCPServers = &builtIns
			metrics.CustomMCPServerEntryCount = &customCount
		}
	}

	var authProviders storagev1.AuthProviderList
	if err := storageClient.List(ctx, &authProviders, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		logMetricError("authentication provider type", err)
	} else {
		providerType := ""
		for _, provider := range authProviders.Items {
			if provider.Status.Configured && (providerType == "" || provider.Name < providerType) {
				providerType = provider.Name
			}
		}
		metrics.AuthProviderType = &providerType
	}

	var skills storagev1.SkillList
	if err := storageClient.List(ctx, &skills, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		logMetricError("managed skills", err)
	} else {
		count := int64(len(skills.Items))
		metrics.ManagedSkillCount = &count
	}

	return metrics
}

// collectMCPEntryMetrics returns built-in catalog usage and the number of custom catalog entries.
func collectMCPEntryMetrics(ctx context.Context, storageClient kclient.Reader, defaultMCPCatalogPath string, deploymentCounts map[string]int64) ([]clienttypes.ProductTelemetryBuiltInMCPServer, int64, error) {
	var entries storagev1.MCPServerCatalogEntryList
	if err := storageClient.List(ctx, &entries, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, 0, err
	}

	builtIns := make([]clienttypes.ProductTelemetryBuiltInMCPServer, 0)
	var customCount int64
	for _, entry := range entries.Items {
		if defaultMCPCatalogPath == "" || entry.Spec.SourceURL != defaultMCPCatalogPath {
			customCount++
			continue
		}

		id := entry.Spec.Manifest.EntryKey
		if id == "" {
			id = entry.Name
		}
		builtIns = append(builtIns, clienttypes.ProductTelemetryBuiltInMCPServer{
			ID:              id,
			Name:            entry.Spec.Manifest.Name,
			DeploymentCount: deploymentCounts[entry.Name],
			UserCount:       int64(entry.Status.UserCount),
		})
	}

	sort.Slice(builtIns, func(i, j int) bool {
		return builtIns[i].ID < builtIns[j].ID
	})
	return builtIns, customCount, nil
}

// logMetricError records an unavailable metric without interrupting telemetry collection.
func logMetricError(metric string, err error) {
	slog.Warn("failed to collect product telemetry metric", "metric", metric, "error", err)
}

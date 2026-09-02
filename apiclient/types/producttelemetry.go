package types

import (
	"time"
)

const (
	ProductTelemetryDistributionUnlicensed ProductTelemetryDistribution = "Unlicensed"
	ProductTelemetryDistributionCommunity  ProductTelemetryDistribution = "Community"
	ProductTelemetryDistributionEnterprise ProductTelemetryDistribution = "Enterprise"
	ProductTelemetryDistributionCloud      ProductTelemetryDistribution = "Cloud"
)

// ProductTelemetryDistribution identifies the Obot distribution sending telemetry.
type ProductTelemetryDistribution string

// ProductTelemetryRequest is the telemetry report Obot sends to Upgrade Server.
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
type ProductTelemetryRequest struct {
	// InstallationID is the stable identifier used by Obot's upgrade service.
	InstallationID string `json:"installationID"`
	// LicenseMachineID is the persisted Keygen machine fingerprint.
	LicenseMachineID string                       `json:"licenseMachineID"`
	ReportedAt       time.Time                    `json:"reportedAt"`
	Distribution     ProductTelemetryDistribution `json:"distribution"`
	Engine           string                       `json:"engine"`
	CurrentVersion   string                       `json:"currentVersion"`
	Metrics          *ProductTelemetryMetrics     `json:"metrics,omitempty"`
}

// ProductTelemetryMetrics contains the aggregate metrics authorized for a report.
// Nil fields are encoded as null to distinguish unavailable values from measured zeroes.
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
type ProductTelemetryMetrics struct {
	TotalUsers                  *int64                              `json:"totalUsers"`
	ActiveUsers                 *int64                              `json:"activeUsers"`
	DeployedMCPServers          *int64                              `json:"deployedMCPServers"`
	CustomMCPServerEntryCount   *int64                              `json:"customMCPServerEntryCount"`
	BuiltInMCPServers           *[]ProductTelemetryBuiltInMCPServer `json:"builtInMCPServers"`
	AuthProviderType            *string                             `json:"authProviderType"`
	MCPAuditLogCount            *int64                              `json:"mcpAuditLogCount"`
	LLMAuditLogCount            *int64                              `json:"llmAuditLogCount"`
	SentryScanCount             *int64                              `json:"sentryScanCount"`
	SentryEnforcementEventCount *int64                              `json:"sentryEnforcementEventCount"`
	ManagedSkillCount           *int64                              `json:"managedSkillCount"`
}

// ProductTelemetryBuiltInMCPServer contains aggregate usage for a built-in MCP server.
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
type ProductTelemetryBuiltInMCPServer struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	DeploymentCount int64  `json:"deploymentCount"`
	UserCount       int64  `json:"userCount"`
}

package types

import "time"

// ProductTelemetryRequest is the telemetry report Obot sends to Upgrade Server.
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
type ProductTelemetryRequest struct {
	InstallationID string                  `json:"installationID"`
	ReportedAt     time.Time               `json:"reportedAt"`
	Metrics        ProductTelemetryMetrics `json:"metrics"`
}

// ProductTelemetryMetrics contains the aggregate metrics authorized for a report.
// Nil fields are encoded as null to distinguish unavailable values from measured zeroes.
// +k8s:deepcopy-gen=false
// +k8s:openapi-gen=false
type ProductTelemetryMetrics struct {
	TotalUsers         *int64                              `json:"totalUsers"`
	ActiveUsers        *int64                              `json:"activeUsers"`
	DeployedMCPServers *int64                              `json:"deployedMCPServers"`
	BuiltInMCPServers  *[]ProductTelemetryBuiltInMCPServer `json:"builtInMCPServers"`
	AuthProviderType   *string                             `json:"authProviderType"`
	MCPAuditLogCount   *int64                              `json:"mcpAuditLogCount"`
	LLMAuditLogCount   *int64                              `json:"llmAuditLogCount"`
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

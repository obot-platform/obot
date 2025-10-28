package types

// AuditLogExportCreateRequest represents a request to create an audit log export
type AuditLogExportCreateRequest struct {
	Name      string                `json:"name"`
	StartTime Time                  `json:"startTime"`
	EndTime   Time                  `json:"endTime"`
	Filters   AuditLogExportFilters `json:"filters,omitempty"`
	Bucket    string                `json:"bucket,omitempty"`
	KeyPrefix string                `json:"keyPrefix,omitempty"`
}

// AuditLogExportResponse represents an audit log export
type AuditLogExportResponse struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	StorageProvider StorageProviderType   `json:"storageProvider"`
	Bucket          string                `json:"bucket,omitempty"`
	KeyPrefix       string                `json:"keyPrefix,omitempty"`
	StartTime       Time                  `json:"startTime"`
	EndTime         Time                  `json:"endTime"`
	Filters         AuditLogExportFilters `json:"filters,omitempty"`
	State           string                `json:"state"`
	Error           string                `json:"error,omitempty"`
	ExportSize      int64                 `json:"exportSize,omitempty"`
	ExportPath      string                `json:"exportPath,omitempty"`
	StartedAt       Time                  `json:"startedAt,omitempty"`
	CompletedAt     Time                  `json:"completedAt,omitempty"`
	CreatedAt       Time                  `json:"createdAt"`
}

// AuditLogExportListResponse represents a list of audit log exports
type AuditLogExportListResponse struct {
	Items []AuditLogExportResponse `json:"items"`
	Total int64                    `json:"total"`
}

// ScheduledAuditLogExportCreateRequest represents a request to create a scheduled audit log export
type ScheduledAuditLogExportCreateRequest struct {
	Name                  string                `json:"name"`
	Bucket                string                `json:"bucket,omitempty"`
	KeyPrefix             string                `json:"keyPrefix,omitempty"`
	Schedule              Schedule              `json:"schedule"`
	RetentionPeriodInDays int                   `json:"retentionPeriodInDays,omitempty"`
	Filters               AuditLogExportFilters `json:"filters,omitempty"`
}

// ScheduledAuditLogExportUpdateRequest represents a request to update a scheduled audit log export
type ScheduledAuditLogExportUpdateRequest struct {
	Name                  *string                `json:"name,omitempty"`
	Enabled               *bool                  `json:"enabled,omitempty"`
	Schedule              *Schedule              `json:"schedule,omitempty"`
	RetentionPeriodInDays *int                   `json:"retentionPeriodInDays,omitempty"`
	Filters               *AuditLogExportFilters `json:"filters,omitempty"`
	Bucket                *string                `json:"bucket,omitempty"`
	KeyPrefix             *string                `json:"keyPrefix,omitempty"`
}

// ScheduledAuditLogExportResponse represents a scheduled audit log export
type ScheduledAuditLogExportResponse struct {
	ID                    string                `json:"id"`
	Bucket                string                `json:"bucket"`
	KeyPrefix             string                `json:"keyPrefix"`
	Name                  string                `json:"name"`
	Enabled               bool                  `json:"enabled"`
	Schedule              Schedule              `json:"schedule"`
	RetentionPeriodInDays int                   `json:"retentionPeriodInDays,omitempty"`
	Filters               AuditLogExportFilters `json:"filters,omitempty"`
	LastRunAt             Time                  `json:"lastRunAt,omitempty"`
}

// ScheduledAuditLogExportListResponse represents a list of scheduled audit log exports
type ScheduledAuditLogExportListResponse struct {
	Items []ScheduledAuditLogExportResponse `json:"items"`
	Total int64                             `json:"total"`
}

// AuditLogExportFilters represents filters for audit log export
type AuditLogExportFilters struct {
	UserIDs                    []string `json:"userIDs,omitempty"`
	MCPIDs                     []string `json:"mcpIDs,omitempty"`
	MCPServerDisplayNames      []string `json:"mcpServerDisplayNames,omitempty"`
	MCPServerCatalogEntryNames []string `json:"mcpServerCatalogEntryNames,omitempty"`
	CallTypes                  []string `json:"callTypes,omitempty"`
	CallIdentifiers            []string `json:"callIdentifiers,omitempty"`
	SessionIDs                 []string `json:"sessionIDs,omitempty"`
	ClientNames                []string `json:"clientNames,omitempty"`
	ClientVersions             []string `json:"clientVersions,omitempty"`
	ResponseStatuses           []string `json:"responseStatuses,omitempty"`
	ClientIPs                  []string `json:"clientIPs,omitempty"`
	Query                      string   `json:"query,omitempty"`
}

// StorageCredentialsTestRequest represents a request to test storage credentials
type StorageCredentialsTestRequest struct {
	StorageConfig
}

// StorageCredentialsTestResponse represents a response to a credentials test
type StorageCredentialsTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type StorageCredentialsResponse struct {
	Provider            StorageProviderType `json:"provider"`
	UseWorkloadIdentity bool                `json:"useWorkloadIdentity"`
	StorageConfig
}

type AuditLogExportState string

const (
	AuditLogExportStatePending   AuditLogExportState = "pending"
	AuditLogExportStateRunning   AuditLogExportState = "running"
	AuditLogExportStateCompleted AuditLogExportState = "completed"
	AuditLogExportStateFailed    AuditLogExportState = "failed"
)

type StorageProviderType string

const (
	StorageProviderS3        StorageProviderType = "s3"
	StorageProviderGCS       StorageProviderType = "gcs"
	StorageProviderAzureBlob StorageProviderType = "azure"
	StorageProviderCustomS3  StorageProviderType = "custom"
)

type StorageProviderConfigInput struct {
	Provider            StorageProviderType `json:"provider"`
	UseWorkloadIdentity bool                `json:"useWorkloadIdentity,omitempty"`
	StorageConfig
}

type StorageConfig struct {
	// S3-compatible storage config
	S3Config *S3Config `json:"s3Config,omitempty"`
	// Google Cloud Storage config
	GCSConfig *GCSConfig `json:"gcsConfig,omitempty"`
	// Azure Blob Storage config
	AzureConfig *AzureConfig `json:"azureConfig,omitempty"`
	// Custom S3-compatible storage config
	CustomS3Config *CustomS3Config `json:"customS3Config,omitempty"`
}

type S3Config struct {
	Region string `json:"region"`

	AccessKeyID     string `json:"accessKeyID,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

type GCSConfig struct {
	ServiceAccountJSON string `json:"serviceAccountJSON,omitempty"`
}

type AzureConfig struct {
	StorageAccount string `json:"storageAccount,omitempty"`
	ClientID       string `json:"clientID,omitempty"`
	TenantID       string `json:"tenantID,omitempty"`
	ClientSecret   string `json:"clientSecret,omitempty"`
}

type CustomS3Config struct {
	Endpoint string `json:"endpoint"`
	Region   string `json:"region"`

	AccessKeyID     string `json:"accessKeyID,omitempty"`
	SecretAccessKey string `json:"secretAccessKey,omitempty"`
}

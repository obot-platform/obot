package auditlogexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlog"
	"github.com/obot-platform/obot/pkg/auditlogexport"
	client "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const batchSize = 10_000

// Handler reconciles AuditLogExport resources and streams normalized JSONL events to the
// configured object-storage provider.
type Handler struct {
	gatewayClient *client.Client
	credProvider  *auditlogexport.CredentialProvider
}

// NewHandler constructs an audit-log export controller handler backed by gatewayClient.
func NewHandler(gatewayClient *client.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
		credProvider:  auditlogexport.NewCredentialProvider(gatewayClient),
	}
}

// ExportAuditLogs reconciles one AuditLogExport resource. It marks pending exports as running,
// streams their selected events, and records either the completed result or terminal failure.
func (h *Handler) ExportAuditLogs(req router.Request, _ router.Response) error {
	export := req.Object.(*v1.AuditLogExport)

	if export.Status.State == types.AuditLogExportStateCompleted || export.Status.State == types.AuditLogExportStateFailed {
		return nil
	}

	export.Status.State = types.AuditLogExportStateRunning
	export.Status.StartedAt = &metav1.Time{Time: time.Now()}

	if err := req.Client.Status().Update(req.Ctx, export); err != nil {
		return fmt.Errorf("failed to update export status: %w", err)
	}

	var err error
	switch export.Spec.EffectiveType() {
	case types.AuditLogTypeMCP:
		presentOptions := auditlog.PresentOptions{
			IncludeDetails:  true,
			PayloadRedacted: !export.Spec.WithRequestAndResponse,
		}
		err = performExport(req.Ctx, h.credProvider, export, "mcp-audit-logs", h.fetchMCPAuditLogs, func(log gatewaytypes.MCPAuditLog) types.AuditLogEvent {
			return auditlog.Present(log, presentOptions)
		})
	case types.AuditLogTypeLLM:
		err = performExport(req.Ctx, h.credProvider, export, "llm-audit-logs", h.fetchLLMAuditLogs, gatewaytypes.ConvertLLMAuditLog)
	default:
		err = fmt.Errorf("unsupported audit log export type %q", export.Spec.Type)
	}
	if err != nil {
		export.Status.State = types.AuditLogExportStateFailed
		export.Status.Error = err.Error()

		if statusErr := req.Client.Status().Update(req.Ctx, export); statusErr != nil {
			return fmt.Errorf("failed to update failed export status: %w", statusErr)
		}

		return fmt.Errorf("audit log export failed: %w", err)
	}

	return req.Client.Status().Update(req.Ctx, export)
}

func (h *Handler) fetchMCPAuditLogs(ctx context.Context, export *v1.AuditLogExport, limit, offset int) ([]gatewaytypes.MCPAuditLog, error) {
	opts := mcpAuditLogOptionsFromExport(export, limit, offset)
	logs, _, err := h.gatewayClient.GetMCPAuditLogs(ctx, opts)
	return logs, err
}

func (h *Handler) fetchLLMAuditLogs(ctx context.Context, export *v1.AuditLogExport, limit, offset int) ([]gatewaytypes.LLMAuditLog, error) {
	opts := llmAuditLogOptionsFromExport(export, limit, offset)
	logs, _, err := h.gatewayClient.GetLLMAuditLogs(ctx, opts)
	return logs, err
}

func mcpAuditLogOptionsFromExport(export *v1.AuditLogExport, limit, offset int) client.MCPAuditLogOptions {
	filters := export.Spec.Filters
	if filters == nil {
		filters = &types.AuditLogExportFilters{}
	}
	return client.MCPAuditLogOptions{
		StartTime:                 export.Spec.StartTime.Time,
		EndTime:                   export.Spec.EndTime.Time,
		SourceTypes:               auditlog.NormalizeSourceTypes(filters.SourceTypes),
		Actor:                     filters.Actors,
		Operation:                 filters.Operations,
		MCPServer:                 filters.MCPServers,
		Tool:                      filters.Tools,
		Outcome:                   filters.Outcomes,
		Client:                    filters.Clients,
		UserID:                    filters.UserIDs,
		MCPID:                     filters.MCPIDs,
		MCPServerDisplayName:      filters.MCPServerDisplayNames,
		MCPServerCatalogEntryName: filters.MCPServerCatalogEntryNames,
		CallType:                  filters.CallTypes,
		CallIdentifier:            filters.CallIdentifiers,
		SessionID:                 filters.SessionIDs,
		ClientName:                filters.ClientNames,
		ClientVersion:             filters.ClientVersions,
		ResponseStatus:            filters.ResponseStatuses,
		ClientIP:                  filters.ClientIPs,
		AgentProvider:             filters.AgentProviders,
		Status:                    filters.Statuses,
		ToolName:                  filters.ToolNames,
		ToolKind:                  filters.ToolKinds,
		DeviceID:                  filters.DeviceIDs,
		Query:                     filters.Query,
		Limit:                     limit,
		Offset:                    offset,
		WithRequestAndResponse:    export.Spec.WithRequestAndResponse,
	}
}

func llmAuditLogOptionsFromExport(export *v1.AuditLogExport, limit, offset int) client.LLMAuditLogOptions {
	filters := export.Spec.LLMFilters
	if filters == nil {
		filters = &types.LLMAuditLogExportFilters{}
	}
	return client.LLMAuditLogOptions{
		StartTime:              export.Spec.StartTime.Time,
		EndTime:                export.Spec.EndTime.Time,
		UserID:                 filters.UserIDs,
		ModelProvider:          filters.ModelProviders,
		TargetModel:            filters.TargetModels,
		RequestPath:            filters.RequestPaths,
		ResponseStatus:         filters.ResponseStatuses,
		Outcome:                filters.Outcomes,
		UserAgent:              filters.UserAgents,
		ClientSessionID:        filters.ClientSessionIDs,
		MessagePolicyTriggered: filters.MessagePolicyTriggered,
		Query:                  filters.Query,
		Limit:                  limit,
		Offset:                 offset,
		WithSensitiveFields:    export.Spec.WithRequestAndResponse,
		SortBy:                 "created_at",
		SortOrder:              "asc",
	}
}

// performExport streams audit logs to configured object storage and marks the export completed.
// The fetch function provides source-specific audit log batches; convert maps each record to its JSONL export shape.
func performExport[T any, U any](
	ctx context.Context,
	credProvider *auditlogexport.CredentialProvider,
	export *v1.AuditLogExport,
	defaultPrefix string,
	fetch func(context.Context, *v1.AuditLogExport, int, int) ([]T, error),
	convert func(T) U,
) error {
	storageConfig, err := credProvider.GetStorageConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get storage config: %w", err)
	}
	if storageConfig == nil {
		return fmt.Errorf("storage config is nil")
	}

	var provider types.StorageProviderType
	switch {
	case storageConfig.S3Config != nil:
		provider = types.StorageProviderS3
	case storageConfig.GCSConfig != nil:
		provider = types.StorageProviderGCS
	case storageConfig.AzureConfig != nil:
		provider = types.StorageProviderAzureBlob
	case storageConfig.CustomS3Config != nil:
		provider = types.StorageProviderCustomS3
	default:
		return fmt.Errorf("invalid storage config, no storage provider found")
	}

	storageProvider, err := auditlogexport.NewStorageProvider(provider)
	if err != nil {
		return fmt.Errorf("failed to create storage provider: %w", err)
	}

	export.Status.StorageProvider = provider

	exportPath := generateExportPath(export.Spec.Name, export.Spec.KeyPrefix, defaultPrefix)
	exportSize, err := streamingExport(ctx, *storageConfig, storageProvider, export, export.Spec.Bucket, exportPath, fetch, convert)
	if err != nil {
		return fmt.Errorf("failed to perform streaming export: %w", err)
	}

	export.Status.ExportSize = exportSize
	export.Status.ExportPath = exportPath
	export.Status.State = types.AuditLogExportStateCompleted
	export.Status.CompletedAt = &metav1.Time{Time: time.Now()}

	return nil
}

func formatLogs[T any, U any](logs []T, convert func(T) U) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	for _, log := range logs {
		if err := enc.Encode(convert(log)); err != nil {
			return nil, fmt.Errorf("failed to marshal log entry: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// streamingExport pipes formatted batches directly to storage so large exports do not need to buffer in memory.
func streamingExport[T any, U any](
	ctx context.Context,
	storageConfig types.StorageConfig,
	storageProvider auditlogexport.StorageProvider,
	export *v1.AuditLogExport,
	bucket, exportPath string,
	fetch func(context.Context, *v1.AuditLogExport, int, int) ([]T, error),
	convert func(T) U,
) (totalSize int64, err error) {
	offset := 0
	batchNumber := 0

	pr, pw := io.Pipe()
	defer pr.Close()

	uploadErrCh := make(chan error, 1)
	go func() {
		defer close(uploadErrCh)
		err := storageProvider.Upload(ctx, storageConfig, bucket, exportPath, pr)
		_ = pr.CloseWithError(err)
		uploadErrCh <- err
	}()

	var writerClosed bool
	defer func() {
		if err != nil {
			if !writerClosed {
				_ = pw.CloseWithError(err)
			}
			<-uploadErrCh
		}
	}()

	for {
		logs, err := fetch(ctx, export, batchSize, offset)
		if err != nil {
			return 0, fmt.Errorf("failed to get audit logs batch %d: %w", batchNumber, err)
		}
		if len(logs) == 0 {
			break
		}

		batchData, err := formatLogs(logs, convert)
		if err != nil {
			return 0, fmt.Errorf("failed to format logs batch %d: %w", batchNumber, err)
		}
		if _, err := pw.Write(batchData); err != nil {
			return 0, fmt.Errorf("failed to write to pipe: %w", err)
		}

		totalSize += int64(len(batchData))
		offset += len(logs)
		batchNumber++
	}

	writerClosed = true
	if err := pw.Close(); err != nil {
		return totalSize, fmt.Errorf("failed to close pipe: %w", err)
	}
	if err := <-uploadErrCh; err != nil {
		return totalSize, fmt.Errorf("upload failed: %w", err)
	}

	return totalSize, nil
}

func generateExportPath(name, keyPrefix, defaultPrefix string) string {
	now := time.Now()
	filename := fmt.Sprintf("%s-%s.jsonl", name, now.Format(time.RFC3339))

	if keyPrefix == "" {
		keyPrefix = fmt.Sprintf("%s/%04d/%02d/%02d", defaultPrefix, now.Year(), now.Month(), now.Day())
	}
	if keyPrefix != "" && !strings.HasSuffix(keyPrefix, "/") {
		keyPrefix += "/"
	}

	return keyPrefix + filename
}

package auditlogexport

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlogexport"
	"github.com/obot-platform/obot/pkg/controller/handlers/auditlogexportcommon"
	client "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Handler struct {
	gatewayClient *client.Client
	credProvider  *auditlogexport.CredentialProvider
}

func NewHandler(gatewayClient *client.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
		credProvider:  auditlogexport.NewCredentialProvider(gatewayClient),
	}
}

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

	if err := h.performExport(req.Ctx, export); err != nil {
		export.Status.State = types.AuditLogExportStateFailed
		export.Status.Error = err.Error()

		if statusErr := req.Client.Status().Update(req.Ctx, export); statusErr != nil {
			return fmt.Errorf("failed to update failed export status: %w", statusErr)
		}

		return fmt.Errorf("audit log export failed: %w", err)
	}

	return req.Client.Status().Update(req.Ctx, export)
}

func (h *Handler) performExport(ctx context.Context, export *v1.AuditLogExport) error {
	storageConfig, err := h.credProvider.GetStorageConfig(ctx)
	if err != nil {
		return fmt.Errorf("failed to get storage config: %w", err)
	}
	if storageConfig == nil {
		return fmt.Errorf("storage config is nil")
	}

	var provider types.StorageProviderType
	if storageConfig.S3Config != nil {
		provider = types.StorageProviderS3
	} else if storageConfig.GCSConfig != nil {
		provider = types.StorageProviderGCS
	} else if storageConfig.AzureConfig != nil {
		provider = types.StorageProviderAzureBlob
	} else if storageConfig.CustomS3Config != nil {
		provider = types.StorageProviderCustomS3
	} else {
		return fmt.Errorf("invalid storage config, no storage provider found")
	}

	// Create storage provider
	storageProvider, err := auditlogexport.NewStorageProvider(provider)
	if err != nil {
		return fmt.Errorf("failed to create storage provider: %w", err)
	}

	export.Status.StorageProvider = provider

	exportPath := auditlogexportcommon.GenerateExportPath(export.Spec.Name, export.Spec.KeyPrefix, "mcp-audit-logs")

	// Use streaming export with batching
	exportSize, err := h.streamingExport(ctx, export, storageProvider, exportPath)
	if err != nil {
		return fmt.Errorf("failed to perform streaming export: %w", err)
	}

	// Update export status with results
	export.Status.ExportSize = exportSize
	export.Status.ExportPath = exportPath
	export.Status.State = types.AuditLogExportStateCompleted
	export.Status.CompletedAt = &metav1.Time{Time: time.Now()}

	return nil
}

func (h *Handler) streamingExport(ctx context.Context, export *v1.AuditLogExport, storageProvider auditlogexport.StorageProvider, exportPath string) (int64, error) {
	storageConfig, err := h.credProvider.GetStorageConfig(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to get storage config: %w", err)
	}

	const batchSize = 10000 // Process 10,000 records per batch

	var totalSize int64
	offset := 0
	batchNumber := 0

	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()

	uploadErrCh := make(chan error, 1)
	go func() {
		defer close(uploadErrCh)
		err := storageProvider.Upload(ctx, *storageConfig, export.Spec.Bucket, exportPath, pr)
		if err != nil {
			_ = pr.CloseWithError(err)
		}
		uploadErrCh <- err
	}()

	for {
		// Prepare batch options
		opts := client.MCPAuditLogOptions{
			StartTime:                 export.Spec.StartTime.Time,
			EndTime:                   export.Spec.EndTime.Time,
			UserID:                    export.Spec.Filters.UserIDs,
			MCPID:                     export.Spec.Filters.MCPIDs,
			MCPServerDisplayName:      export.Spec.Filters.MCPServerDisplayNames,
			MCPServerCatalogEntryName: export.Spec.Filters.MCPServerCatalogEntryNames,
			CallType:                  export.Spec.Filters.CallTypes,
			CallIdentifier:            export.Spec.Filters.CallIdentifiers,
			SessionID:                 export.Spec.Filters.SessionIDs,
			ClientName:                export.Spec.Filters.ClientNames,
			ClientVersion:             export.Spec.Filters.ClientVersions,
			ResponseStatus:            export.Spec.Filters.ResponseStatuses,
			ClientIP:                  export.Spec.Filters.ClientIPs,
			Query:                     export.Spec.Filters.Query,
			Limit:                     batchSize,
			Offset:                    offset,
			WithRequestAndResponse:    export.Spec.WithRequestAndResponse,
		}

		// Get batch of logs from gateway
		logs, _, err := h.gatewayClient.GetMCPAuditLogs(ctx, opts)
		if err != nil {
			return 0, fmt.Errorf("failed to get audit logs batch %d: %w", batchNumber, err)
		}

		// If no logs in this batch, we're done
		if len(logs) == 0 {
			break
		}

		// Convert logs to the desired format
		batchData, err := auditlogexportcommon.FormatLogs(logs, gatewaytypes.ConvertMCPAuditLog)
		if err != nil {
			return 0, fmt.Errorf("failed to format logs batch %d: %w", batchNumber, err)
		}

		_, err = pw.Write(batchData)
		if err != nil {
			return 0, fmt.Errorf("failed to write to pipe: %w", err)
		}

		totalSize += int64(len(batchData))
		offset += len(logs)
		batchNumber++
	}

	if err := pw.Close(); err != nil {
		return totalSize, fmt.Errorf("failed to close pipe: %w", err)
	}

	// Wait for upload to complete
	if err := <-uploadErrCh; err != nil {
		return totalSize, fmt.Errorf("upload failed: %w", err)
	}

	return totalSize, nil
}

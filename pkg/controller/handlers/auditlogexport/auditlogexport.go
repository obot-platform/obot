package auditlogexport

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlogexport"
	client "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	client           kclient.Client
	gptClient        *gptscript.GPTScript
	gatewayClient    *client.Client
	credProvider     *auditlogexport.GPTScriptCredentialProvider
	encryptionConfig *encryptionconfig.EncryptionConfiguration
}

func NewHandler(client kclient.Client, gptClient *gptscript.GPTScript, gatewayClient *client.Client, encryptionConfig *encryptionconfig.EncryptionConfiguration) *Handler {
	return &Handler{
		client:           client,
		gptClient:        gptClient,
		gatewayClient:    gatewayClient,
		credProvider:     auditlogexport.NewGPTScriptCredentialProvider(gptClient),
		encryptionConfig: encryptionConfig,
	}
}

func (h *Handler) ExportAuditLogs(req router.Request, _ router.Response) error {
	export := req.Object.(*v1.AuditLogExport)

	if export.Status.State == types.AuditLogExportStateCompleted || export.Status.State == types.AuditLogExportStateFailed {
		return nil
	}

	export.Status.State = types.AuditLogExportStateRunning
	export.Status.StartedAt = &metav1.Time{Time: time.Now()}

	if err := h.client.Status().Update(req.Ctx, export); err != nil {
		return fmt.Errorf("failed to update export status: %w", err)
	}

	if err := h.performExport(req.Ctx, export); err != nil {
		export.Status.State = types.AuditLogExportStateFailed
		export.Status.Error = err.Error()

		if statusErr := h.client.Status().Update(req.Ctx, export); statusErr != nil {
			return fmt.Errorf("failed to update failed export status: %w", statusErr)
		}

		return fmt.Errorf("audit log export failed: %w", err)
	}

	return h.client.Status().Update(req.Ctx, export)
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
	}

	// Create storage provider
	storageProvider, err := auditlogexport.NewStorageProvider(provider, h.credProvider)
	if err != nil {
		return fmt.Errorf("failed to create storage provider: %w", err)
	}

	export.Status.StorageProvider = provider

	// Generate export path
	exportPath := h.generateExportPath(export)

	// Use streaming export with batching
	_, exportSize, err := h.streamingExport(ctx, export, storageProvider, exportPath)
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

func (h *Handler) streamingExport(ctx context.Context, export *v1.AuditLogExport, storageProvider auditlogexport.StorageProvider, exportPath string) (int64, int64, error) {
	storageConfig, err := h.credProvider.GetStorageConfig(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get storage config: %w", err)
	}

	const batchSize = 10000 // Process 10,000 records per batch

	var totalRecords int64
	var totalSize int64
	offset := 0
	batchNumber := 0

	pr, pw := io.Pipe()
	defer pr.Close()

	uploadErrCh := make(chan error, 1)
	go func() {
		defer close(uploadErrCh)
		err := storageProvider.Upload(ctx, *storageConfig, export.Spec.Bucket, exportPath, pr)
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
		logs, total, err := h.gatewayClient.GetMCPAuditLogs(ctx, opts)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get audit logs batch %d: %w", batchNumber, err)
		}

		totalRecords = total

		// If no logs in this batch, we're done
		if len(logs) == 0 {
			break
		}

		// Convert logs to the desired format
		batchData, err := h.formatLogs(logs)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to format logs batch %d: %w", batchNumber, err)
		}

		_, err = pw.Write(batchData)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to write to pipe: %w", err)
		}

		totalSize += int64(len(batchData))
		offset += len(logs)
		batchNumber++
	}

	if err := pw.Close(); err != nil {
		return totalRecords, totalSize, fmt.Errorf("failed to close pipe: %w", err)
	}

	// Wait for upload to complete
	if err := <-uploadErrCh; err != nil {
		return totalRecords, totalSize, fmt.Errorf("upload failed: %w", err)
	}

	return totalRecords, totalSize, nil
}

func (h *Handler) formatLogs(logs []gatewaytypes.MCPAuditLog) ([]byte, error) {
	var lines []string

	// Convert each log to a JSON line
	for _, log := range logs {
		apiLog := gatewaytypes.ConvertMCPAuditLog(log)
		jsonBytes, err := json.Marshal(apiLog)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal log entry: %w", err)
		}
		lines = append(lines, string(jsonBytes))
	}

	// Join with newlines and add a final newline
	result := strings.Join(lines, "\n")
	if len(lines) > 0 {
		result += "\n"
	}

	return []byte(result), nil
}

func (h *Handler) generateExportPath(export *v1.AuditLogExport) string {
	now := time.Now()
	timestamp := now.Format(time.RFC3339)
	filename := fmt.Sprintf("%s-%s.jsonl", export.Spec.Name, timestamp)

	// Use keyPrefix if provided, otherwise use default date-based prefix
	keyPrefix := export.Spec.KeyPrefix
	if keyPrefix == "" {
		// Generate default prefix with year/month/day
		keyPrefix = fmt.Sprintf("mcp-audit-logs/%04d/%02d/%02d", now.Year(), now.Month(), now.Day())
	}

	// Ensure keyPrefix ends with / if it's not empty
	if keyPrefix != "" && !strings.HasSuffix(keyPrefix, "/") {
		keyPrefix += "/"
	}

	return keyPrefix + filename
}

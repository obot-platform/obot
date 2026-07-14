package auditlogexport

import (
	"context"
	"fmt"
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

	var err error
	switch export.Spec.EffectiveType() {
	case types.AuditLogTypeMCP:
		err = auditlogexportcommon.PerformExport(req.Ctx, h.credProvider, export, "mcp-audit-logs", h.fetchMCPAuditLogs, gatewaytypes.ConvertMCPAuditLog)
	case types.AuditLogTypeLLM:
		err = auditlogexportcommon.PerformExport(req.Ctx, h.credProvider, export, "llm-audit-logs", h.fetchLLMAuditLogs, gatewaytypes.ConvertLLMAuditLog)
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
	return client.MCPAuditLogOptions{
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
		Limit:                     limit,
		Offset:                    offset,
		WithRequestAndResponse:    export.Spec.WithRequestAndResponse,
	}
}

func llmAuditLogOptionsFromExport(export *v1.AuditLogExport, limit, offset int) client.LLMAuditLogOptions {
	return client.LLMAuditLogOptions{
		StartTime:           export.Spec.StartTime.Time,
		EndTime:             export.Spec.EndTime.Time,
		UserID:              export.Spec.LLMFilters.UserIDs,
		ModelProvider:       export.Spec.LLMFilters.ModelProviders,
		TargetModel:         export.Spec.LLMFilters.TargetModels,
		RequestPath:         export.Spec.LLMFilters.RequestPaths,
		ResponseStatus:      export.Spec.LLMFilters.ResponseStatuses,
		Outcome:             export.Spec.LLMFilters.Outcomes,
		Client:              export.Spec.LLMFilters.Clients,
		ClientSessionID:     export.Spec.LLMFilters.ClientSessionIDs,
		Query:               export.Spec.LLMFilters.Query,
		Limit:               limit,
		Offset:              offset,
		WithSensitiveFields: export.Spec.WithRequestAndResponse,
		SortBy:              "created_at",
		SortOrder:           "asc",
	}
}

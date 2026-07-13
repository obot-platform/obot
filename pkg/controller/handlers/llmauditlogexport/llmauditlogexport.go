package llmauditlogexport

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
	export := req.Object.(*v1.LLMAuditLogExport)
	if export.Status.State == types.AuditLogExportStateCompleted || export.Status.State == types.AuditLogExportStateFailed {
		return nil
	}

	export.Status.State = types.AuditLogExportStateRunning
	export.Status.StartedAt = &metav1.Time{Time: time.Now()}
	if err := req.Client.Status().Update(req.Ctx, export); err != nil {
		return fmt.Errorf("failed to update export status: %w", err)
	}

	err := auditlogexportcommon.PerformExport(req.Ctx, h.credProvider, export, "llm-audit-logs", h.fetchAuditLogs, gatewaytypes.ConvertLLMAuditLog)
	if err != nil {
		export.Status.State = types.AuditLogExportStateFailed
		export.Status.Error = err.Error()
		if statusErr := req.Client.Status().Update(req.Ctx, export); statusErr != nil {
			return fmt.Errorf("failed to update failed export status: %w", statusErr)
		}
		return fmt.Errorf("LLM audit log export failed: %w", err)
	}

	return req.Client.Status().Update(req.Ctx, export)
}

func (h *Handler) fetchAuditLogs(ctx context.Context, export *v1.LLMAuditLogExport, limit, offset int) ([]gatewaytypes.LLMAuditLog, error) {
	opts := llmAuditLogOptionsFromExport(export, limit, offset)
	logs, _, err := h.gatewayClient.GetLLMAuditLogs(ctx, opts)
	return logs, err
}

func llmAuditLogOptionsFromExport(export *v1.LLMAuditLogExport, limit, offset int) client.LLMAuditLogOptions {
	return client.LLMAuditLogOptions{
		StartTime:           export.Spec.StartTime.Time,
		EndTime:             export.Spec.EndTime.Time,
		UserID:              export.Spec.Filters.UserIDs,
		ModelProvider:       export.Spec.Filters.ModelProviders,
		TargetModel:         export.Spec.Filters.TargetModels,
		RequestPath:         export.Spec.Filters.RequestPaths,
		ResponseStatus:      export.Spec.Filters.ResponseStatuses,
		Outcome:             export.Spec.Filters.Outcomes,
		Client:              export.Spec.Filters.Clients,
		ClientSessionID:     export.Spec.Filters.ClientSessionIDs,
		Query:               export.Spec.Filters.Query,
		Limit:               limit,
		Offset:              offset,
		WithSensitiveFields: export.Spec.WithSensitiveFields,
		SortBy:              "created_at",
		SortOrder:           "asc",
	}
}

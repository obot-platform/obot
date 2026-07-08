package handlers

import (
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *AuditLogExportHandler) CreateLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	var createReq types.LLMAuditLogExportCreateRequest
	if err := req.Read(&createReq); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}
	if err := validateLLMExportRequest(&createReq); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	export := &v1.LLMAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.LLMAuditLogExportPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.LLMAuditLogExportSpec{
			Name:                createReq.Name,
			StartTime:           metav1.NewTime(createReq.StartTime.GetTime()),
			EndTime:             metav1.NewTime(createReq.EndTime.GetTime()),
			Filters:             createReq.Filters,
			WithSensitiveFields: req.UserIsAuditor(),
			Bucket:              createReq.Bucket,
			KeyPrefix:           createReq.KeyPrefix,
		},
	}
	if err := req.Storage.Create(req.Context(), export); err != nil {
		return err
	}
	return req.Write(convertLLMExportToAPI(export))
}

func (h *AuditLogExportHandler) ListLLMAuditLogExports(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	var exports v1.LLMAuditLogExportList
	if err := req.Storage.List(req.Context(), &exports, &kclient.ListOptions{Namespace: req.Namespace()}); err != nil {
		return err
	}

	result := make([]types.LLMAuditLogExportResponse, 0, len(exports.Items))
	for _, export := range exports.Items {
		result = append(result, convertLLMExportToAPI(&export))
	}
	return req.Write(types.LLMAuditLogExportListResponse{Items: result, Total: int64(len(result))})
}

func (h *AuditLogExportHandler) GetLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("export ID is required")
	}

	var export v1.LLMAuditLogExport
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Name: exportName, Namespace: req.Namespace()}, &export); err != nil {
		return err
	}
	return req.Write(convertLLMExportToAPI(&export))
}

func (h *AuditLogExportHandler) DeleteLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("export ID is required")
	}
	return req.Storage.Delete(req.Context(), &v1.LLMAuditLogExport{ObjectMeta: metav1.ObjectMeta{Name: exportName, Namespace: req.Namespace()}})
}

func (h *AuditLogExportHandler) CreateScheduledLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	var createReq types.ScheduledLLMAuditLogExportCreateRequest
	if err := req.Read(&createReq); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}
	if createReq.Name == "" {
		return types.NewErrBadRequest("validation failed: name is required")
	}
	if createReq.Bucket == "" {
		return types.NewErrBadRequest("validation failed: bucket is required")
	}

	scheduledExport := &v1.ScheduledLLMAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ScheduledLLMAuditLogExportPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ScheduledLLMAuditLogExportSpec{
			Name:                  createReq.Name,
			Enabled:               true,
			Schedule:              h.convertSchedule(createReq.Schedule),
			RetentionPeriodInDays: createReq.RetentionPeriodInDays,
			Filters:               createReq.Filters,
			WithSensitiveFields:   req.UserIsAuditor(),
			Bucket:                createReq.Bucket,
			KeyPrefix:             createReq.KeyPrefix,
		},
	}
	if err := req.Storage.Create(req.Context(), scheduledExport); err != nil {
		return err
	}
	return req.Write(h.convertScheduledLLMExportToAPI(scheduledExport))
}

func (h *AuditLogExportHandler) ListScheduledLLMAuditLogExports(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	var scheduledExports v1.ScheduledLLMAuditLogExportList
	if err := req.Storage.List(req.Context(), &scheduledExports, &kclient.ListOptions{Namespace: req.Namespace()}); err != nil {
		return err
	}

	result := make([]types.ScheduledLLMAuditLogExportResponse, 0, len(scheduledExports.Items))
	for _, export := range scheduledExports.Items {
		result = append(result, h.convertScheduledLLMExportToAPI(&export))
	}
	return req.Write(types.ScheduledLLMAuditLogExportListResponse{Items: result, Total: int64(len(result))})
}

func (h *AuditLogExportHandler) GetScheduledLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("scheduled export ID is required")
	}

	var scheduledExport v1.ScheduledLLMAuditLogExport
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Name: exportName, Namespace: req.Namespace()}, &scheduledExport); err != nil {
		return err
	}
	return req.Write(h.convertScheduledLLMExportToAPI(&scheduledExport))
}

func (h *AuditLogExportHandler) UpdateScheduledLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("scheduled export ID is required")
	}

	var updateReq types.ScheduledLLMAuditLogExportUpdateRequest
	if err := req.Read(&updateReq); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	var scheduledExport v1.ScheduledLLMAuditLogExport
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Name: exportName, Namespace: req.Namespace()}, &scheduledExport); err != nil {
		return err
	}
	if !req.UserIsAuditor() && scheduledExport.Spec.WithSensitiveFields {
		return types.NewErrForbidden("you are not authorized to edit this scheduled export")
	}

	if updateReq.Enabled != nil {
		scheduledExport.Spec.Enabled = *updateReq.Enabled
	}
	if updateReq.Schedule != nil {
		scheduledExport.Spec.Schedule = h.convertSchedule(*updateReq.Schedule)
	}
	if updateReq.RetentionPeriodInDays != nil {
		scheduledExport.Spec.RetentionPeriodInDays = *updateReq.RetentionPeriodInDays
	}
	if updateReq.Filters != nil {
		scheduledExport.Spec.Filters = *updateReq.Filters
	}
	if updateReq.Bucket != nil {
		scheduledExport.Spec.Bucket = *updateReq.Bucket
	}
	if updateReq.KeyPrefix != nil {
		scheduledExport.Spec.KeyPrefix = *updateReq.KeyPrefix
	}
	if updateReq.Name != nil {
		scheduledExport.Spec.Name = *updateReq.Name
	}

	if err := req.Storage.Update(req.Context(), &scheduledExport); err != nil {
		return err
	}
	return req.Write(h.convertScheduledLLMExportToAPI(&scheduledExport))
}

func (h *AuditLogExportHandler) DeleteScheduledLLMAuditLogExport(req api.Context) error {
	if err := requireLLMAuditExportAccess(req); err != nil {
		return err
	}

	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("scheduled export ID is required")
	}
	return req.Storage.Delete(req.Context(), &v1.ScheduledLLMAuditLogExport{ObjectMeta: metav1.ObjectMeta{Name: exportName, Namespace: req.Namespace()}})
}

func requireLLMAuditExportAccess(req api.Context) error {
	if !req.UserIsAdmin() && !req.UserIsAuditor() {
		return types.NewErrHTTP(http.StatusNotFound, "not found")
	}
	return nil
}

func validateLLMExportRequest(req *types.LLMAuditLogExportCreateRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.Bucket == "" {
		return fmt.Errorf("bucket is required")
	}
	if req.StartTime.GetTime().After(req.EndTime.GetTime()) {
		return fmt.Errorf("start time must be before end time")
	}
	return nil
}

func convertLLMExportToAPI(export *v1.LLMAuditLogExport) types.LLMAuditLogExportResponse {
	result := types.LLMAuditLogExportResponse{
		ID:              export.Name,
		Name:            export.Spec.Name,
		StorageProvider: export.Status.StorageProvider,
		Bucket:          export.Spec.Bucket,
		KeyPrefix:       export.Spec.KeyPrefix,
		StartTime:       types.Time{Time: export.Spec.StartTime.Time},
		EndTime:         types.Time{Time: export.Spec.EndTime.Time},
		Filters:         export.Spec.Filters,
		State:           string(export.Status.State),
		Error:           export.Status.Error,
		ExportSize:      export.Status.ExportSize,
		ExportPath:      export.Status.ExportPath,
		CreatedAt:       types.Time{Time: export.CreationTimestamp.Time},
	}
	if export.Status.StartedAt != nil {
		result.StartedAt = types.Time{Time: export.Status.StartedAt.Time}
	}
	if export.Status.CompletedAt != nil {
		result.CompletedAt = types.Time{Time: export.Status.CompletedAt.Time}
	}
	return result
}

func (h *AuditLogExportHandler) convertScheduledLLMExportToAPI(export *v1.ScheduledLLMAuditLogExport) types.ScheduledLLMAuditLogExportResponse {
	result := types.ScheduledLLMAuditLogExportResponse{
		ID:                    export.Name,
		Bucket:                export.Spec.Bucket,
		KeyPrefix:             export.Spec.KeyPrefix,
		Name:                  export.Spec.Name,
		Enabled:               export.Spec.Enabled,
		Schedule:              h.convertScheduleToAPI(export.Spec.Schedule),
		RetentionPeriodInDays: export.Spec.RetentionPeriodInDays,
		Filters:               export.Spec.Filters,
	}
	if export.Status.LastRunAt != nil {
		result.LastRunAt = types.Time{Time: export.Status.LastRunAt.Time}
	}
	return result
}

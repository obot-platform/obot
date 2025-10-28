package handlers

import (
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/auditlogexport"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/server/options/encryptionconfig"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type AuditLogExportHandler struct {
	client           kclient.Client
	gptClient        *gptscript.GPTScript
	credProvider     *auditlogexport.GPTScriptCredentialProvider
	encryptionConfig *encryptionconfig.EncryptionConfiguration
}

func NewAuditLogExportHandler(client kclient.Client, gptClient *gptscript.GPTScript, encryptionConfig *encryptionconfig.EncryptionConfiguration) *AuditLogExportHandler {
	return &AuditLogExportHandler{
		client:           client,
		gptClient:        gptClient,
		credProvider:     auditlogexport.NewGPTScriptCredentialProvider(gptClient),
		encryptionConfig: encryptionConfig,
	}
}

// CreateAuditLogExport creates a new audit log export
func (h *AuditLogExportHandler) CreateAuditLogExport(req api.Context) error {
	var createReq types.AuditLogExportCreateRequest
	if err := req.Read(&createReq); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	// Validate the request
	if err := h.validateExportRequest(&createReq); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	// Create the AuditLogExport resource
	export := &v1.AuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AuditLogExportPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.AuditLogExportSpec{
			Name:                   createReq.Name,
			StartTime:              metav1.NewTime(createReq.StartTime.GetTime()),
			EndTime:                metav1.NewTime(createReq.EndTime.GetTime()),
			Filters:                createReq.Filters,
			WithRequestAndResponse: req.UserIsAuditor(),
			Bucket:                 createReq.Bucket,
			KeyPrefix:              createReq.KeyPrefix,
		},
		Status: v1.AuditLogExportStatus{
			State: types.AuditLogExportStatePending,
		},
	}

	if err := h.client.Create(req.Context(), export); err != nil {
		return err
	}

	return req.Write(h.convertExportToAPI(export))
}

// ListAuditLogExports lists audit log exports
func (h *AuditLogExportHandler) ListAuditLogExports(req api.Context) error {
	var exports v1.AuditLogExportList
	if err := h.client.List(req.Context(), &exports, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	result := make([]types.AuditLogExportResponse, 0, len(exports.Items))
	for _, export := range exports.Items {
		result = append(result, h.convertExportToAPI(&export))
	}

	return req.Write(types.AuditLogExportListResponse{
		Items: result,
		Total: int64(len(result)),
	})
}

// GetAuditLogExport gets a specific audit log export
func (h *AuditLogExportHandler) GetAuditLogExport(req api.Context) error {
	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("export ID is required")
	}

	var export v1.AuditLogExport
	if err := h.client.Get(req.Context(), kclient.ObjectKey{
		Name:      exportName,
		Namespace: req.Namespace(),
	}, &export); err != nil {
		return err
	}

	return req.Write(h.convertExportToAPI(&export))
}

// DeleteAuditLogExport deletes an audit log export
func (h *AuditLogExportHandler) DeleteAuditLogExport(req api.Context) error {
	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("export ID is required")
	}

	export := &v1.AuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      exportName,
			Namespace: req.Namespace(),
		},
	}

	return h.client.Delete(req.Context(), export)
}

// CreateScheduledAuditLogExport creates a new scheduled audit log export
func (h *AuditLogExportHandler) CreateScheduledAuditLogExport(req api.Context) error {
	var createReq types.ScheduledAuditLogExportCreateRequest
	if err := req.Read(&createReq); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	// Validate the request
	if err := h.validateScheduledExportRequest(&createReq); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	// Create the ScheduledAuditLogExport resource
	scheduledExport := &v1.ScheduledAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ScheduledAuditLogExportPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ScheduledAuditLogExportSpec{
			Name:                   createReq.Name,
			Enabled:                true,
			Schedule:               h.convertSchedule(createReq.Schedule),
			RetentionPeriodInDays:  createReq.RetentionPeriodInDays,
			Filters:                createReq.Filters,
			WithRequestAndResponse: req.UserIsAuditor(),
			Bucket:                 createReq.Bucket,
			KeyPrefix:              createReq.KeyPrefix,
		},
	}

	if err := h.client.Create(req.Context(), scheduledExport); err != nil {
		return err
	}

	return req.Write(h.convertScheduledExportToAPI(scheduledExport))
}

// ListScheduledAuditLogExports lists scheduled audit log exports
func (h *AuditLogExportHandler) ListScheduledAuditLogExports(req api.Context) error {
	var scheduledExports v1.ScheduledAuditLogExportList
	if err := h.client.List(req.Context(), &scheduledExports, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	result := make([]types.ScheduledAuditLogExportResponse, 0, len(scheduledExports.Items))
	for _, export := range scheduledExports.Items {
		result = append(result, h.convertScheduledExportToAPI(&export))
	}

	return req.Write(types.ScheduledAuditLogExportListResponse{
		Items: result,
		Total: int64(len(result)),
	})
}

// GetScheduledAuditLogExport gets a specific scheduled audit log export
func (h *AuditLogExportHandler) GetScheduledAuditLogExport(req api.Context) error {
	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("scheduled export ID is required")
	}

	var scheduledExport v1.ScheduledAuditLogExport
	if err := h.client.Get(req.Context(), kclient.ObjectKey{
		Name:      exportName,
		Namespace: req.Namespace(),
	}, &scheduledExport); err != nil {
		return err
	}

	return req.Write(h.convertScheduledExportToAPI(&scheduledExport))
}

// UpdateScheduledAuditLogExport updates a scheduled audit log export
func (h *AuditLogExportHandler) UpdateScheduledAuditLogExport(req api.Context) error {
	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("scheduled export ID is required")
	}

	var updateReq types.ScheduledAuditLogExportUpdateRequest
	if err := req.Read(&updateReq); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	var scheduledExport v1.ScheduledAuditLogExport
	if err := h.client.Get(req.Context(), kclient.ObjectKey{
		Name:      exportName,
		Namespace: req.Namespace(),
	}, &scheduledExport); err != nil {
		return err
	}

	// Update the spec based on the request
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

	if err := h.client.Update(req.Context(), &scheduledExport); err != nil {
		return err
	}

	return req.Write(h.convertScheduledExportToAPI(&scheduledExport))
}

// DeleteScheduledAuditLogExport deletes a scheduled audit log export
func (h *AuditLogExportHandler) DeleteScheduledAuditLogExport(req api.Context) error {
	exportName := req.PathValue("id")
	if exportName == "" {
		return types.NewErrBadRequest("scheduled export ID is required")
	}

	scheduledExport := &v1.ScheduledAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			Name:      exportName,
			Namespace: req.Namespace(),
		},
	}

	return h.client.Delete(req.Context(), scheduledExport)
}

// ConfigureStorageCredentials configures storage provider credentials
func (h *AuditLogExportHandler) ConfigureStorageCredentials(req api.Context) error {
	var storageConfig types.StorageProviderConfigInput
	if err := req.Read(&storageConfig); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	err := h.credProvider.StoreCredentials(req.Context(), storageConfig)
	if err != nil {
		return err
	}

	return req.Write(map[string]string{
		"status": "credentials configured successfully",
	})
}

// GetStorageCredentials gets the storage provider credentials
func (h *AuditLogExportHandler) GetStorageCredentials(req api.Context) error {
	storageConfig, err := h.credProvider.GetStorageConfig(req.Context())
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to get storage credentials: %w", err)
	} else if errors.As(err, &gptscript.ErrNotFound{}) {
		return types.NewErrNotFound("storage credentials not found")
	}

	result := types.StorageCredentialsResponse{}

	// remove any sensitive information from the storage config
	if storageConfig.S3Config != nil {
		if storageConfig.S3Config.AccessKeyID != "" || storageConfig.S3Config.SecretAccessKey != "" {
			storageConfig.S3Config.AccessKeyID = ""
			storageConfig.S3Config.SecretAccessKey = ""
		} else {
			result.UseWorkloadIdentity = true
		}
		result.Provider = types.StorageProviderS3
		result.S3Config = storageConfig.S3Config
	} else if storageConfig.GCSConfig != nil {
		if storageConfig.GCSConfig.ServiceAccountJSON != "" {
			storageConfig.GCSConfig.ServiceAccountJSON = ""
		} else {
			result.UseWorkloadIdentity = true
		}
		result.Provider = types.StorageProviderGCS
		result.GCSConfig = storageConfig.GCSConfig
	} else if storageConfig.AzureConfig != nil {
		if storageConfig.AzureConfig.ClientID != "" || storageConfig.AzureConfig.TenantID != "" || storageConfig.AzureConfig.ClientSecret != "" {
			storageConfig.AzureConfig.ClientID = ""
			storageConfig.AzureConfig.TenantID = ""
			storageConfig.AzureConfig.ClientSecret = ""
		} else {
			result.UseWorkloadIdentity = true
		}
		result.Provider = types.StorageProviderAzureBlob
		result.AzureConfig = storageConfig.AzureConfig
	} else if storageConfig.CustomS3Config != nil {
		if storageConfig.CustomS3Config.AccessKeyID != "" || storageConfig.CustomS3Config.SecretAccessKey != "" {
			storageConfig.CustomS3Config.AccessKeyID = ""
			storageConfig.CustomS3Config.SecretAccessKey = ""
		}
		result.Provider = types.StorageProviderCustomS3
		result.CustomS3Config = storageConfig.CustomS3Config
	}

	return req.Write(result)
}

// DeleteStorageCredentials deletes the storage provider credentials
func (h *AuditLogExportHandler) DeleteStorageCredentials(req api.Context) error {
	err := h.credProvider.DeleteCredentials(req.Context())
	if err != nil {
		return err
	}
	return req.Write(map[string]string{
		"status": "credentials deleted successfully",
	})
}

// TestStorageCredentials tests storage provider credentials
func (h *AuditLogExportHandler) TestStorageCredentials(req api.Context) error {
	var storageConfig types.StorageCredentialsTestRequest
	if err := req.Read(&storageConfig); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	err := h.credProvider.TestCredentials(req.Context(), storageConfig.StorageConfig)
	if err != nil {
		return req.Write(types.StorageCredentialsTestResponse{
			Success: false,
			Message: err.Error(),
		})
	}

	return req.Write(types.StorageCredentialsTestResponse{
		Success: true,
		Message: "credentials are valid and working",
	})
}

// Helper methods for conversions
func (h *AuditLogExportHandler) validateExportRequest(req *types.AuditLogExportCreateRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.StartTime.GetTime().After(req.EndTime.GetTime()) {
		return fmt.Errorf("start time must be before end time")
	}
	return nil
}

func (h *AuditLogExportHandler) validateScheduledExportRequest(req *types.ScheduledAuditLogExportCreateRequest) error {
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

func (h *AuditLogExportHandler) convertSchedule(schedule types.Schedule) v1.Schedule {
	return v1.Schedule{
		Interval: schedule.Interval,
		Hour:     schedule.Hour,
		Minute:   schedule.Minute,
		Day:      schedule.Day,
		Weekday:  schedule.Weekday,
		TimeZone: schedule.TimeZone,
	}
}

func (h *AuditLogExportHandler) convertExportToAPI(export *v1.AuditLogExport) types.AuditLogExportResponse {
	result := types.AuditLogExportResponse{
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

func (h *AuditLogExportHandler) convertScheduledExportToAPI(export *v1.ScheduledAuditLogExport) types.ScheduledAuditLogExportResponse {
	result := types.ScheduledAuditLogExportResponse{
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

func (h *AuditLogExportHandler) convertScheduleToAPI(schedule v1.Schedule) types.Schedule {
	return types.Schedule{
		Interval: schedule.Interval,
		Hour:     schedule.Hour,
		Minute:   schedule.Minute,
		Day:      schedule.Day,
		Weekday:  schedule.Weekday,
		TimeZone: schedule.TimeZone,
	}
}

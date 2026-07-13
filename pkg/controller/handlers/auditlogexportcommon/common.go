package auditlogexportcommon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auditlogexport"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/obot-platform/nah/pkg/router"
)

const batchSize = 10_000

// Export is the small common surface shared by MCP and LLM audit log export resources.
type Export interface {
	Bucket() string
	SpecName() string
	KeyPrefix() string
	ExportStatus() *v1.AuditLogExportStatus
}

// ScheduledExport is the common controller surface shared by scheduled MCP and LLM audit log exports.
type ScheduledExport interface {
	client.Object
	Enabled() bool
	GetSchedule() v1.Schedule
	LastRunAt() *metav1.Time
	SetLastRunAt(metav1.Time)
}

// PerformExport streams audit logs to configured object storage and marks the export completed.
// The fetch function provides resource-specific audit log batches; convert maps each record to its JSONL export shape.
func PerformExport[E Export, T any, U any](
	ctx context.Context,
	credProvider *auditlogexport.CredentialProvider,
	export E,
	defaultPrefix string,
	fetch func(context.Context, E, int, int) ([]T, error),
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

	status := export.ExportStatus()
	status.StorageProvider = provider

	exportPath := generateExportPath(export.SpecName(), export.KeyPrefix(), defaultPrefix)
	exportSize, err := streamingExport(ctx, *storageConfig, storageProvider, export, export.Bucket(), exportPath, fetch, convert)
	if err != nil {
		return fmt.Errorf("failed to perform streaming export: %w", err)
	}

	status.ExportSize = exportSize
	status.ExportPath = exportPath
	status.State = types.AuditLogExportStateCompleted
	status.CompletedAt = &metav1.Time{Time: time.Now()}

	return nil
}

// ScheduleExports runs the shared scheduled-export controller flow and delegates resource creation to createExport.
func ScheduleExports[T ScheduledExport](
	req router.Request,
	resp router.Response,
	createExport func(router.Request, T, time.Time) error,
) error {
	scheduledExport, ok := req.Object.(T)
	if !ok {
		return fmt.Errorf("unexpected scheduled audit log export type %T", req.Object)
	}

	if !scheduledExport.Enabled() {
		return nil
	}

	next, err := calculateNextRunTime(scheduledExport)
	if err != nil {
		return fmt.Errorf("failed to calculate next run time: %w", err)
	}

	if until := time.Until(next); until > 0 {
		if until < 10*time.Hour {
			resp.RetryAfter(until)
		}
		return nil
	}

	if err := createExport(req, scheduledExport, next); err != nil {
		return err
	}

	scheduledExport.SetLastRunAt(metav1.Now())

	return req.Client.Update(req.Ctx, scheduledExport)
}

// formatLogs writes one converted audit log per line in JSONL format.
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
func streamingExport[E any, T any, U any](
	ctx context.Context,
	storageConfig types.StorageConfig,
	storageProvider auditlogexport.StorageProvider,
	export E,
	bucket, exportPath string,
	fetch func(context.Context, E, int, int) ([]T, error),
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

// generateExportPath returns either the user-provided prefix or the date-based default prefix plus a timestamped JSONL filename.
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

// getScheduleAndTimezone converts the UI schedule model into a cron expression.
func getScheduleAndTimezone(schedule v1.Schedule) (string, string) {
	cron := ""
	switch schedule.Interval {
	case "hourly":
		cron = fmt.Sprintf("%d * * * *", schedule.Minute)
	case "daily":
		cron = fmt.Sprintf("%d %d * * *", schedule.Minute, schedule.Hour)
	case "weekly":
		cron = fmt.Sprintf("%d %d * * %d", schedule.Minute, schedule.Hour, schedule.Weekday)
	case "monthly":
		if schedule.Day < 0 {
			// The day being -1 means the last day of the month. The cron parsing package we use uses `L` for this.
			cron = fmt.Sprintf("%d %d L * *", schedule.Minute, schedule.Hour)
		} else if schedule.Day == 0 {
			cron = fmt.Sprintf("%d %d 1 * *", schedule.Minute, schedule.Hour)
		} else {
			cron = fmt.Sprintf("%d %d %d * *", schedule.Minute, schedule.Hour, schedule.Day)
		}
	}

	return cron, schedule.TimeZone
}

// calculateNextRunTime calculates the next scheduled export run from the last run, or creation time for first run.
func calculateNextRunTime(scheduledExport ScheduledExport) (time.Time, error) {
	lastRun := scheduledExport.LastRunAt()
	if lastRun == nil || lastRun.IsZero() {
		lastRun = &metav1.Time{Time: scheduledExport.GetCreationTimestamp().Time}
	}

	cron, timezone := getScheduleAndTimezone(scheduledExport.GetSchedule())
	var location *time.Location
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err == nil {
			location = loc
		}
	}
	if location != nil {
		lastRun = &metav1.Time{Time: lastRun.In(location)}
	}

	next, err := gronx.NextTickAfter(cron, lastRun.Time, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse schedule: %w", err)
	}

	return next, nil
}

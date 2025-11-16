package scheduledauditlogexport

import (
	"fmt"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Handler struct {
}

func NewHandler() *Handler {
	return &Handler{}
}

func (h *Handler) ScheduleExports(req router.Request, resp router.Response) error {
	scheduledExport := req.Object.(*v1.ScheduledAuditLogExport)

	if !scheduledExport.Spec.Enabled {
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

	if err := h.createExportFromSchedule(req, scheduledExport, next); err != nil {
		return err
	}

	scheduledExport.Status.LastRunAt = &[]metav1.Time{metav1.Now()}[0]

	return req.Client.Update(req.Ctx, scheduledExport)
}

func (h *Handler) createExportFromSchedule(req router.Request, scheduledExport *v1.ScheduledAuditLogExport, nextRunAt time.Time) error {
	var startTime time.Time
	if scheduledExport.Spec.RetentionPeriodInDays < 0 {
		startTime = time.Time{}
	} else {
		startTime = nextRunAt.Add(-24 * time.Hour * time.Duration(scheduledExport.Spec.RetentionPeriodInDays))
	}

	export := &v1.AuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.AuditLogExportPrefix,
			Namespace:    scheduledExport.Namespace,
		},
		Spec: v1.AuditLogExportSpec{
			Name:                   fmt.Sprintf("%s-%d", scheduledExport.Spec.Name, scheduledExport.Status.TotalExportsCreated+1),
			Bucket:                 scheduledExport.Spec.Bucket,
			KeyPrefix:              scheduledExport.Spec.KeyPrefix,
			StartTime:              metav1.NewTime(startTime),
			EndTime:                metav1.NewTime(nextRunAt),
			Filters:                scheduledExport.Spec.Filters,
			WithRequestAndResponse: scheduledExport.Spec.WithRequestAndResponse,
		},
	}

	if err := req.Client.Create(req.Ctx, export); err != nil {
		return fmt.Errorf("failed to create audit log export: %w", err)
	}

	scheduledExport.Status.TotalExportsCreated++

	return nil
}

func GetScheduleAndTimezone(scheduledExport *v1.ScheduledAuditLogExport) (string, string) {
	schedule := ""
	switch scheduledExport.Spec.Schedule.Interval {
	case "hourly":
		schedule = fmt.Sprintf("%d * * * *", scheduledExport.Spec.Schedule.Minute)
	case "daily":
		schedule = fmt.Sprintf("%d %d * * *", scheduledExport.Spec.Schedule.Minute, scheduledExport.Spec.Schedule.Hour)
	case "weekly":
		schedule = fmt.Sprintf("%d %d * * %d", scheduledExport.Spec.Schedule.Minute, scheduledExport.Spec.Schedule.Hour, scheduledExport.Spec.Schedule.Weekday)
	case "monthly":
		if scheduledExport.Spec.Schedule.Day < 0 {
			// The day being -1 means the last day of the month. The cron parsing package we use uses `L` for this.
			schedule = fmt.Sprintf("%d %d L * *", scheduledExport.Spec.Schedule.Minute, scheduledExport.Spec.Schedule.Hour)
		} else if scheduledExport.Spec.Schedule.Day == 0 {
			schedule = fmt.Sprintf("%d %d 1 * *", scheduledExport.Spec.Schedule.Minute, scheduledExport.Spec.Schedule.Hour)
		} else {
			schedule = fmt.Sprintf("%d %d %d * *", scheduledExport.Spec.Schedule.Minute, scheduledExport.Spec.Schedule.Hour, scheduledExport.Spec.Schedule.Day)
		}
	}
	return schedule, scheduledExport.Spec.Schedule.TimeZone
}

func calculateNextRunTime(scheduledExport *v1.ScheduledAuditLogExport) (time.Time, error) {
	lastRun := scheduledExport.Status.LastRunAt
	if lastRun.IsZero() {
		lastRun = &metav1.Time{Time: scheduledExport.CreationTimestamp.Time}
	}

	schedule, timezone := GetScheduleAndTimezone(scheduledExport)
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

	next, err := gronx.NextTickAfter(schedule, lastRun.Time, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse schedule: %w", err)
	}

	return next, nil
}

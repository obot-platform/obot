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

type Handler struct{}

func NewHandler() *Handler {
	return nil
}

func (*Handler) ScheduleExports(req router.Request, resp router.Response) error {
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

	if err := createExportFromSchedule(req, scheduledExport, next); err != nil {
		return err
	}

	now := metav1.Now()
	scheduledExport.Status.LastRunAt = &now

	return req.Client.Update(req.Ctx, scheduledExport)
}

func createExportFromSchedule(req router.Request, scheduledExport *v1.ScheduledAuditLogExport, nextRunAt time.Time) error {
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
			Type:                   scheduledExport.Spec.EffectiveType(),
			Bucket:                 scheduledExport.Spec.Bucket,
			KeyPrefix:              scheduledExport.Spec.KeyPrefix,
			StartTime:              metav1.NewTime(startTime),
			EndTime:                metav1.NewTime(nextRunAt),
			Filters:                scheduledExport.Spec.Filters,
			LLMFilters:             scheduledExport.Spec.LLMFilters,
			WithRequestAndResponse: scheduledExport.Spec.WithRequestAndResponse,
		},
	}

	if err := req.Client.Create(req.Ctx, export); err != nil {
		return fmt.Errorf("failed to create audit log export: %w", err)
	}

	scheduledExport.Status.TotalExportsCreated++

	return nil
}

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
			// The day being -1 means the last day of the month. The cron parser uses L for this.
			cron = fmt.Sprintf("%d %d L * *", schedule.Minute, schedule.Hour)
		} else if schedule.Day == 0 {
			cron = fmt.Sprintf("%d %d 1 * *", schedule.Minute, schedule.Hour)
		} else {
			cron = fmt.Sprintf("%d %d %d * *", schedule.Minute, schedule.Hour, schedule.Day)
		}
	}

	return cron, schedule.TimeZone
}

func calculateNextRunTime(scheduledExport *v1.ScheduledAuditLogExport) (time.Time, error) {
	lastRun := scheduledExport.Status.LastRunAt
	if lastRun.IsZero() {
		lastRun = &metav1.Time{Time: scheduledExport.CreationTimestamp.Time}
	}

	cron, timezone := getScheduleAndTimezone(scheduledExport.Spec.Schedule)
	if timezone != "" {
		if location, err := time.LoadLocation(timezone); err == nil {
			lastRun = &metav1.Time{Time: lastRun.In(location)}
		}
	}

	next, err := gronx.NextTickAfter(cron, lastRun.Time, false)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse schedule: %w", err)
	}

	return next, nil
}

package scheduledauditlogexport

import (
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pendingPollInterval = 30 * time.Second
)

type Handler struct{}

func NewHandler() *Handler {
	return nil
}

func (*Handler) ScheduleExports(req router.Request, resp router.Response) error {
	scheduled := req.Object.(*v1.ScheduledAuditLogExport)

	// Completion acknowledgement runs even while disabled. Only choosing new windows
	// depends on Enabled; otherwise disabling could strand a successful upload forever.
	if scheduled.Status.PendingExport != nil {
		return reconcilePendingExport(req, resp, scheduled)
	}
	if !scheduled.Spec.Enabled {
		return nil
	}
	next, err := calculateNextRunTime(scheduled)
	if err != nil {
		return fmt.Errorf("failed to calculate next run time: %w", err)
	}
	if until := time.Until(next); until > 0 {
		if until < 10*time.Hour {
			resp.RetryAfter(until)
		}
		return nil
	}

	scheduled.Status.PendingExport = pendingExportFromSchedule(scheduled, next)
	scheduled.Status.TotalExportsCreated++
	now := metav1.Now()
	scheduled.Status.LastRunAt = &now
	// Persist the exact window and configuration before creating its deterministic
	// job. A failed Create or a lost response cannot slide the initial backfill.
	if err := req.Client.Status().Update(req.Ctx, scheduled); err != nil {
		return err
	}
	return reconcilePendingExport(req, resp, scheduled)
}

// reconcilePendingExport recreates a missing fixed-window job or acknowledges its
// durable completion before allowing the schedule to choose another window.
func reconcilePendingExport(req router.Request, resp router.Response, scheduled *v1.ScheduledAuditLogExport) error {
	pending := scheduled.Status.PendingExport
	export := &v1.AuditLogExport{}
	err := req.Client.Get(req.Ctx, kclient.ObjectKey{Namespace: scheduled.Namespace, Name: pending.Name}, export)
	if apierrors.IsNotFound(err) {
		export = &v1.AuditLogExport{
			Name:      pending.Name,
			Namespace: scheduled.Namespace,
			Spec:      *pending.Spec.DeepCopy(),
		}
		if err := req.Client.Create(req.Ctx, export); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create audit log export: %w", err)
		}
	} else if err != nil {
		return err
	} else {
		if export.Spec.ScheduleUID != string(scheduled.UID) {
			return fmt.Errorf("pending audit log export %q belongs to a different schedule", pending.Name)
		}
		if export.Status.State == types.AuditLogExportStateCompleted {
			// Only a durably completed job can advance coverage. If this update fails,
			// the next reconciliation consumes the same result without another upload.
			scheduled.Status.LastExportedThrough = export.Spec.EndTime.DeepCopy()
			scheduled.Status.PendingExport = nil
			if err := req.Client.Status().Update(req.Ctx, scheduled); err != nil {
				return err
			}
			if scheduled.Spec.Enabled {
				resp.RetryAfter(time.Second)
			}
			return nil
		}
	}
	resp.RetryAfter(pendingPollInterval)
	return nil
}

// pendingExportFromSchedule captures the next window and configuration before
// job creation so retries keep their identity and initial backfill boundaries.
func pendingExportFromSchedule(scheduled *v1.ScheduledAuditLogExport, cutoff time.Time) *v1.ScheduledAuditLogExportPending {
	var start time.Time
	if scheduled.Status.LastExportedThrough != nil {
		start = scheduled.Status.LastExportedThrough.Add(-5 * time.Second)
	} else if scheduled.Spec.RetentionPeriodInDays >= 0 {
		start = cutoff.Add(-24 * time.Hour * time.Duration(scheduled.Spec.RetentionPeriodInDays))
	}
	// UID prevents deleted/recreated schedules from adopting an old job. The
	// sequence remains stable across retries because PendingExport is persisted first.
	identity := fmt.Sprintf("%s/%s/%s/%d", scheduled.Namespace, scheduled.Name, scheduled.UID, scheduled.Status.TotalExportsCreated+1)
	hash := sha256.Sum256([]byte(identity))
	return &v1.ScheduledAuditLogExportPending{
		Name: fmt.Sprintf("%s%x", system.AuditLogExportPrefix, hash[:16]),
		Spec: v1.AuditLogExportSpec{
			Name:                   fmt.Sprintf("%s-%d", scheduled.Spec.Name, scheduled.Status.TotalExportsCreated+1),
			Type:                   scheduled.Spec.EffectiveType(),
			Bucket:                 scheduled.Spec.Bucket,
			KeyPrefix:              scheduled.Spec.KeyPrefix,
			StartTime:              metav1.NewTime(start),
			EndTime:                metav1.NewTime(cutoff),
			Filters:                scheduled.Spec.Filters,
			LLMFilters:             scheduled.Spec.LLMFilters,
			WithRequestAndResponse: scheduled.Spec.WithRequestAndResponse,
			ScheduleUID:            string(scheduled.UID),
		},
	}
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

package scheduledauditlogexport

import (
	"fmt"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/controller/handlers/auditlogexportcommon"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Handler struct{}

func NewHandler() *Handler {
	return nil
}

func (*Handler) ScheduleExports(req router.Request, resp router.Response) error {
	return auditlogexportcommon.ScheduleExports(req, resp, createExportFromSchedule)
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

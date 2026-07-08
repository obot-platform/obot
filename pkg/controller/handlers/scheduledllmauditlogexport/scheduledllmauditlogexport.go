package scheduledllmauditlogexport

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
	return &Handler{}
}

func (*Handler) ScheduleExports(req router.Request, resp router.Response) error {
	return auditlogexportcommon.ScheduleExports(req, resp, createExportFromSchedule)
}

func createExportFromSchedule(req router.Request, scheduledExport *v1.ScheduledLLMAuditLogExport, nextRunAt time.Time) error {
	var startTime time.Time
	if scheduledExport.Spec.RetentionPeriodInDays < 0 {
		startTime = time.Time{}
	} else {
		startTime = nextRunAt.Add(-24 * time.Hour * time.Duration(scheduledExport.Spec.RetentionPeriodInDays))
	}

	export := &v1.LLMAuditLogExport{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.LLMAuditLogExportPrefix,
			Namespace:    scheduledExport.Namespace,
		},
		Spec: v1.LLMAuditLogExportSpec{
			Name:                fmt.Sprintf("%s-%d", scheduledExport.Spec.Name, scheduledExport.Status.TotalExportsCreated+1),
			Bucket:              scheduledExport.Spec.Bucket,
			KeyPrefix:           scheduledExport.Spec.KeyPrefix,
			StartTime:           metav1.NewTime(startTime),
			EndTime:             metav1.NewTime(nextRunAt),
			Filters:             scheduledExport.Spec.Filters,
			WithSensitiveFields: scheduledExport.Spec.WithSensitiveFields,
		},
	}
	if err := req.Client.Create(req.Ctx, export); err != nil {
		return fmt.Errorf("failed to create LLM audit log export: %w", err)
	}
	scheduledExport.Status.TotalExportsCreated++
	return nil
}

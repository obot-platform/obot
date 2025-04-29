package cronjob

import (
	"fmt"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierror "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func GetScheduleAndTimezone(cronJob v1.CronJob) (string, string) {
	if cronJob.Spec.TaskSchedule != nil {
		schedule := ""
		switch cronJob.Spec.TaskSchedule.Interval {
		case "hourly":
			schedule = fmt.Sprintf("%d * * * *", cronJob.Spec.TaskSchedule.Minute)
		case "daily":
			schedule = fmt.Sprintf("%d %d * * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour)
		case "weekly":
			schedule = fmt.Sprintf("%d %d * * %d", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour, cronJob.Spec.TaskSchedule.Weekday)
		case "monthly":
			if cronJob.Spec.TaskSchedule.Day < 0 {
				// The day being -1 means the last day of the month. The cron parsing package we use uses `L` for this.
				schedule = fmt.Sprintf("%d %d L * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour)
			} else if cronJob.Spec.TaskSchedule.Day == 0 {
				schedule = fmt.Sprintf("%d %d 1 * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour)
			} else {
				schedule = fmt.Sprintf("%d %d %d * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour, cronJob.Spec.TaskSchedule.Day)
			}
		}
		return schedule, cronJob.Spec.TaskSchedule.TimeZone
	}
	return cronJob.Spec.Schedule, ""
}

func (h *Handler) Run(req router.Request, resp router.Response) error {
	cj := req.Object.(*v1.CronJob)
	next, err := calculateNextRunTime(*cj)
	if err != nil {
		return fmt.Errorf("failed to calculate next run time: %w", err)
	}

	if until := time.Until(next); until > 0 {
		resp.RetryAfter(until)
		return nil
	}

	var workflow v1.Workflow
	if err := req.Get(&workflow, cj.Namespace, cj.Spec.WorkflowName); apierror.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	if err = req.Client.Create(req.Ctx,
		&v1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.WorkflowExecutionPrefix,
				Namespace:    req.Namespace,
			},
			Spec: v1.WorkflowExecutionSpec{
				WorkflowName: workflow.Name,
				Input:        cj.Spec.Input,
				CronJobName:  cj.Name,
				ThreadName:   cj.Spec.ThreadName,
			},
		},
	); err != nil {
		return err
	}

	cj.Status.LastRunStartedAt = &[]metav1.Time{metav1.Now()}[0]
	return nil
}

func calculateNextRunTime(cronJob v1.CronJob) (time.Time, error) {
	lastRun := cronJob.Status.LastRunStartedAt
	if lastRun.IsZero() {
		lastRun = &metav1.Time{Time: cronJob.CreationTimestamp.Time}
	}

	schedule, timezone := GetScheduleAndTimezone(cronJob)
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

func (h *Handler) SetSuccessRunTime(req router.Request, _ router.Response) error {
	cj := req.Object.(*v1.CronJob)

	var workflowExecutions v1.WorkflowExecutionList
	if err := req.List(&workflowExecutions, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{"spec.cronJobName": cj.Name}),
		Namespace:     cj.Namespace,
	}); err != nil {
		return err
	}

	for _, execution := range workflowExecutions.Items {
		if execution.Status.State == types.WorkflowStateComplete && (cj.Status.LastSuccessfulRunCompleted == nil || cj.Status.LastSuccessfulRunCompleted.Before(execution.Status.EndTime)) {
			cj.Status.LastSuccessfulRunCompleted = execution.Status.EndTime
		}
	}

	return nil
}

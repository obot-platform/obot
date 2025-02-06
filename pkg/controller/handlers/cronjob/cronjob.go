package cronjob

import (
	"fmt"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
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
		switch cronJob.Spec.TaskSchedule.Interval {
		case "hourly":
			return fmt.Sprintf("%d * * * *", cronJob.Spec.TaskSchedule.Minute), cronJob.Spec.TaskSchedule.Timezone
		case "daily":
			return fmt.Sprintf("%d %d * * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour), cronJob.Spec.TaskSchedule.Timezone
		case "weekly":
			return fmt.Sprintf("%d %d * * %d", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour, cronJob.Spec.TaskSchedule.Weekday), cronJob.Spec.TaskSchedule.Timezone
		case "monthly":
			if cronJob.Spec.TaskSchedule.Day == -1 {
				// The day being -1 means the last day of the month. The cron parsing package we use uses `L` for this.
				return fmt.Sprintf("%d %d L * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour), cronJob.Spec.TaskSchedule.Timezone
			}
			return fmt.Sprintf("%d %d %d * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour, cronJob.Spec.TaskSchedule.Day), cronJob.Spec.TaskSchedule.Timezone
		}
	}
	return cronJob.Spec.Schedule, cronJob.Spec.Timezone
}

func (h *Handler) Run(req router.Request, resp router.Response) error {
	cj := req.Object.(*v1.CronJob)
	lastRun := cj.Status.LastRunStartedAt
	schedule, timezone := GetScheduleAndTimezone(*cj)
	var location *time.Location
	if timezone != "" {
		loc, err := time.LoadLocation(timezone)
		if err == nil {
			location = loc
		}
	}
	if lastRun.IsZero() {
		if location != nil {
			lastRun = &metav1.Time{Time: time.Now().In(location)}
		} else {
			lastRun = &metav1.Time{Time: time.Now()}
		}
	}

	next, err := gronx.NextTickAfter(schedule, lastRun.Time, false)
	if err != nil {
		return fmt.Errorf("failed to parse schedule: %w", err)
	}

	if until := time.Until(next); until > 0 {
		resp.RetryAfter(until)
		return nil
	}

	var workflow v1.Workflow
	if err := alias.Get(req.Ctx, req.Client, &workflow, cj.Namespace, cj.Spec.Workflow); err != nil {
		if apierror.IsNotFound(err) {
			return nil
		}
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

	if location != nil {
		cj.Status.LastRunStartedAt = &metav1.Time{Time: time.Now().In(location)}
	} else {
		cj.Status.LastRunStartedAt = &metav1.Time{Time: time.Now()}
	}

	return nil
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

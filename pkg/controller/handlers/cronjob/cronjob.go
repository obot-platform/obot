package cronjob

import (
	"fmt"
	"time"

	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/alias"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/acorn-io/nah/pkg/router"
	"github.com/robfig/cron/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func GetSchedule(cronJob v1.CronJob) string {
	if cronJob.Spec.TaskSchedule != nil {
		switch cronJob.Spec.TaskSchedule.Interval {
		case "hourly":
			return fmt.Sprintf("%d * * * *", cronJob.Spec.TaskSchedule.Minute)
		case "daily":
			return fmt.Sprintf("%d %d * * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour)
		case "weekly":
			return fmt.Sprintf("%d %d * * %d", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour, cronJob.Spec.TaskSchedule.Weekday)
		case "monthly":
			return fmt.Sprintf("%d %d %d * *", cronJob.Spec.TaskSchedule.Minute, cronJob.Spec.TaskSchedule.Hour, cronJob.Spec.TaskSchedule.Day)
		}
	}
	return cronJob.Spec.Schedule
}

func (h *Handler) Run(req router.Request, resp router.Response) error {
	cj := req.Object.(*v1.CronJob)
	lastRun := cj.Status.LastRunStartedAt
	if lastRun.IsZero() {
		lastRun = &cj.CreationTimestamp
	}

	sched, err := cron.ParseStandard(GetSchedule(*cj))
	if err != nil {
		return fmt.Errorf("failed to parse schedule: %w", err)
	}

	if until := time.Until(sched.Next(lastRun.Time)); until > 0 {
		resp.RetryAfter(until)
		return nil
	}

	var workflow v1.Workflow
	if err := alias.Get(req.Ctx, req.Client, &workflow, cj.Namespace, cj.Spec.Workflow); err != nil {
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

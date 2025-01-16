package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/adhocore/gronx"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/cronjob"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CronJobHandler struct{}

func NewCronJobHandler() *CronJobHandler {
	return &CronJobHandler{}
}

func (a *CronJobHandler) List(req api.Context) error {
	var cronJobs v1.CronJobList
	if err := req.List(&cronJobs); err != nil {
		return err
	}

	items := make([]types.CronJob, 0, len(cronJobs.Items))
	for _, cronJob := range cronJobs.Items {
		items = append(items, convertCronJob(cronJob))
	}
	return req.Write(types.CronJobList{Items: items})
}

func (a *CronJobHandler) Create(req api.Context) error {
	manifest, err := parseAndValidateCronManifest(req)
	if err != nil {
		return err
	}

	cronJob := v1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.CronJobPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.CronJobSpec{
			CronJobManifest: *manifest,
		},
	}

	if err = req.Create(&cronJob); err != nil {
		return err
	}

	return req.WriteCreated(convertCronJob(cronJob))
}

func (a *CronJobHandler) Update(req api.Context) error {
	var (
		id      = req.PathValue("id")
		cronJob v1.CronJob
	)

	if err := req.Get(&cronJob, id); err != nil {
		return err
	}

	manifest, err := parseAndValidateCronManifest(req)
	if err != nil {
		return err
	}

	cronJob.Spec.CronJobManifest = *manifest
	if err = req.Update(&cronJob); err != nil {
		return err
	}

	return req.Write(convertCronJob(cronJob))
}

func (a *CronJobHandler) Delete(req api.Context) error {
	var (
		id = req.PathValue("id")
	)

	return req.Delete(&v1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      id,
			Namespace: req.Namespace(),
		},
	})
}

func (a *CronJobHandler) ByID(req api.Context) error {
	var cronJob v1.CronJob
	if err := req.Get(&cronJob, req.PathValue("id")); err != nil {
		return err
	}

	return req.Write(convertCronJob(cronJob))
}

func (a *CronJobHandler) Execute(req api.Context) error {
	var cronJob v1.CronJob
	if err := req.Get(&cronJob, req.PathValue("id")); err != nil {
		return err
	}

	var workflow v1.Workflow
	if err := alias.Get(req.Context(), req.Storage, &workflow, cronJob.Namespace, cronJob.Spec.Workflow); err != nil {
		return err
	}

	if err := req.Create(&v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowExecutionPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WorkflowExecutionSpec{
			WorkflowName: workflow.Name,
			Input:        cronJob.Spec.Input,
			CronJobName:  cronJob.Name,
		},
	}); err != nil {
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func convertCronJob(cronJob v1.CronJob) types.CronJob {
	var nextRunAt *time.Time
	if next, err := gronx.NextTick(cronjob.GetSchedule(cronJob), false); err == nil {
		nextRunAt = &next
	}

	return types.CronJob{
		Metadata:                   MetadataFrom(&cronJob),
		CronJobManifest:            cronJob.Spec.CronJobManifest,
		LastRunStartedAt:           v1.NewTime(cronJob.Status.LastRunStartedAt),
		LastSuccessfulRunCompleted: v1.NewTime(cronJob.Status.LastSuccessfulRunCompleted),
		NextRunAt:                  types.NewTimeFromPointer(nextRunAt),
	}
}

func parseAndValidateCronManifest(req api.Context) (*types.CronJobManifest, error) {
	var manifest types.CronJobManifest
	if err := req.Read(&manifest); err != nil {
		return nil, err
	}
	if !gronx.IsValid(manifest.Schedule) {
		return nil, apierrors.NewBadRequest(fmt.Sprintf("invalid schedule %s", manifest.Schedule))
	}

	var workflow v1.Workflow
	if err := alias.Get(req.Context(), req.Storage, &workflow, req.Namespace(), manifest.Workflow); err != nil {
		return nil, err
	}

	return &manifest, nil
}

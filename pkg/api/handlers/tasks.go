package handlers

import (
	"net/http"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/api"
	"github.com/acorn-io/acorn/pkg/events"
	"github.com/acorn-io/acorn/pkg/invoke"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/acorn-io/acorn/pkg/wait"
	"github.com/acorn-io/nah/pkg/name"
	"github.com/acorn-io/nah/pkg/randomtoken"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type TaskHandler struct {
	invoker *invoke.Invoker
	events  *events.Emitter
}

func NewTaskHandler(invoker *invoke.Invoker, events *events.Emitter) *TaskHandler {
	return &TaskHandler{
		invoker: invoker,
		events:  events,
	}
}

func (t *TaskHandler) Abort(req api.Context) error {
	var taskRunID = req.PathValue("run_id")

	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	if taskRunID == "" {
		taskRunID = editorWFE(req, workflow.Name)
	}

	wfe, err := wait.For(req.Context(), req.Storage, &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskRunID,
			Namespace: req.Namespace(),
		},
	}, func(wfe *v1.WorkflowExecution) (bool, error) {
		return wfe.Status.ThreadName != "", nil
	})
	if err != nil {
		return err
	}

	if wfe.Spec.ThreadName != userThread.Name && workflow.Name != wfe.Spec.WorkflowName {
		return types.NewErrHttp(http.StatusForbidden, "task run does not belong to the thread")
	}

	var thread v1.Thread
	if err := req.Get(&thread, wfe.Status.ThreadName); err != nil {
		return err
	}

	return abortThread(req, &thread)
}

func (t *TaskHandler) Events(req api.Context) error {
	var taskRunID = req.PathValue("run_id")

	workflow, thread, err := t.getTask(req)
	if err != nil {
		return err
	}

	if taskRunID == "" {
		taskRunID = editorWFE(req, workflow.Name)
	}

	wfe, err := wait.For(req.Context(), req.Storage, &v1.WorkflowExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskRunID,
			Namespace: req.Namespace(),
		},
	}, func(wfe *v1.WorkflowExecution) (bool, error) {
		return wfe.Status.ThreadName != "", nil
	}, wait.Option{
		Timeout:       10 * time.Minute,
		WaitForExists: true,
	})
	if err != nil {
		return err
	}

	if wfe.Spec.ThreadName != thread.Name && workflow.Name != wfe.Spec.WorkflowName {
		return types.NewErrHttp(http.StatusForbidden, "task run does not belong to the user")
	}

	_, events, err := t.events.Watch(req.Context(), req.Namespace(), events.WatchOptions{
		History:                  true,
		MaxRuns:                  100,
		ThreadName:               wfe.Status.ThreadName,
		Follow:                   true,
		FollowWorkflowExecutions: true,
	})
	if err != nil {
		return err
	}

	return req.WriteEvents(events)
}

func editorWFE(req api.Context, workflowName string) string {
	return name.SafeHashConcatName(system.ThreadPrefix, workflowName, req.User.GetUID())
}

func (t *TaskHandler) AbortRun(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	var (
		wfe   v1.WorkflowExecution
		runID = req.PathValue("run_id")
	)

	if runID == "editor" {
		runID = editorWFE(req, workflow.Name)
	}

	if err := req.Get(&wfe, runID); err != nil {
		return err
	}

	if wfe.Spec.WorkflowName != workflow.Name {
		return types.NewErrNotFound("task run not found")
	}

	var thread v1.Thread
	if err := req.Get(&thread, wfe.Status.ThreadName); err != nil {
		return err
	}

	return abortThread(req, &thread)
}

func (t *TaskHandler) GetRun(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	var (
		wfe   v1.WorkflowExecution
		runID = req.PathValue("run_id")
	)
	if runID == "editor" {
		runID = editorWFE(req, workflow.Name)
	}
	if err := req.Get(&wfe, runID); err != nil {
		return err
	}
	if wfe.Spec.WorkflowName != workflow.Name {
		return types.NewErrNotFound("task run not found")
	}
	return req.Write(convertTaskRun(workflow, &wfe))
}

func (t *TaskHandler) DeleteRun(req api.Context) error {
	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	var (
		wfe v1.WorkflowExecution
	)
	if err := req.Get(&wfe, req.PathValue("run_id")); err != nil {
		return err
	}

	if wfe.Spec.ThreadName != userThread.Name || wfe.Spec.WorkflowName != workflow.Name {
		return types.NewErrHttp(http.StatusForbidden, "task run does not belong to the user")
	}

	return req.Delete(&wfe)
}

func (t *TaskHandler) ListRuns(req api.Context) error {
	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	var wfeList v1.WorkflowExecutionList
	if err := req.List(&wfeList, kclient.MatchingFields{
		"spec.workflowName": workflow.Name,
		"spec.threadName":   userThread.Name,
	}); err != nil {
		return err
	}

	var (
		result    types.TaskRunList
		editorWFE = editorWFE(req, workflow.Name)
	)

	for _, wfe := range wfeList.Items {
		if wfe.Name == editorWFE {
			continue
		}
		result.Items = append(result.Items, convertTaskRun(workflow, &wfe))
	}

	return req.Write(result)
}

func (t *TaskHandler) Run(req api.Context) error {
	var (
		stepID = req.Request.URL.Query().Get("step")
	)

	input, err := req.Body()
	if err != nil {
		return err
	}

	if !utf8.Valid(input) {
		return types.NewErrBadRequest("invalid non-utf8 input")
	}

	if string(input) == "{}" {
		input = nil
	}

	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	var wfe *v1.WorkflowExecution
	if stepID == "" {
		wfe = &v1.WorkflowExecution{
			TypeMeta: metav1.TypeMeta{},
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.WorkflowExecutionPrefix,
				Namespace:    req.Namespace(),
			},
			Spec: v1.WorkflowExecutionSpec{
				Input:        string(input),
				ThreadName:   userThread.Name,
				WorkflowName: workflow.Name,
			},
			Status: v1.WorkflowExecutionStatus{},
		}
		if err := req.Create(wfe); err != nil {
			return err
		}
	} else {
		resp, err := t.invoker.Workflow(req.Context(), req.Storage, workflow, string(input), invoke.WorkflowOptions{
			WorkflowExecutionName: editorWFE(req, workflow.Name),
			OwningThreadName:      userThread.Name,
			StepID:                stepID,
		})
		if err != nil {
			return err
		}
		wfe = resp.WorkflowExecution
	}

	return req.WriteCreated(convertTaskRun(workflow, wfe))
}

func convertTaskRun(workflow *v1.Workflow, wfe *v1.WorkflowExecution) types.TaskRun {
	var endTime *types.Time
	if wfe.Status.EndTime != nil {
		endTime = types.NewTime(wfe.Status.EndTime.Time)
	}
	return types.TaskRun{
		Metadata:  MetadataFrom(wfe),
		TaskID:    workflow.Name,
		Input:     wfe.Spec.Input,
		Task:      convertTaskManifest(wfe.Status.WorkflowManifest),
		StartTime: types.NewTime(wfe.CreationTimestamp.Time),
		EndTime:   endTime,
	}
}

func (t *TaskHandler) Delete(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return req.Delete(workflow)
}

func (t *TaskHandler) Update(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	_, _, manifest, task, err := t.getAssistantThreadAndManifestFromRequest(req)
	if err != nil {
		return err
	}

	alias := workflow.Spec.Manifest.Alias
	if alias == "" {
		alias, err = randomtoken.Generate()
		if err != nil {
			return err
		}
		alias = alias[:16]
	}

	workflow.Spec.Manifest = manifest
	workflow.Spec.Manifest.Alias = alias
	if err := req.Update(workflow); err != nil {
		return err
	}

	trigger, err := t.updateTrigger(req, workflow, task)
	if err != nil {
		return err
	}

	return req.Write(convertTask(*workflow, trigger))
}

type triggers struct {
	CronJob *v1.CronJob
	Webhook *v1.Webhook
	Email   *v1.EmailReceiver
}

func validate(task types.TaskManifest) error {
	var count int
	if task.Schedule != nil {
		count++
	}
	if task.Webhook != nil {
		count++
	}
	if task.Email != nil {
		count++
	}
	if task.OnDemand != nil {
		count++
	}
	if count > 1 {
		return types.NewErrBadRequest("only one trigger is allowed, schedule, webhook, onDemand, or email")
	}
	return nil
}

func (t *TaskHandler) updateTrigger(req api.Context, workflow *v1.Workflow, task types.TaskManifest) (*triggers, error) {
	if err := validate(task); err != nil {
		return nil, err
	}

	var trigger triggers

	if err := t.updateCron(req, workflow, task, &trigger); err != nil {
		return nil, err
	}

	if err := t.updateWebhook(req, workflow, task, &trigger); err != nil {
		return nil, err
	}

	if err := t.updateEmail(req, workflow, task, &trigger); err != nil {
		return nil, err
	}

	return &trigger, nil
}

func (t *TaskHandler) updateEmail(req api.Context, workflow *v1.Workflow, task types.TaskManifest, trigger *triggers) error {
	emailName := name.SafeHashConcatName(system.EmailReceiverPrefix, workflow.Name)

	var email v1.EmailReceiver
	if err := req.Get(&email, emailName); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	if task.Email == nil {
		if email.Name != "" {
			return req.Delete(&email)
		}
		return nil
	}

	if email.Name == "" {
		email = v1.EmailReceiver{
			ObjectMeta: metav1.ObjectMeta{
				Name:      emailName,
				Namespace: req.Namespace(),
			},
			Spec: v1.EmailReceiverSpec{
				EmailReceiverManifest: types.EmailReceiverManifest{
					User:     workflow.Spec.Manifest.Alias,
					Workflow: workflow.Name,
				},
				ThreadName: workflow.Spec.ThreadName,
			},
		}
		if err := req.Create(&email); err != nil {
			return err
		}
	}

	trigger.Email = &email
	return nil
}

func (t *TaskHandler) updateWebhook(req api.Context, workflow *v1.Workflow, task types.TaskManifest, trigger *triggers) error {
	webhookName := name.SafeHashConcatName(system.WebhookPrefix, workflow.Name)

	var webhook v1.Webhook
	if err := req.Get(&webhook, webhookName); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	if task.Webhook == nil {
		if webhook.Name != "" {
			return req.Delete(&webhook)
		}
		return nil
	}

	if webhook.Name == "" {
		webhook = v1.Webhook{
			ObjectMeta: metav1.ObjectMeta{
				Name:      webhookName,
				Namespace: req.Namespace(),
			},
			Spec: v1.WebhookSpec{
				WebhookManifest: types.WebhookManifest{
					Alias:    workflow.Spec.Manifest.Alias,
					Workflow: workflow.Name,
				},
				ThreadName: workflow.Spec.ThreadName,
			},
		}
		if err := req.Create(&webhook); err != nil {
			return err
		}
	}

	trigger.Webhook = &webhook
	return nil
}

func (t *TaskHandler) updateCron(req api.Context, workflow *v1.Workflow, task types.TaskManifest, trigger *triggers) error {
	cronName := name.SafeHashConcatName(system.CronJobPrefix, workflow.Name)

	var cron v1.CronJob
	if err := req.Get(&cron, cronName); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	if task.Schedule == nil {
		if cron.Name != "" {
			return req.Delete(&cron)
		}
		return nil
	}

	if cron.Name == "" {
		cron = v1.CronJob{
			ObjectMeta: metav1.ObjectMeta{
				Name:      cronName,
				Namespace: req.Namespace(),
			},
			Spec: v1.CronJobSpec{
				CronJobManifest: types.CronJobManifest{
					Workflow:     workflow.Name,
					TaskSchedule: task.Schedule,
				},
				ThreadName: workflow.Spec.ThreadName,
			},
		}
		if err := req.Create(&cron); err != nil {
			return err
		}
		trigger.CronJob = &cron
		return nil
	}

	trigger.CronJob = &cron
	if cron.Spec.TaskSchedule == nil || *cron.Spec.TaskSchedule != *task.Schedule {
		cron.Spec.TaskSchedule = task.Schedule
		return req.Update(&cron)
	}

	return nil
}

func (t *TaskHandler) getAssistantThreadAndManifestFromRequest(req api.Context) (*v1.Agent, *v1.Thread, types.WorkflowManifest, types.TaskManifest, error) {
	assistantID := req.PathValue("assistant_id")

	assistant, err := getAssistant(req, assistantID)
	if err != nil {
		return nil, nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	thread, err := getUserThread(req, assistantID)
	if err != nil {
		return nil, nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	var manifest types.TaskManifest
	if err := req.Read(&manifest); err != nil {
		return nil, nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	return assistant, thread, toWorkflowManifest(assistant, thread, manifest), manifest, nil
}

func (t *TaskHandler) Create(req api.Context) error {
	_, thread, workflowManifest, taskManifest, err := t.getAssistantThreadAndManifestFromRequest(req)
	if err != nil {
		return err
	}

	var workspaces v1.WorkspaceList
	err = req.List(&workspaces, kclient.MatchingFields{
		"status.workspaceID": thread.Status.WorkspaceID,
	})
	if err != nil {
		return err
	}

	if len(workspaces.Items) == 0 {
		return types.NewErrBadRequest("no workspace found for the thread")
	}

	if len(workspaces.Items) != 1 {
		return types.NewErrBadRequest("multiple workspaces found for the thread")
	}

	workflowManifest.Alias, err = randomtoken.Generate()
	if err != nil {
		return err
	}
	workflowManifest.Alias = workflowManifest.Alias[:16]

	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WorkflowSpec{
			ThreadName:        thread.Name,
			Manifest:          workflowManifest,
			KnowledgeSetNames: thread.Status.KnowledgeSetNames,
			WorkspaceName:     workspaces.Items[0].Name,
		},
	}

	if err := req.Create(&workflow); err != nil {
		return err
	}

	trigger, err := t.updateTrigger(req, &workflow, taskManifest)
	if err != nil {
		_ = req.Delete(&workflow)
		return err
	}

	return req.WriteCreated(convertTask(workflow, trigger))
}

func toWorkflowManifest(agent *v1.Agent, thread *v1.Thread, manifest types.TaskManifest) types.WorkflowManifest {
	workflowManifest := types.WorkflowManifest{
		AgentManifest: agent.Spec.Manifest,
	}

	workflowManifest.AgentManifest.Env = append(workflowManifest.AgentManifest.Env, types.EnvVar{
		Name:  "DATABASE_WORKSPACE_ID",
		Value: thread.Status.WorkspaceID,
	})

	for _, tool := range thread.Spec.Manifest.Tools {
		if !slices.Contains(workflowManifest.Tools, tool) {
			workflowManifest.Tools = append(workflowManifest.Tools, tool)
		}
	}

	workflowManifest.Steps = toWorkflowSteps(manifest.Steps)
	workflowManifest.Name = manifest.Name
	workflowManifest.Description = manifest.Description

	if manifest.OnDemand != nil {
		workflowManifest.Params = manifest.OnDemand.Params
	}

	return workflowManifest
}

func toWorkflowSteps(steps []types.TaskStep) []types.Step {
	workflowSteps := make([]types.Step, 0, len(steps))
	for _, step := range steps {
		workflowSteps = append(workflowSteps, types.Step{
			ID:   step.ID,
			Step: step.Step,
			If:   toWorkflowIf(step.If),
		})
	}
	return workflowSteps
}

func toWorkflowIf(ifStep *types.TaskIf) *types.If {
	if ifStep == nil {
		return nil
	}
	return &types.If{
		Condition: ifStep.Condition,
		Steps:     toWorkflowSteps(ifStep.Steps),
		Else:      toWorkflowSteps(ifStep.Else),
	}
}

func (t *TaskHandler) Get(req api.Context) error {
	task, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	var cron v1.CronJob
	if err := req.Get(&cron, name.SafeHashConcatName(system.CronJobPrefix, task.Name)); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	var webhook v1.Webhook
	if err := req.Get(&webhook, name.SafeHashConcatName(system.WebhookPrefix, task.Name)); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	var email v1.EmailReceiver
	if err := req.Get(&email, name.SafeHashConcatName(system.EmailReceiverPrefix, task.Name)); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	return req.Write(convertTask(*task, &triggers{
		CronJob: &cron,
		Webhook: &webhook,
		Email:   &email,
	}))
}

func (t *TaskHandler) getTask(req api.Context) (*v1.Workflow, *v1.Thread, error) {
	assistantID := req.PathValue("assistant_id")

	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return nil, nil, err
	}

	thread, err := getUserThread(req, assistantID)
	if err != nil {
		return nil, nil, err
	}

	if workflow.Spec.ThreadName != thread.Name {
		return nil, nil, types.NewErrHttp(http.StatusForbidden, "task does not belong to the thread")
	}

	return &workflow, thread, nil
}

func (t *TaskHandler) List(req api.Context) error {
	assistantID := req.PathValue("assistant_id")

	thread, err := getUserThread(req, assistantID)
	if err != nil {
		return err
	}

	var crons v1.CronJobList
	if err := req.List(&crons, kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}

	cronMap := make(map[string]*v1.CronJob, len(crons.Items))
	for i := range crons.Items {
		cronMap[crons.Items[i].Name] = &crons.Items[i]
	}

	var webhooks v1.WebhookList
	if err := req.List(&webhooks, kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}

	webhookMap := make(map[string]*v1.Webhook, len(webhooks.Items))
	for i := range webhooks.Items {
		webhookMap[webhooks.Items[i].Name] = &webhooks.Items[i]
	}

	var emailReceivers v1.EmailReceiverList
	if err := req.List(&emailReceivers, kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}

	emailReceiverMap := make(map[string]*v1.EmailReceiver, len(emailReceivers.Items))
	for i := range emailReceivers.Items {
		emailReceiverMap[emailReceivers.Items[i].Name] = &emailReceivers.Items[i]
	}

	var workflows v1.WorkflowList
	if err := req.List(&workflows, kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}

	taskList := types.TaskList{Items: make([]types.Task, 0, len(workflows.Items))}

	for _, workflow := range workflows.Items {
		taskList.Items = append(taskList.Items, convertTask(workflow, &triggers{
			CronJob: cronMap[name.SafeHashConcatName(system.CronJobPrefix, workflow.Name)],
			Webhook: webhookMap[name.SafeHashConcatName(system.WebhookPrefix, workflow.Name)],
			Email:   emailReceiverMap[name.SafeHashConcatName(system.EmailReceiverPrefix, workflow.Name)],
		}))
	}

	return req.Write(taskList)
}

func convertTaskManifest(manifest *types.WorkflowManifest) types.TaskManifest {
	if manifest == nil {
		return types.TaskManifest{}
	}
	return types.TaskManifest{
		Name:        manifest.Name,
		Description: manifest.Description,
		Steps:       toTaskSteps(manifest.Steps),
	}
}

func convertTask(workflow v1.Workflow, trigger *triggers) types.Task {
	task := types.Task{
		Metadata:     MetadataFrom(&workflow),
		TaskManifest: convertTaskManifest(&workflow.Spec.Manifest),
		Alias:        workflow.Namespace + "/" + workflow.Spec.Manifest.Alias,
	}
	if trigger != nil && trigger.CronJob != nil && trigger.CronJob.Name != "" {
		task.Schedule = trigger.CronJob.Spec.TaskSchedule
	}
	if trigger != nil && trigger.Webhook != nil && trigger.Webhook.Name != "" {
		task.Webhook = &types.TaskWebhook{}
	}
	if trigger != nil && trigger.Email != nil && trigger.Email.Name != "" {
		task.Email = &types.TaskEmail{}
	}
	if len(workflow.Spec.Manifest.Params) > 0 {
		task.OnDemand = &types.TaskOnDemand{
			Params: workflow.Spec.Manifest.Params,
		}
	}

	return task
}

func toTaskSteps(steps []types.Step) []types.TaskStep {
	taskSteps := make([]types.TaskStep, 0, len(steps))
	for _, step := range steps {
		taskSteps = append(taskSteps, types.TaskStep{
			ID:   step.ID,
			Step: step.Step,
			If:   toIf(step.If),
		})
	}
	return taskSteps
}

func toIf(ifStep *types.If) *types.TaskIf {
	if ifStep == nil {
		return nil
	}
	return &types.TaskIf{
		Condition: ifStep.Condition,
		Steps:     toTaskSteps(ifStep.Steps),
		Else:      toTaskSteps(ifStep.Else),
	}
}

package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/apiclient"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/invoke"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type TaskHandler struct {
	invoker   *invoke.Invoker
	events    *events.Emitter
	gptscript *gptscript.GPTScript
	serverURL string
}

func NewTaskHandler(invoker *invoke.Invoker, events *events.Emitter, gptscript *gptscript.GPTScript, serverURL string) *TaskHandler {
	return &TaskHandler{
		invoker:   invoker,
		events:    events,
		gptscript: gptscript,
		serverURL: serverURL,
	}
}

func (t *TaskHandler) Abort(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.Namespace()); err != nil {
		return err
	}

	return t.abort(req, &workflow, "")
}

func (t *TaskHandler) AbortFromScope(req api.Context) error {
	workflow, projectThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.abort(req, workflow, projectThread.Name)
}

func (t *TaskHandler) abort(req api.Context, workflow *v1.Workflow, threadName string) error {
	taskRunID := req.PathValue("run_id")

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

	if threadName == "" {
		threadName = wfe.Spec.ThreadName
	}
	if wfe.Spec.ThreadName != threadName && wfe.Spec.WorkflowName != workflow.Name {
		return types.NewErrHTTP(http.StatusForbidden, "task run does not belong to the thread")
	}

	var thread v1.Thread
	if err := req.Get(&thread, wfe.Status.ThreadName); err != nil {
		return err
	}

	return abortThread(req, &thread)
}

func (t *TaskHandler) Events(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.Namespace()); err != nil {
		return err
	}

	return t.streamEvents(req, &workflow, "")
}

func (t *TaskHandler) EventsFromScope(req api.Context) error {
	workflow, thread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.streamEvents(req, workflow, thread.Name)
}

func (t *TaskHandler) streamEvents(req api.Context, workflow *v1.Workflow, threadName string) error {
	taskRunID := req.PathValue("run_id")

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

	if threadName == "" {
		threadName = wfe.Spec.ThreadName
	}
	if wfe.Spec.ThreadName != threadName && workflow.Name != wfe.Spec.WorkflowName {
		return types.NewErrHTTP(http.StatusForbidden, "task run does not belong to the user")
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

func (t *TaskHandler) AbortRun(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.abortRun(req, &workflow)
}

func (t *TaskHandler) AbortRunFromScope(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.abortRun(req, workflow)
}

func (t *TaskHandler) abortRun(req api.Context, workflow *v1.Workflow) error {
	var (
		wfe   v1.WorkflowExecution
		runID = req.PathValue("run_id")
	)

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

func (t *TaskHandler) GetRunFromScope(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.getRun(req, workflow)
}

func (t *TaskHandler) getRun(req api.Context, workflow *v1.Workflow) error {
	var (
		wfe   v1.WorkflowExecution
		runID = req.PathValue("run_id")
	)
	if err := req.Get(&wfe, runID); err != nil {
		return err
	}
	if wfe.Spec.WorkflowName != workflow.Name {
		return types.NewErrNotFound("task run not found")
	}
	return req.Write(convertTaskRun(workflow, &wfe))
}

func (t *TaskHandler) DeleteRun(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.deleteRun(req, &workflow, "")
}

func (t *TaskHandler) DeleteRunFromScope(req api.Context) error {
	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.deleteRun(req, workflow, userThread.Name)
}

func (t *TaskHandler) deleteRun(req api.Context, workflow *v1.Workflow, threadName string) error {
	var wfe v1.WorkflowExecution
	if err := req.Get(&wfe, req.PathValue("run_id")); err != nil {
		return err
	}

	if threadName == "" {
		threadName = wfe.Spec.ThreadName
	}
	if wfe.Spec.ThreadName != threadName && wfe.Spec.WorkflowName != workflow.Name {
		return types.NewErrHTTP(http.StatusForbidden, "task run does not belong to the user")
	}

	return req.Delete(&wfe)
}

func (t *TaskHandler) ListRuns(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.listRuns(req, &workflow, nil)
}

func (t *TaskHandler) ListRunsFromScope(req api.Context) error {
	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.listRuns(req, workflow, userThread)
}

func (t *TaskHandler) listRuns(req api.Context, workflow *v1.Workflow, userThread *v1.Thread) error {
	selector := kclient.MatchingFields{
		"spec.workflowName": workflow.Name,
	}
	if userThread != nil && userThread.Name != "" {
		selector["spec.threadName"] = userThread.Name
	}

	var wfeList v1.WorkflowExecutionList
	if err := req.List(&wfeList, selector); err != nil {
		return err
	}

	var (
		result types.TaskRunList
	)

	for _, wfe := range wfeList.Items {
		result.Items = append(result.Items, convertTaskRun(workflow, &wfe))
	}

	return req.Write(result)
}

func (t *TaskHandler) Run(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.run(req, &workflow, "")
}

func (t *TaskHandler) RunFromScope(req api.Context) error {
	workflow, projectThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.run(req, workflow, projectThread.Name)
}

func (t *TaskHandler) run(req api.Context, workflow *v1.Workflow, threadName string) error {
	stepID := req.PathValue("step_id")
	runID := req.PathValue("run_id")
	taskBreadCrumb := req.Request.Header.Get(apiclient.TaskBreadCrumbHeader)

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

	var wfe *v1.WorkflowExecution
	if threadName == "" {
		resp, err := t.invoker.Workflow(req.Context(), req.Storage, workflow, string(input), invoke.WorkflowOptions{
			StepID: stepID,
		})
		if err != nil {
			return err
		}
		wfe = resp.WorkflowExecution
	} else if stepID == "" || runID == "" {
		wfe = &v1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.WorkflowExecutionPrefix,
				Namespace:    req.Namespace(),
			},
			Spec: v1.WorkflowExecutionSpec{
				Input:          string(input),
				ThreadName:     threadName,
				WorkflowName:   workflow.Name,
				RunUntilStep:   req.URL.Query().Get("stepID"),
				RunName:        getRunIDFromUser(req),
				TaskBreakCrumb: taskBreadCrumb,
			},
		}
		if err := req.Create(wfe); err != nil {
			return err
		}
	} else {
		resp, err := t.invoker.Workflow(req.Context(), req.Storage, workflow, string(input), invoke.WorkflowOptions{
			WorkflowExecutionName: runID,
			StepID:                stepID,
		})
		if err != nil {
			return err
		}
		wfe = resp.WorkflowExecution
	}

	return req.WriteCreated(convertTaskRun(workflow, wfe))
}

func getRunIDFromUser(req api.Context) string {
	v := req.User.GetExtra()["obot:runID"]
	if len(v) == 1 {
		return v[0]
	}
	return ""
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
		Output:    wfe.Status.Output,
		ThreadID:  wfe.Status.ThreadName,
		Task:      ConvertTaskManifest(wfe.Status.WorkflowManifest),
		StartTime: types.NewTime(wfe.CreationTimestamp.Time),
		EndTime:   endTime,
		Error:     wfe.Status.Error,
	}
}

func (t *TaskHandler) Delete(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	return req.Delete(&workflow)
}

func (t *TaskHandler) DeleteFromScope(req api.Context) error {
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
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	_, manifest, task, err := t.getThreadAndManifestFromWorkflow(req, &workflow)
	if err != nil {
		return err
	}

	manifest.Alias = workflow.Spec.Manifest.Alias
	if manifest.Alias == "" {
		manifest.Alias, err = randomtoken.Generate()
		if err != nil {
			return err
		}
	}

	workflow.Spec.Manifest = manifest
	if err := req.Update(&workflow); err != nil {
		return err
	}

	trigger, err := t.updateTrigger(req, &workflow, task)
	if err != nil {
		return err
	}

	return req.Write(convertTask(workflow, trigger))
}

func (t *TaskHandler) UpdateFromScope(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	_, manifest, task, err := t.getThreadAndManifestFromRequest(req)
	if err != nil {
		return err
	}

	manifest.Alias = workflow.Spec.Manifest.Alias
	if manifest.Alias == "" {
		manifest.Alias, err = randomtoken.Generate()
		if err != nil {
			return err
		}
	}

	workflow.Spec.Manifest = manifest
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
	if task.OnSlackMessage != nil {
		count++
	}
	if task.OnDiscordMessage != nil {
		count++
	}
	if count > 1 {
		return types.NewErrBadRequest("only one trigger is allowed, schedule, webhook, onDemand, onSlackMessage, or email")
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

	if err := t.updateSlack(req, workflow, task); err != nil {
		return nil, err
	}

	if err := t.updateDiscord(req, workflow, task); err != nil {
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
					Alias:        workflow.Spec.Manifest.Alias,
					WorkflowName: workflow.Name,
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
					Alias:        workflow.Spec.Manifest.Alias,
					WorkflowName: workflow.Name,
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
					WorkflowName: workflow.Name,
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

func (t *TaskHandler) updateSlack(req api.Context, workflow *v1.Workflow, task types.TaskManifest) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Storage.Get(req.Context(), kclient.ObjectKeyFromObject(workflow), workflow); err != nil {
			return err
		}
		if task.OnSlackMessage == nil {
			workflow.Spec.Manifest.OnSlackMessage = nil
		} else {
			workflow.Spec.Manifest.OnSlackMessage = &types.TaskOnSlackMessage{}
		}
		return req.Update(workflow)
	})
}

func (t *TaskHandler) updateDiscord(req api.Context, workflow *v1.Workflow, task types.TaskManifest) error {
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Storage.Get(req.Context(), kclient.ObjectKeyFromObject(workflow), workflow); err != nil {
			return err
		}
		if task.OnDiscordMessage == nil {
			workflow.Spec.Manifest.OnDiscordMessage = nil
		} else {
			workflow.Spec.Manifest.OnDiscordMessage = &types.TaskOnDiscordMessage{}
		}
		return req.Update(workflow)
	})
}

func (t *TaskHandler) getThreadAndManifestFromRequest(req api.Context) (*v1.Thread, types.WorkflowManifest, types.TaskManifest, error) {
	thread, err := getThreadForScope(req)
	if err != nil {
		return nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	wfManifest, manifest, err := t.getManifestFromRequest(req)
	return thread, wfManifest, manifest, err
}

func (t *TaskHandler) getManifestFromRequest(req api.Context) (types.WorkflowManifest, types.TaskManifest, error) {
	var manifest types.TaskManifest
	if err := req.Read(&manifest); err != nil {
		return types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	wfManifest := ToWorkflowManifest(manifest)
	return wfManifest, manifest, nil
}

func (t *TaskHandler) getThreadAndManifestFromWorkflow(req api.Context, workflow *v1.Workflow) (*v1.Thread, types.WorkflowManifest, types.TaskManifest, error) {
	var thread v1.Thread
	if err := req.Get(&thread, workflow.Spec.ThreadName); err != nil {
		return nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	wfManifest, manifest, err := t.getManifestFromRequest(req)
	return &thread, wfManifest, manifest, err
}

func (t *TaskHandler) CreateFromScope(req api.Context) error {
	thread, workflowManifest, taskManifest, err := t.getThreadAndManifestFromRequest(req)
	if err != nil {
		return err
	}

	var workspace v1.Workspace
	if err := req.Get(&workspace, thread.Status.WorkspaceName); err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	workflowManifest.Alias, err = randomtoken.Generate()
	if err != nil {
		return fmt.Errorf("failed to generate alias: %w", err)
	}

	if len(workflowManifest.Alias) > 12 {
		workflowManifest.Alias = workflowManifest.Alias[:12]
	}

	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WorkflowSpec{
			ThreadName: thread.Name,
			Manifest:   workflowManifest,
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

func ToWorkflowManifest(manifest types.TaskManifest) types.WorkflowManifest {
	return types.WorkflowManifest{
		Name:           manifest.Name,
		Description:    manifest.Description,
		Steps:          toWorkflowSteps(manifest.Steps),
		Params:         toParams(manifest),
		OnSlackMessage: manifest.OnSlackMessage,
	}
}

func toParams(manifest types.TaskManifest) map[string]string {
	if manifest.OnDemand != nil {
		return manifest.OnDemand.Params
	}
	return nil
}

func toWorkflowSteps(steps []types.TaskStep) []types.Step {
	workflowSteps := make([]types.Step, 0, len(steps))
	for _, step := range steps {
		workflowSteps = append(workflowSteps, types.Step(step))
	}
	return workflowSteps
}

func (t *TaskHandler) Get(req api.Context) error {
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.get(req, &workflow)
}

func (t *TaskHandler) GetFromScope(req api.Context) error {
	workflow, _, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.get(req, workflow)
}

func (t *TaskHandler) get(req api.Context, workflow *v1.Workflow) error {
	var cron v1.CronJob
	if err := req.Get(&cron, name.SafeHashConcatName(system.CronJobPrefix, workflow.Name)); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	var webhook v1.Webhook
	if err := req.Get(&webhook, name.SafeHashConcatName(system.WebhookPrefix, workflow.Name)); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	var email v1.EmailReceiver
	if err := req.Get(&email, name.SafeHashConcatName(system.EmailReceiverPrefix, workflow.Name)); kclient.IgnoreNotFound(err) != nil {
		return err
	}

	return req.Write(convertTask(*workflow, &triggers{
		CronJob: &cron,
		Webhook: &webhook,
		Email:   &email,
	}))
}

func (t *TaskHandler) getTask(req api.Context) (*v1.Workflow, *v1.Thread, error) {
	thread, err := getThreadForScope(req)
	if err != nil {
		return nil, nil, err
	}

	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return nil, nil, err
	}

	if thread.Spec.Project {
		if workflow.Spec.ThreadName != thread.Name {
			return nil, nil, types.NewErrHTTP(http.StatusForbidden, "task does not belong to the thread")
		}
		return &workflow, thread, nil
	}

	if thread.Spec.ParentThreadName == "" || workflow.Spec.ThreadName != thread.Spec.ParentThreadName {
		return nil, nil, types.NewErrHTTP(http.StatusForbidden, "task does not belong to the thread")
	}

	var projectThread v1.Thread
	if err := req.Get(&projectThread, thread.Spec.ParentThreadName); err != nil {
		return nil, nil, err
	}

	return &workflow, &projectThread, nil
}

func getThreadForScope(req api.Context) (*v1.Thread, error) {
	var (
		assistantID = req.PathValue("assistant_id")
		threadID    = req.PathValue("thread_id")
	)

	if threadID != "" {
		var thread v1.Thread
		if err := req.Get(&thread, threadID); err != nil {
			return nil, err
		}
		return &thread, nil
	}

	if assistantID == "" {
		// This isn't actually required, but was more just left over from a refactor and let it be
		return nil, types.NewErrBadRequest("assistant scope is required")
	}

	thread, err := getProjectThread(req)
	if err != nil {
		return nil, err
	}

	taskID := req.PathValue("task_id")
	runID := req.PathValue("run_id")
	if taskID != "" && runID != "" {
		var wfe v1.WorkflowExecution
		if err := req.Get(&wfe, runID); err != nil {
			return nil, err
		}
		if wfe.Spec.ThreadName != thread.Name {
			return nil, types.NewErrHTTP(http.StatusForbidden, "task run does not belong to the thread")
		}
		if wfe.Spec.WorkflowName != taskID {
			return nil, types.NewErrNotFound("task run not found")
		}
		if wfe.Status.ThreadName == "" {
			return nil, apierrors.NewNotFound(schema.GroupResource{
				Resource: "runs",
			}, runID)
		}
		return thread, req.Get(thread, wfe.Status.ThreadName)
	}

	return thread, nil
}

func (t *TaskHandler) List(req api.Context) error {
	return t.list(req, nil)
}

func (t *TaskHandler) ListFromScope(req api.Context) error {
	thread, err := getThreadForScope(req)
	if err != nil {
		return err
	}

	return t.list(req, thread)
}

func (t *TaskHandler) list(req api.Context, thread *v1.Thread) error {
	selector := kclient.MatchingFields{}

	if thread != nil && thread.Name != "" {
		if !thread.Spec.Project && thread.Spec.ParentThreadName != "" {
			selector["spec.threadName"] = thread.Spec.ParentThreadName
		} else {
			selector["spec.threadName"] = thread.Name
		}
	}

	var crons v1.CronJobList
	if err := req.List(&crons, selector); err != nil {
		return err
	}

	cronMap := make(map[string]*v1.CronJob, len(crons.Items))
	for i := range crons.Items {
		cronMap[crons.Items[i].Name] = &crons.Items[i]
	}

	var webhooks v1.WebhookList
	if err := req.List(&webhooks, selector); err != nil {
		return err
	}

	webhookMap := make(map[string]*v1.Webhook, len(webhooks.Items))
	for i := range webhooks.Items {
		webhookMap[webhooks.Items[i].Name] = &webhooks.Items[i]
	}

	var emailReceivers v1.EmailReceiverList
	if err := req.List(&emailReceivers, selector); err != nil {
		return err
	}

	emailReceiverMap := make(map[string]*v1.EmailReceiver, len(emailReceivers.Items))
	for i := range emailReceivers.Items {
		emailReceiverMap[emailReceivers.Items[i].Name] = &emailReceivers.Items[i]
	}

	var workflows v1.WorkflowList
	if err := req.List(&workflows, selector); err != nil {
		return err
	}

	taskList := types.TaskList{Items: make([]types.Task, 0, len(workflows.Items))}

	for _, workflow := range workflows.Items {
		if !workflow.DeletionTimestamp.IsZero() {
			continue
		}
		taskList.Items = append(taskList.Items, convertTask(workflow, &triggers{
			CronJob: cronMap[name.SafeHashConcatName(system.CronJobPrefix, workflow.Name)],
			Webhook: webhookMap[name.SafeHashConcatName(system.WebhookPrefix, workflow.Name)],
			Email:   emailReceiverMap[name.SafeHashConcatName(system.EmailReceiverPrefix, workflow.Name)],
		}))
	}

	return req.Write(taskList)
}

func ConvertTaskManifest(manifest *types.WorkflowManifest) types.TaskManifest {
	if manifest == nil {
		return types.TaskManifest{}
	}
	return types.TaskManifest{
		Name:             manifest.Name,
		Description:      manifest.Description,
		Steps:            toTaskSteps(manifest.Steps),
		OnSlackMessage:   manifest.OnSlackMessage,
		OnDiscordMessage: manifest.OnDiscordMessage,
	}
}

func convertTask(workflow v1.Workflow, trigger *triggers) types.Task {
	task := types.Task{
		Metadata:     MetadataFrom(&workflow),
		TaskManifest: ConvertTaskManifest(&workflow.Spec.Manifest),
		ProjectID:    strings.Replace(workflow.Spec.ThreadName, system.ThreadPrefix, system.ProjectPrefix, 1),
		Alias:        workflow.Spec.Manifest.Alias,
		Managed:      workflow.Spec.Managed,
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
		taskSteps = append(taskSteps, types.TaskStep(step))
	}
	return taskSteps
}

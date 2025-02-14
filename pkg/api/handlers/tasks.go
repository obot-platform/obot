package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/events"
	"github.com/obot-platform/obot/pkg/invoke"
	"github.com/obot-platform/obot/pkg/render"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.abort(req, workflow, userThread.Name)
}

func (t *TaskHandler) abort(req api.Context, workflow *v1.Workflow, threadName string) error {
	taskRunID := req.PathValue("run_id")
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

func editorWFE(req api.Context, workflowName string) string {
	return name.SafeHashConcatName(system.ThreadPrefix, workflowName, req.User.GetUID())
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
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.getRun(req, &workflow)
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
	var workflow v1.Workflow
	if err := req.Get(&workflow, req.PathValue("id")); err != nil {
		return err
	}

	return t.run(req, &workflow, "")
}

func (t *TaskHandler) RunFromScope(req api.Context) error {
	workflow, userThread, err := t.getTask(req)
	if err != nil {
		return err
	}

	return t.run(req, workflow, userThread.Name)
}

func (t *TaskHandler) run(req api.Context, workflow *v1.Workflow, threadName string) error {
	stepID := req.Request.URL.Query().Get("step")

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
	} else if stepID == "" {
		wfe = &v1.WorkflowExecution{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.WorkflowExecutionPrefix,
				Namespace:    req.Namespace(),
			},
			Spec: v1.WorkflowExecutionSpec{
				Input:        string(input),
				ThreadName:   threadName,
				WorkflowName: workflow.Name,
			},
		}
		if err := req.Create(wfe); err != nil {
			return err
		}
	} else {
		resp, err := t.invoker.Workflow(req.Context(), req.Storage, workflow, string(input), invoke.WorkflowOptions{
			WorkflowExecutionName: editorWFE(req, workflow.Name),
			OwningThreadName:      threadName,
			StepID:                stepID,
			ThreadCredentialScope: new(bool),
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
					Alias:    workflow.Spec.Manifest.Alias,
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

func (t *TaskHandler) getThreadAndManifestFromRequest(req api.Context) (*v1.Thread, types.WorkflowManifest, types.TaskManifest, error) {
	thread, err := getThreadForScope(req)
	if err != nil {
		return nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	wfManifest, manifest, err := t.getManifestFromRequest(req, thread)
	return thread, wfManifest, manifest, err
}

func (t *TaskHandler) getManifestFromRequest(req api.Context, thread *v1.Thread) (types.WorkflowManifest, types.TaskManifest, error) {
	var agent v1.Agent
	if err := req.Get(&agent, thread.Spec.AgentName); err != nil {
		return types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	var manifest types.TaskManifest
	if err := req.Read(&manifest); err != nil {
		return types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	wfManifest, err := ToWorkflowManifest(req.Context(), req.Storage, &agent, thread, manifest)
	if err != nil {
		return types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	return wfManifest, manifest, nil
}

func (t *TaskHandler) getThreadAndManifestFromWorkflow(req api.Context, workflow *v1.Workflow) (*v1.Thread, types.WorkflowManifest, types.TaskManifest, error) {
	var thread v1.Thread
	if err := req.Get(&thread, workflow.Spec.ThreadName); err != nil {
		return nil, types.WorkflowManifest{}, types.TaskManifest{}, err
	}

	wfManifest, manifest, err := t.getManifestFromRequest(req, &thread)
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
		return err
	}
	workflowManifest.Alias = workflowManifest.Alias[:16]

	var additionalCredentialContexts []string
	if thread.Spec.AgentName != "" {
		additionalCredentialContexts = append(additionalCredentialContexts, thread.Spec.AgentName)
	}
	workflow := v1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.WorkflowPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.WorkflowSpec{
			ThreadName:                   thread.Name,
			Manifest:                     workflowManifest,
			KnowledgeSetNames:            thread.Status.KnowledgeSetNames,
			WorkspaceName:                workspace.Name,
			CredentialContextID:          thread.Name,
			AdditionalCredentialContexts: additionalCredentialContexts,
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

func (t *TaskHandler) Script(req api.Context) error {
	var (
		id     = req.Request.PathValue("id")
		stepID = req.Request.URL.Query().Get("step")
		wf     v1.Workflow
	)
	if err := req.Get(&wf, id); err != nil {
		return fmt.Errorf("failed to get workflow with id %s: %w", id, err)
	}

	step, _ := types.FindStep(&wf.Spec.Manifest, stepID)
	agent, err := render.Workflow(req.Context(), req.Storage, &wf, render.WorkflowOptions{
		Step: step,
	})
	if err != nil {
		return err
	}

	tools, extraEnv, err := render.Agent(req.Context(), req.Storage, agent, t.serverURL, render.AgentOptions{})
	if err != nil {
		return err
	}

	nodes := gptscript.ToolDefsToNodes(tools)
	nodes = append(nodes, gptscript.Node{
		TextNode: &gptscript.TextNode{
			Text: "!obot-extra-env\n" + strings.Join(extraEnv, "\n"),
		},
	})

	script, err := req.GPTClient.Fmt(req.Context(), nodes)
	if err != nil {
		return err
	}

	return req.Write(script)
}

func (t *TaskHandler) Authenticate(req api.Context) error {
	var (
		id       = req.PathValue("id")
		workflow v1.Workflow
		tools    []string
	)

	if err := req.Read(&tools); err != nil {
		return fmt.Errorf("failed to read tools from request body: %w", err)
	}

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for authentication")
	}

	if err := req.Get(&workflow, id); err != nil {
		return err
	}

	agent, err := render.Workflow(req.Context(), req.Storage, &workflow, render.WorkflowOptions{})
	if err != nil {
		return err
	}

	resp, err := runAuthForAgent(req.Context(), req.Storage, t.invoker, t.gptscript, agent, id, tools)
	if err != nil {
		return err
	}
	defer func() {
		resp.Close()
		if kickErr := kickWorkflow(req.Context(), req.Storage, &workflow); kickErr != nil && err == nil {
			err = fmt.Errorf("failed to update workflow status: %w", kickErr)
		}
	}()

	req.ResponseWriter.Header().Set("X-Obot-Thread-Id", resp.Thread.Name)
	return req.WriteEvents(resp.Events)
}

func (t *TaskHandler) DeAuthenticate(req api.Context) error {
	var (
		id    = req.PathValue("id")
		wf    v1.Workflow
		tools []string
	)

	if err := req.Read(&tools); err != nil {
		return fmt.Errorf("failed to read tools from request body: %w", err)
	}

	if len(tools) == 0 {
		return types.NewErrBadRequest("no tools provided for de-authentication")
	}

	if err := req.Get(&wf, id); err != nil {
		return err
	}

	errs := removeToolCredentials(req.Context(), req.Storage, t.gptscript, id, wf.Namespace, tools)

	if err := kickWorkflow(req.Context(), req.Storage, &wf); err != nil {
		errs = append(errs, fmt.Errorf("failed to update workflow status: %w", err))
	}

	return errors.Join(errs...)
}

func (t *TaskHandler) EnsureCredentialForKnowledgeSource(req api.Context) error {
	var wf v1.Workflow
	if err := req.Get(&wf, req.PathValue("id")); err != nil {
		return err
	}

	var knowledgeSet v1.KnowledgeSet
	if len(wf.Status.KnowledgeSetNames) > 0 {
		if err := req.Get(&knowledgeSet, wf.Status.KnowledgeSetNames[0]); err != nil {
			return fmt.Errorf("failed to get workflow knowledge set: %w", err)
		}
	}

	ref := req.PathValue("ref")
	authStatus := wf.Status.AuthStatus[ref]

	// If auth is not required, then don't continue.
	if authStatus.Required != nil && !*authStatus.Required {
		resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}

		return req.WriteCreated(resp)
	}

	var toolReference v1.ToolReference
	if err := req.Get(&toolReference, ref); err != nil {
		return fmt.Errorf("failed to get tool reference %v", ref)
	}
	if toolReference.Status.Tool == nil {
		return types.NewErrHTTP(http.StatusTooEarly, "tool reference is not ready yet")
	}

	if len(toolReference.Status.Tool.Credentials) == 0 {
		// The only way to get here is if the controller hasn't set the field yet.
		if wf.Status.AuthStatus == nil {
			wf.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
		}

		authStatus.Required = &[]bool{false}[0]
		wf.Status.AuthStatus[ref] = authStatus
		resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
		if err != nil {
			return err
		}

		return req.WriteCreated(resp)
	}

	oauthLogin := &v1.OAuthAppLogin{
		ObjectMeta: metav1.ObjectMeta{
			Name:      system.OAuthAppLoginPrefix + wf.Name + ref,
			Namespace: req.Namespace(),
		},
		Spec: v1.OAuthAppLoginSpec{
			CredentialContext: wf.Name,
			ToolReference:     ref,
			OAuthApps:         []string{toolReference.Status.Tool.Metadata["oauth"]},
		},
	}

	if err := req.Delete(oauthLogin); err != nil {
		return err
	}

	oauthLogin, err := wait.For(req.Context(), req.Storage, oauthLogin, func(obj *v1.OAuthAppLogin) (bool, error) {
		return obj.Status.External.Authenticated || obj.Status.External.Error != "" || obj.Status.External.URL != "", nil
	}, wait.Option{
		Create: true,
	})
	if err != nil {
		return fmt.Errorf("failed to ensure credential for workflow %q: %w", wf.Name, err)
	}

	// Don't need to actually update the knowledge ref, there is a controller that will do that.
	if wf.Status.AuthStatus == nil {
		wf.Status.AuthStatus = make(map[string]types.OAuthAppLoginAuthStatus)
	}
	wf.Status.AuthStatus[ref] = oauthLogin.Status.External

	resp, err := convertWorkflow(wf, knowledgeSet.Status.TextEmbeddingModel, req.APIBaseURL)
	if err != nil {
		return err
	}

	return req.WriteCreated(resp)
}

func ToWorkflowManifest(ctx context.Context, c kclient.Client, agent *v1.Agent, thread *v1.Thread, manifest types.TaskManifest) (types.WorkflowManifest, error) {
	workflowManifest := types.WorkflowManifest{
		AgentManifest: agent.Spec.Manifest,
	}

	workflowManifest.AgentManifest.Env = append(workflowManifest.AgentManifest.Env, types.EnvVar{
		Name:  "DATABASE_WORKSPACE_ID",
		Value: thread.Status.WorkspaceID,
	})

	for _, env := range thread.Spec.Env {
		workflowManifest.AgentManifest.Env = append(workflowManifest.AgentManifest.Env, types.EnvVar{
			Name:     env,
			Existing: true,
		})
	}

	for _, tool := range thread.Spec.Manifest.Tools {
		if !slices.Contains(workflowManifest.Tools, tool) {
			workflowManifest.Tools = append(workflowManifest.Tools, tool)
		}
	}

	workflowManifest.Steps = toWorkflowSteps(manifest.Steps)
	workflowManifest.Name = manifest.Name
	workflowManifest.Description = manifest.Description

	credTool, err := render.ResolveToolReference(ctx, c, "", thread.Namespace, system.ExistingCredTool)
	if err != nil {
		return types.WorkflowManifest{}, err
	}
	workflowManifest.Credentials = []string{credTool + " as " + thread.Name}

	if manifest.OnDemand != nil {
		workflowManifest.Params = manifest.OnDemand.Params
	}

	return workflowManifest, nil
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

	if workflow.Spec.ThreadName != thread.Name {
		return nil, nil, types.NewErrHTTP(http.StatusForbidden, "task does not belong to the thread")
	}

	return &workflow, thread, nil
}

func getThreadForScope(req api.Context) (*v1.Thread, error) {
	assistantID := req.PathValue("assistant_id")

	if assistantID != "" {
		thread, err := getProjectThread(req)
		if err != nil {
			return nil, err
		}

		taskID := req.PathValue("task_id")
		runID := req.PathValue("run_id")
		if taskID != "" && runID != "" {
			if runID == "editor" {
				runID = editorWFE(req, taskID)
			}
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
			return thread, req.Get(thread, wfe.Status.ThreadName)
		}

		return thread, nil
	}

	threadID := req.PathValue("thread_id")

	var thread v1.Thread
	if err := req.Get(&thread, threadID); err != nil {
		return nil, err
	}
	return &thread, nil
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
		selector["spec.threadName"] = thread.Name
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
		Name:        manifest.Name,
		Description: manifest.Description,
		Steps:       toTaskSteps(manifest.Steps),
	}
}

func convertTask(workflow v1.Workflow, trigger *triggers) types.Task {
	task := types.Task{
		Metadata:     MetadataFrom(&workflow),
		TaskManifest: ConvertTaskManifest(&workflow.Spec.Manifest),
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

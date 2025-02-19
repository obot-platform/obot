package threads

import (
	"context"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/randomtoken"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/api/handlers"
	"github.com/obot-platform/obot/pkg/create"
	"github.com/obot-platform/obot/pkg/projects"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptScript *gptscript.GPTScript
}

func NewHandler(gptScript *gptscript.GPTScript) *Handler {
	return &Handler{gptScript: gptScript}
}

func (t *Handler) WorkflowState(req router.Request, _ router.Response) error {
	var (
		thread = req.Object.(*v1.Thread)
		wfe    v1.WorkflowExecution
	)

	if thread.Spec.WorkflowExecutionName != "" {
		if err := req.Get(&wfe, thread.Namespace, thread.Spec.WorkflowExecutionName); err != nil {
			return err
		}
		thread.Status.WorkflowState = wfe.Status.State
	}

	return nil
}

func getWorkspace(ctx context.Context, c kclient.WithWatch, thread *v1.Thread) (*v1.Workspace, error) {
	if thread.Spec.WorkspaceName != "" {
		ws := new(v1.Workspace)
		return ws, c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: thread.Spec.WorkspaceName}, ws)
	}

	if thread.Spec.ParentThreadName != "" {
		parentThread, err := projects.Recurse(ctx, c, thread, func(parentThread *v1.Thread) (bool, error) {
			return parentThread.Status.WorkspaceName != "", nil
		})
		if err != nil {
			return nil, err
		}
		ws := new(v1.Workspace)
		return ws, c.Get(ctx, kclient.ObjectKey{Namespace: thread.Namespace, Name: parentThread.Status.WorkspaceName}, ws)
	}

	ws := &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  thread.Namespace,
			Name:       system.WorkspacePrefix + thread.Name,
			Finalizers: []string{v1.WorkspaceFinalizer},
		},
		Spec: v1.WorkspaceSpec{
			ThreadName:         thread.Name,
			FromWorkspaceNames: thread.Spec.FromWorkspaceNames,
		},
	}

	return ws, create.IfNotExists(ctx, c, ws)
}

func (t *Handler) CreateWorkspaces(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	ws, err := getWorkspace(req.Ctx, req.Client, thread)
	if err != nil || ws.Status.WorkspaceID == "" {
		return err
	}

	var update bool
	if thread.Status.WorkspaceID != ws.Status.WorkspaceID {
		update = true
		thread.Status.WorkspaceID = ws.Status.WorkspaceID
	}
	if thread.Status.WorkspaceName != ws.Name {
		update = true
		thread.Status.WorkspaceName = ws.Name
	}
	if update {
		return req.Client.Status().Update(req.Ctx, thread)
	}
	return nil
}

func (t *Handler) CreateFromTemplate(req router.Request, _ router.Response) (err error) {
	thread := req.Object.(*v1.Thread)
	if thread.Spec.ThreadTemplateName == "" ||
		thread.Status.TemplateLoaded ||
		len(thread.Status.KnowledgeSetNames) == 0 ||
		thread.Status.WorkspaceName == "" ||
		thread.Status.WorkspaceID == "" {
		return nil
	}

	defer func() {
		if err == nil {
			thread.Status.TemplateLoaded = true
			err = req.Client.Status().Update(req.Ctx, thread)
		}
	}()

	tt := new(v1.ThreadTemplate)
	if err := req.Get(tt, req.Namespace, thread.Spec.ThreadTemplateName); err != nil {
		return err
	}

	if len(tt.Status.Tasks) == 0 {
		return nil
	}

	agent := new(v1.Agent)
	if err := req.Get(agent, req.Namespace, thread.Spec.AgentName); err != nil {
		return err
	}

	var tasks v1.WorkflowList
	if err := req.Client.List(req.Ctx, &tasks, kclient.InNamespace(req.Namespace), kclient.MatchingFields{
		"spec.threadName": thread.Name,
	}); err != nil {
		return err
	}

	for _, taskToCreate := range tt.Status.Tasks {
		var found bool
		for _, existingTask := range tasks.Items {
			if existingTask.Spec.Manifest.Name == taskToCreate.Name {
				found = true
				break
			}
		}

		if found {
			continue
		}

		manifest, err := handlers.ToWorkflowManifest(req.Ctx, req.Client, agent, thread, taskToCreate)
		if err != nil {
			return err
		}

		alias, err := randomtoken.Generate()
		if err != nil {
			return err
		}
		manifest.Alias = alias[:16]

		task := v1.Workflow{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.WorkflowPrefix,
				Namespace:    req.Namespace,
			},
			Spec: v1.WorkflowSpec{
				ThreadName:          thread.Name,
				Manifest:            manifest,
				CredentialContextID: thread.Name,
				KnowledgeSetNames:   thread.Status.KnowledgeSetNames,
				WorkspaceName:       thread.Status.WorkspaceName,
			},
		}

		if err := req.Client.Create(req.Ctx, &task); err != nil {
			return err
		}
	}

	return nil
}

func (t *Handler) CreateKnowledgeSet(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if len(thread.Status.KnowledgeSetNames) > 0 || thread.Spec.AgentName == "" {
		return nil
	}

	if thread.Spec.ParentThreadName != "" {
		parentThread, err := projects.Recurse(req.Ctx, req.Client, thread, func(parentThread *v1.Thread) (bool, error) {
			return len(parentThread.Status.KnowledgeSetNames) > 0, nil
		})
		if err != nil {
			return err
		}
		if len(parentThread.Status.KnowledgeSetNames) == 0 {
			return nil
		}
		thread.Status.KnowledgeSetNames = parentThread.Status.KnowledgeSetNames
		return req.Client.Status().Update(req.Ctx, thread)
	}

	var cloneWorkspaceName string
	if thread.Spec.ThreadTemplateName != "" {
		tt := new(v1.ThreadTemplate)
		if err := req.Get(tt, req.Namespace, thread.Spec.ThreadTemplateName); err != nil {
			return err
		}
		if !tt.Status.Ready {
			return nil
		}
		cloneWorkspaceName = tt.Status.KnowledgeWorkspaceName
	}

	ws := &v1.KnowledgeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name.SafeConcatName(system.KnowledgeSetPrefix, thread.Name),
			Namespace:  req.Namespace,
			Finalizers: []string{v1.KnowledgeSetFinalizer},
		},
		Spec: v1.KnowledgeSetSpec{
			ThreadName:             thread.Name,
			TextEmbeddingModel:     thread.Spec.TextEmbeddingModel,
			CloneFromWorkspaceName: cloneWorkspaceName,
		},
	}

	if err := create.OrGet(req.Ctx, req.Client, ws); err != nil {
		return err
	}

	if ws.Spec.TextEmbeddingModel != thread.Spec.TextEmbeddingModel {
		// The thread knowledge set must have the same text embedding model as its agent.
		ws.Spec.TextEmbeddingModel = thread.Spec.TextEmbeddingModel
		if err := req.Client.Update(req.Ctx, ws); err != nil {
			return err
		}
	}

	thread.Status.KnowledgeSetNames = append(thread.Status.KnowledgeSetNames, ws.Name)
	return req.Client.Status().Update(req.Ctx, thread)
}

func (t *Handler) CleanupEphemeralThreads(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if !thread.Spec.Ephemeral && !thread.Spec.SystemTask {
		// Everything here this is just to catch "ephemeral" threads from before ephemeral threads were implemented.
		thread.Spec.Ephemeral = thread.Spec.AgentName == "" &&
			thread.Spec.ParentThreadName == "" &&
			thread.Spec.WebhookName == "" &&
			thread.Spec.CronJobName == "" &&
			thread.Spec.EmailReceiverName == "" &&
			thread.Spec.WorkflowName == "" &&
			thread.Spec.WorkflowExecutionName == "" &&
			thread.Spec.OAuthAppLoginName == "" &&
			thread.Spec.KnowledgeSetName == "" &&
			thread.Spec.KnowledgeSourceName == ""
		if thread.Spec.Ephemeral {
			return req.Client.Update(req.Ctx, thread)
		}
	}
	if !thread.Spec.Ephemeral ||
		thread.CreationTimestamp.After(time.Now().Add(-12*time.Hour)) {
		return nil
	}

	return kclient.IgnoreNotFound(req.Delete(thread))
}

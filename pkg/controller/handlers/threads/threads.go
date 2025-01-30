package threads

import (
	"context"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/create"
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

func (t *Handler) CreateKnowledgeSet(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)
	if len(thread.Status.KnowledgeSetNames) > 0 || thread.Spec.AgentName == "" {
		return nil
	}

	ws := &v1.KnowledgeSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name.SafeConcatName(system.KnowledgeSetPrefix, thread.Name),
			Namespace:  req.Namespace,
			Finalizers: []string{v1.KnowledgeSetFinalizer},
		},
		Spec: v1.KnowledgeSetSpec{
			ThreadName:         thread.Name,
			TextEmbeddingModel: thread.Spec.TextEmbeddingModel,
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

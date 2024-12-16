package threads

import (
	"context"

	"github.com/acorn-io/acorn/pkg/create"
	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/system"
	"github.com/acorn-io/acorn/pkg/wait"
	"github.com/acorn-io/nah/pkg/name"
	"github.com/acorn-io/nah/pkg/router"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func WorkflowState(req router.Request, _ router.Response) error {
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
		return wait.For(ctx, c, &v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: thread.Namespace,
				Name:      thread.Spec.WorkspaceName,
			},
		}, func(ws *v1.Workspace) (bool, error) {
			return ws.Status.WorkspaceID != "", nil
		})
	}

	return wait.For(ctx, c, &v1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  thread.Namespace,
			Name:       system.WorkspacePrefix + thread.Name,
			Finalizers: []string{v1.WorkspaceFinalizer},
		},
		Spec: v1.WorkspaceSpec{
			ThreadName:         thread.Name,
			FromWorkspaceNames: thread.Spec.FromWorkspaceNames,
		},
	}, func(ws *v1.Workspace) (bool, error) {
		return ws.Status.WorkspaceID != "", nil
	}, wait.Option{
		Create: true,
	})
}

func CreateWorkspaces(req router.Request, _ router.Response) error {
	thread := req.Object.(*v1.Thread)

	if thread.Status.WorkspaceID != "" {
		return nil
	}

	ws, err := getWorkspace(req.Ctx, req.Client, thread)
	if err != nil {
		return err
	}

	thread.Status.WorkspaceID = ws.Status.WorkspaceID
	return req.Client.Status().Update(req.Ctx, thread)
}

func CreateKnowledgeSet(req router.Request, _ router.Response) error {
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

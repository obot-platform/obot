package workspace

import (
	"context"

	v1 "github.com/acorn-io/acorn/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/acorn-io/acorn/pkg/wait"
	"github.com/acorn-io/nah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gptScript         *gptscript.GPTScript
	workspaceProvider string
}

func New(gClient *gptscript.GPTScript, wp string) *Handler {
	return &Handler{
		gptScript:         gClient,
		workspaceProvider: wp,
	}
}

func getWorkspaceIDs(ctx context.Context, c kclient.WithWatch, ws *v1.Workspace) (wsIDs []string, _ error) {
	for _, wsName := range ws.Spec.FromWorkspaceNames {
		ws, err := wait.For(ctx, c, &v1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: ws.Namespace,
				Name:      wsName,
			},
		}, func(ws *v1.Workspace) (bool, error) {
			return ws.Status.WorkspaceID != "", nil
		})
		if err != nil {
			return nil, err
		}
		wsIDs = append(wsIDs, ws.Status.WorkspaceID)
	}

	return
}

func (a *Handler) CreateWorkspace(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if ws.Status.WorkspaceID != "" {
		return nil
	}

	providerType := a.workspaceProvider
	wsIDs, err := getWorkspaceIDs(req.Ctx, req.Client, ws)
	if err != nil {
		return err
	}

	workspaceID, err := a.gptScript.CreateWorkspace(req.Ctx, providerType, wsIDs...)
	if err != nil {
		return err
	}

	ws.Status.WorkspaceID = workspaceID
	if err = req.Client.Status().Update(req.Ctx, ws); err != nil {
		_ = a.gptScript.DeleteWorkspace(req.Ctx, workspaceID)
		return err
	}

	return nil
}

func (a *Handler) RemoveWorkspace(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if ws.Status.WorkspaceID == "" {
		return nil
	}

	return a.gptScript.DeleteWorkspace(req.Ctx, ws.Status.WorkspaceID)
}

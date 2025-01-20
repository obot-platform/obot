package workspace

import (
	"context"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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

func getWorkspaceIDs(ctx context.Context, c kclient.WithWatch, ws *v1.Workspace) ([]string, bool, error) {
	var (
		wsIDs       []string
		dependentWS v1.Workspace
	)
	for _, wsName := range ws.Spec.FromWorkspaceNames {
		if err := c.Get(ctx, router.Key(ws.Namespace, wsName), &dependentWS); err != nil || dependentWS.Status.WorkspaceID == "" {
			return nil, false, err
		}
		wsIDs = append(wsIDs, dependentWS.Status.WorkspaceID)
	}

	return wsIDs, true, nil
}

func (a *Handler) CreateWorkspace(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if ws.Status.WorkspaceID != "" {
		return nil
	}

	providerType := a.workspaceProvider
	wsIDs, allReady, err := getWorkspaceIDs(req.Ctx, req.Client, ws)
	if err != nil || !allReady {
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

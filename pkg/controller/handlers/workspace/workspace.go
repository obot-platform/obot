package workspace

import (
	"github.com/acorn-io/baaah/pkg/router"
	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
	wclient "github.com/otto8-ai/workspace-provider/pkg/client"
)

type Handler struct {
	workspaceClient   *wclient.Client
	workspaceProvider string
}

func New(wc *wclient.Client, wp string) *Handler {
	return &Handler{
		workspaceClient:   wc,
		workspaceProvider: wp,
	}
}

func (a *Handler) CreateWorkspace(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if ws.Status.WorkspaceID != "" {
		return nil
	}
	if ws.Spec.WorkspaceID != "" {
		// If the workspace ID is specified, use it.
		ws.Status.WorkspaceID = ws.Spec.WorkspaceID
		return nil
	}

	workspaceID, err := a.workspaceClient.Create(req.Ctx, a.workspaceProvider, ws.Spec.FromWorkspaces...)
	if err != nil {
		return err
	}

	ws.Status.WorkspaceID = workspaceID

	if err = req.Client.Status().Update(req.Ctx, ws); err != nil {
		_ = a.workspaceClient.Rm(req.Ctx, workspaceID)
		return err
	}

	return nil
}

func (a *Handler) RemoveWorkspace(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if ws.Status.WorkspaceID != "" {
		if err := a.workspaceClient.Rm(req.Ctx, ws.Status.WorkspaceID); err != nil {
			return err
		}
	} else if ws.Spec.WorkspaceID != "" {
		if err := a.workspaceClient.Rm(req.Ctx, ws.Spec.WorkspaceID); err != nil {
			return err
		}
	}

	return nil
}

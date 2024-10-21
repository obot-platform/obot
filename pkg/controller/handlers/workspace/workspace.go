package workspace

import (
	"github.com/acorn-io/baaah/pkg/router"
	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.gptscript.ai/v1"
)

type Handler struct {
	gptscript         *gptscript.GPTScript
	workspaceProvider string
}

func New(gClient *gptscript.GPTScript, wp string) *Handler {
	return &Handler{
		gptscript:         gClient,
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

	providerType := a.workspaceProvider
	if ws.Spec.IsKnowledge {
		// Knowledge files should be stored locally.
		providerType = "directory"
	}

	workspaceID, err := a.gptscript.CreateWorkspace(req.Ctx, providerType, ws.Spec.FromWorkspaces...)
	if err != nil {
		return err
	}

	ws.Status.WorkspaceID = workspaceID

	if err = req.Client.Status().Update(req.Ctx, ws); err != nil {
		_ = a.gptscript.DeleteWorkspace(req.Ctx, gptscript.DeleteWorkspaceOptions{WorkspaceID: workspaceID})
		return err
	}

	return nil
}

func (a *Handler) RemoveWorkspace(req router.Request, _ router.Response) error {
	ws := req.Object.(*v1.Workspace)
	if ws.Status.WorkspaceID != "" {
		if err := a.gptscript.DeleteWorkspace(req.Ctx, gptscript.DeleteWorkspaceOptions{WorkspaceID: ws.Status.WorkspaceID}); err != nil {
			return err
		}
	} else if ws.Spec.WorkspaceID != "" {
		if err := a.gptscript.DeleteWorkspace(req.Ctx, gptscript.DeleteWorkspaceOptions{WorkspaceID: ws.Spec.WorkspaceID}); err != nil {
			return err
		}
	}

	return nil
}

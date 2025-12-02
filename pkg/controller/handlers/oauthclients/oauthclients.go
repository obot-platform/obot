package oauthclients

import (
	"errors"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

type Handler struct {
	gptClient *gptscript.GPTScript
}

func NewHandler(gptClient *gptscript.GPTScript) *Handler {
	return &Handler{
		gptClient: gptClient,
	}
}

func (h *Handler) CleanupOAuthClientCred(req router.Request, _ router.Response) error {
	o := req.Object.(*v1.OAuthClient)

	if o.Spec.MCPServerName == "" {
		return nil
	}

	if err := h.gptClient.DeleteCredential(req.Ctx, o.Spec.MCPServerName, o.Spec.MCPServerName); !errors.As(err, &gptscript.ErrNotFound{}) {
		return err
	}

	return nil
}

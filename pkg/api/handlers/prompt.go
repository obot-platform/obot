package handlers

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api"
)

type PromptHandler struct {
	gptScript *gptscript.GPTScript
}

func NewPromptHandler(gClient *gptscript.GPTScript) *PromptHandler {
	return &PromptHandler{
		gptScript: gClient,
	}
}

func (p *PromptHandler) Prompt(req api.Context) error {
	var promptResponse gptscript.PromptResponse
	if err := req.Read(&promptResponse); err != nil {
		return err
	}
	return p.gptScript.PromptResponse(req.Context(), promptResponse)
}

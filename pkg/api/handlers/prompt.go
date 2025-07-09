package handlers

import (
	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/obot/pkg/api"
)

type PromptHandler struct{}

func NewPromptHandler() *PromptHandler {
	return &PromptHandler{}
}

func (p *PromptHandler) Prompt(req api.Context) error {
	var promptResponse gptscript.PromptResponse
	if err := req.Read(&promptResponse); err != nil {
		return err
	}
	return req.GPTClient.PromptResponse(req.Context(), promptResponse)
}

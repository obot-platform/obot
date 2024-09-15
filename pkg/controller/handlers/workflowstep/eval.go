package workflowstep

import (
	"strings"

	v1 "github.com/gptscript-ai/otto/pkg/storage/apis/otto.gptscript.ai/v1"
)

func (h *Handler) getInput(step *v1.WorkflowStep) (string, error) {
	var content []string
	if step.Spec.Step.Input != "" {
		content = append(content, step.Spec.Step.Input)
	}
	return strings.Join(content, "\n"), nil
}

package toolreference

import (
	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

var toolMigrations = map[string]string{
	"file-summarizer-file-summarizer": "file-summarizer",
}

func (h *Handler) MigrateToolNames(req router.Request, _ router.Response) error {
	if len(toolMigrations) == 0 {
		return nil
	}

	var tools []string

	switch o := req.Object.(type) {
	case *v1.Agent:
		tools = o.Spec.Manifest.Tools
	case *v1.Workflow:
		tools = o.Spec.Manifest.Tools
	case *v1.Thread:
		tools = o.Spec.Manifest.Tools
	case *v1.WorkflowStep:
		tools = o.Spec.Step.Tools

	default:
		return nil
	}

	modified := false
	for i, tool := range tools {
		if newName, shouldMigrate := toolMigrations[tool]; shouldMigrate {
			tools[i] = newName
			modified = true
		}
	}

	if !modified {
		return nil
	}

	return req.Client.Update(req.Ctx, req.Object)
}

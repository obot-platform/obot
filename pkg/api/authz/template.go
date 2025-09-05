package authz

import (
	"net/http"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkTemplate(req *http.Request, resources *Resources) (bool, error) {
	// For project-scoped template routes (/api/assistants/{assistant_id}/projects/{project_id}/template),
	// authorization is handled by the project check, not a separate template check
	if resources.TemplateID == "" {
		return true, nil
	}

	// For global template routes (/api/templates/{template_public_id}),
	// check if the template exists and is publicly accessible
	var templateShareList v1.ThreadShareList
	err := a.cache.List(req.Context(), &templateShareList, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.publicID": resources.TemplateID,
		"spec.template": "true",
	})

	return len(templateShareList.Items) > 0, err
}

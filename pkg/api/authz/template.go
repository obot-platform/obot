package authz

import (
	"net/http"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkTemplate(req *http.Request, resources *Resources) (bool, error) {
	if resources.TemplateID == "" {
		return true, nil
	}

	var templateShareList v1.ThreadShareList
	err := a.storage.List(req.Context(), &templateShareList, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
		"spec.publicID": resources.TemplateID,
	})

	return len(templateShareList.Items) > 0, err
}

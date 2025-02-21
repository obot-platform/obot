package authz

import (
	"context"
	"net/http"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (a *Authorizer) checkTemplate(req *http.Request, resources *Resources, user user.Info) (bool, error) {
	if resources.TemplateID == "" {
		return true, nil
	}

	var (
		agentID      string
		validUserIDs = getValidUserIDs(user)
		template     v1.ThreadTemplate
	)

	if err := a.storage.Get(req.Context(), router.Key(system.DefaultNamespace, resources.TemplateID), &template); err != nil {
		return false, err
	}

	if resources.Authorizated.Project != nil {
		return resources.Authorizated.Project.Name == template.Spec.ProjectThreadName, nil
	}

	if resources.Authorizated.Assistant != nil {
		agentID = resources.Authorizated.Assistant.Name
	}

	if !a.templateIsAuthorized(req.Context(), agentID, &template, validUserIDs) {
		return false, nil
	}

	resources.Authorizated.Template = &template
	return true, nil
}

func (a *Authorizer) templateIsAuthorized(ctx context.Context, agentID string, template *v1.ThreadTemplate, validUserIDs []string) bool {
	if agentID != "" {
		// If agent is available, make sure it's related
		if template.Status.AgentName != agentID {
			return false
		}
	}

	if slices.Contains(validUserIDs, template.Spec.UserID) {
		return true
	}

	for _, userID := range validUserIDs {
		var access v1.ThreadTemplateAuthorizationList
		err := a.storage.List(ctx, &access, kclient.InNamespace(system.DefaultNamespace), kclient.MatchingFields{
			"spec.userID":     userID,
			"spec.templateID": template.Name,
		})
		if err == nil && len(access.Items) == 1 {
			return true
		}
	}
	return false
}

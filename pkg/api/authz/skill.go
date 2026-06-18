package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkSkill(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.SkillID == "" || u.IsAdmin || u.IsAuditor {
		return true, nil
	}

	var skill v1.Skill
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.SkillID), &skill); err != nil {
		return false, err
	}

	return a.skillHelper.UserHasAccessToSkill(u, &skill)
}

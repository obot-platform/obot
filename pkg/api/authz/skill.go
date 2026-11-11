package authz

import (
	"net/http"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkSkill(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.SkillID == "" {
		return true, nil
	}
	// Administrators, read-only administrators and auditors may read every skill. An agent never may, whatever its user
	// info carries: it has no role, so only the skill access rules that match it grant it a skill. authorizeAgent never
	// marks an agent as any of them; this check keeps that true however checkSkill is reached.
	if u.CanViewAdmin() && !principal.ClaimsAgent(u) {
		return true, nil
	}

	var skill v1.Skill
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.SkillID), &skill); err != nil {
		return false, err
	}

	return a.skillHelper.UserHasAccessToSkill(u, &skill)
}

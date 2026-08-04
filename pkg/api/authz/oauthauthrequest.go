package authz

import (
	"net/http"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp/connectroute"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

func (a *Authorizer) checkOAuthAuthRequest(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.OAuthAuthRequestID == "" {
		return true, nil
	}

	var oauthAuthRequest v1.OAuthAuthRequest
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.OAuthAuthRequestID), &oauthAuthRequest); err != nil {
		return false, err
	}

	if oauthAuthRequest.Spec.UserID != 0 && strconv.FormatUint(uint64(oauthAuthRequest.Spec.UserID), 10) != u.GetUID() {
		return false, nil
	}
	if _, versioned, err := connectroute.ParseVersionedResource(oauthAuthRequest.Spec.Resource); err != nil {
		return false, err
	} else if versioned {
		return u.IsAdmin, nil
	}

	return CheckMCPIDAccess(req.Context(), a.uncached, a.acrHelper, u, oauthAuthRequest.Spec.MCPID)
}

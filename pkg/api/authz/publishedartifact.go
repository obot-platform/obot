package authz

import (
	"net/http"
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/publishedartifact"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apiserver/pkg/authentication/user"
)

func (a *Authorizer) checkPublishedArtifact(req *http.Request, resources *Resources, u user.Info) (bool, error) {
	if resources.PublishedArtifactID == "" {
		return true, nil
	}

	var artifact v1.PublishedArtifact
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.PublishedArtifactID), &artifact); err != nil {
		return false, err
	}

	isAdmin := slices.Contains(u.GetGroups(), types.GroupAdmin)
	if isAdmin || artifact.Spec.AuthorID == u.GetUID() {
		resources.Authorizated.PublishedArtifact = &artifact
		return true, nil
	}

	if req.Method == http.MethodPut || req.Method == http.MethodDelete {
		return false, nil
	}

	var version int
	if resources.ArtifactVersion != "" {
		if parsed, err := strconv.Atoi(resources.ArtifactVersion); err == nil && parsed >= 1 {
			version = parsed
		}
	} else if v := req.URL.Query().Get("version"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			version = parsed
		}
	}

	if version != 0 {
		if !publishedartifact.CanAccessVersion(&artifact, version, u, isAdmin) {
			return false, nil
		}
		resources.Authorizated.PublishedArtifact = &artifact
		return true, nil
	}

	if publishedartifact.CanAccess(&artifact, u, isAdmin) {
		resources.Authorizated.PublishedArtifact = &artifact
		return true, nil
	}

	return false, nil
}

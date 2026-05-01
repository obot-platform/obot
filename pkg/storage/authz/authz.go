package authz

import (
	"context"
	"slices"
	"strings"

	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

const (
	AdminName          = "admin"
	AdminGroup         = "system:admin"
	AuthenticatedGroup = "system:authenticated"
)

type Authorizer struct {
}

func (*Authorizer) Authorize(_ context.Context, a authorizer.Attributes) (authorized authorizer.Decision, reason string, err error) {
	if slices.Contains(a.GetUser().GetGroups(), AdminGroup) {
		return authorizer.DecisionAllow, "", nil
	}
	if a.GetUser().GetName() == "system:apiserver" {
		return authorizer.DecisionAllow, "", nil
	}
	if a.GetUser().GetName() == "system:serviceaccount:"+serviceaccounts.NetworkPolicyProvider &&
		a.IsResourceRequest() &&
		a.GetAPIGroup() == "obot.obot.ai" &&
		a.GetResource() == "mcpnetworkpolicys" &&
		slices.Contains([]string{"get", "list", "watch"}, a.GetVerb()) {
		return authorizer.DecisionAllow, "", nil
	}
	if a.GetUser().GetName() == "system:serviceaccount:"+serviceaccounts.NetworkPolicyProvider &&
		!a.IsResourceRequest() &&
		a.GetVerb() == "get" &&
		(a.GetPath() == "/api" ||
			a.GetPath() == "/apis" ||
			strings.HasPrefix(a.GetPath(), "/api/") ||
			strings.HasPrefix(a.GetPath(), "/apis/")) {
		return authorizer.DecisionAllow, "", nil
	}
	return authorizer.DecisionNoOpinion, "", nil
}

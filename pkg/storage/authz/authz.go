package authz

import (
	"context"
	"slices"

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
	return authorizer.DecisionNoOpinion, "", nil
}

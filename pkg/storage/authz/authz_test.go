package authz

import (
	"testing"

	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func TestServiceAccountsHaveNoImplicitAuthorization(t *testing.T) {
	a := &Authorizer{}
	decision, _, err := a.Authorize(t.Context(), authorizer.AttributesRecord{
		User: &user.DefaultInfo{
			Name: "system:serviceaccount:" + serviceaccounts.NetworkPolicyProvider,
			Groups: []string{
				AuthenticatedGroup,
				serviceaccounts.Group,
			},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != authorizer.DecisionNoOpinion {
		t.Fatalf("expected no opinion for service account, got %v", decision)
	}
}

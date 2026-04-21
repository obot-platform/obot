package authz

import (
	"testing"

	"github.com/obot-platform/obot/pkg/serviceaccounts"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/authorization/authorizer"
)

func networkPolicyProviderUser() *user.DefaultInfo {
	return &user.DefaultInfo{
		Name: "system:serviceaccount:" + serviceaccounts.NetworkPolicyProvider,
		Groups: []string{
			AuthenticatedGroup,
			serviceaccounts.Group,
		},
	}
}

func TestServiceAccountsHaveNoImplicitAuthorization(t *testing.T) {
	a := &Authorizer{}
	decision, _, err := a.Authorize(t.Context(), authorizer.AttributesRecord{
		User: networkPolicyProviderUser(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != authorizer.DecisionNoOpinion {
		t.Fatalf("expected no opinion for service account, got %v", decision)
	}
}

func TestNetworkPolicyProviderServiceAccountCanReadMCPNetworkPolicies(t *testing.T) {
	a := &Authorizer{}
	for _, verb := range []string{"get", "list", "watch"} {
		decision, _, err := a.Authorize(t.Context(), authorizer.AttributesRecord{
			User:            networkPolicyProviderUser(),
			Verb:            verb,
			ResourceRequest: true,
			APIGroup:        "obot.obot.ai",
			Resource:        "mcpnetworkpolicys",
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", verb, err)
		}
		if decision != authorizer.DecisionAllow {
			t.Fatalf("%s: expected allow, got %v", verb, decision)
		}
	}
}

func TestNetworkPolicyProviderServiceAccountStillCannotReadUnrelatedResources(t *testing.T) {
	a := &Authorizer{}
	decision, _, err := a.Authorize(t.Context(), authorizer.AttributesRecord{
		User:            networkPolicyProviderUser(),
		Verb:            "list",
		ResourceRequest: true,
		APIGroup:        "obot.obot.ai",
		Resource:        "threads",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision != authorizer.DecisionNoOpinion {
		t.Fatalf("expected no opinion for unrelated resource, got %v", decision)
	}
}

func TestNetworkPolicyProviderServiceAccountCanReadDiscoveryEndpoints(t *testing.T) {
	a := &Authorizer{}
	for _, path := range []string{
		"/api",
		"/apis",
		"/apis/obot.obot.ai/v1",
	} {
		decision, _, err := a.Authorize(t.Context(), authorizer.AttributesRecord{
			User:            networkPolicyProviderUser(),
			Verb:            "get",
			ResourceRequest: false,
			Path:            path,
		})
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", path, err)
		}
		if decision != authorizer.DecisionAllow {
			t.Fatalf("%s: expected allow, got %v", path, decision)
		}
	}
}

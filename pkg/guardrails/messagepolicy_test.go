package guardrails

import (
	"context"
	"errors"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

// testProfiles stands in for creating profiles: policies for everyone get the everyone
// profile, others one of their own.
func testProfiles(_ context.Context, p LegacyPolicy) (string, error) {
	for _, s := range p.Manifest.Subjects {
		if s.Type == apitypes.SubjectTypeSelector {
			return "everyone", nil
		}
	}
	return "profile-" + p.ID, nil
}

func TestMigrateMessagePolicies(t *testing.T) {
	store := &fakeStore{}
	policies := []LegacyPolicy{
		{
			ID: "mp1",
			Manifest: apitypes.MessagePolicyManifest{
				DisplayName: "No travel",
				Definition:  "Do not book travel.",
				Direction:   apitypes.PolicyDirectionUserMessage,
				Subjects:    []apitypes.Subject{{Type: apitypes.SubjectTypeGroup, ID: "sales"}},
			},
		},
		{
			ID: "mp2",
			Manifest: apitypes.MessagePolicyManifest{
				DisplayName: "No deletes",
				Definition:  "Never delete files.",
				Direction:   apitypes.PolicyDirectionToolCalls,
				Subjects:    []apitypes.Subject{{Type: apitypes.SubjectTypeSelector, ID: "*"}},
			},
		},
		{
			ID:       "mp3",
			Manifest: apitypes.MessagePolicyManifest{DisplayName: "Unassigned", Definition: "x", Direction: apitypes.PolicyDirectionBoth},
		},
	}
	if err := MigrateMessagePolicies(context.Background(), store, policies, testProfiles); err != nil {
		t.Fatal(err)
	}
	// Running again (after an interrupted run) creates nothing new.
	if err := MigrateMessagePolicies(context.Background(), store, policies, testProfiles); err != nil {
		t.Fatal(err)
	}
	if len(store.policies) != 3 || len(store.guardrails) != 3 {
		t.Fatalf("expected 3 guardrail policies and guardrails, got %d and %d", len(store.policies), len(store.guardrails))
	}
	byID := map[string]types.GuardrailPolicy{}
	for _, r := range store.policies {
		byID[r.ID] = r
	}
	for id, r := range byID {
		if !r.FailClosed {
			t.Errorf("guardrail policy %s doesn't fail closed; message policies blocked when the judge failed", id)
		}
		if r.Enabled {
			t.Errorf("guardrail policy %s is enabled; converted guardrail policies start disabled until an administrator enables them", id)
		}
	}
	if r := byID["mp-mp1"]; r.ApplyTo != types.GuardrailApplyInput || r.Action != types.GuardrailActionBlock || r.ProfileID != "profile-mp1" {
		t.Fatalf("unexpected guardrail policy %+v", r)
	}
	if r := byID["mp-mp2"]; r.ApplyTo != types.GuardrailApplyOutput || r.ProfileID != "everyone" {
		t.Fatalf("unexpected guardrail policy %+v", r)
	}
	// A policy that applied to no one gets a profile of its own, not the everyone profile, so
	// enabling its guardrail policy doesn't apply it to everyone.
	if r := byID["mp-mp3"]; r.ApplyTo != types.GuardrailApplyBoth || r.ProfileID != "profile-mp3" {
		t.Fatalf("unexpected guardrail policy %+v", r)
	}
}

// Message policies can be assigned to Obot groups, such as Obot Admin. Profiles can too, so
// the converted guardrail policies keep applying to them.
func TestMigrateMessagePoliciesWithObotGroups(t *testing.T) {
	store := &fakeStore{}
	policies := []LegacyPolicy{
		{
			ID: "admins",
			Manifest: apitypes.MessagePolicyManifest{
				DisplayName: "Admins only",
				Definition:  "x",
				Direction:   apitypes.PolicyDirectionBoth,
				Subjects: []apitypes.Subject{
					{
						Type: apitypes.SubjectTypeObotGroup,
						ID:   apitypes.GroupAdmin,
					},
				},
			},
		},
	}
	if err := MigrateMessagePolicies(context.Background(), store, policies, testProfiles); err != nil {
		t.Fatal(err)
	}
	if len(store.policies) != 1 || store.policies[0].ProfileID != "profile-admins" {
		t.Fatalf("a policy for an Obot group must keep it, in a profile of its own: %+v", store.policies)
	}
}

func TestMigrateMessagePoliciesProfileFailure(t *testing.T) {
	store := &fakeStore{}
	policies := []LegacyPolicy{{ID: "p", Manifest: apitypes.MessagePolicyManifest{DisplayName: "P", Definition: "x"}}}
	failing := func(context.Context, LegacyPolicy) (string, error) { return "", errors.New("storage unavailable") }
	if err := MigrateMessagePolicies(context.Background(), store, policies, failing); err == nil {
		t.Fatal("expected the conversion to fail, so it is retried")
	}
	if len(store.policies) != 0 {
		t.Fatalf("no guardrail policy may be created without a profile: %+v", store.policies)
	}
}

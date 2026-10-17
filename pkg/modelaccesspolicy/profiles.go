package modelaccesspolicy

import (
	"context"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultPolicyName is the model access policy Obot creates for everyone when there are
	// none: the default gateway profile.
	DefaultPolicyName = system.ModelAccessPolicyPrefix + "-default"
	// everyoneProfileName is the profile created for everyone when policies exist but none
	// applies to everyone.
	everyoneProfileName = system.ModelAccessPolicyPrefix + "-everyone"
)

// DefaultPolicy is the model access policy Obot creates for everyone when there are none. It
// grants the given models, the default model aliases.
func DefaultPolicy(models []types.ModelResource) *v1.ModelAccessPolicy {
	return &v1.ModelAccessPolicy{
		Name:      DefaultPolicyName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: types.ModelAccessPolicyManifest{
				DisplayName: "Default Policy",
				Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
				Models:      models,
			},
		},
	}
}

// AppliesToEveryone reports whether a policy's subjects include everyone.
func AppliesToEveryone(subjects []types.Subject) bool {
	return slices.ContainsFunc(subjects, func(s types.Subject) bool {
		return s.Type == types.SubjectTypeSelector && s.ID == "*"
	})
}

// EnsureEveryoneProfile returns a gateway profile that applies to everyone, for the guardrail
// policies upgrades create from message policies that applied to everyone. Guardrail policies
// apply through profile membership, not models, so a profile without models works for them. It is the
// Default Policy, or another policy for everyone. With no policies at all, it creates the
// Default Policy, as Obot does on first start, so that doesn't change; with policies but none
// for everyone, it creates an "Everyone" profile that grants no models. Names are fixed, so
// replicas starting together create one.
func EnsureEveryoneProfile(ctx context.Context, c kclient.Client) (string, error) {
	var policies v1.ModelAccessPolicyList
	if err := c.List(ctx, &policies, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return "", err
	}
	var everyone []string
	for _, p := range policies.Items {
		if p.DeletionTimestamp.IsZero() && AppliesToEveryone(p.Spec.Manifest.Subjects) {
			everyone = append(everyone, p.Name)
		}
	}
	for _, preferred := range []string{DefaultPolicyName, everyoneProfileName} {
		if slices.Contains(everyone, preferred) {
			return preferred, nil
		}
	}
	if len(everyone) > 0 {
		slices.Sort(everyone)
		return everyone[0], nil
	}

	profile := &v1.ModelAccessPolicy{
		Name:      everyoneProfileName,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: types.ModelAccessPolicyManifest{
				DisplayName: "Everyone",
				Subjects:    []types.Subject{{Type: types.SubjectTypeSelector, ID: "*"}},
			},
		},
	}
	if len(policies.Items) == 0 {
		profile = firstStartDefaultPolicy()
	}
	if err := kclient.IgnoreAlreadyExists(c.Create(ctx, profile)); err != nil {
		return "", err
	}
	return profile.Name, nil
}

// firstStartDefaultPolicy is the Default Policy Obot creates on first start, granting the
// default model aliases.
func firstStartDefaultPolicy() *v1.ModelAccessPolicy {
	var models []types.ModelResource
	for _, alias := range []types.DefaultModelAliasType{types.DefaultModelAliasTypeLLM, types.DefaultModelAliasTypeLLMMini} {
		models = append(models, types.ModelResource{ID: types.DefaultModelAliasRefPrefix + string(alias)})
	}
	return DefaultPolicy(models)
}

// EnsureProfiles returns the names of every gateway profile, for settings upgrades carry over
// to each of them. With no policies at all, it creates the Default Policy, as Obot does on
// first start, and returns it. Its name is fixed, so replicas starting together create one.
func EnsureProfiles(ctx context.Context, c kclient.Client) ([]string, error) {
	var policies v1.ModelAccessPolicyList
	if err := c.List(ctx, &policies, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return nil, err
	}
	if len(policies.Items) == 0 {
		profile := firstStartDefaultPolicy()
		if err := kclient.IgnoreAlreadyExists(c.Create(ctx, profile)); err != nil {
			return nil, err
		}
		return []string{profile.Name}, nil
	}
	var names []string
	for _, p := range policies.Items {
		if p.DeletionTimestamp.IsZero() {
			names = append(names, p.Name)
		}
	}
	slices.Sort(names)
	return names, nil
}

// EnsureProfile creates a gateway profile with the given name, display name and subjects,
// and no models, unless it exists.
func EnsureProfile(ctx context.Context, c kclient.Client, name, displayName string, subjects []types.Subject) error {
	return kclient.IgnoreAlreadyExists(c.Create(ctx, &v1.ModelAccessPolicy{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: types.ModelAccessPolicyManifest{
				DisplayName: displayName,
				Subjects:    subjects,
			},
		},
	}))
}

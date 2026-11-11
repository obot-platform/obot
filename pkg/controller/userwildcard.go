package controller

import (
	"context"
	"log/slog"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// userWildcardSubject is the user wildcard (users:*). Validation has always accepted it, because a user subject
	// only needs an ID, but until agent identities it matched no one, since no user's ID is "*". vMCP profiles, model
	// access policies, skill access rules and guardrail filters now read it as every user.
	userWildcardSubject = types.Subject{Type: types.SubjectTypeUser, ID: types.SubjectWildcardID}
)

// warnUserWildcardSubjects logs each vMCP profile, model access policy, skill access rule and guardrail filter that was
// saved with the user wildcard, so that an administrator can review policies that now apply to every user. It changes nothing, and a
// failure to list is only logged: the check is informational and must not keep Obot from starting.
//
// Access control rules and message policies are not listed. Agents never match access control rules, so they keep
// the meaning the wildcard had there, which is no one, and validation refuses new ones. Message policies are legacy:
// nothing enforces them, and no new ones can be created.
func warnUserWildcardSubjects(ctx context.Context, client kclient.Reader) {
	warn := func(kind, name, displayName, detail string) {
		slog.Warn("A policy names every user with the user wildcard subject (type user, ID *), which matched no one before agent identities and now matches every user. Review it, and remove the subject if the policy should not apply to everyone.",
			"kind", kind, "name", name, "displayName", displayName, "detail", detail)
	}

	var vmcps v1.VMCPList
	if err := client.List(ctx, &vmcps, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		slog.Error("Failed to list vMCPs to check for user wildcard subjects", "error", err)
	} else {
		for _, vmcp := range vmcps.Items {
			for _, profile := range vmcp.Spec.Manifest.Profiles {
				if slices.Contains(profile.Subjects, userWildcardSubject) {
					warn("VMCP", vmcp.Name, vmcp.Spec.Manifest.DisplayName, "profile "+profile.Name)
				}
			}
		}
	}

	var policies v1.ModelAccessPolicyList
	if err := client.List(ctx, &policies, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		slog.Error("Failed to list model access policies to check for user wildcard subjects", "error", err)
	} else {
		for _, policy := range policies.Items {
			if slices.Contains(policy.Spec.Manifest.Subjects, userWildcardSubject) {
				warn("ModelAccessPolicy", policy.Name, policy.Spec.Manifest.DisplayName, "")
			}
		}
	}

	// A guardrail filter with the user wildcard now applies to every user rather than to no one.
	var filters v1.MCPWebhookValidationList
	if err := client.List(ctx, &filters, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		slog.Error("Failed to list MCP filters to check for user wildcard subjects", "error", err)
	} else {
		for _, filter := range filters.Items {
			if g := filter.Spec.Manifest.Guardrail; g != nil && slices.Contains(g.Subjects, types.MCPGuardrailFilterSubject{Type: types.MCPGuardrailSubjectUser, ID: types.SubjectWildcardID}) {
				warn("MCPWebhookValidation", filter.Name, filter.Spec.Manifest.Name, "guardrail filter")
			}
		}
	}

	var rules v1.SkillAccessRuleList
	if err := client.List(ctx, &rules, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		slog.Error("Failed to list skill access rules to check for user wildcard subjects", "error", err)
	} else {
		for _, rule := range rules.Items {
			if slices.Contains(rule.Spec.Manifest.Subjects, userWildcardSubject) {
				warn("SkillAccessRule", rule.Name, rule.Spec.Manifest.DisplayName, "")
			}
		}
	}
}

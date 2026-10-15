package controller

import (
	"context"
	"fmt"
	"reflect"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/catalogmigration"
	"github.com/obot-platform/obot/pkg/controller/handlers/compositemigration"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	vmcpEverythingAdminGroupsMigrationName = "migrated_vmcp_everything_admin_obot_groups"
)

func migrateDefaultModelAccessPolicyModels(ctx context.Context, client kclient.Client) error {
	var policy v1.ModelAccessPolicy
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      system.ModelAccessPolicyPrefix + "-default",
	}, &policy); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get default model access policy: %w", err)
	}

	models := make([]types.ModelResource, 0, len(policy.Spec.Manifest.Models))
	for _, model := range policy.Spec.Manifest.Models {
		err := modelaccesspolicy.ValidateModelResource(
			ctx,
			client,
			policy.Namespace,
			model,
		)
		if err == nil {
			models = append(models, model)
			continue
		}
		if modelaccesspolicy.IsInvalidModelResource(err) || apierrors.IsNotFound(err) {
			continue
		}
		return fmt.Errorf("failed to validate default model access policy: %w", err)
	}

	if len(models) == len(policy.Spec.Manifest.Models) {
		return nil
	}

	policy.Spec.Manifest.Models = models
	if err := client.Update(ctx, &policy); err != nil {
		return fmt.Errorf("failed to update default model access policy: %w", err)
	}

	return nil
}

func addCatalogIDToAccessControlRules(ctx context.Context, client kclient.Client) error {
	var acRules v1.AccessControlRuleList
	if err := client.List(ctx, &acRules); err != nil {
		return err
	}

	// Iterate over each AccessControlRule and add CatalogID
	for _, acRule := range acRules.Items {
		if acRule.Spec.MCPCatalogID == "" && acRule.Spec.PowerUserWorkspaceID == "" {
			acRule.Spec.MCPCatalogID = system.DefaultCatalog
			if err := client.Update(ctx, &acRule); err != nil {
				return err
			}
		}
	}

	return nil
}

// migrateAuditLogExportSourceTypes makes the implicit MCP source selection on legacy
// scheduled MCP audit-log exports explicit. The export UI needs an explicit source selection
// when editing a schedule, while old schedules predate sourceTypes entirely.
func migrateAuditLogExportSourceTypes(ctx context.Context, client kclient.Client) error {
	var schedules v1.ScheduledAuditLogExportList
	if err := client.List(ctx, &schedules); err != nil {
		return err
	}

	for i := range schedules.Items {
		schedule := &schedules.Items[i]
		if schedule.Spec.EffectiveType() != types.AuditLogTypeMCP {
			continue
		}
		if schedule.Spec.Filters != nil && len(schedule.Spec.Filters.SourceTypes) > 0 {
			continue
		}

		if schedule.Spec.Filters == nil {
			schedule.Spec.Filters = &types.AuditLogExportFilters{}
		}
		schedule.Spec.Filters.SourceTypes = []types.AuditLogSourceType{types.AuditLogSourceTypeMCP}
		if err := client.Update(ctx, schedule); err != nil {
			return fmt.Errorf("failed to migrate scheduled audit-log export %s: %w", schedule.Name, err)
		}
	}

	return nil
}

func migrateVMCPDefaultAdminGroups(ctx context.Context, client kclient.Client) error {
	var vmcps v1.VMCPList
	if err := client.List(ctx, &vmcps); err != nil {
		return err
	}

	for i := range vmcps.Items {
		vmcp := &vmcps.Items[i]
		var changed bool
		for j := range vmcp.Spec.Manifest.Profiles {
			profile := &vmcp.Spec.Manifest.Profiles[j]
			if profile.Name != "default" || !profile.Permissions.AllowAllComponents {
				continue
			}
			for k := range profile.Subjects {
				subject := &profile.Subjects[k]
				if subject.Type == types.SubjectTypeGroup && subject.ID == types.GroupAdmin {
					subject.Type = types.SubjectTypeObotGroup
					changed = true
				}
			}
		}
		if changed {
			if err := client.Update(ctx, vmcp); err != nil {
				return fmt.Errorf("failed to migrate vMCP %s/%s: %w", vmcp.Namespace, vmcp.Name, err)
			}
		}
	}

	return nil
}

func migrateEverythingAccessControlRuleAdminGroup(ctx context.Context, client kclient.Client) error {
	var rule v1.AccessControlRule
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      "acr1-everything",
	}, &rule); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to get everything access control rule: %w", err)
	}

	if rule.Spec.MCPCatalogID != system.DefaultCatalog ||
		rule.Spec.PowerUserWorkspaceID != "" {
		return nil
	}

	var changed bool
	for i := range rule.Spec.Manifest.Subjects {
		subject := &rule.Spec.Manifest.Subjects[i]
		if subject.Type == types.SubjectTypeGroup && subject.ID == types.GroupAdmin {
			subject.Type = types.SubjectTypeObotGroup
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if err := client.Update(ctx, &rule); err != nil {
		return fmt.Errorf("failed to migrate everything access control rule: %w", err)
	}
	return nil
}

func migrateVMCPEverythingAdminGroups(ctx context.Context, client kclient.Client) error {
	var rules v1.AccessControlRuleList
	if err := client.List(ctx, &rules, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return fmt.Errorf("failed to list access control rules: %w", err)
	}

	var rule *v1.AccessControlRule
	for i := range rules.Items {
		if rules.Items[i].Name == "acr1-everything" {
			rule = &rules.Items[i]
			break
		}
	}
	if rule == nil || rule.Spec.MCPCatalogID != system.DefaultCatalog || rule.Spec.PowerUserWorkspaceID != "" {
		return nil
	}

	var vmcps v1.VMCPList
	if err := client.List(ctx, &vmcps, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}

	for i := range vmcps.Items {
		vmcp := &vmcps.Items[i]
		if vmcp.Spec.LegacySlug == "" || vmcp.Spec.UserID != "" {
			continue
		}
		// Workspace migrations reserve an owner profile even when components reference the default catalog.
		if slices.ContainsFunc(vmcp.Spec.Manifest.Profiles, func(p types.VMCPProfile) bool { return p.Name == catalogmigration.OwnerProfileName }) {
			continue
		}

		var source v1.MCPServerCatalogEntry
		err := client.Get(ctx, kclient.ObjectKey{Namespace: vmcp.Namespace, Name: vmcp.Spec.LegacySlug}, &source)
		if err == nil {
			if source.Spec.MCPCatalogName != system.DefaultCatalog || source.Spec.PowerUserWorkspaceID != "" {
				continue
			}
		} else if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get migrated source %s: %w", vmcp.Spec.LegacySlug, err)
		} else if vmcp.Annotations[compositemigration.DefaultCatalogSourceAnnotation] != "true" {
			// Only standalone migration components inherit their parent's catalog scope.
			if vmcp.Name != catalogmigration.VMCPName(vmcp.Namespace, vmcp.Spec.LegacySlug) ||
				len(vmcp.Spec.Manifest.Components) != 1 ||
				vmcp.Spec.Manifest.Components[0].ID != vmcp.Spec.LegacySlug ||
				vmcp.Spec.Manifest.Components[0].MCPCatalogID != system.DefaultCatalog {
				continue
			}
		}

		profileIndex := -1
		for j, profile := range vmcp.Spec.Manifest.Profiles {
			// Replay the allocator against preceding profiles, including its numeric collision fallbacks.
			if !migratedRuleProfileMatches(profile, vmcp.Spec.Manifest.Profiles[:j], *rule, vmcp.Spec.LegacySlug) {
				continue
			}
			// shortcut: profiles retain no rule IDs; preserve ambiguous grants until migrations persist provenance.
			if slices.ContainsFunc(rules.Items, func(other v1.AccessControlRule) bool {
				return other.Name != rule.Name &&
					other.Spec.MCPCatalogID == rule.Spec.MCPCatalogID &&
					other.Spec.PowerUserWorkspaceID == rule.Spec.PowerUserWorkspaceID &&
					migratedRuleProfileMatches(profile, vmcp.Spec.Manifest.Profiles[:j], other, vmcp.Spec.LegacySlug)
			}) {
				continue
			}
			if profileIndex != -1 {
				profileIndex = -1
				break
			}
			profileIndex = j
		}
		if profileIndex == -1 {
			continue
		}

		var changed bool
		profile := &vmcp.Spec.Manifest.Profiles[profileIndex]
		for k := range profile.Subjects {
			subject := &profile.Subjects[k]
			if subject.Type == types.SubjectTypeGroup && subject.ID == types.GroupAdmin {
				subject.Type = types.SubjectTypeObotGroup
				changed = true
			}
		}
		if changed {
			if err := client.Update(ctx, vmcp); err != nil {
				return fmt.Errorf("failed to migrate vMCP %s/%s: %w", vmcp.Namespace, vmcp.Name, err)
			}
		}
	}

	return nil
}

func migratedRuleProfileMatches(profile types.VMCPProfile, preceding []types.VMCPProfile, rule v1.AccessControlRule, source string) bool {
	if !slices.ContainsFunc(rule.Spec.Manifest.Resources, func(r types.Resource) bool {
		return (r.Type == types.ResourceTypeSelector && r.ID == "*") ||
			((r.Type == types.ResourceTypeMCPServer || r.Type == types.ResourceTypeMCPServerCatalogEntry) && r.ID == source)
	}) {
		return false
	}

	profiles := catalogmigration.AppendRuleProfile(slices.Clone(preceding), rule)
	if len(profiles) == len(preceding) {
		return false
	}
	expected := profiles[len(preceding)]
	return profile.Name == expected.Name && reflect.DeepEqual(profile.Permissions, expected.Permissions) &&
		slices.EqualFunc(profile.Subjects, expected.Subjects, func(a, b types.Subject) bool {
			if a.Type == types.SubjectTypeGroup && a.ID == types.GroupAdmin {
				a.Type = types.SubjectTypeObotGroup
			}
			if b.Type == types.SubjectTypeGroup && b.ID == types.GroupAdmin {
				b.Type = types.SubjectTypeObotGroup
			}
			return a == b
		})
}

func deleteToolReferenceOwnedModels(ctx context.Context, client kclient.Client) error {
	var models v1.ModelList
	if err := client.List(ctx, &models); err != nil {
		return err
	}

	for i := range models.Items {
		model := &models.Items[i]
		for _, owner := range model.OwnerReferences {
			if owner.Kind != "ToolReference" {
				continue
			}

			if err := kclient.IgnoreNotFound(client.Delete(ctx, model)); err != nil {
				return fmt.Errorf("failed to delete ToolReference-owned model %s/%s: %w", model.Namespace, model.Name, err)
			}
			break
		}
	}

	return nil
}

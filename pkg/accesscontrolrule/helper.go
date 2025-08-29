package accesscontrolrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
)

type Helper struct {
	acrIndexer gocache.Indexer
}

func NewAccessControlRuleHelper(acrIndexer gocache.Indexer) *Helper {
	return &Helper{
		acrIndexer: acrIndexer,
	}
}

func (h *Helper) GetAccessControlRulesForUser(namespace, userID string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrIndexer.ByIndex("user-ids", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for user: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res, ok := acr.(*v1.AccessControlRule)
		if ok && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetAccessControlRulesForMCPServer returns all AccessControlRules that contain the specified MCP server name
func (h *Helper) GetAccessControlRulesForMCPServer(namespace, serverName string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrIndexer.ByIndex("server-names", serverName)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for MCP server: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res, ok := acr.(*v1.AccessControlRule)
		if ok && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetAccessControlRulesForMCPServerCatalogEntry returns all AccessControlRules that contain the specified catalog entry name
func (h *Helper) GetAccessControlRulesForMCPServerCatalogEntry(namespace, entryName string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrIndexer.ByIndex("catalog-entry-names", entryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for MCP server catalog entry: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res, ok := acr.(*v1.AccessControlRule)
		if ok && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetAccessControlRulesForSelector returns all AccessControlRules that contain the specified selector
func (h *Helper) GetAccessControlRulesForSelector(namespace, selector string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrIndexer.ByIndex("selectors", selector)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for selector: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res, ok := acr.(*v1.AccessControlRule)
		if ok && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// Catalog-scoped lookup methods

// GetAccessControlRulesForMCPServerInCatalog returns all AccessControlRules that contain the specified MCP server name within a catalog
func (h *Helper) GetAccessControlRulesForMCPServerInCatalog(namespace, serverName, catalogID string) ([]v1.AccessControlRule, error) {
	rules, err := h.GetAccessControlRulesForMCPServer(namespace, serverName)
	if err != nil {
		return nil, err
	}

	result := make([]v1.AccessControlRule, 0, len(rules))
	for _, rule := range rules {
		// Include rules that match the catalog ID
		if rule.Spec.MCPCatalogID == catalogID {
			result = append(result, rule)
		}
	}

	return result, nil
}

// GetAccessControlRulesForMCPServerCatalogEntryInCatalog returns all AccessControlRules that contain the specified catalog entry name within a catalog
func (h *Helper) GetAccessControlRulesForMCPServerCatalogEntryInCatalog(namespace, entryName, catalogID string) ([]v1.AccessControlRule, error) {
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntry(namespace, entryName)
	if err != nil {
		return nil, err
	}

	result := make([]v1.AccessControlRule, 0, len(rules))
	for _, rule := range rules {
		// Include rules that match the catalog ID
		if rule.Spec.MCPCatalogID == catalogID {
			result = append(result, rule)
		}
	}

	return result, nil
}

// GetAccessControlRulesForSelectorInCatalog returns all AccessControlRules that contain the specified selector within a catalog
func (h *Helper) GetAccessControlRulesForSelectorInCatalog(namespace, selector, catalogID string) ([]v1.AccessControlRule, error) {
	rules, err := h.GetAccessControlRulesForSelector(namespace, selector)
	if err != nil {
		return nil, err
	}

	result := make([]v1.AccessControlRule, 0, len(rules))
	for _, rule := range rules {
		// Include rules that match the catalog ID
		if rule.Spec.MCPCatalogID == catalogID {
			result = append(result, rule)
		}
	}

	return result, nil
}

// UserHasAccessToMCPServerInCatalog checks if a user has access to a specific MCP server through AccessControlRules
// This method now requires the catalog ID to ensure proper scoping
func (h *Helper) UserHasAccessToMCPServerInCatalog(user kuser.Info, serverName, catalogID string) (bool, error) {
	// See if there is a selector that this user is included on in the specified catalog.
	selectorRules, err := h.GetAccessControlRulesForSelectorInCatalog(system.DefaultNamespace, "*", catalogID)
	if err != nil {
		return false, err
	}

	var (
		userID = user.GetUID()
		groups = authGroupSet(user)
	)
	for _, rule := range selectorRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			switch subject.Type {
			case types.SubjectTypeUser:
				if subject.ID == userID {
					return true, nil
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true, nil
				}
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true, nil
				}
			}
		}
	}

	// Now see if there is a rule that includes this specific server in the catalog.
	rules, err := h.GetAccessControlRulesForMCPServerInCatalog(system.DefaultNamespace, serverName, catalogID)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			switch subject.Type {
			case types.SubjectTypeUser:
				if subject.ID == userID {
					return true, nil
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true, nil
				}
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// UserHasAccessToMCPServer provides backward compatibility, defaulting to the default catalog
func (h *Helper) UserHasAccessToMCPServer(user kuser.Info, serverName string) (bool, error) {
	return h.UserHasAccessToMCPServerInCatalog(user, serverName, system.DefaultCatalog)
}

// UserHasAccessToMCPServerCatalogEntryInCatalog checks if a user has access to a specific catalog entry through AccessControlRules
// This method now requires the catalog ID to ensure proper scoping
func (h *Helper) UserHasAccessToMCPServerCatalogEntryInCatalog(user kuser.Info, entryName, catalogID string) (bool, error) {
	// See if there is a selector that this user is included on in the specified catalog.
	selectorRules, err := h.GetAccessControlRulesForSelectorInCatalog(system.DefaultNamespace, "*", catalogID)
	if err != nil {
		return false, err
	}

	var (
		userID = user.GetUID()
		groups = authGroupSet(user)
	)
	for _, rule := range selectorRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			switch subject.Type {
			case types.SubjectTypeUser:
				if subject.ID == userID {
					return true, nil
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true, nil
				}
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true, nil
				}
			}
		}
	}

	// Now see if there is a rule that includes this specific catalog entry.
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntryInCatalog(system.DefaultNamespace, entryName, catalogID)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			switch subject.Type {
			case types.SubjectTypeUser:
				if subject.ID == userID {
					return true, nil
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true, nil
				}
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// UserHasAccessToMCPServerCatalogEntry provides backward compatibility, defaulting to the default catalog
func (h *Helper) UserHasAccessToMCPServerCatalogEntry(user kuser.Info, entryName string) (bool, error) {
	return h.UserHasAccessToMCPServerCatalogEntryInCatalog(user, entryName, system.DefaultCatalog)
}

func authGroupSet(user kuser.Info) map[string]struct{} {
	groups := user.GetExtra()["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}

// Workspace-specific access control methods

// UserHasAccessToWorkspaceResource checks if a user has access to a resource within a PowerUserWorkspace
func (h *Helper) UserHasAccessToWorkspaceResource(user kuser.Info, workspaceID string, resourceType, resourceName string) (bool, error) {
	// Check if there are workspace-scoped ACRs that grant access
	rules, err := h.GetAccessControlRulesForWorkspace(system.DefaultNamespace, workspaceID)
	if err != nil {
		return false, err
	}

	var (
		userID = user.GetUID()
		groups = authGroupSet(user)
	)

	for _, rule := range rules {
		// Check if the rule includes the resource
		hasResource := false
		for _, resource := range rule.Spec.Manifest.Resources {
			if (resource.Type == types.ResourceType(resourceType) && resource.ID == resourceName) ||
				(resource.Type == types.ResourceTypeSelector && resource.ID == "*") {
				hasResource = true
				break
			}
		}

		if !hasResource {
			continue
		}

		// Check if the user is included in the subjects
		for _, subject := range rule.Spec.Manifest.Subjects {
			switch subject.Type {
			case types.SubjectTypeUser:
				if subject.ID == userID {
					return true, nil
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true, nil
				}
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// GetAccessControlRulesForWorkspace returns all AccessControlRules scoped to a specific workspace
func (h *Helper) GetAccessControlRulesForWorkspace(namespace, workspaceID string) ([]v1.AccessControlRule, error) {
	// Get all ACRs and filter by workspace
	var allRules v1.AccessControlRuleList
	// Note: This would need to be implemented with proper indexing for performance
	// For now, we'll implement a basic version that can be optimized later

	// This is a placeholder - in a real implementation, we'd need to add indexing
	// for PowerUserWorkspaceName field in the services/config.go file
	result := make([]v1.AccessControlRule, 0)

	// TODO: Add proper indexing for workspace-scoped ACRs in services/config.go
	// Similar to how user-ids, server-names, etc. are indexed

	return result, nil
}

// UserCanCreateInWorkspace checks if a user can create resources in a workspace
func (h *Helper) UserCanCreateInWorkspace(user kuser.Info, workspaceID string) (bool, error) {
	// This would typically check if the user owns the workspace
	// For now, return false as this needs workspace ownership verification
	// which should be handled by the PowerUserWorkspace authorization
	return false, nil
}

// UserCanManageWorkspaceACRs checks if a user can manage Access Control Rules in a workspace
func (h *Helper) UserCanManageWorkspaceACRs(user kuser.Info, workspaceID string) (bool, error) {
	// This should check if the user has PowerUserPlus role and owns the workspace
	// For now, return false as this needs role verification which should be handled
	// by the PowerUserWorkspace authorization
	return false, nil
}

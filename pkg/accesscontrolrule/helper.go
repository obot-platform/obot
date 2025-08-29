package accesscontrolrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	types2 "github.com/obot-platform/obot/apiclient/types"
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

// Workspace-scoped access control methods

// UserHasAccessToWorkspaceResource checks if a user has access to a resource within a specific PowerUserWorkspace
func (h *Helper) UserHasAccessToWorkspaceResource(user kuser.Info, workspaceID, resourceName, resourceType string) (bool, error) {
	// Admin users have access to all resources
	if h.isUserAdmin(user) {
		return true, nil
	}

	// Users can access resources in their own workspace
	if h.isUserWorkspaceOwner(user, workspaceID) {
		return true, nil
	}

	// Check if user has explicit access through ACRs created by the workspace owner
	rules, err := h.GetAccessControlRulesForWorkspace(system.DefaultNamespace, workspaceID)
	if err != nil {
		return false, err
	}

	userID := user.GetUID()
	groups := authGroupSet(user)

	for _, rule := range rules {
		// Check if this rule grants access to the specific resource
		if h.ruleGrantsAccessToResource(rule, resourceName, resourceType) {
			// Check if user matches any subject in the rule
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
	}

	return false, nil
}

// GetAccessControlRulesForWorkspace returns all AccessControlRules owned by a specific workspace
func (h *Helper) GetAccessControlRulesForWorkspace(namespace, workspaceID string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrIndexer.ByIndex("workspace-ids", workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for workspace: %w", err)
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

// UserCanCreateMCPServerInWorkspace checks if a user can create an MCP server in the specified workspace
func (h *Helper) UserCanCreateMCPServerInWorkspace(user kuser.Info, workspaceID string, serverType MCPServerType) (bool, error) {
	// Admin users can create any type of MCP server
	if h.isUserAdmin(user) {
		return true, nil
	}

	// Users can create servers in their own workspace based on their role
	if h.isUserWorkspaceOwner(user, workspaceID) {
		// Get the user's role from the workspace
		workspace, err := h.getWorkspaceByID(workspaceID)
		if err != nil {
			return false, err
		}

		switch serverType {
		case MCPServerTypeSingleUser, MCPServerTypeCatalogEntry:
			// Power Users and Power User Plus can create single-user servers and catalog entries
			return workspace.Spec.Role == types2.RolePowerUser || workspace.Spec.Role == types2.RolePowerUserPlus, nil
		case MCPServerTypeMultiUser:
			// Only Power User Plus can create multi-user servers
			return workspace.Spec.Role == types2.RolePowerUserPlus, nil
		}
	}

	return false, nil
}

// UserCanCreateACRInWorkspace checks if a user can create an AccessControlRule in the specified workspace
func (h *Helper) UserCanCreateACRInWorkspace(user kuser.Info, workspaceID string) (bool, error) {
	// Admin users can create ACRs anywhere
	if h.isUserAdmin(user) {
		return true, nil
	}

	// Users can create ACRs in their own workspace if they're Power User Plus
	if h.isUserWorkspaceOwner(user, workspaceID) {
		workspace, err := h.getWorkspaceByID(workspaceID)
		if err != nil {
			return false, err
		}
		return workspace.Spec.Role == types2.RolePowerUserPlus, nil
	}

	return false, nil
}

// Helper methods

func (h *Helper) isUserAdmin(user kuser.Info) bool {
	groups := user.GetGroups()
	for _, group := range groups {
		if group == "admin" { // This should match authz.AdminGroup
			return true
		}
	}
	return false
}

func (h *Helper) isUserWorkspaceOwner(user kuser.Info, workspaceID string) bool {
	// Workspace IDs are in format "workspace-{userID}"
	expectedWorkspaceID := fmt.Sprintf("workspace-%s", user.GetUID())
	return workspaceID == expectedWorkspaceID
}

func (h *Helper) ruleGrantsAccessToResource(rule v1.AccessControlRule, resourceName, resourceType string) bool {
	for _, resource := range rule.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeSelector:
			if resource.ID == "*" {
				return true
			}
		case types.ResourceTypeMCPServer:
			if resourceType == "mcpserver" && resource.ID == resourceName {
				return true
			}
		case types.ResourceTypeMCPServerCatalogEntry:
			if resourceType == "mcpservercatalogentry" && resource.ID == resourceName {
				return true
			}
		}
	}
	return false
}

// This is a placeholder - in a real implementation, this would query the storage layer
func (h *Helper) getWorkspaceByID(workspaceID string) (*v1.PowerUserWorkspace, error) {
	// This would need to be implemented to actually fetch the workspace from storage
	// For now, returning an error to indicate it needs implementation
	return nil, fmt.Errorf("getWorkspaceByID not implemented")
}

// MCPServerType represents the type of MCP server
type MCPServerType string

const (
	MCPServerTypeSingleUser    MCPServerType = "single-user"
	MCPServerTypeMultiUser     MCPServerType = "multi-user"
	MCPServerTypeCatalogEntry  MCPServerType = "catalog-entry"
)

func authGroupSet(user kuser.Info) map[string]struct{} {
	groups := user.GetExtra()["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}

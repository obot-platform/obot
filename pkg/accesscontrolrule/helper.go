package accesscontrolrule

import (
	"context"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	gocache "k8s.io/client-go/tools/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Helper struct {
	acrIndexer gocache.Indexer
	client     client.Client
}

func NewAccessControlRuleHelper(acrIndexer gocache.Indexer, client client.Client) *Helper {
	return &Helper{
		acrIndexer: acrIndexer,
		client:     client,
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
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
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
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
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
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
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
		if ok && res.Namespace == namespace && res.DeletionTimestamp.IsZero() {
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

// HasWildcardAccessToMCPServerCatalogEntryInCatalog checks if there are ACRs with wildcard selector for an entry
func (h *Helper) HasWildcardAccessToMCPServerCatalogEntryInCatalog(entryName, catalogID string) (bool, error) {
	// Check wildcard selector rules first
	selectorRules, err := h.GetAccessControlRulesForSelectorInCatalog(
		system.DefaultNamespace, "*", catalogID)
	if err != nil {
		return false, err
	}

	for _, rule := range selectorRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				return true, nil
			}
		}
	}

	// Check specific entry rules
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntryInCatalog(
		system.DefaultNamespace, entryName, catalogID)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				return true, nil
			}
		}
	}

	return false, nil
}

// HasWildcardAccessToMCPServerInCatalog checks if there are ACRs with wildcard selector for a server
func (h *Helper) HasWildcardAccessToMCPServerInCatalog(serverName, catalogID string) (bool, error) {
	// Check wildcard selector rules first
	selectorRules, err := h.GetAccessControlRulesForSelectorInCatalog(
		system.DefaultNamespace, "*", catalogID)
	if err != nil {
		return false, err
	}

	for _, rule := range selectorRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				return true, nil
			}
		}
	}

	// Check server-specific rules
	rules, err := h.GetAccessControlRulesForMCPServerInCatalog(
		system.DefaultNamespace, serverName, catalogID)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if subject.Type == types.SubjectTypeSelector && subject.ID == "*" {
				return true, nil
			}
		}
	}

	return false, nil
}

// Workspace-scoped lookup methods

// GetAccessControlRulesForMCPServerInWorkspace returns all AccessControlRules that contain the specified MCP server name within a workspace
func (h *Helper) GetAccessControlRulesForMCPServerInWorkspace(namespace, serverName, workspaceID string) ([]v1.AccessControlRule, error) {
	rules, err := h.GetAccessControlRulesForMCPServer(namespace, serverName)
	if err != nil {
		return nil, err
	}

	result := make([]v1.AccessControlRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Spec.PowerUserWorkspaceID == workspaceID {
			result = append(result, rule)
		}
	}

	return result, nil
}

// GetAccessControlRulesForMCPServerCatalogEntryInWorkspace returns all AccessControlRules that contain the specified catalog entry name within a workspace
func (h *Helper) GetAccessControlRulesForMCPServerCatalogEntryInWorkspace(namespace, entryName, workspaceID string) ([]v1.AccessControlRule, error) {
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntry(namespace, entryName)
	if err != nil {
		return nil, err
	}

	result := make([]v1.AccessControlRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Spec.PowerUserWorkspaceID == workspaceID {
			result = append(result, rule)
		}
	}

	return result, nil
}

// GetAccessControlRulesForSelectorInWorkspace returns all AccessControlRules that contain the specified selector within a workspace
func (h *Helper) GetAccessControlRulesForSelectorInWorkspace(namespace, selector, workspaceID string) ([]v1.AccessControlRule, error) {
	rules, err := h.GetAccessControlRulesForSelector(namespace, selector)
	if err != nil {
		return nil, err
	}

	result := make([]v1.AccessControlRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Spec.PowerUserWorkspaceID == workspaceID {
			result = append(result, rule)
		}
	}

	return result, nil
}

// UserHasAccessToMCPServerInWorkspace checks if a user has access to a specific MCP server through workspace-scoped AccessControlRules
func (h *Helper) UserHasAccessToMCPServerInWorkspace(user kuser.Info, serverName, workspaceID, serverUserID string) (bool, error) {
	var (
		userID = user.GetUID()
		groups = authGroupSet(user)
	)

	// If the server is owned by the current user, they have access to it to ignore the AccessControlRules
	if serverUserID == userID {
		return true, nil
	}

	// See if there is a selector that this user is included on in the specified workspace.
	selectorRules, err := h.GetAccessControlRulesForSelectorInWorkspace(system.DefaultNamespace, "*", workspaceID)
	if err != nil {
		return false, err
	}

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

	// Now see if there is a rule that includes this specific server in the workspace.
	rules, err := h.GetAccessControlRulesForMCPServerInWorkspace(system.DefaultNamespace, serverName, workspaceID)
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

// UserHasAccessToMCPServerCatalogEntryInWorkspace checks if a user has access to a specific catalog entry through workspace-scoped AccessControlRules
func (h *Helper) UserHasAccessToMCPServerCatalogEntryInWorkspace(ctx context.Context, user kuser.Info, entryName, workspaceID string) (bool, error) {
	// See if there is a selector that this user is included on in the specified workspace.
	selectorRules, err := h.GetAccessControlRulesForSelectorInWorkspace(system.DefaultNamespace, "*", workspaceID)
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
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntryInWorkspace(system.DefaultNamespace, entryName, workspaceID)
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

	// If the workspace is owned by the current user, they have access to all entries in the workspace
	if workspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := h.client.Get(ctx, client.ObjectKey{Namespace: system.DefaultNamespace, Name: workspaceID}, &workspace); err != nil {
			return false, err
		}
		if workspace.Spec.UserID == userID {
			return true, nil
		}
	}

	return false, nil
}

func authGroupSet(user kuser.Info) map[string]struct{} {
	groups := user.GetExtra()["auth_provider_groups"]
	set := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		set[group] = struct{}{}
	}
	return set
}

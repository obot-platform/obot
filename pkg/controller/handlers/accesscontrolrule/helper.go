package accesscontrolrule

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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

// UserHasAccessToMCPServer checks if a user has access to a specific MCP server through AccessControlRules
func (h *Helper) UserHasAccessToMCPServer(userID, serverName string) (bool, error) {
	// See if there is a selector that this user is included on.
	selectorRules, err := h.GetAccessControlRulesForSelector(system.DefaultNamespace, "*")
	if err != nil {
		return false, err
	}

	for _, rule := range selectorRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if (subject.Type == types.SubjectTypeUser && subject.ID == userID) || (subject.Type == types.SubjectTypeSelector && subject.ID == "*") {
				return true, nil
			}
		}
	}

	// Now see if there is a rule that includes this specific server.
	rules, err := h.GetAccessControlRulesForMCPServer(system.DefaultNamespace, serverName)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if (subject.Type == types.SubjectTypeUser && subject.ID == userID) || (subject.Type == types.SubjectTypeSelector && subject.ID == "*") {
				return true, nil
			}
		}
	}

	return false, nil
}

// UserHasAccessToMCPServerCatalogEntry checks if a user has access to a specific catalog entry through AccessControlRules
func (h *Helper) UserHasAccessToMCPServerCatalogEntry(userID, entryName string) (bool, error) {
	// See if there is a selector that this user is included on.
	selectorRules, err := h.GetAccessControlRulesForSelector(system.DefaultNamespace, "*")
	if err != nil {
		return false, err
	}

	for _, rule := range selectorRules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if (subject.Type == types.SubjectTypeUser && subject.ID == userID) || (subject.Type == types.SubjectTypeSelector && subject.ID == "*") {
				return true, nil
			}
		}
	}

	// Now see if there is a rule that includes this specific catalog entry.
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntry(system.DefaultNamespace, entryName)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, subject := range rule.Spec.Manifest.Subjects {
			if (subject.Type == types.SubjectTypeUser && subject.ID == userID) || (subject.Type == types.SubjectTypeSelector && subject.ID == "*") {
				return true, nil
			}
		}
	}

	return false, nil
}

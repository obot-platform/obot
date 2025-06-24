package accesscontrolrule

import (
	"fmt"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	gocache "k8s.io/client-go/tools/cache"
)

type Helper struct {
	acrInformer gocache.SharedIndexInformer
}

func NewAccessControlRuleHelper(acrInformer gocache.SharedIndexInformer) *Helper {
	return &Helper{
		acrInformer: acrInformer,
	}
}

// GetAccessControlRulesForUser returns all AccessControlRules that contain the specified user ID
func (h *Helper) GetAccessControlRulesForUser(namespace, userID string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrInformer.GetIndexer().ByIndex("user-ids", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for user: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res := acr.(*v1.AccessControlRule)
		if res != nil && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetAccessControlRulesForMCPServer returns all AccessControlRules that contain the specified MCP server name
func (h *Helper) GetAccessControlRulesForMCPServer(namespace, serverName string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrInformer.GetIndexer().ByIndex("server-names", serverName)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for MCP server: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res := acr.(*v1.AccessControlRule)
		if res != nil && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// GetAccessControlRulesForMCPServerCatalogEntry returns all AccessControlRules that contain the specified catalog entry name
func (h *Helper) GetAccessControlRulesForMCPServerCatalogEntry(namespace, entryName string) ([]v1.AccessControlRule, error) {
	acrs, err := h.acrInformer.GetIndexer().ByIndex("catalog-entry-names", entryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get access control rules for MCP server catalog entry: %w", err)
	}

	result := make([]v1.AccessControlRule, 0, len(acrs))
	for _, acr := range acrs {
		res := acr.(*v1.AccessControlRule)
		if res != nil && res.Namespace == namespace {
			result = append(result, *res)
		}
	}

	return result, nil
}

// UserHasAccessToMCPServer checks if a user has access to a specific MCP server through AccessControlRules
func (h *Helper) UserHasAccessToMCPServer(userID, serverName string) (bool, error) {
	rules, err := h.GetAccessControlRulesForMCPServer(system.DefaultNamespace, serverName)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, uid := range rule.Spec.UserIDs {
			if uid == userID || uid == "*" {
				return true, nil
			}
		}
	}

	return false, nil
}

// UserHasAccessToMCPServerCatalogEntry checks if a user has access to a specific catalog entry through AccessControlRules
func (h *Helper) UserHasAccessToMCPServerCatalogEntry(userID, entryName string) (bool, error) {
	rules, err := h.GetAccessControlRulesForMCPServerCatalogEntry(system.DefaultNamespace, entryName)
	if err != nil {
		return false, err
	}

	for _, rule := range rules {
		for _, uid := range rule.Spec.UserIDs {
			if uid == userID || uid == "*" {
				return true, nil
			}
		}
	}

	return false, nil
}

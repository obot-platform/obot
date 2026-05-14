package mcp

import (
	"fmt"
	"slices"

	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

type WebhookHelper struct {
	indexer cache.Indexer
	baseURL string
}

func NewWebhookHelper(indexer cache.Indexer, baseURL string) *WebhookHelper {
	return &WebhookHelper{
		indexer: indexer,
		baseURL: baseURL,
	}
}

type Webhook struct {
	Name, DisplayName string
	URL               string
	ToolName          string
	Definitions       []string
	MutateAllowed     bool
}

func (wh *WebhookHelper) GetWebhooksForMCPServer(serverConfig ServerConfig) ([]Webhook, error) {
	var result []Webhook
	webhookSeen := make(map[string]struct{})

	objs, err := wh.indexer.ByIndex("server-names", serverConfig.MCPServerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from MCP server index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-entry-names", serverConfig.MCPCatalogEntryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog entry index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("selectors", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from selector index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-names", serverConfig.MCPCatalogName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	return result, nil
}

func (wh *WebhookHelper) appendWebhooks(namespace string, objs []any, seen map[string]struct{}, result []Webhook) []Webhook {
	result = slices.Grow(result, len(objs))

	for _, mwv := range objs {
		res, ok := mwv.(*v1.MCPWebhookValidation)
		if ok && res.Namespace == namespace && !res.Spec.Manifest.Disabled && res.Status.Configured {
			url := system.MCPConnectURL(wh.baseURL, system.SystemMCPServerPrefix+res.Name)
			if _, seen := seen[url]; seen {
				continue
			}

			seen[url] = struct{}{}

			displayName := res.Spec.Manifest.Name
			if displayName == "" {
				displayName = res.Name
			}

			toolName := res.Spec.Manifest.ToolName
			if toolName == "" {
				toolName = defaultWebhookToolName
			}

			result = append(result, Webhook{
				Name:          res.Name,
				DisplayName:   displayName,
				URL:           url,
				ToolName:      toolName,
				Definitions:   res.Spec.Manifest.Selectors.Strings(),
				MutateAllowed: res.Spec.Manifest.AllowedToMutate,
			})
		}
	}

	return result
}

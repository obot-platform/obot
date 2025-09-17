package mcp

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gptscript-ai/go-gptscript"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

type WebhookHelper struct {
	indexer cache.Indexer
}

func NewWebhookHelper(indexer cache.Indexer) *WebhookHelper {
	return &WebhookHelper{
		indexer: indexer,
	}
}

type Webhook struct {
	URL, Secret string
}

func (wh *WebhookHelper) GetWebhooksForMCPServer(ctx context.Context, gptClient *gptscript.GPTScript, mcpServerNamespace, mcpServerName, mcpServerCatalogEntryName, mcpServerCatalogName, method, identifier string) ([]Webhook, error) {
	var result []Webhook
	webhookSeen := make(map[string]struct{})

	objs, err := wh.indexer.ByIndex("server-names", mcpServerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from MCP server index: %w", err)
	}

	result = appendWebhooks(ctx, gptClient, mcpServerNamespace, method, identifier, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-entry-names", mcpServerCatalogEntryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog entry index: %w", err)
	}

	result = appendWebhooks(ctx, gptClient, mcpServerNamespace, method, identifier, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("selectors", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from selector index: %w", err)
	}

	result = appendWebhooks(ctx, gptClient, mcpServerNamespace, method, identifier, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-names", mcpServerCatalogName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog index: %w", err)
	}

	result = appendWebhooks(ctx, gptClient, mcpServerNamespace, method, identifier, objs, webhookSeen, result)

	return result, nil
}

func appendWebhooks(ctx context.Context, gptClient *gptscript.GPTScript, namespace, method, identifier string, objs []any, seen map[string]struct{}, result []Webhook) []Webhook {
	var credEnv map[string]string
	result = slices.Grow(result, len(objs))

	for _, mwv := range objs {
		res, ok := mwv.(*v1.MCPWebhookValidation)
		if ok && res.Namespace == namespace {
			url := res.Spec.Manifest.URL
			if _, seen := seen[url]; seen || !res.Spec.Manifest.Selectors.Matches(method, identifier) {
				continue
			}

			seen[url] = struct{}{}
			if credEnv == nil {
				// Only reveal the credential once
				cred, err := gptClient.RevealCredential(ctx, []string{system.MCPWebhookValidationCredentialContext}, res.Name)
				if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
					continue
				}

				credEnv = cred.Env
				if credEnv == nil {
					credEnv = make(map[string]string)
				}
			}

			result = append(result, Webhook{
				URL:    url,
				Secret: credEnv["secret"],
			})
		}
	}

	return result
}

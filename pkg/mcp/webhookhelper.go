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
	indexer          cache.Indexer
	defaultBaseImage string
}

func NewWebhookHelper(indexer cache.Indexer, defaultBaseImage string) *WebhookHelper {
	return &WebhookHelper{
		indexer:          indexer,
		defaultBaseImage: defaultBaseImage,
	}
}

type Webhook struct {
	Name, DisplayName  string
	URL, Secret, Image string
	Definitions        []string
}

func (wh *WebhookHelper) GetWebhooksForMCPServer(ctx context.Context, gptClient *gptscript.GPTScript, serverConfig ServerConfig) ([]Webhook, error) {
	var result []Webhook
	webhookSeen := make(map[string]struct{})

	objs, err := wh.indexer.ByIndex("server-names", serverConfig.MCPServerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from MCP server index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-entry-names", serverConfig.MCPCatalogEntryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog entry index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("selectors", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from selector index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	objs, err = wh.indexer.ByIndex("catalog-names", serverConfig.MCPCatalogName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog index: %w", err)
	}

	result = wh.appendWebhooks(ctx, gptClient, serverConfig.MCPServerNamespace, objs, webhookSeen, result)

	return result, nil
}

func (wh *WebhookHelper) appendWebhooks(ctx context.Context, gptClient *gptscript.GPTScript, namespace string, objs []any, seen map[string]struct{}, result []Webhook) []Webhook {
	var credEnv map[string]string
	result = slices.Grow(result, len(objs))

	for _, mwv := range objs {
		res, ok := mwv.(*v1.MCPWebhookValidation)
		if ok && res.Namespace == namespace && !res.Spec.Manifest.Disabled {
			url := res.Spec.Manifest.URL
			if _, seen := seen[url]; seen {
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
					// Set this to something non-nil so we don't fetch the credential again.
					credEnv = make(map[string]string)
				}
			}

			displayName := res.Spec.Manifest.Name
			if displayName == "" {
				displayName = res.Name
			}

			result = append(result, Webhook{
				Name:        res.Name,
				DisplayName: displayName,
				URL:         url,
				Secret:      credEnv["secret"],
				Image:       wh.defaultBaseImage,
				Definitions: res.Spec.Manifest.Selectors.Strings(),
			})
		}
	}

	return result
}

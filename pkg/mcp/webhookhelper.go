package mcp

import (
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/client-go/tools/cache"
)

type WebhookHelper struct {
	indexer cache.Indexer
	baseURL string
}

type Webhook struct {
	Name, DisplayName string
	URL               string
	Audience          string
	ToolName          string
	Definitions       types.MCPSelectors
	MutateAllowed     bool
	// Guardrail is set for a guardrail filter, which runs in-process: it has no URL,
	// audience or tool.
	Guardrail *types.MCPGuardrailFilterConfig `json:",omitempty"`
}

func NewWebhookHelper(indexer cache.Indexer, baseURL string) *WebhookHelper {
	return &WebhookHelper{
		indexer: indexer,
		baseURL: baseURL,
	}
}

func (wh *WebhookHelper) GetWebhooksForMCPServer(serverConfig ServerConfig, transformHostname func(string) string) ([]Webhook, error) {
	var result []Webhook
	webhookSeen := make(map[string]struct{})

	objs, err := wh.indexer.ByIndex("server-names", serverConfig.MCPServerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from MCP server index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result, transformHostname)

	objs, err = wh.indexer.ByIndex("catalog-entry-names", serverConfig.MCPCatalogEntryName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog entry index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result, transformHostname)

	objs, err = wh.indexer.ByIndex("selectors", "*")
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from selector index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result, transformHostname)

	objs, err = wh.indexer.ByIndex("catalog-names", serverConfig.MCPCatalogName)
	if err != nil {
		return nil, fmt.Errorf("failed to get webhooks from catalog index: %w", err)
	}

	result = wh.appendWebhooks(serverConfig.MCPServerNamespace, objs, webhookSeen, result, transformHostname)

	return result, nil
}

func (wh *WebhookHelper) appendWebhooks(namespace string, objs []any, seen map[string]struct{}, result []Webhook, transformHostname func(string) string) []Webhook {
	result = slices.Grow(result, len(objs))

	for _, mwv := range objs {
		res, ok := mwv.(*v1.MCPWebhookValidation)
		// A guardrail filter is used even when its status isn't configured, e.g. because a
		// guardrail was deleted: it then fails at runtime, and blocks if it fails closed.
		if ok && res.Namespace == namespace && !res.Spec.Manifest.Disabled && (res.Status.Configured || res.Spec.Manifest.Guardrail != nil) {
			url := system.MCPConnectURL(wh.baseURL, system.SystemMCPServerPrefix+res.Name)
			if _, seen := seen[url]; seen {
				continue
			}

			seen[url] = struct{}{}

			displayName := res.Spec.Manifest.Name
			if displayName == "" {
				displayName = res.Name
			}

			if guardrail := res.Spec.Manifest.Guardrail; guardrail != nil {
				result = append(result, Webhook{
					Name:        res.Name,
					DisplayName: displayName,
					Definitions: res.Spec.Manifest.Selectors,
					Guardrail:   guardrail.DeepCopy(),
				})
				continue
			}

			toolName := res.Spec.Manifest.ToolName
			if toolName == "" {
				toolName = defaultWebhookToolName
			}

			result = append(result, Webhook{
				Name:          res.Name,
				DisplayName:   displayName,
				URL:           transformHostname(url),
				Audience:      url,
				ToolName:      toolName,
				Definitions:   res.Spec.Manifest.Selectors,
				MutateAllowed: res.Spec.Manifest.AllowedToMutate,
			})
		}
	}

	return result
}

// FilterServerManifest returns the manifest of the system MCP server that runs a filter. A built-in filter runs its own
// manifest, and a webhook filter runs image, which forwards to its URL. The filter controller deploys it, and billing
// measures it.
func FilterServerManifest(webhookValidation *v1.MCPWebhookValidation, image string) types.SystemMCPServerManifest {
	var manifest types.SystemMCPServerManifest
	if webhookValidation.Spec.Manifest.SystemMCPServerManifest != nil {
		manifest = *webhookValidation.Spec.Manifest.SystemMCPServerManifest.DeepCopy()
	} else {
		manifest = types.SystemMCPServerManifest{
			ShortDescription: "Managed webhook validation server",
			Enabled:          new(!webhookValidation.Spec.Manifest.Disabled),
			Runtime:          types.RuntimeContainerized,
			ContainerizedConfig: &types.ContainerizedRuntimeConfig{
				Image: image,
				Port:  8099,
				Path:  "/mcp",
			},
			Config: []types.MCPConfig{{
				Key:   "WEBHOOK_URL",
				Value: webhookValidation.Spec.Manifest.URL,
				Usage: types.Env,
			},
				{
					Key:       "WEBHOOK_SECRET",
					Sensitive: true,
					Usage:     types.Env,
				},
				{
					Key:   "PORT",
					Value: "8099",
					Usage: types.Env,
				}},
		}
	}

	// Name it for the filter, falling back to the manifest's name and then the filter's ID.
	if name := webhookValidation.Spec.Manifest.Name; name != "" {
		manifest.Name = name
	} else if manifest.Name == "" {
		manifest.Name = webhookValidation.Name
	}

	return manifest
}

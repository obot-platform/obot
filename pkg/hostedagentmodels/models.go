// Package hostedagentmodels resolves the models a hosted agent may use.
//
// It exists because two callers need the same answer and must not disagree:
// the controller, which writes the resolved endpoints into a sandbox's config,
// and the API, which decides whether an agent can be launched at all. If the
// API said an agent was launchable and the controller then resolved no models,
// the user would get a sandbox that cannot do anything.
package hostedagentmodels

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Wire protocols a model can be reached over.
//
// These are distinct request and response formats, not vendors: a client
// written for the Responses API cannot talk to a chat-completions endpoint, and
// neither can talk to Anthropic's Messages API. Codex speaks Responses, Claude
// Code speaks Messages, and a library such as LiteLLM speaks chat completions --
// so this is what decides whether a given harness can use a given model.
const (
	APIAnthropic             = "anthropic"
	APIOpenAIResponses       = "openai-responses"
	APIOpenAIChatCompletions = "openai-chat-completions"
)

// AllAPIs is every protocol a harness may declare.
var AllAPIs = []string{APIAnthropic, APIOpenAIResponses, APIOpenAIChatCompletions}

// Model is one usable model: resolved to a concrete target, a provider, and the
// protocols it can be reached over.
type Model struct {
	ID       string
	Model    string
	Provider string
	// APIs is every protocol this model accepts. A model is usually reachable
	// over more than one -- an OpenAI endpoint serves both Responses and chat
	// completions -- so a single value would exclude clients that would work.
	APIs []string
	// Dialect is what the model declares, empty when it declares nothing.
	Dialect string
}

// Speaks reports whether this model accepts the given protocol. An empty
// protocol means the caller has no requirement, which nothing fails.
func (m Model) Speaks(api string) bool {
	return api == "" || slices.Contains(m.APIs, api)
}

// Resolve turns an agent's model selections into the models it can actually
// use. A selection is a model ID, an "obot://<alias>" reference, the id a
// provider itself uses ("claude-opus-4-8"), or "*" for everything the
// installation has.
//
// Order is meaningful: selections are added in the order given, and the
// wildcard last, so a template can list the models it prefers and fall back to
// whatever exists. Default reads the first entry when the installation's alias
// names nothing the agent may use, which is what turns that fallback from an
// arbitrary choice into a stated one.
//
// providers, when non-empty, restricts the result to models from those model
// providers, named by their stable identifier.
//
// A selection that resolves to nothing is skipped rather than failing: an agent
// may have several, and an alias can be unbound on a fresh install. The result
// is therefore what is genuinely usable, which is what both callers need.
func Resolve(ctx context.Context, client kclient.Client, namespace string, selections, providers []string) ([]Model, error) {
	var (
		models   []Model
		seen     = map[string]struct{}{}
		wildcard bool
		usable   []Model
		listed   bool
	)

	// Read at most once. A template may name several models the way a provider
	// does, and the wildcard needs the same list, so each would otherwise cost
	// its own read.
	usableModels := func() ([]Model, error) {
		if !listed {
			var err error
			if usable, err = listUsable(ctx, client, namespace); err != nil {
				return nil, err
			}
			listed = true
		}
		return usable, nil
	}

	add := func(model *Model) {
		if model == nil {
			return
		}
		// An agent that names its providers is restricted to them. This is what
		// makes "Claude Code needs Anthropic" expressible without inventing a
		// separate notion of protocol: the template names the provider it was
		// written against.
		if len(providers) > 0 && !slices.Contains(providers, model.Provider) {
			return
		}
		if _, ok := seen[model.ID]; ok {
			return
		}
		apis := APIsFor(model.Provider, model.Dialect)
		if len(apis) == 0 {
			// No proxy route serves this provider, so the model cannot be
			// reached however it was selected.
			return
		}
		seen[model.ID] = struct{}{}
		model.APIs = apis
		models = append(models, *model)
	}

	for _, selection := range selections {
		switch selection = strings.TrimSpace(selection); selection {
		case "":
			continue
		case "*":
			wildcard = true
			continue
		}

		model, err := resolveOne(ctx, client, namespace, selection)
		if err != nil {
			// Not an alias or a resource name, so try it as the id a provider
			// itself uses. A resource id is a per-installation hash, which a
			// template shipped to every installation cannot know; "claude-opus-
			// 4-8" is the same everywhere, and is the only way a template can
			// state a preference that survives being copied.
			all, listErr := usableModels()
			if listErr != nil {
				return nil, listErr
			}
			if model = matchTarget(all, selection, providers); model == nil {
				continue
			}
		}
		add(model)
	}

	if wildcard {
		all, err := usableModels()
		if err != nil {
			return nil, err
		}
		for i := range all {
			add(&all[i])
		}
	}
	return models, nil
}

// matchTarget finds the model a provider-native id names.
//
// More than one provider can serve the same id -- "claude-opus-4-8" through
// both Anthropic and Bedrock -- so the agent's own providers decide which was
// meant. Without that, a preference could resolve to a provider the agent may
// not use and would then be dropped by the caller, leaving the template looking
// configured while changing nothing.
//
// models arrives sorted, so a name served by several allowed providers resolves
// the same way on every reconcile rather than reordering the sandbox's config.
func matchTarget(models []Model, target string, providers []string) *Model {
	for i := range models {
		if models[i].Model != target {
			continue
		}
		if len(providers) > 0 && !slices.Contains(providers, models[i].Provider) {
			continue
		}
		return &models[i]
	}
	return nil
}

// Default returns the model to reach for, following the installation's llm
// alias and falling back to the first listed.
func Default(ctx context.Context, client kclient.Client, namespace string, models []Model) string {
	if len(models) == 0 {
		return ""
	}
	if resolved, err := resolveOne(ctx, client, namespace,
		types.DefaultModelAliasRefPrefix+string(types.DefaultModelAliasTypeLLM)); err == nil && resolved != nil {
		for _, model := range models {
			if model.ID == resolved.ID {
				return model.ID
			}
		}
	}
	return models[0].ID
}

// APIsFor reports the protocols a model accepts.
//
// A declared dialect is authoritative and narrows the model to exactly one
// protocol, which is how an administrator states that an endpoint serves only
// one. Without a declaration the provider decides, and OpenAI-shaped providers
// are credited with both OpenAI protocols: their endpoints generally serve both,
// and being too narrow here would hide a model that works. Being too broad
// fails later at the endpoint, with an error naming the format -- which is the
// better failure of the two.
func APIsFor(provider, dialect string) []string {
	switch dialect {
	case "AnthropicMessages":
		return []string{APIAnthropic}
	case "OpenAIResponses", "OpenResponses":
		return []string{APIOpenAIResponses}
	case "OpenAIChatCompletions":
		return []string{APIOpenAIChatCompletions}
	}

	switch provider {
	case system.AnthropicModelProvider:
		return []string{APIAnthropic}
	case system.OpenAIModelProvider,
		system.GenericResponsesModelProvider,
		system.AmazonBedrockModelProvider,
		system.AmazonBedrockAPIKeyModelProvider,
		system.AzureModelProvider,
		system.AzureEntraModelProvider:
		return []string{APIOpenAIResponses, APIOpenAIChatCompletions}
	default:
		return nil
	}
}

// ProxyPath is the segment of Obot's LLM proxy route that serves a provider.
func ProxyPath(provider string) (string, bool) {
	switch provider {
	case system.AnthropicModelProvider:
		return "anthropic", true
	case system.OpenAIModelProvider:
		return "openai", true
	case system.GenericResponsesModelProvider:
		return "generic-responses", true
	case system.AmazonBedrockModelProvider:
		return "aws-bedrock", true
	case system.AmazonBedrockAPIKeyModelProvider:
		return "aws-bedrock-api-key", true
	case system.AzureModelProvider:
		return "azure", true
	case system.AzureEntraModelProvider:
		return "azure-entra", true
	default:
		return "", false
	}
}

func resolveOne(ctx context.Context, client kclient.Client, namespace, selection string) (*Model, error) {
	if alias, ok := strings.CutPrefix(selection, types.DefaultModelAliasRefPrefix); ok {
		var defaultAlias v1.DefaultModelAlias
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: alias}, &defaultAlias); err != nil {
			return nil, fmt.Errorf("resolve alias %q: %w", alias, err)
		}
		if defaultAlias.Spec.Manifest.Model == "" {
			return nil, fmt.Errorf("alias %q is not bound to a model", alias)
		}
		return lookup(ctx, client, namespace, defaultAlias.Spec.Manifest.Model)
	}
	return lookup(ctx, client, namespace, selection)
}

func lookup(ctx context.Context, client kclient.Client, namespace, name string) (*Model, error) {
	var model v1.Model
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: name}, &model); err != nil {
		return nil, fmt.Errorf("read model %q: %w", name, err)
	}
	target := strings.TrimSpace(model.Spec.Manifest.TargetModel)
	if target == "" {
		return nil, fmt.Errorf("model %q has no target model", name)
	}
	return &Model{
		ID:       model.Name,
		Model:    target,
		Provider: model.Spec.Manifest.ModelProvider,
		Dialect:  model.Spec.Manifest.Dialect,
	}, nil
}

func listUsable(ctx context.Context, client kclient.Client, namespace string) ([]Model, error) {
	var list v1.ModelList
	if err := client.List(ctx, &list, &kclient.ListOptions{Namespace: namespace}); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}

	result := make([]Model, 0, len(list.Items))
	for _, model := range list.Items {
		manifest := model.Spec.Manifest
		if !manifest.Active || manifest.Usage != types.ModelUsageLLM {
			continue
		}
		if strings.TrimSpace(manifest.TargetModel) == "" {
			continue
		}
		result = append(result, Model{
			ID:       model.Name,
			Model:    manifest.TargetModel,
			Provider: manifest.ModelProvider,
			Dialect:  manifest.Dialect,
		})
	}
	// Stable order: this feeds a sandbox's config, which feeds its revision, and
	// a walk that reordered itself would restart every sandbox for nothing.
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, nil
}

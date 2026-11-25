// Package mcptester contains the provider-neutral contracts and adapters used
// by the stateless MCP tester chat endpoint.
package mcptester

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	llmtypes "github.com/obot-platform/obot/pkg/llm"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ModelResolutionErrorMissing      ModelResolutionErrorKind = "missing"
	ModelResolutionErrorInactive     ModelResolutionErrorKind = "inactive"
	ModelResolutionErrorInaccessible ModelResolutionErrorKind = "inaccessible"
	ModelResolutionErrorInvalid      ModelResolutionErrorKind = "invalid"
)

type ResolvedModel struct {
	ID        string
	Target    string
	Provider  string
	Dialect   llmtypes.Dialect
	ProxyPath string
	APIPath   string
}

type ModelAccessResolver interface {
	UserHasAccessToModel(kuser.Info, string) (bool, error)
}

type ModelResolutionErrorKind string

type ModelResolutionError struct {
	Kind ModelResolutionErrorKind
	Err  error
}

func (e *ModelResolutionError) Error() string {
	return e.Err.Error()
}

func (e *ModelResolutionError) Unwrap() error {
	return e.Err
}

func IsModelResolutionError(err error, kind ModelResolutionErrorKind) bool {
	var resolutionErr *ModelResolutionError
	return errors.As(err, &resolutionErr) && resolutionErr.Kind == kind
}

func modelResolutionError(kind ModelResolutionErrorKind, format string, args ...any) error {
	return &ModelResolutionError{
		Kind: kind,
		Err:  fmt.Errorf(format, args...),
	}
}

func resolveDefaultLLMAlias(ctx context.Context, client kclient.Client) (*v1.DefaultModelAlias, error) {
	name := string(types.DefaultModelAliasTypeLLM)
	if object, err := alias.GetFromScope(ctx, client, "Model", system.DefaultNamespace, name); err == nil {
		if defaultAlias, ok := object.(*v1.DefaultModelAlias); ok {
			return defaultAlias, nil
		}
	}

	var defaultAlias v1.DefaultModelAlias
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      name,
	}, &defaultAlias); err != nil {
		return nil, err
	}
	return &defaultAlias, nil
}

// ResolveDefaultModel resolves only the configured llm alias. It intentionally
// has no modelProxy: a missing, unbound, inactive, non-LLM, inaccessible, or
// unsupported model makes tester Chat unavailable while leaving MCP inspection
// unaffected.
func ResolveDefaultModel(ctx context.Context, client kclient.Client, helper ModelAccessResolver, user kuser.Info) (ResolvedModel, error) {
	defaultAlias, err := resolveDefaultLLMAlias(ctx, client)
	if err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "read default llm alias: %w", err)
	}

	modelID := strings.TrimSpace(defaultAlias.Spec.Manifest.Model)
	if modelID == "" {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "default llm alias is not configured")
	}

	return resolveModel(ctx, client, helper, user, modelID, "default llm model")
}

// ResolveModel resolves a specific LLM model by name or alias and applies the
// same checks as ResolveDefaultModel: the model must be active, have LLM usage
// and a target model, be accessible to user, and be served by the LLM proxy.
func ResolveModel(ctx context.Context, client kclient.Client, helper ModelAccessResolver, user kuser.Info, modelID string) (ResolvedModel, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "llm model is required")
	}
	return resolveModel(ctx, client, helper, user, modelID, "llm model")
}

func resolveModel(ctx context.Context, client kclient.Client, helper ModelAccessResolver, user kuser.Info, modelID, label string) (ResolvedModel, error) {
	// Models are aliasable too, so the alias target may itself be a logical name.
	var model v1.Model
	if err := alias.Get(ctx, client, &model, system.DefaultNamespace, modelID); err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "read %s %q: %w", label, modelID, err)
	}

	manifest := model.Spec.Manifest
	if !manifest.Active {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInactive, "%s %q is not active", label, model.Name)
	}
	if manifest.Usage != types.ModelUsageLLM {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "%s %q does not have llm usage", label, model.Name)
	}
	if strings.TrimSpace(manifest.TargetModel) == "" {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "%s %q has no target model", label, model.Name)
	}

	allowed, err := helper.UserHasAccessToModel(user, model.Name)
	if err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInaccessible, "check access to %s %q: %w", label, model.Name, err)
	}
	if !allowed {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInaccessible, "user does not have access to %s %q", label, model.Name)
	}

	proxyPath, ok := proxyPath(manifest.ModelProvider)
	if !ok {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "%s provider %q has no LLM proxy route", label, manifest.ModelProvider)
	}

	dialect := defaultDialect(manifest)
	apiPath, err := apiPathForDialect(dialect)
	if err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "%v", err)
	}

	return ResolvedModel{
		ID:        model.Name,
		Target:    manifest.TargetModel,
		Provider:  manifest.ModelProvider,
		Dialect:   dialect,
		ProxyPath: proxyPath,
		APIPath:   apiPath,
	}, nil
}

func defaultDialect(manifest types.ModelManifest) llmtypes.Dialect {
	if manifest.Dialect != "" {
		return llmtypes.Dialect(manifest.Dialect)
	}
	return llmtypes.DialectOpenAIResponses
}

func apiPathForDialect(dialect llmtypes.Dialect) (string, error) {
	switch dialect {
	case llmtypes.DialectAnthropicMessages:
		return "/v1/messages", nil
	case llmtypes.DialectOpenAIResponses, llmtypes.DialectOpenResponses:
		return "/v1/responses", nil
	case llmtypes.DialectOpenAIChatCompletions:
		return "/v1/chat/completions", nil
	default:
		return "", fmt.Errorf("llm model uses unsupported dialect %q", dialect)
	}
}

// proxyPath returns the LLM proxy route for a model's provider, relative to /api/llm-proxy.
// Every model is served by the LLM gateway, whose route is the proxy's root: the API path
// (/v1/...) follows /api/llm-proxy directly.
func proxyPath(provider string) (string, bool) {
	if provider == system.LLMGatewayModelProvider {
		return "", true
	}
	return "", false
}

// Package mcptester contains the provider-neutral contracts and adapters used
// by the stateless MCP tester chat endpoint.
package mcptester

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/hostedagentmodels"
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

// ResolveDefaultModel resolves only the configured llm alias. It intentionally
// has no fallback: a missing, unbound, inactive, non-LLM, inaccessible, or
// unsupported model makes tester Chat unavailable while leaving MCP inspection
// unaffected.
func ResolveDefaultModel(ctx context.Context, client kclient.Client, helper ModelAccessResolver, user kuser.Info) (ResolvedModel, error) {
	var defaultAlias v1.DefaultModelAlias
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      string(types.DefaultModelAliasTypeLLM),
	}, &defaultAlias); err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "read default llm alias: %w", err)
	}

	modelID := strings.TrimSpace(defaultAlias.Spec.Manifest.Model)
	if modelID == "" {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "default llm alias is not configured")
	}

	var model v1.Model
	if err := client.Get(ctx, kclient.ObjectKey{
		Namespace: system.DefaultNamespace,
		Name:      modelID,
	}, &model); err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorMissing, "read default llm model %q: %w", modelID, err)
	}

	manifest := model.Spec.Manifest
	if !manifest.Active {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInactive, "default llm model %q is not active", model.Name)
	}
	if manifest.Usage != types.ModelUsageLLM {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "default llm model %q does not have llm usage", model.Name)
	}
	if strings.TrimSpace(manifest.TargetModel) == "" {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "default llm model %q has no target model", model.Name)
	}

	allowed, err := helper.UserHasAccessToModel(user, model.Name)
	if err != nil {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInaccessible, "check access to default llm model %q: %w", model.Name, err)
	}
	if !allowed {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInaccessible, "user does not have access to default llm model %q", model.Name)
	}

	proxyPath, ok := hostedagentmodels.ProxyPath(manifest.ModelProvider)
	if !ok {
		return ResolvedModel{}, modelResolutionError(ModelResolutionErrorInvalid, "default llm model provider %q has no LLM proxy route", manifest.ModelProvider)
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
	if manifest.ModelProvider == system.AnthropicModelProvider {
		return llmtypes.DialectAnthropicMessages
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
		return "", fmt.Errorf("default llm model uses unsupported dialect %q", dialect)
	}
}

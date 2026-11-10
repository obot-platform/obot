package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	types "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/alias"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/modelaccesspolicy"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// scopedListingFilter is called only for permitted targets in a successful list.
// Its snapshots belong to this response, so the next request sees alias and grant changes.
func (b *bifrostProviderBackend) scopedListingFilter(l *llmProviderProxy, req api.Context, scope string) func(string) (bool, error) {
	var (
		references, aliasKeys map[string]struct{}
		allowedModels         map[string]bool
		allowAll              bool
	)
	return func(native string) (bool, error) {
		if references == nil {
			var models v1.ModelList
			if err := req.Storage.List(req.Context(), &models, kclient.InNamespace(system.DefaultNamespace)); err != nil {
				return false, fmt.Errorf("listing model references: %w", err)
			}
			var aliases v1.AliasList
			if err := req.Storage.List(req.Context(), &aliases); err != nil {
				return false, fmt.Errorf("listing model aliases: %w", err)
			}
			var err error
			allowedModels, allowAll, err = l.mapHelper.GetUserAllowedModels(req.User)
			if err != nil {
				return false, err
			}
			references = make(map[string]struct{}, len(models.Items))
			for _, model := range models.Items {
				// Inactive and legacy resources can still intercept a saved reference.
				references[model.Name] = struct{}{}
				if model.Spec.Manifest.ModelProvider == system.LLMGatewayModelProvider {
					// Older gateway records can have unqualified exact targets.
					references[model.Spec.Manifest.TargetModel] = struct{}{}
				}
			}
			aliasKeys = make(map[string]struct{}, len(aliases.Items))
			for _, a := range aliases.Items {
				aliasKeys[a.Name] = struct{}{}
			}
		}

		mapped, err := b.engine.MigratedReference(req.Context(), native)
		if err != nil {
			return false, err
		}
		if mapped == "" {
			_, named := references[native]
			_, aliased := aliasKeys[alias.KeyFromScopeID("Model", native)]
			// Target filtering already proves a granted exact scoped match exists.
			// Slash names take the native-first path before aliases or exact targets.
			if strings.Contains(native, "/") || (!named && !aliased) {
				return true, nil
			}
		}

		// Keep inference authoritative for saved aliases, resources and mappings.
		model, err := b.resolveRequestModel(l, req, native)
		if err != nil {
			// Known unusable references are omitted; storage/helper failures fail the list.
			_, unavailable := errors.AsType[*modelaccesspolicy.ModelReferenceUnavailableError](err)
			if apierrors.IsNotFound(err) || unavailable {
				return false, nil
			}
			if e, ok := errors.AsType[*types.ErrHTTP](err); ok && (e.Code == http.StatusBadRequest || e.Code == http.StatusNotFound || e.Code == http.StatusGone) {
				return false, nil
			}
			return false, err
		}
		// Duplicate targets can have different grants; check the record actually selected.
		return model != nil && model.Spec.Manifest.ModelProvider == system.LLMGatewayModelProvider &&
			model.Spec.Manifest.TargetModel == scope+"/"+native && (allowAll || allowedModels[model.Name]), nil
	}
}

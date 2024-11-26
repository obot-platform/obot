package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	v1 "github.com/otto8-ai/otto8/pkg/storage/apis/otto.otto8.ai/v1"
	"github.com/otto8-ai/otto8/pkg/storage/selectors"
	"github.com/otto8-ai/otto8/pkg/system"
	"github.com/otto8-ai/otto8/pkg/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ModelHandler struct{}

func NewModelHandler() *ModelHandler {
	return &ModelHandler{}
}

func (a *ModelHandler) List(req api.Context) error {
	var modelList v1.ModelList
	if err := req.List(&modelList); err != nil {
		return err
	}

	respList := make([]types.Model, 0, len(modelList.Items))
	for _, model := range modelList.Items {
		resp, err := convertModel(req.Context(), req.Storage, model)
		if err != nil {
			return err
		}

		respList = append(respList, resp)
	}

	return req.Write(types.ModelList{Items: respList})
}

func (a *ModelHandler) ByID(req api.Context) error {
	var model v1.Model
	if err := req.Get(&model, req.PathValue("id")); err != nil {
		return err
	}

	resp, err := convertModel(req.Context(), req.Storage, model)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Update(req api.Context) error {
	var model types.ModelManifest
	if err := req.Read(&model); err != nil {
		return err
	}

	var existing v1.Model
	if err := req.Get(&existing, req.PathValue("id")); err != nil {
		return err
	}

	existing.Spec.Manifest = model

	if err := validateModelManifestAndSetDefaults(&existing); err != nil {
		return err
	}

	if err := req.Update(&existing); err != nil {
		return err
	}

	processedModel, err := wait.For(req.Context(), req.Storage, &existing, func(model *v1.Model) bool {
		return model.Generation == model.Status.AliasObservedGeneration
	})
	if err != nil {
		return fmt.Errorf("failed to update model: %w", err)
	}

	resp, err := convertModel(req.Context(), req.Storage, *processedModel)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Create(req api.Context) error {
	var modelManifest types.ModelManifest
	if err := req.Read(&modelManifest); err != nil {
		return err
	}

	if modelManifest.ModelProvider == "" {
		return types.NewErrBadRequest("model provider is required")
	}

	var toolRef v1.ToolReference
	if err := req.Get(&toolRef, modelManifest.ModelProvider); err != nil {
		return err
	}

	if toolRef.Spec.Type != types.ToolReferenceTypeModelProvider {
		return types.NewErrBadRequest("model provider %s must be of type %s not %s", modelManifest.ModelProvider, types.ToolReferenceTypeModelProvider, toolRef.Spec.Type)
	}

	model := &v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ModelPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ModelSpec{
			Manifest: modelManifest,
		},
	}

	if err := validateModelManifestAndSetDefaults(model); err != nil {
		return err
	}

	model, err := wait.For(req.Context(), req.Storage, model, func(model *v1.Model) bool {
		return model.Generation == model.Status.AliasObservedGeneration
	}, wait.Option{Create: true})
	if err != nil {
		return fmt.Errorf("failed to create model: %w", err)
	}

	resp, err := convertModel(req.Context(), req.Storage, *model)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Delete(req api.Context) error {
	model := req.PathValue("id")
	var agents v1.AgentList
	if err := req.Storage.List(req.Context(), &agents, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.manifest.model": model,
		})),
		Namespace: req.Namespace(),
	}); err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	if len(agents.Items) > 0 {
		return types.NewErrHttp(http.StatusPreconditionFailed, fmt.Sprintf("model %q is used by %d agents", model, len(agents.Items)))
	}

	return req.Delete(&v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func convertModel(ctx context.Context, c kclient.Client, model v1.Model) (types.Model, error) {
	var toolRef v1.ToolReference
	if err := c.Get(ctx, kclient.ObjectKey{Namespace: model.Namespace, Name: model.Spec.Manifest.ModelProvider}, &toolRef); err != nil {
		return types.Model{}, err
	}

	return types.Model{
		Metadata:      MetadataFrom(&model),
		ModelManifest: model.Spec.Manifest,
		ModelStatus: types.ModelStatus{
			ModelProviderStatus: *convertModelProviderToolRef(toolRef),
			AliasAssigned:       model.Status.AliasAssigned,
		},
	}, nil
}

func convertModelProviderToolRef(toolRef v1.ToolReference) *types.ModelProviderStatus {
	var missingEnvVars []string
	if toolRef.Status.Tool != nil && toolRef.Status.Tool.Metadata["envVars"] != "" {
		for _, envVar := range strings.Split(toolRef.Status.Tool.Metadata["envVars"], ",") {
			if os.Getenv(envVar) == "" {
				missingEnvVars = append(missingEnvVars, envVar)
			}
		}
	}

	return &types.ModelProviderStatus{
		Configured:     toolRef.Status.Tool != nil && len(missingEnvVars) == 0,
		MissingEnvVars: missingEnvVars,
	}
}

func validateModelManifestAndSetDefaults(newModel *v1.Model) error {
	var errs []error
	if newModel.Spec.Manifest.TargetModel == "" {
		errs = append(errs, fmt.Errorf("field targetModel is required"))
	}
	if newModel.Spec.Manifest.ModelProvider == "" {
		errs = append(errs, fmt.Errorf("field modelProvider is required"))
	}

	if newModel.Spec.Manifest.Usage == "" {
		newModel.Spec.Manifest.Usage = types.ModelUsageLLM
	}

	return errors.Join(errs...)
}

package handlers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/system"
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

	var toolRefList v1.ToolReferenceList
	if err := req.Storage.List(req.Context(), &toolRefList, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.type": string(types.ToolReferenceTypeModelProvider),
		}),
		Namespace: req.Namespace(),
	}); err != nil {
		return err
	}

	toolRefMap := make(map[string]v1.ToolReference)
	for _, toolRef := range toolRefList.Items {
		toolRefMap[toolRef.Name] = toolRef
	}

	respList := make([]types.Model, 0, len(modelList.Items))
	for _, model := range modelList.Items {
		toolRef, ok := toolRefMap[model.Spec.Manifest.ModelProvider]
		if !ok {
			return types.NewErrNotFound("tool reference %s not found", model.Spec.Manifest.ModelProvider)
		}

		resp, err := convertModel(model, toolRef)
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

	var toolRef v1.ToolReference
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: model.Namespace, Name: model.Spec.Manifest.ModelProvider}, &toolRef); err != nil {
		return err
	}

	resp, err := convertModel(model, toolRef)
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

	var toolRef v1.ToolReference
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: existing.Namespace, Name: existing.Spec.Manifest.ModelProvider}, &toolRef); err != nil {
		return err
	}

	resp, err := convertModel(existing, toolRef)
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

	if err := req.Create(model); err != nil {
		return err
	}

	resp, err := convertModel(*model, toolRef)
	if err != nil {
		return err
	}

	return req.Write(resp)
}

func (a *ModelHandler) Delete(req api.Context) error {
	model := req.PathValue("id")
	var agents v1.AgentList
	if err := req.List(&agents, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.manifest.model": model,
		})),
	}); err != nil {
		return fmt.Errorf("failed to list agents: %w", err)
	}

	if len(agents.Items) > 0 {
		return types.NewErrHTTP(http.StatusPreconditionFailed, fmt.Sprintf("model %q is used by %d agents", model, len(agents.Items)))
	}

	return req.Delete(&v1.Model{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("id"),
			Namespace: req.Namespace(),
		},
	})
}

func convertModel(model v1.Model, toolRef v1.ToolReference) (types.Model, error) {
	var (
		aliasAssigned *bool
		toolName      string
	)
	if model.Generation == model.Status.ObservedGeneration {
		aliasAssigned = &model.Status.AliasAssigned
	}
	if toolRef.Status.Tool != nil {
		toolName = toolRef.Status.Tool.Name
	}

	return types.Model{
		Metadata:      MetadataFrom(&model),
		ModelManifest: model.Spec.Manifest,
		ModelStatus: types.ModelStatus{
			AliasAssigned:     aliasAssigned,
			ModelProviderName: toolName,
		},
	}, nil
}

func validateModelManifestAndSetDefaults(newModel *v1.Model) error {
	var errs []error
	if newModel.Spec.Manifest.TargetModel == "" {
		errs = append(errs, fmt.Errorf("field targetModel is required"))
	}
	if newModel.Spec.Manifest.ModelProvider == "" {
		errs = append(errs, fmt.Errorf("field modelProvider is required"))
	}

	return errors.Join(errs...)
}

package model

import (
	"errors"
	"log/slog"

	"github.com/obot-platform/nah/pkg/apply"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type Handler struct {
	gatewayClient *client.Client
}

func NewHandler(gatewayClient *client.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (*Handler) RemoveApplyUpdateAnnotation(req router.Request, _ router.Response) error {
	model := req.Object.(*v1.Model)
	if _, ok := model.Annotations[apply.AnnotationUpdate]; !ok {
		return nil
	}

	delete(model.Annotations, apply.AnnotationUpdate)
	return req.Client.Update(req.Ctx, model)
}

func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	model := req.Object.(*v1.Model)

	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, model.Namespace, model.Spec.Manifest.ModelProvider); apierrors.IsNotFound(err) {
		slog.Info("Deleting model because model provider has been deleted", "model", model.Name, "modelProvider", model.Spec.Manifest.ModelProvider)
		return req.Delete(model)
	} else if err != nil {
		return err
	}

	// If the credential is not found then the model provider has been deconfigured.
	_, err := h.gatewayClient.RevealCredential(req.Ctx, []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name)
	if _, ok := errors.AsType[client.CredentialNotFoundError](err); ok {
		slog.Info("Deleting model because model provider has been deconfigured", "model", model.Name, "modelProvider", model.Spec.Manifest.ModelProvider)
		return req.Delete(model)
	}
	return err
}

// EnsureModelInfo projects ModelInfo onto the respective Model's status.
func (*Handler) EnsureModelInfo(req router.Request, _ router.Response) error {
	model := req.Object.(*v1.Model)
	if model.Spec.Manifest.ModelProvider == "" || model.Spec.Manifest.TargetModel == "" {
		return nil
	}

	var (
		infoName  = v1.ModelInfoName(model.Spec.Manifest.ModelProvider, model.Spec.Manifest.TargetModel)
		modelInfo v1.ModelInfo
	)
	if err := kclient.IgnoreNotFound(req.Get(&modelInfo, model.Namespace, infoName)); err != nil {
		return err
	}

	model.Status.Cost = modelInfo.Spec.Cost

	return nil
}

package model

import (
	"errors"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var log = logger.Package()

type Handler struct {
	gatewayClient *client.Client
}

func NewHandler(gatewayClient *client.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) Cleanup(req router.Request, _ router.Response) error {
	model := req.Object.(*v1.Model)

	var modelProvider v1.ModelProvider
	if err := req.Get(&modelProvider, model.Namespace, model.Spec.Manifest.ModelProvider); apierrors.IsNotFound(err) {
		log.Infof("Deleting model %s because model provider %s has been deleted", model.Name, model.Spec.Manifest.ModelProvider)
		return req.Delete(model)
	} else if err != nil {
		return err
	}

	// If the credential is not found then the model provider has been deconfigured.
	_, err := h.gatewayClient.RevealCredential(req.Ctx, []string{modelProvider.Name, system.GenericModelProviderCredentialContext}, modelProvider.Name)
	if _, ok := errors.AsType[client.CredentialNotFoundError](err); ok {
		log.Infof("Deleting model %s because model provider %s has been deconfigured", model.Name, model.Spec.Manifest.ModelProvider)
		return req.Delete(model)
	}
	return err
}

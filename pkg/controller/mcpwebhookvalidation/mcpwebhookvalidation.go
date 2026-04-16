package mcpwebhookvalidation

import (
	"errors"
	"fmt"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Handler struct {
	gptClient        *gptscript.GPTScript
	webhookBaseImage string
}

func New(gptClient *gptscript.GPTScript, webhookBaseImage string) *Handler {
	return &Handler{
		gptClient:        gptClient,
		webhookBaseImage: webhookBaseImage,
	}
}

func (h *Handler) CleanupResources(req router.Request, _ router.Response) error {
	webhookValidation := req.Object.(*v1.MCPWebhookValidation)
	newResources := make([]types.Resource, 0, len(webhookValidation.Spec.Manifest.Resources))

	var (
		mcpServer    v1.MCPServer
		catalogEntry v1.MCPServerCatalogEntry
		mcpCatalog   v1.MCPCatalog
		err          error
	)
	for _, resource := range webhookValidation.Spec.Manifest.Resources {
		switch resource.Type {
		case types.ResourceTypeSelector:
			newResources = append(newResources, resource)
		case types.ResourceTypeMCPServer:
			if err = req.Get(&mcpServer, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp server %s: %w", resource.ID, err)
			}
		case types.ResourceTypeMCPServerCatalogEntry:
			if err = req.Get(&catalogEntry, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp server catalog entry %s: %w", resource.ID, err)
			}
		case types.ResourceTypeMcpCatalog:
			if err = req.Get(&mcpCatalog, req.Namespace, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp catalog %s: %w", resource.ID, err)
			}
		}
	}

	if len(newResources) != len(webhookValidation.Spec.Manifest.Resources) {
		webhookValidation.Spec.Manifest.Resources = newResources
		return req.Client.Update(req.Ctx, webhookValidation)
	}

	return nil
}

func (h *Handler) EnsureSystemServer(req router.Request, _ router.Response) error {
	webhookValidation := req.Object.(*v1.MCPWebhookValidation)

	secret, err := h.getWebhookSecret(req, webhookValidation.Name)
	if err != nil {
		return err
	}

	desired := desiredSystemServer(webhookValidation, h.webhookBaseImage)

	webhookMCPCredential, err := h.gptClient.RevealCredential(req.Ctx, []string{desired.Name}, desired.Name)
	if errors.As(err, &gptscript.ErrNotFound{}) || err == nil && webhookMCPCredential.Env["WEBHOOK_SECRET"] != secret {
		if err := h.gptClient.CreateCredential(req.Ctx, gptscript.Credential{
			Context:  desired.Name,
			ToolName: desired.Name,
			Type:     gptscript.CredentialTypeTool,
			Env: map[string]string{
				"WEBHOOK_SECRET": secret,
			},
		}); err != nil {
			return fmt.Errorf("failed to create credential for webhook validation server %s: %w", webhookValidation.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get credential for webhook validation server %s: %w", webhookValidation.Name, err)
	}

	var existing v1.SystemMCPServer
	if err = req.Get(&existing, webhookValidation.Namespace, desired.Name); apierrors.IsNotFound(err) {
		return req.Client.Create(req.Ctx, desired)
	} else if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		return nil
	}

	existing.Spec = desired.Spec
	return req.Client.Update(req.Ctx, &existing)
}

func desiredSystemServer(webhookValidation *v1.MCPWebhookValidation, image string) *v1.SystemMCPServer {
	displayName := webhookValidation.Spec.Manifest.Name
	if displayName == "" {
		displayName = webhookValidation.Name
	}

	return &v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:       system.SystemMCPServerPrefix + webhookValidation.Name,
			Namespace:  webhookValidation.Namespace,
			Finalizers: []string{v1.SystemMCPServerFinalizer},
		},
		Spec: v1.SystemMCPServerSpec{
			WebhookValidationName: webhookValidation.Name,
			Manifest: types.SystemMCPServerManifest{
				Name:             displayName,
				ShortDescription: "Managed webhook validation server",
				Enabled:          !webhookValidation.Spec.Manifest.Disabled,
				Runtime:          types.RuntimeContainerized,
				ContainerizedConfig: &types.ContainerizedRuntimeConfig{
					Image: image,
					Port:  8099,
					Path:  "/mcp",
				},
				Env: []types.MCPEnv{
					{
						MCPHeader: types.MCPHeader{Key: "WEBHOOK_URL", Value: webhookValidation.Spec.Manifest.URL},
					},
					{
						MCPHeader: types.MCPHeader{Key: "WEBHOOK_SECRET", Sensitive: true},
					},
					{
						MCPHeader: types.MCPHeader{Key: "PORT", Value: "8099"},
					},
				},
			},
		},
	}
}

func (h *Handler) getWebhookSecret(req router.Request, name string) (string, error) {
	cred, err := h.gptClient.RevealCredential(req.Ctx, []string{system.MCPWebhookValidationCredentialContext}, name)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return "", nil
		}
		return "", err
	}
	return cred.Env["secret"], nil
}

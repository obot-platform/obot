package mcpwebhookvalidation

import (
	"errors"
	"fmt"
	"maps"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
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

	cred, err := h.getWebhookCredential(req, webhookValidation.Name)
	if err != nil {
		return err
	}

	desired := desiredSystemServer(webhookValidation, h.webhookBaseImage)

	webhookMCPCredential, err := h.gptClient.RevealCredential(req.Ctx, []string{desired.Name}, desired.Name)
	if errors.As(err, &gptscript.ErrNotFound{}) || err == nil && !maps.Equal(cred, webhookMCPCredential.Env) {
		if err := h.gptClient.CreateCredential(req.Ctx, gptscript.Credential{
			Context:  desired.Name,
			ToolName: desired.Name,
			Type:     gptscript.CredentialTypeTool,
			Env:      cred,
		}); err != nil {
			return fmt.Errorf("failed to create credential for webhook validation server %s: %w", webhookValidation.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get credential for webhook validation server %s: %w", webhookValidation.Name, err)
	}

	webhookValidation.Status.Configured = systemmcpserver.IsSystemServerConfigured(req.Ctx, h.gptClient, desired)

	var existing v1.SystemMCPServer
	if err = req.Get(&existing, webhookValidation.Namespace, desired.Name); apierrors.IsNotFound(err) {
		return req.Client.Create(req.Ctx, &desired)
	} else if err != nil {
		return err
	}

	if equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		return nil
	}

	existing.Spec = desired.Spec
	return req.Client.Update(req.Ctx, &existing)
}

func desiredSystemServer(webhookValidation *v1.MCPWebhookValidation, image string) v1.SystemMCPServer {
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
		}
	}

	manifest.Name = webhookValidation.Spec.Manifest.Name
	if manifest.Name == "" {
		manifest.Name = webhookValidation.Spec.Manifest.SystemMCPServerManifest.Name
		if manifest.Name == "" {
			manifest.Name = webhookValidation.Name
		}
	}

	return v1.SystemMCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:       system.SystemMCPServerPrefix + webhookValidation.Name,
			Namespace:  webhookValidation.Namespace,
			Finalizers: []string{v1.SystemMCPServerFinalizer},
		},
		Spec: v1.SystemMCPServerSpec{
			WebhookValidationName: webhookValidation.Name,
			Manifest:              manifest,
		},
	}
}

func (h *Handler) getWebhookCredential(req router.Request, name string) (map[string]string, error) {
	cred, err := h.gptClient.RevealCredential(req.Ctx, []string{system.MCPWebhookValidationCredentialContext}, name)
	if err != nil {
		if errors.As(err, &gptscript.ErrNotFound{}) {
			return nil, nil
		}
		return nil, err
	}

	if s := cred.Env["secret"]; s != "" {
		delete(cred.Env, "secret")
		cred.Env["WEBHOOK_SECRET"] = s
	}
	return cred.Env, nil
}

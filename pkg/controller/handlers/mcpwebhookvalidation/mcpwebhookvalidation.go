package mcpwebhookvalidation

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/controller/handlers/systemmcpserver"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/guardrails"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// guardrailStatusInterval is how often a guardrail filter's status is refreshed, since
	// guardrails are stored outside the resources that trigger this controller.
	guardrailStatusInterval = time.Minute
)

// guardrailStore looks up guardrails. *gateway.Client implements it.
type guardrailStore interface {
	GetGuardrail(ctx context.Context, id string) (*gatewaytypes.Guardrail, error)
	GuardrailSecrets(ctx context.Context, id string) (map[string]string, error)
}

type Handler struct {
	gatewayClient    *gateway.Client
	guardrails       guardrailStore
	webhookBaseImage string
}

func New(gatewayClient *gateway.Client, webhookBaseImage string) *Handler {
	return &Handler{
		gatewayClient:    gatewayClient,
		guardrails:       gatewayClient,
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
			if system.IsVMCPID(resource.ID) {
				err = getConfirmingNotFound(req, &v1.VMCP{}, resource.ID)
			} else {
				err = getConfirmingNotFound(req, &mcpServer, resource.ID)
			}
			if err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp server %s: %w", resource.ID, err)
			}
		case types.ResourceTypeMCPServerCatalogEntry:
			if err = getConfirmingNotFound(req, &catalogEntry, resource.ID); err == nil {
				newResources = append(newResources, resource)
			} else if !apierrors.IsNotFound(err) {
				return fmt.Errorf("failed to get mcp server catalog entry %s: %w", resource.ID, err)
			}
		case types.ResourceTypeMcpCatalog:
			if err = getConfirmingNotFound(req, &mcpCatalog, resource.ID); err == nil {
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

// getConfirmingNotFound gets a resource from the cache, and confirms with storage when the
// cache doesn't have it. A server, catalog entry or catalog created moments before the filter
// that targets it may not have reached the cache yet, and must not be pruned from the filter
// as if it were deleted: the filter would silently stop applying to it.
func getConfirmingNotFound(req router.Request, obj kclient.Object, name string) error {
	err := req.Get(obj, req.Namespace, name)
	if apierrors.IsNotFound(err) {
		err = req.Get(untriggered.UncachedGet(obj), req.Namespace, name)
	}
	return err
}

func (h *Handler) EnsureSystemServer(req router.Request, resp router.Response) error {
	webhookValidation := req.Object.(*v1.MCPWebhookValidation)
	if webhookValidation.Spec.Manifest.Guardrail != nil {
		return h.ensureGuardrailFilter(req, resp, webhookValidation)
	}

	cred, err := h.getWebhookCredential(req, webhookValidation.Name)
	if err != nil {
		return err
	}

	desired := desiredSystemServer(webhookValidation, h.webhookBaseImage)

	webhookMCPCredential, err := h.gatewayClient.RevealCredential(req.Ctx, []string{desired.Name}, desired.Name)
	if errors.As(err, &gateway.CredentialNotFoundError{}) || err == nil && !maps.Equal(cred, webhookMCPCredential.Secrets) {
		if err := h.gatewayClient.UpsertCredential(req.Ctx, gatewaytypes.Credential{
			Context: desired.Name,
			Name:    desired.Name,
			Secrets: cred,
		}); err != nil {
			return fmt.Errorf("failed to create credential for webhook validation server %s: %w", webhookValidation.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get credential for webhook validation server %s: %w", webhookValidation.Name, err)
	}

	webhookValidation.Status.Configured = systemmcpserver.IsSystemServerConfigured(req.Ctx, h.gatewayClient, desired)

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
			Config: []types.MCPConfig{{
				Key:   "WEBHOOK_URL",
				Value: webhookValidation.Spec.Manifest.URL,
				Usage: types.Env,
			},
				{
					Key:       "WEBHOOK_SECRET",
					Sensitive: true,
					Usage:     types.Env,
				},
				{
					Key:   "PORT",
					Value: "8099",
					Usage: types.Env,
				}},
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
		Name:       system.SystemMCPServerPrefix + webhookValidation.Name,
		Namespace:  webhookValidation.Namespace,
		Finalizers: []string{v1.SystemMCPServerFinalizer},
		Spec: v1.SystemMCPServerSpec{
			WebhookValidationName: webhookValidation.Name,
			Manifest:              manifest,
		},
	}
}

func (h *Handler) getWebhookCredential(req router.Request, name string) (map[string]string, error) {
	cred, err := h.gatewayClient.RevealCredential(req.Ctx, []string{system.MCPWebhookValidationCredentialContext}, name)
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil, nil
		}
		return nil, err
	}

	if s := cred.Secrets["secret"]; s != "" {
		delete(cred.Secrets, "secret")
		cred.Secrets["WEBHOOK_SECRET"] = s
	}
	return cred.Secrets, nil
}

// ensureGuardrailFilter keeps a guardrail filter's status current. A guardrail filter runs
// in-process, so it has no system MCP server; one left from before the filter became a
// guardrail filter is removed.
func (h *Handler) ensureGuardrailFilter(req router.Request, resp router.Response, webhookValidation *v1.MCPWebhookValidation) error {
	var existing v1.SystemMCPServer
	if err := req.Get(&existing, webhookValidation.Namespace, system.SystemMCPServerPrefix+webhookValidation.Name); err == nil {
		if err := req.Client.Delete(req.Ctx, &existing); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete the system MCP server of guardrail filter %s: %w", webhookValidation.Name, err)
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	configured, err := h.guardrailsUsable(req.Ctx, webhookValidation.Spec.Manifest.Guardrail.GuardrailIDs)
	if err != nil {
		return err
	}
	webhookValidation.Status.Configured = configured
	resp.RetryAfter(guardrailStatusInterval)
	return nil
}

// guardrailsUsable reports whether a guardrail filter has guardrails and each exists
// and can be used.
func (h *Handler) guardrailsUsable(ctx context.Context, ids []string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	for _, id := range ids {
		guardrail, err := h.guardrails.GetGuardrail(ctx, id)
		if gateway.ErrGuardrailNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, fmt.Errorf("failed to get guardrail %s: %w", id, err)
		}
		secrets, err := h.guardrails.GuardrailSecrets(ctx, id)
		if err != nil {
			return false, fmt.Errorf("failed to get guardrail %s settings: %w", id, err)
		}
		if guardrails.GuardrailError(*guardrail, secrets) != "" {
			return false, nil
		}
	}
	return true, nil
}

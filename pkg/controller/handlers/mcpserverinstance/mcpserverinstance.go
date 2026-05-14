package mcpserverinstance

import (
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var log = logger.Package()

type Handler struct {
	gatewayClient *gateway.Client
}

func New(gatewayClient *gateway.Client) *Handler {
	return &Handler{
		gatewayClient: gatewayClient,
	}
}

func (h *Handler) MigrationDeleteSingleUserInstances(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	var server v1.MCPServer
	if err := req.Get(&server, req.Namespace, instance.Spec.MCPServerName); err != nil {
		return err
	}

	if server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == "" {
		// This server is unshared (neither catalog-shared nor workspace-scoped), so it should not have any server instances.
		// Delete this instance.
		log.Infof("Deleting invalid single-user MCPServerInstance for unshared server: instance=%s server=%s", instance.Name, server.Name)
		return req.Delete(instance)
	}

	return nil
}

func (h *Handler) UpdateMultiUserConfig(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)

	var server v1.MCPServer
	if err := req.Get(&server, req.Namespace, instance.Spec.MCPServerName); apierrors.IsNotFound(err) {
		// The server no longer exists, another controller will delete this instance, so do nothing.
		return nil
	} else if err != nil {
		return err
	}

	if server.Spec.MCPCatalogID != "" || server.Spec.PowerUserWorkspaceID != "" {
		if !equality.Semantic.DeepEqual(instance.Spec.MultiUserConfig, server.Spec.Manifest.MultiUserConfig) {
			instance.Spec.MultiUserConfig = server.Spec.Manifest.MultiUserConfig
			return req.Client.Update(req.Ctx, instance)
		}
	}

	return nil
}

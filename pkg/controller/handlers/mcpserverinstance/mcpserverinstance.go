package mcpserverinstance

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/controller/handlers/mcpservercatalogentry"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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

	if server.Spec.IsSingleUser() {
		// This server is single-user, so it should not have any server instances.
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

	if !server.Spec.IsSingleUser() {
		if !equality.Semantic.DeepEqual(instance.Spec.MultiUserConfig, server.Spec.Manifest.MultiUserConfig) {
			instance.Spec.MultiUserConfig = server.Spec.Manifest.MultiUserConfig
			return req.Client.Update(req.Ctx, instance)
		}
	}

	return nil
}

// EnsureUserCounts refreshes denormalized user counts affected by an MCPServerInstance change.
func (h *Handler) EnsureUserCounts(req router.Request, _ router.Response) error {
	instance := req.Object.(*v1.MCPServerInstance)
	if instance.Spec.MCPServerName == "" {
		return nil
	}

	var server v1.MCPServer
	if err := req.Get(&server, req.Namespace, instance.Spec.MCPServerName); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	if err := updateMCPServerInstanceUserCount(req, &server); err != nil {
		return err
	}

	if server.Spec.MCPServerCatalogEntryName == "" || server.Spec.CompositeName != "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, req.Namespace, server.Spec.MCPServerCatalogEntryName); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	userCount, err := mcpservercatalogentry.UserCountForEntry(req, entry)
	if err != nil {
		return err
	}

	return mcpservercatalogentry.UpdateUserCount(req, &entry, userCount)
}

func updateMCPServerInstanceUserCount(req router.Request, server *v1.MCPServer) error {
	if server.Spec.IsSingleUser() {
		if server.Status.MCPServerInstanceUserCount == nil {
			return nil
		}

		server.Status.MCPServerInstanceUserCount = nil
		return req.Client.Status().Update(req.Ctx, server)
	}

	userCount, err := mcpServerInstanceUserCount(req, server.Name)
	if err != nil {
		return err
	}

	if oldUserCount := server.Status.MCPServerInstanceUserCount; oldUserCount == nil || *oldUserCount != userCount {
		log.Infof("Updated MCP server instance user count: server=%s newCount=%d", server.Name, userCount)
		server.Status.MCPServerInstanceUserCount = &userCount
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func mcpServerInstanceUserCount(req router.Request, serverName string) (int, error) {
	var mcpServerInstances v1.MCPServerInstanceList
	if err := req.List(&mcpServerInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", serverName),
		Namespace:     system.DefaultNamespace,
	}); err != nil {
		return 0, fmt.Errorf("failed to list MCP server instances: %w", err)
	}

	uniqueUsers := make(map[string]struct{}, len(mcpServerInstances.Items))
	for _, instance := range mcpServerInstances.Items {
		if instance.DeletionTimestamp.IsZero() && instance.Spec.UserID != "" {
			uniqueUsers[instance.Spec.UserID] = struct{}{}
		}
	}

	return len(uniqueUsers), nil
}

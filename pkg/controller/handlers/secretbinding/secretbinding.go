package secretbinding

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// Handler records which secret bindings on each MCPServer cannot be resolved, so
// API handlers can report missing configuration without reading Secrets.
type Handler struct {
	storageClient kclient.Client
	// secretClient reads Secrets from the local router's cache. It is nil when MCP
	// servers do not run on Kubernetes, in which case no binding resolves.
	secretClient  kclient.Client
	obotNamespace string
	allowedLabel  string
}

// New returns a Handler. Both clients should be cached: the Secret handler lists every
// MCPServer through storageClient for each Secret event, and secretClient serves every
// binding lookup, so uncached clients would send that traffic to the API servers.
func New(storageClient, secretClient kclient.Client, obotNamespace, allowedLabel string) *Handler {
	return &Handler{
		storageClient: storageClient,
		secretClient:  secretClient,
		obotNamespace: obotNamespace,
		allowedLabel:  allowedLabel,
	}
}

// SyncMCPServerStatus updates an MCPServer's unresolved secret bindings when it changes.
func (h *Handler) SyncMCPServerStatus(req router.Request, _ router.Response) error {
	return h.syncMCPServer(req.Ctx, req.Client, req.Object.(*v1.MCPServer))
}

// SyncMCPServersForSecret updates the unresolved secret bindings of every MCPServer
// bound to a Secret in the obot namespace when that Secret changes or is deleted.
func (h *Handler) SyncMCPServersForSecret(req router.Request, _ router.Response) error {
	var servers v1.MCPServerList
	if err := h.storageClient.List(req.Ctx, &servers, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	for i := range servers.Items {
		server := &servers.Items[i]
		if !mcp.ReferencesSecret(server.Spec.Manifest.Config, req.Name) {
			continue
		}
		if err := h.syncMCPServer(req.Ctx, h.storageClient, server); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) syncMCPServer(ctx context.Context, client kclient.Client, server *v1.MCPServer) error {
	unresolved, err := mcp.UnresolvedSecretBindingKeys(ctx, h.secretClient, h.obotNamespace, server.Spec.Manifest.Config, h.allowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings for MCP server %s: %w", server.Name, err)
	}
	hash := mcp.SecretBindingsCheckHash(server.Spec.Manifest.Config, h.allowedLabel)
	if slices.Equal(server.Status.UnresolvedSecretBindings, unresolved) && server.Status.SecretBindingsCheckHash == hash {
		return nil
	}

	server.Status.UnresolvedSecretBindings = unresolved
	server.Status.SecretBindingsCheckHash = hash
	return client.Status().Update(ctx, server)
}

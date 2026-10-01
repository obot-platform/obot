package mcpservercatalogentry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type staticConfigurationCredentialClient interface {
	ListCredentials(ctx context.Context, opts gclient.ListCredentialsOptions) ([]gatewaytypes.Credential, error)
	DeleteCredential(ctx context.Context, credentialContext, name string) (bool, error)
}

// RemoveCredentials removes the credentials owned by a catalog entry when it is deleted.
func (h *Handler) RemoveCredentials(req router.Request, _ router.Response) error {
	if err := removeOAuthCredentials(req, h.gatewayClient); err != nil {
		return err
	}
	return removeStaticConfigurationCredentials(req, h.gatewayClient)
}

// removeStaticConfigurationCredentials deletes the revisions of a deleted entry's static
// configuration. A revision referenced by an MCPServer or vMCP snapshot is kept, because those
// resolve their static values without depending on the entry.
func removeStaticConfigurationCredentials(req router.Request, creds staticConfigurationCredentialClient) error {
	entry := req.Object.(*v1.MCPServerCatalogEntry)
	credentialContext := mcp.StaticConfigurationCredentialContext(entry.Name)

	credentials, err := creds.ListCredentials(req.Ctx, gclient.ListCredentialsOptions{CredentialContexts: []string{credentialContext}})
	if err != nil {
		return fmt.Errorf("failed to list static configuration credentials: %w", err)
	}
	if len(credentials) == 0 {
		return nil
	}

	referenced, err := referencedStaticConfigurationRevisions(req, entry)
	if err != nil {
		return err
	}

	for _, credential := range credentials {
		revision, ok := mcp.StaticConfigurationRevisionFromCredentialName(credential.Name)
		if !ok {
			continue
		}
		if _, ok := referenced[revision]; ok {
			continue
		}
		if _, err := creds.DeleteCredential(req.Ctx, credentialContext, credential.Name); err != nil {
			return fmt.Errorf("failed to delete static configuration credential: %w", err)
		}
		slog.Info("Removed static configuration credential for deleted MCP catalog entry", "entry", entry.Name, "revision", revision)
	}
	return nil
}

func referencedStaticConfigurationRevisions(req router.Request, entry *v1.MCPServerCatalogEntry) (map[string]struct{}, error) {
	referenced := make(map[string]struct{})

	var servers v1.MCPServerList
	if err := req.List(&servers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerCatalogEntryName", entry.Name),
		Namespace:     entry.Namespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list MCP servers: %w", err)
	}
	for _, server := range servers.Items {
		if revision := server.Spec.Manifest.StaticConfigurationRevision; revision != "" {
			referenced[revision] = struct{}{}
		}
	}

	var vmcps v1.VMCPList
	if err := req.List(&vmcps, &kclient.ListOptions{Namespace: entry.Namespace}); err != nil {
		return nil, fmt.Errorf("failed to list vMCPs: %w", err)
	}
	var instances v1.VMCPInstanceList
	if err := req.List(&instances, &kclient.ListOptions{Namespace: entry.Namespace}); err != nil {
		return nil, fmt.Errorf("failed to list vMCP instances: %w", err)
	}

	var components []types.VMCPComponent
	for _, vmcp := range vmcps.Items {
		components = append(components, vmcp.Spec.Manifest.Components...)
	}
	for _, instance := range instances.Items {
		components = append(components, instance.Spec.LegacyComponents...)
	}
	for _, component := range components {
		if revision := component.CatalogEntry.Manifest.StaticConfigurationRevision; component.MCPServerCatalogEntryID == entry.Name && revision != "" {
			referenced[revision] = struct{}{}
		}
	}
	return referenced, nil
}

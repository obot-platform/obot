package handlers

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ServerInstancesHandler struct {
	acrHelper *accesscontrolrule.Helper
	serverURL string
}

func NewServerInstancesHandler(acrHelper *accesscontrolrule.Helper, serverURL string) *ServerInstancesHandler {
	return &ServerInstancesHandler{
		acrHelper: acrHelper,
		serverURL: serverURL,
	}
}

func (h *ServerInstancesHandler) ListServerInstances(req api.Context) error {
	var (
		instances v1.MCPServerInstanceList
		err       error
	)
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		err = req.List(&instances)
	} else {
		err = req.List(&instances, kclient.MatchingFields{
			"spec.userID": req.User.GetUID(),
		})
	}
	if err != nil {
		return err
	}

	convertedInstances := make([]types.MCPServerInstance, 0, len(instances.Items))
	for _, instance := range instances.Items {
		// Hide template and component instances from user list view
		if instance.Spec.Template || instance.Spec.CompositeName != "" {
			continue
		}

		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}

		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, h.serverURL, slug))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

func (h *ServerInstancesHandler) GetServerInstance(req api.Context) error {
	var instance v1.MCPServerInstance
	if err := req.Get(&instance, req.PathValue("mcp_server_instance_id")); err != nil {
		return err
	}

	slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
	if err != nil {
		return fmt.Errorf("failed to determine slug: %v", err)
	}

	return req.Write(ConvertMCPServerInstance(instance, h.serverURL, slug))
}

func (h *ServerInstancesHandler) CreateServerInstance(req api.Context) error {
	var input struct {
		MCPServerID string `json:"mcpServerID"`
	}
	if err := req.Read(&input); err != nil {
		return types.NewErrBadRequest("failed to read server name: %v", err)
	}

	var server v1.MCPServer
	if err := req.Get(&server, input.MCPServerID); err != nil {
		if errors.IsNotFound(err) {
			return types.NewErrNotFound("MCP server not found")
		}
		return fmt.Errorf("failed to get MCP server: %v", err)
	}

	if !req.UserIsAdmin() {
		// Make sure the non-admin user is allowed to create an instance for this server.
		var (
			hasAccess bool
			err       error
		)

		if server.Spec.MCPCatalogID != "" {
			hasAccess, err = h.acrHelper.UserHasAccessToMCPServerInCatalog(req.User, server.Name, server.Spec.MCPCatalogID)
		} else if server.Spec.PowerUserWorkspaceID != "" {
			hasAccess, err = h.acrHelper.UserHasAccessToMCPServerInWorkspace(req.User, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
		}

		if err != nil {
			return err
		}
		if !hasAccess {
			return types.NewErrNotFound("MCP server not found")
		}
	}

	var entryName string
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
			return err
		}
		entryName = entry.Name
	}

	instance := v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:       fmt.Sprintf("%s-%s-%s", system.MCPServerInstancePrefix, req.User.GetUID(), input.MCPServerID),
			Namespace:  req.Namespace(),
			Finalizers: []string{v1.MCPServerInstanceFinalizer},
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID:                    req.User.GetUID(),
			MCPServerName:             input.MCPServerID,
			MCPCatalogName:            server.Spec.MCPCatalogID,
			MCPServerCatalogEntryName: entryName,
			PowerUserWorkspaceID:      server.Spec.PowerUserWorkspaceID,
		},
	}

	if err := req.Create(&instance); err != nil {
		if errors.IsAlreadyExists(err) {
			return types.NewErrAlreadyExists("MCP server instance already exists")
		}
		return fmt.Errorf("failed to create MCP server instance: %v", err)
	}

	slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
	if err != nil {
		return fmt.Errorf("failed to determine slug: %v", err)
	}

	return req.WriteCreated(ConvertMCPServerInstance(instance, h.serverURL, slug))
}

func (h *ServerInstancesHandler) DeleteServerInstance(req api.Context) error {
	return req.Delete(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("mcp_server_instance_id"),
			Namespace: req.Namespace(),
		},
	})
}

func (h *ServerInstancesHandler) ClearOAuthCredentials(req api.Context) error {
	var mcpServerInstance v1.MCPServerInstance
	if err := req.Get(&mcpServerInstance, req.PathValue("mcp_server_instance_id")); err != nil {
		return err
	}

	if err := req.GatewayClient.DeleteMCPOAuthTokens(req.Context(), req.User.GetUID(), mcpServerInstance.Name); err != nil {
		return fmt.Errorf("failed to delete OAuth credentials: %v", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func ConvertMCPServerInstance(instance v1.MCPServerInstance, serverURL, slug string) types.MCPServerInstance {
	return types.MCPServerInstance{
		Metadata:                MetadataFrom(&instance),
		UserID:                  instance.Spec.UserID,
		MCPServerID:             instance.Spec.MCPServerName,
		MCPCatalogID:            instance.Spec.MCPCatalogName,
		MCPServerCatalogEntryID: instance.Spec.MCPServerCatalogEntryName,
		PowerUserWorkspaceID:    instance.Spec.PowerUserWorkspaceID,
		ConnectURL:              fmt.Sprintf("%s/mcp-connect/%s", serverURL, slug),
	}
}

func (h *ServerInstancesHandler) ListServerInstancesForServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	serverID := req.PathValue("mcp_server_id")

	// First, verify the server exists and belongs to the correct scope
	var server v1.MCPServer
	if err := req.Get(&server, serverID); err != nil {
		return err
	}

	// Verify server belongs to the requested scope
	if catalogID != "" && server.Spec.MCPCatalogID != catalogID {
		return types.NewErrNotFound("MCP server not found")
	} else if workspaceID != "" && server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	// List instances for this specific server
	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, kclient.MatchingFields{
		"spec.mcpServerName": serverID,
	}); err != nil {
		return err
	}

	convertedInstances := make([]types.MCPServerInstance, 0, len(instances.Items))
	for _, instance := range instances.Items {
		// Hide component instances
		if instance.Spec.CompositeName != "" {
			continue
		}
		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}
		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, h.serverURL, slug))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

func SlugForMCPServerInstance(ctx context.Context, client kclient.Client, instance v1.MCPServerInstance) (string, error) {
	var instancesWithServerName v1.MCPServerInstanceList
	if err := client.List(ctx, &instancesWithServerName, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.mcpServerName": instance.Spec.MCPServerName,
			"spec.userID":        instance.Spec.UserID,
			"spec.template":      "false",
			"spec.compositeName": "",
		}),
	}); err != nil {
		return "", fmt.Errorf("failed to find MCP server catalog entry for server: %w", err)
	}

	slices.SortFunc(instancesWithServerName.Items, func(a, b v1.MCPServerInstance) int {
		return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
	})

	slug := instance.Spec.MCPServerName
	if instancesWithServerName.Items[0].Name != instance.Name {
		slug = instance.Name
	}

	return slug, nil
}

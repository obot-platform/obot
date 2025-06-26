package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, kclient.MatchingFields{
		"spec.userID": req.User.GetUID(),
	}); err != nil {
		return err
	}

	var convertedInstances []types.MCPServerInstance
	for _, instance := range instances.Items {
		convertedInstances = append(convertedInstances, convertMCPServerInstance(instance, h.serverURL))
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

	return req.Write(convertMCPServerInstance(instance, h.serverURL))
}

func (h *ServerInstancesHandler) CreateServerInstance(req api.Context) error {
	var input struct {
		MCPServerID string `json:"mcpServerID"`
	}
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to read server name: %w", err)
	}

	var server v1.MCPServer
	if err := req.Get(&server, input.MCPServerID); err != nil {
		return fmt.Errorf("failed to get MCP server: %w", err)
	}

	// Make sure the user is allowed to access this MCP server.
	if server.Spec.SharedWithinMCPCatalogName == system.DefaultCatalog {
		hasAccess, err := h.acrHelper.UserHasAccessToMCPServer(req.User.GetUID(), server.Name)
		if err != nil {
			return err
		}
		if !hasAccess {
			return types.NewErrNotFound("MCP server not found")
		}
	} else if server.Spec.UserID != req.User.GetUID() {
		return types.NewErrNotFound("MCP server not found")
	}

	instance := v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", system.MCPServerInstancePrefix, req.User.GetUID(), input.MCPServerID),
			Namespace: req.Namespace(),
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID:         req.User.GetUID(),
			MCPServerName:  input.MCPServerID,
			MCPCatalogName: server.Spec.SharedWithinMCPCatalogName,
		},
	}

	if err := req.Create(&instance); err != nil {
		return fmt.Errorf("failed to create MCP server instance: %w", err)
	}

	return req.WriteCreated(convertMCPServerInstance(instance, h.serverURL))
}

func (h *ServerInstancesHandler) DeleteServerInstance(req api.Context) error {
	var instance v1.MCPServerInstance
	if err := req.Get(&instance, req.PathValue("mcp_server_instance_id")); err != nil {
		return fmt.Errorf("failed to get MCP server instance: %w", err)
	}

	if err := req.Delete(&instance); err != nil {
		return fmt.Errorf("failed to delete MCP server instance: %w", err)
	}

	return req.Write(convertMCPServerInstance(instance, h.serverURL))
}

func convertMCPServerInstance(instance v1.MCPServerInstance, serverURL string) types.MCPServerInstance {
	return types.MCPServerInstance{
		Metadata:     MetadataFrom(&instance),
		UserID:       instance.Spec.UserID,
		MCPServerID:  instance.Spec.MCPServerName,
		MCPCatalogID: instance.Spec.MCPCatalogName,
		ConnectURL:   fmt.Sprintf("%s/mcp-connect/%s", serverURL, instance.Name),
	}
}

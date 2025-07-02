package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/controller/handlers/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/api/errors"
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
	var (
		instances v1.MCPServerInstanceList
		err       error
	)
	if req.UserIsAdmin() && req.URL.Query().Get("all") == "true" {
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
		return types.NewErrBadRequest("failed to read server name: %v", err)
	}

	var server v1.MCPServer
	if err := req.Get(&server, input.MCPServerID); err != nil {
		if errors.IsNotFound(err) {
			return types.NewErrNotFound("MCP server not found")
		}
		return fmt.Errorf("failed to get MCP server: %v", err)
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

	var (
		catalogName = server.Spec.SharedWithinMCPCatalogName
		entryName   string
	)
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
			return err
		}
		catalogName = entry.Spec.MCPCatalogName
		entryName = entry.Name
	}

	instance := v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", system.MCPServerInstancePrefix, req.User.GetUID(), input.MCPServerID),
			Namespace: req.Namespace(),
		},
		Spec: v1.MCPServerInstanceSpec{
			UserID:                    req.User.GetUID(),
			MCPServerName:             input.MCPServerID,
			MCPCatalogName:            catalogName,
			MCPServerCatalogEntryName: entryName,
		},
	}

	if err := req.Create(&instance); err != nil {
		if errors.IsAlreadyExists(err) {
			return types.NewErrAlreadyExists("MCP server instance already exists")
		}
		return fmt.Errorf("failed to create MCP server instance: %v", err)
	}

	return req.WriteCreated(convertMCPServerInstance(instance, h.serverURL))
}

func (h *ServerInstancesHandler) DeleteServerInstance(req api.Context) error {
	return req.Delete(&v1.MCPServerInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("mcp_server_instance_id"),
			Namespace: req.Namespace(),
		},
	})
}

func convertMCPServerInstance(instance v1.MCPServerInstance, serverURL string) types.MCPServerInstance {
	return types.MCPServerInstance{
		Metadata:                MetadataFrom(&instance),
		UserID:                  instance.Spec.UserID,
		MCPServerID:             instance.Spec.MCPServerName,
		MCPCatalogID:            instance.Spec.MCPCatalogName,
		MCPServerCatalogEntryID: instance.Spec.MCPServerCatalogEntryName,
		ConnectURL:              fmt.Sprintf("%s/mcp-connect/%s", serverURL, instance.Name),
	}
}

func (h *ServerInstancesHandler) AdminListServerInstancesForEntryInCatalog(req api.Context) error {
	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, kclient.MatchingFields{
		"spec.mcpServerCatalogEntryName": req.PathValue("entry_id"),
		"spec.mcpCatalogName":            req.PathValue("catalog_id"),
	}); err != nil {
		return err
	}

	convertedInstances := make([]types.MCPServerInstance, 0, len(instances.Items))
	for _, instance := range instances.Items {
		convertedInstances = append(convertedInstances, convertMCPServerInstance(instance, h.serverURL))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

func (h *ServerInstancesHandler) AdminListServerInstancesForServerInCatalog(req api.Context) error {
	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, kclient.MatchingFields{
		"spec.mcpServerName":  req.PathValue("mcp_server_id"),
		"spec.mcpCatalogName": req.PathValue("catalog_id"),
	}); err != nil {
		return err
	}

	convertedInstances := make([]types.MCPServerInstance, 0, len(instances.Items))
	for _, instance := range instances.Items {
		convertedInstances = append(convertedInstances, convertMCPServerInstance(instance, h.serverURL))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

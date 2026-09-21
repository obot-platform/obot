package handlers

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ServerInstancesHandler struct {
	serverURL string
}

func NewServerInstancesHandler(serverURL string) *ServerInstancesHandler {
	return &ServerInstancesHandler{
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
		if instance.Spec.Template || instance.Spec.CompositeName != "" || instance.Spec.VMCPInstanceID != "" {
			continue
		}

		cred, err := mcpServerInstanceCredEnv(req, instance)
		if err != nil {
			return fmt.Errorf("failed to get credentials for instance %s: %w", instance.Name, err)
		}

		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}

		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, cred, h.serverURL, slug))
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

	credEnv, err := mcpServerInstanceCredEnv(req, instance)
	if err != nil {
		return err
	}

	return req.Write(ConvertMCPServerInstance(instance, credEnv, h.serverURL, slug))
}

func ConvertMCPServerInstance(instance v1.MCPServerInstance, credEnv map[string]string, serverURL, slug string) types.MCPServerInstance {
	missingHeaders := mcpServerInstanceMissingHeaders(instance, credEnv)

	return types.MCPServerInstance{
		Metadata:                MetadataFrom(&instance),
		Configured:              len(missingHeaders) == 0,
		MissingRequiredHeaders:  missingHeaders,
		UserID:                  instance.Spec.UserID,
		MCPServerID:             instance.Spec.MCPServerName,
		MCPCatalogID:            instance.Spec.MCPCatalogName,
		MCPServerCatalogEntryID: instance.Spec.MCPServerCatalogEntryName,
		PowerUserWorkspaceID:    instance.Spec.PowerUserWorkspaceID,
		ConnectURL:              fmt.Sprintf("%s/mcp-connect/%s", serverURL, slug),
		Config:                  instance.Spec.Config,
	}
}

func mcpServerInstanceCredEnv(req api.Context, instance v1.MCPServerInstance) (map[string]string, error) {
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{MCPServerInstanceCredentialContext(instance)}, instance.Name)
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find credential: %w", err)
	}

	return cred.Secrets, nil
}

func MCPServerInstanceCredentialContext(instance v1.MCPServerInstance) string {
	return fmt.Sprintf("%s-%s", instance.Spec.UserID, instance.Name)
}

func mcpServerInstanceMissingHeaders(instance v1.MCPServerInstance, credEnv map[string]string) []string {
	if instance.Spec.Config == nil {
		return nil
	}

	var missingHeaders []string

	for _, header := range instance.Spec.Config {
		val := credEnv[header.Key]
		if (val == "" || !mcp.ConfigurationOptionValueValid(header.ToHeader(), credEnv)) && (header.Required || val != "") {
			missingHeaders = append(missingHeaders, header.Key)
		}
	}

	return missingHeaders
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
		if instance.Spec.CompositeName != "" || instance.Spec.VMCPInstanceID != "" {
			continue
		}
		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}
		credEnv, err := mcpServerInstanceCredEnv(req, instance)
		if err != nil {
			return err
		}
		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, credEnv, h.serverURL, slug))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

func SlugForMCPServerInstance(ctx context.Context, client kclient.Client, instance v1.MCPServerInstance) (string, error) {
	if instance.Spec.VMCPInstanceID != "" {
		return instance.Name, nil
	}
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

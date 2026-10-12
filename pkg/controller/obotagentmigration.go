package controller

import (
	"context"
	"fmt"

	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	obotAgentRemovalMigrationName = "remove_obot_agents"

	// obotAgentAPIKeyNamePrefix prefixes the name of the API key an Obot Agent created for its MCP server.
	obotAgentAPIKeyNamePrefix = "nanobot-agent-"
	// obotAgentSearchServerName is the system MCP server Obot Agents used to search for MCP servers.
	obotAgentSearchServerName = system.SystemMCPServerPrefix + "obot-mcp-server"
)

type obotAgentAPIKeys interface {
	ListAllAPIKeys(ctx context.Context, opts gclient.APIKeyListOptions) ([]gatewaytypes.APIKey, error)
	RevokeAPIKeyByID(ctx context.Context, keyID uint) error
}

// deleteObotAgentResources deletes what the removed Obot Agent feature left behind: the MCP servers
// the agents ran on, the API keys those servers used, and the system MCP server the agents searched with.
func deleteObotAgentResources(ctx context.Context, client kclient.Client, apiKeys obotAgentAPIKeys) error {
	var servers v1.MCPServerList
	if err := client.List(ctx, &servers); err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	var agentServers []*v1.MCPServer
	keyNames := make(map[string]struct{})
	for i := range servers.Items {
		if servers.Items[i].Spec.NanobotAgentID != "" {
			agentServers = append(agentServers, &servers.Items[i])
			keyNames[obotAgentAPIKeyNamePrefix+servers.Items[i].Name] = struct{}{}
		}
	}

	// The agents revoked these keys when they were deleted. Revoke them before deleting the servers,
	// because the servers are how the keys are found if this has to run again.
	if len(keyNames) > 0 {
		keys, err := apiKeys.ListAllAPIKeys(ctx, gclient.APIKeyListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list API keys: %w", err)
		}
		for _, key := range keys {
			if _, ok := keyNames[key.Name]; !ok {
				continue
			}
			if err := apiKeys.RevokeAPIKeyByID(ctx, key.ID); err != nil {
				return fmt.Errorf("failed to revoke Obot Agent API key %d: %w", key.ID, err)
			}
		}
	}

	for _, server := range agentServers {
		if err := kclient.IgnoreNotFound(client.Delete(ctx, server)); err != nil {
			return fmt.Errorf("failed to delete Obot Agent MCP server %s/%s: %w", server.Namespace, server.Name, err)
		}
	}

	if err := kclient.IgnoreNotFound(client.Delete(ctx, &v1.SystemMCPServer{
		Name:      obotAgentSearchServerName,
		Namespace: system.DefaultNamespace,
	})); err != nil {
		return fmt.Errorf("failed to delete the Obot MCP server: %w", err)
	}

	return nil
}

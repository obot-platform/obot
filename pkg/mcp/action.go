package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	requestTimeUpdateInterval = 15 * time.Minute
)

var (
	actionEnvVarRegex = regexp.MustCompile(`\${([^}]+)}`)
)

func (sm *SessionManager) ServerForActionWithConnectID(ctx context.Context, id string, user kuser.Info) (string, v1.MCPServer, ServerConfig, error) {
	id, server, config, _, err := sm.serverForActionWithConnectID(ctx, id, user, false)
	return id, server, config, err
}

func (sm *SessionManager) ServerForActionWithConnectIDAllowMissingConfig(ctx context.Context, id string, user kuser.Info) (string, v1.MCPServer, ServerConfig, []string, error) {
	return sm.serverForActionWithConnectID(ctx, id, user, true)
}

func (sm *SessionManager) serverForActionWithConnectID(ctx context.Context, id string, user kuser.Info, allowMissingConfig bool) (string, v1.MCPServer, ServerConfig, []string, error) {
	userID := principal.ResourceOwnerID(user)
	if vmcp, instance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, id, userID); err != nil {
		return "", v1.MCPServer{}, ServerConfig{}, nil, err
	} else if vmcp != nil {
		server, config, err := sm.serverForVMCPAction(ctx, id, user, vmcp, instance)
		if err != nil {
			return "", v1.MCPServer{}, ServerConfig{}, nil, err
		}
		return id, server, config, nil, nil
	}

	server, instance, err := sm.serverOrInstanceFromConnectURL(ctx, id, userID)
	if err != nil {
		return "", v1.MCPServer{}, ServerConfig{}, nil, err
	}

	switch {
	case instance.Name != "":
		server, config, missingConfig, err := sm.serverFromMCPServerInstance(ctx, instance, userID, allowMissingConfig)
		return instance.Name, server, config, missingConfig, err
	case server.Name != "":
		config, missingConfig, err := sm.serverConfigForAction(ctx, server, userID, allowMissingConfig)
		return server.Name, server, config, missingConfig, err
	default:
		return "", v1.MCPServer{}, ServerConfig{}, nil, fmt.Errorf("unknown MCP server ID %s", id)
	}
}

func (sm *SessionManager) ServerForAction(ctx context.Context, id string, user kuser.Info) (v1.MCPServer, ServerConfig, error) {
	userID := principal.ResourceOwnerID(user)
	if system.IsMCPServerInstanceID(id) {
		_, server, config, _, err := sm.serverForActionWithConnectID(ctx, id, user, false)
		return server, config, err
	}
	if vmcp, instance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, id, userID); err != nil {
		return v1.MCPServer{}, ServerConfig{}, err
	} else if vmcp != nil {
		return sm.serverForVMCPAction(ctx, id, user, vmcp, instance)
	}

	var server v1.MCPServer
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &server); err != nil {
		return server, ServerConfig{}, err
	}

	if server.Spec.VMCPID != "" {
		// The gateway only reaches shared vMCP components through the caller's connection to them.
		connectionID, err := sm.sharedVMCPComponentConnection(ctx, server, userID)
		if err != nil {
			return server, ServerConfig{}, err
		}
		_, componentServer, serverConfig, _, err := sm.serverForActionWithConnectID(ctx, connectionID, user, false)
		return componentServer, serverConfig, err
	}

	serverConfig, _, err := sm.serverConfigForAction(ctx, server, userID, false)
	return server, serverConfig, err
}

// sharedVMCPComponentConnection returns the name of the user's connection to a shared vMCP component server.
func (sm *SessionManager) sharedVMCPComponentConnection(ctx context.Context, server v1.MCPServer, userID string) (string, error) {
	var connections v1.MCPServerInstanceList
	if err := sm.storageClient.List(ctx, &connections,
		kclient.InNamespace(server.Namespace),
		kclient.MatchingFields{
			"spec.mcpServerName": server.Name,
			"spec.userID":        userID,
		},
	); err != nil {
		return "", err
	}
	for _, connection := range connections.Items {
		if connection.Spec.VMCPInstanceID != "" && connection.Spec.VMCPComponentID == server.Spec.VMCPComponentID && connection.DeletionTimestamp.IsZero() {
			return connection.Name, nil
		}
	}
	return "", types.NewErrBadRequest("connect to vMCP %s to use its shared component %s", server.Spec.VMCPID, server.Name)
}

func (sm *SessionManager) serverForVMCPAction(ctx context.Context, id string, user kuser.Info, vmcp *v1.VMCP, instance *v1.VMCPInstance) (v1.MCPServer, ServerConfig, error) {
	user, err := sm.vmcpResourceOwner(ctx, user)
	if err != nil {
		return v1.MCPServer{}, ServerConfig{}, err
	}
	config, err := sm.serverConfigForVMCP(ctx, vmcp, instance, user)
	if err != nil {
		return v1.MCPServer{}, ServerConfig{}, err
	}
	return v1.MCPServer{
		Name:      id,
		Namespace: config.MCPServerNamespace,
		Spec: v1.MCPServerSpec{
			Manifest: types.MCPServerManifest{
				Name:    config.MCPServerDisplayName,
				Runtime: types.RuntimeVMCP,
			},
			UserID: config.OwnerUserID,
			VMCPID: vmcp.Name,
		},
	}, config, nil
}

func (sm *SessionManager) serverOrInstanceFromConnectURL(ctx context.Context, id, userID string) (v1.MCPServer, v1.MCPServerInstance, error) {
	switch {
	case system.IsMCPServerInstanceID(id):
		var instance v1.MCPServerInstance
		return v1.MCPServer{}, instance, sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &instance)
	case system.IsMCPServerID(id):
		var server v1.MCPServer
		if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &server); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, err
		}

		if !server.Spec.IsSingleUser() && server.Spec.VMCPID == "" {
			var instances v1.MCPServerInstanceList
			if err := sm.storageClient.List(ctx, &instances,
				kclient.InNamespace(system.DefaultNamespace),
				kclient.MatchingFields{
					"spec.mcpServerName": id,
					"spec.userID":        userID,
					"spec.template":      "false",
					"spec.compositeName": "",
				},
			); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}
			if len(instances.Items) == 0 {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an instance of MCP server %s", id)
			}

			slices.SortFunc(instances.Items, func(a, b v1.MCPServerInstance) int {
				return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
			})

			return v1.MCPServer{}, instances.Items[0], nil
		}

		return server, v1.MCPServerInstance{}, nil
	default:
		return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("invalid MCP connection ID %s", id)
	}
}

func (sm *SessionManager) serverFromMCPServerInstance(ctx context.Context, instance v1.MCPServerInstance, userID string, allowMissingConfig bool) (v1.MCPServer, ServerConfig, []string, error) {
	if instance.Spec.UserID != userID {
		return v1.MCPServer{}, ServerConfig{}, nil, types.NewErrForbidden("MCP server instance belongs to another user")
	}
	var server v1.MCPServer
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: instance.Spec.MCPServerName}, &server); err != nil {
		return server, ServerConfig{}, nil, err
	}

	if server.Spec.NeedsURL {
		if allowMissingConfig {
			return server, ServerConfig{}, []string{"URL"}, nil
		}
		return server, ServerConfig{}, nil, fmt.Errorf("mcp server %s needs to update its URL", server.Name)
	}

	addExtractedEnvVars(&server)

	var scope string
	if server.Spec.VMCPID != "" {
		component, err := vmcpaccess.ServerInstanceComponent(ctx, sm.storageClient, instance, server)
		if err != nil {
			return server, ServerConfig{}, nil, err
		}
		server.Spec.Manifest.Config = vmcpaccess.ComponentConfig(component)
		server.Spec.Manifest.StaticConfigurationRevision = component.CatalogEntry.Manifest.StaticConfigurationRevision
		instance.Spec.Config = server.Spec.Manifest.UserConfig()
		scope = server.Spec.VMCPID
	} else if server.Spec.MCPCatalogID != "" {
		scope = server.Spec.MCPCatalogID
	} else if server.Spec.PowerUserWorkspaceID != "" {
		scope = server.Spec.PowerUserWorkspaceID
	} else {
		scope = instance.Spec.UserID
	}

	cred, err := sm.gatewayClient.RevealCredential(ctx, []string{server.CredentialContext(instance.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to find credential: %w", err)
	}

	catalogName, err := sm.catalogNameForServer(ctx, server, true)
	if err != nil {
		return server, ServerConfig{}, nil, err
	}

	mergedEnv, err := MergeBoundCreds(ctx, sm.localCachedClient, sm.obotNamespace, server.Spec.Manifest.Config, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	resolvedServer, err := ResolveServerStaticConfiguration(ctx, sm.gatewayClient, server)
	if err != nil {
		return server, ServerConfig{}, nil, err
	}

	serverConfig, missingConfig, err := ServerToServerConfig(resolvedServer, instance.ValidConnectURLs(sm.baseURL), userID, scope, catalogName, mergedEnv)
	if err != nil {
		return server, ServerConfig{}, nil, err
	}
	if instance.Spec.VMCPInstanceID != "" {
		serverConfig.MCPServerInstanceID = instance.Name
	}

	instanceCredEnv, err := sm.serverInstanceCredEnv(ctx, instance)
	if err != nil {
		return server, ServerConfig{}, nil, err
	}

	var missingInstanceConfig []string
	serverConfig.PassthroughHeaderNames, serverConfig.PassthroughHeaderValues, missingInstanceConfig = serverInstanceHeaders(instance, instanceCredEnv)
	missingConfig = append(missingConfig, missingInstanceConfig...)

	if serverConfig.Webhooks, err = sm.webhooksForServerConfig(serverConfig); err != nil {
		return server, ServerConfig{}, nil, err
	}

	if len(missingConfig) > 0 {
		if allowMissingConfig {
			return server, serverConfig, missingConfig, nil
		}
		return server, ServerConfig{}, missingConfig, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	sm.updateLastRequestTime(ctx, &server)
	return server, serverConfig, nil, nil
}

func (sm *SessionManager) serverConfigForAction(ctx context.Context, server v1.MCPServer, userID string, allowMissingConfig bool) (ServerConfig, []string, error) {
	if server.Spec.NeedsURL {
		if allowMissingConfig {
			return ServerConfig{}, []string{"URL"}, nil
		}
		return ServerConfig{}, nil, types.NewErrBadRequest("mcp server %s needs to update its URL", server.Name)
	}

	var scope string
	if server.Spec.VMCPID != "" {
		scope = server.Spec.VMCPID
	} else if server.Spec.MCPCatalogID != "" {
		scope = server.Spec.MCPCatalogID
	} else if server.Spec.PowerUserWorkspaceID != "" {
		scope = server.Spec.PowerUserWorkspaceID
	} else {
		scope = server.Spec.UserID
	}

	addExtractedEnvVars(&server)

	cred, err := sm.gatewayClient.RevealCredential(ctx, []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return ServerConfig{}, nil, fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := MergeBoundCreds(ctx, sm.localCachedClient, sm.obotNamespace, server.Spec.Manifest.Config, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	catalogName, err := sm.catalogNameForServer(ctx, server, false)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	resolvedServer, err := ResolveServerStaticConfiguration(ctx, sm.gatewayClient, server)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	serverConfig, missingConfig, err := ServerToServerConfig(resolvedServer, server.ValidConnectURLs(sm.baseURL), userID, scope, catalogName, mergedEnv)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	if serverConfig.Webhooks, err = sm.webhooksForServerConfig(serverConfig); err != nil {
		return ServerConfig{}, nil, err
	}

	if len(missingConfig) > 0 {
		if allowMissingConfig {
			return serverConfig, missingConfig, nil
		}

		serverName := server.Spec.Manifest.Name
		if serverName == "" {
			serverName = server.Name
		}
		return ServerConfig{}, missingConfig, types.NewErrBadRequest("missing required config for server %q: %s", serverName, strings.Join(missingConfig, ", "))
	}

	sm.updateLastRequestTime(ctx, &server)
	return serverConfig, nil, nil
}

func (sm *SessionManager) webhooksForServerConfig(serverConfig ServerConfig) ([]Webhook, error) {
	if serverConfig.ComponentMCPServer || serverConfig.SystemMCPServer || sm.webhookHelper == nil {
		return nil, nil
	}

	webhooks, err := sm.webhookHelper.GetWebhooksForMCPServer(serverConfig, sm.TransformObotHostname)
	if err != nil {
		return nil, err
	}

	slices.SortFunc(webhooks, func(a, b Webhook) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})

	return webhooks, nil
}

func (sm *SessionManager) catalogNameForServer(ctx context.Context, server v1.MCPServer, failOnEntryMissing bool) (string, error) {
	catalogName := server.Spec.MCPCatalogID
	if catalogName == "" {
		catalogName = server.Status.MCPCatalogID
	}
	if catalogName == "" {
		catalogName = server.Spec.PowerUserWorkspaceID
	}
	if server.Spec.MCPServerCatalogEntryName == "" {
		return catalogName, nil
	}

	// Composite and vMCP component servers run from the snapshot their parent holds and
	// are deliberately not garbage collected with their catalog entry, so a deleted entry
	// must not stop them from resolving. Every other server is collected along with its
	// entry, so it keeps failing rather than serving a server that is on its way out.
	entryMayBeMissing := server.Spec.VMCPComponentID != "" || (!failOnEntryMissing && server.Spec.CompositeName != "")
	if entryMayBeMissing && catalogName != "" {
		return catalogName, nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: server.Spec.MCPServerCatalogEntryName}, &entry); err != nil {
		if apierrors.IsNotFound(err) && entryMayBeMissing {
			return system.DefaultCatalog, nil
		}
		return "", fmt.Errorf("failed to get MCP server catalog entry: %w", err)
	}

	if catalogName == "" {
		if catalogName = entry.Spec.MCPCatalogName; catalogName == "" {
			catalogName = entry.Spec.PowerUserWorkspaceID
		}
	}
	return catalogName, nil
}

func (sm *SessionManager) updateLastRequestTime(ctx context.Context, server *v1.MCPServer) {
	if time.Since(server.Status.LastRequestTime.Time) <= requestTimeUpdateInterval {
		return
	}

	server.Status.LastRequestTime = metav1.Now()
	if err := sm.storageClient.Status().Update(ctx, server); err != nil && !apierrors.IsConflict(err) {
		// Ignore conflict errors because that just means another request likely beat us to updating here.
		slog.Warn("failed to update mcp server status", "error", err)
	}
}

func (sm *SessionManager) serverInstanceCredEnv(ctx context.Context, instance v1.MCPServerInstance) (map[string]string, error) {
	cred, err := sm.gatewayClient.RevealCredential(ctx, []string{serverInstanceCredentialContext(instance)}, instance.Name)
	if err != nil {
		if errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to find credential: %w", err)
	}

	return cred.Secrets, nil
}

func serverInstanceCredentialContext(instance v1.MCPServerInstance) string {
	return fmt.Sprintf("%s-%s", instance.Spec.UserID, instance.Name)
}

func serverInstanceHeaders(instance v1.MCPServerInstance, credEnv map[string]string) ([]string, []string, []string) {
	var headerNames, headerValues, missingHeaders []string
	for _, header := range instance.Spec.Config {
		if header.Usage != types.Header || !header.UserAllowed {
			continue
		}
		val := credEnv[header.Key]
		if val != "" && ConfigurationOptionValueValid(header.ToHeader(), credEnv) {
			headerNames = append(headerNames, header.Key)
			headerValues = append(headerValues, applyMCPServerInstanceHeaderPrefix(val, header.Prefix))
		} else if header.Required || val != "" {
			missingHeaders = append(missingHeaders, header.Key)
		}
	}

	return headerNames, headerValues, missingHeaders
}

func applyMCPServerInstanceHeaderPrefix(value, prefix string) string {
	if value == "" || strings.HasPrefix(value, prefix) {
		return value
	}
	return prefix + value
}

func extractEnvVars(text string) []string {
	if text == "" {
		return nil
	}

	matches := actionEnvVarRegex.FindAllStringSubmatch(text, -1)
	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}

	return vars
}

func addExtractedEnvVars(server *v1.MCPServer) {
	existing := make(map[string]struct{})
	for _, env := range server.Spec.Manifest.Config {
		existing[env.Key] = struct{}{}
	}

	var toExtract []string
	switch server.Spec.Manifest.Runtime {
	case types.RuntimeUVX:
		if server.Spec.Manifest.UVXConfig != nil {
			toExtract = []string{server.Spec.Manifest.UVXConfig.Command}
			if len(server.Spec.Manifest.UVXConfig.Args) > 0 {
				toExtract = append(toExtract, server.Spec.Manifest.UVXConfig.Args...)
			}
		}
	case types.RuntimeNPX:
		if server.Spec.Manifest.NPXConfig != nil && len(server.Spec.Manifest.NPXConfig.Args) > 0 {
			toExtract = append(toExtract, server.Spec.Manifest.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if server.Spec.Manifest.ContainerizedConfig != nil {
			toExtract = []string{server.Spec.Manifest.ContainerizedConfig.Command}
			if len(server.Spec.Manifest.ContainerizedConfig.Args) > 0 {
				toExtract = append(toExtract, server.Spec.Manifest.ContainerizedConfig.Args...)
			}
		}
	case types.RuntimeRemote:
		if server.Spec.Manifest.RemoteConfig != nil {
			toExtract = []string{server.Spec.Manifest.RemoteConfig.URL}
		}
	}

	for _, v := range toExtract {
		for _, env := range extractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				server.Spec.Manifest.Config = append(server.Spec.Manifest.Config, types.MCPConfig{
					Usage:       types.Env,
					Name:        env,
					Key:         env,
					Description: "Automatically detected variable",
					Sensitive:   true,
					Required:    true,
				})
			}
		}
	}
}

package mcp

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	requestTimeUpdateInterval = 15 * time.Minute

	// compositeSettleTimeout is how long a connect waits for the controller to deploy components.
	compositeSettleTimeout = 30 * time.Second
)

// IDAndAudienceFromConnectURL returns the MCP server or instance name and audience based on the provided connect URL.
// The connect URL could have an MCP server ID, server instance ID, or MCP catalog entry ID.
func (sm *SessionManager) IDAndAudienceFromConnectURL(ctx context.Context, id, userID string) (string, string, error) {
	server, instance, err := sm.serverOrInstanceFromConnectURL(ctx, id, userID)
	if err != nil {
		return "", "", err
	}

	switch {
	case instance.Name != "":
		return instance.Name, instance.Spec.MCPServerName, nil
	case server.Name != "":
		return server.Name, id, nil
	default:
		return "", "", fmt.Errorf("unknown MCP server ID %s", id)
	}
}

func (sm *SessionManager) ServerForActionWithConnectID(ctx context.Context, id, userID string) (string, v1.MCPServer, ServerConfig, error) {
	id, server, config, _, err := sm.serverForActionWithConnectID(ctx, id, userID, false)
	return id, server, config, err
}

func (sm *SessionManager) ServerForActionWithConnectIDAllowMissingConfig(ctx context.Context, id, userID string) (string, v1.MCPServer, ServerConfig, []string, error) {
	return sm.serverForActionWithConnectID(ctx, id, userID, true)
}

func (sm *SessionManager) serverForActionWithConnectID(ctx context.Context, id, userID string, allowMissingConfig bool) (string, v1.MCPServer, ServerConfig, []string, error) {
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

func (sm *SessionManager) ServerForAction(ctx context.Context, id, userID string) (v1.MCPServer, ServerConfig, error) {
	var server v1.MCPServer
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &server); err != nil {
		return server, ServerConfig{}, err
	}

	serverConfig, _, err := sm.serverConfigForAction(ctx, server, userID, false)
	return server, serverConfig, err
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

		if !server.Spec.IsSingleUser() {
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
				instance := v1.MCPServerInstance{
					GenerateName: system.MCPServerInstancePrefix,
					Namespace:    server.Namespace,
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:             id,
						MCPCatalogName:            server.Spec.MCPCatalogID,
						MCPServerCatalogEntryName: server.Spec.MCPServerCatalogEntryName,
						PowerUserWorkspaceID:      server.Spec.PowerUserWorkspaceID,
						UserID:                    userID,
						MultiUserConfig:           server.Spec.Manifest.MultiUserConfig,
					},
				}
				if err := sm.storageClient.Create(ctx, &instance); err != nil {
					return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("user has not configured an instance of MCP server %s", id)
				}

				instances.Items = append(instances.Items, instance)
			}

			slices.SortFunc(instances.Items, func(a, b v1.MCPServerInstance) int {
				return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
			})

			return v1.MCPServer{}, instances.Items[0], nil
		}

		return server, v1.MCPServerInstance{}, nil
	default:
		var entry v1.MCPServerCatalogEntry
		if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &entry); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("catalog entry %s not found", id)
		}
		AddExtractedEnvVarsToCatalogEntry(&entry)

		var servers v1.MCPServerList
		if err := sm.storageClient.List(ctx, &servers,
			kclient.InNamespace(system.DefaultNamespace),
			kclient.MatchingFields{
				"spec.mcpServerCatalogEntryName": id,
				"spec.userID":                    userID,
				"spec.template":                  "false",
				"spec.compositeName":             "",
			},
		); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, err
		}
		if len(servers.Items) == 0 {
			missingAdminConfig, err := sm.entryMissingAdminConfig(ctx, entry)
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to determine required admin configuration for catalog entry %s: %w", id, err)
			}
			if err := missingAdminConfig.Err(id); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}

			allowMissingURL := CatalogEntryRequiresUserURL(entry.Spec.Manifest)
			manifest, err := ServerManifestFromCatalogEntryManifest(false, allowMissingURL, entry.Spec.Manifest, types.MCPServerManifest{})
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because it could not be converted to an MCP server: %v", id, err)
			}
			resourceMaximums, err := sm.EffectiveKubernetesResourceMaximums(ctx, sm.storageClient)
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}
			if err := ValidateServerManifest(ctx, manifest, false, ValidationOptions{
				AllowMissingURL:              allowMissingURL,
				RemoteMCPURLValidationConfig: sm.remoteURLValidationConfig,
				ResourceMaximums:             resourceMaximums,
				ResolveComponent:             ComponentUpstreamResolver(sm.storageClient),
			}); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because its MCP server manifest is invalid: %v", id, err)
			}

			server := v1.MCPServer{
				GenerateName: system.MCPServerPrefix,
				Namespace:    system.DefaultNamespace,
				Spec: v1.MCPServerSpec{
					Manifest:                  manifest,
					UnsupportedTools:          entry.Spec.UnsupportedTools,
					MCPServerCatalogEntryName: id,
					UserID:                    userID,
					NeedsURL:                  allowMissingURL && (manifest.RemoteConfig == nil || manifest.RemoteConfig.URL == ""),
				},
			}
			if err := sm.storageClient.Create(ctx, &server); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to create MCP server for catalog entry %s: %w", id, err)
			}

			if server.Spec.Manifest.Runtime == types.RuntimeComposite &&
				server.Spec.Manifest.CompositeConfig != nil &&
				len(server.Spec.Manifest.CompositeConfig.ComponentServers) > 0 {
				if server, err = WaitForCompositeReady(ctx, sm.storageClient, server, compositeSettleTimeout); err != nil {
					return v1.MCPServer{}, v1.MCPServerInstance{}, err
				}
			}

			servers.Items = append(servers.Items, server)
		}

		slices.SortFunc(servers.Items, func(a, b v1.MCPServer) int {
			return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		})

		server := servers.Items[0]
		if syncConnectServerRemoteConfigFromCatalogEntry(&server, entry) {
			if err := sm.storageClient.Update(ctx, &server); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to update MCP server configuration from catalog entry %s: %w", id, err)
			}
		}

		return server, v1.MCPServerInstance{}, nil
	}
}

func (sm *SessionManager) serverFromMCPServerInstance(ctx context.Context, instance v1.MCPServerInstance, userID string, allowMissingConfig bool) (v1.MCPServer, ServerConfig, []string, error) {
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

	AddExtractedEnvVars(&server)

	var credCtx, scope string
	if server.Spec.MCPCatalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)
		scope = server.Spec.MCPCatalogID
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)
		scope = server.Spec.PowerUserWorkspaceID
	} else {
		credCtx = fmt.Sprintf("%s-%s", instance.Spec.UserID, server.Name)
		scope = instance.Spec.UserID
	}

	cred, err := sm.gatewayClient.RevealCredential(ctx, []string{credCtx}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to find credential: %w", err)
	}

	catalogName, err := sm.catalogNameForServer(ctx, server, true)
	if err != nil {
		return server, ServerConfig{}, nil, err
	}

	mergedEnv, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	serverConfig, missingConfig, err := ServerToServerConfig(server, instance.ValidConnectURLs(sm.baseURL), userID, scope, catalogName, mergedEnv)
	if err != nil {
		return server, ServerConfig{}, nil, err
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

	var (
		credCtxs []string
		scope    string
	)
	if server.Spec.MCPCatalogID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
		scope = server.Spec.MCPCatalogID
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
		scope = server.Spec.PowerUserWorkspaceID
	} else {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name))
		scope = server.Spec.UserID
	}

	AddExtractedEnvVars(&server)

	cred, err := sm.gatewayClient.RevealCredential(ctx, credCtxs, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return ServerConfig{}, nil, fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	catalogName, err := sm.catalogNameForServer(ctx, server, false)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	var (
		serverConfig  ServerConfig
		missingConfig []string
	)
	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		var componentServers v1.MCPServerList
		if err = sm.storageClient.List(ctx, &componentServers,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{"spec.compositeName": server.Name},
		); err != nil {
			return ServerConfig{}, nil, fmt.Errorf("failed to list component servers: %w", err)
		}

		var componentInstances v1.MCPServerInstanceList
		if err = sm.storageClient.List(ctx, &componentInstances,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{"spec.compositeName": server.Name},
		); err != nil {
			return ServerConfig{}, nil, fmt.Errorf("failed to list component servers instances: %w", err)
		}

		if !allowMissingConfig && len(componentServers.Items)+len(componentInstances.Items) == 0 && EnabledComponentCount(server.Spec.Manifest) > 0 {
			return ServerConfig{}, nil, CompositeNoHealthyComponentsError{Composite: server.Name, Errors: server.Status.ComponentErrors}
		}

		serverConfig, missingConfig, err = CompositeServerToServerConfig(server, componentServers.Items, componentInstances.Items, server.ValidConnectURLs(sm.baseURL), sm.httpListenPort, userID, scope, catalogName, mergedEnv)
		componentMissingConfig, componentErr := sm.compositeComponentsMissingConfig(ctx, userID, server, componentServers.Items, componentInstances.Items)
		if componentErr != nil {
			return ServerConfig{}, nil, componentErr
		}
		missingConfig = append(missingConfig, componentMissingConfig...)
	} else {
		serverConfig, missingConfig, err = ServerToServerConfig(server, server.ValidConnectURLs(sm.baseURL), userID, scope, catalogName, mergedEnv)
	}
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
		return ServerConfig{}, missingConfig, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	sm.updateLastRequestTime(ctx, &server)
	return serverConfig, nil, nil
}

func (sm *SessionManager) webhooksForServerConfig(serverConfig ServerConfig) ([]Webhook, error) {
	if serverConfig.ComponentMCPServer || serverConfig.SystemMCPServer || sm.webhookHelper == nil {
		return nil, nil
	}

	webhooks, err := sm.webhookHelper.GetWebhooksForMCPServer(serverConfig)
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

func (sm *SessionManager) compositeComponentsMissingConfig(ctx context.Context, userID string, composite v1.MCPServer, componentServers []v1.MCPServer, componentInstances []v1.MCPServerInstance) ([]string, error) {
	// A disabled component is left out of the runtime config, so its gaps must not block launch.
	disabled := DisabledComponentIDs(composite)

	var missingConfig []string
	for _, component := range componentServers {
		if disabled[component.Spec.MCPServerCatalogEntryName] {
			continue
		}

		_, componentMissingConfig, err := sm.serverConfigForAction(ctx, component, userID, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get config for component server %s: %w", component.Name, err)
		}
		for _, missing := range componentMissingConfig {
			missingConfig = append(missingConfig, fmt.Sprintf("%s: %s", component.Spec.MCPServerCatalogEntryName, missing))
		}
	}

	for _, instance := range componentInstances {
		if disabled[instance.Spec.MCPServerName] {
			continue
		}

		_, _, instanceMissingConfig, err := sm.serverFromMCPServerInstance(ctx, instance, userID, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get config for component server instance %s: %w", instance.Name, err)
		}
		for _, missing := range instanceMissingConfig {
			missingConfig = append(missingConfig, fmt.Sprintf("%s: %s", instance.Spec.MCPServerName, missing))
		}
	}

	return missingConfig, nil
}

// DisabledComponentIDs is keyed by ComponentID so catalog-entry and multi-user components match.
func DisabledComponentIDs(composite v1.MCPServer) map[string]bool {
	if composite.Spec.Manifest.CompositeConfig == nil {
		return nil
	}

	disabled := make(map[string]bool, len(composite.Spec.Manifest.CompositeConfig.ComponentServers))
	for _, component := range composite.Spec.Manifest.CompositeConfig.ComponentServers {
		if id := component.ComponentID(); id != "" && component.Disabled {
			disabled[id] = true
		}
	}

	return disabled
}

func (sm *SessionManager) catalogNameForServer(ctx context.Context, server v1.MCPServer, failOnEntryMissing bool) (string, error) {
	catalogName := server.Spec.MCPCatalogID
	if catalogName == "" {
		catalogName = server.Status.MCPCatalogID
	}
	if catalogName == "" {
		catalogName = server.Spec.PowerUserWorkspaceID
	}
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: server.Spec.MCPServerCatalogEntryName}, &entry); err == nil {
			if catalogName == "" {
				catalogName = entry.Spec.MCPCatalogName
			}
			if catalogName == "" {
				catalogName = entry.Spec.PowerUserWorkspaceID
			}
		} else if !failOnEntryMissing && apierrors.IsNotFound(err) && server.Spec.CompositeName != "" {
			if catalogName == "" {
				catalogName = system.DefaultCatalog
			}
		} else {
			return "", fmt.Errorf("failed to get MCP server catalog entry: %w", err)
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
	if instance.Spec.MultiUserConfig == nil {
		return nil, nil, nil
	}

	var headerNames, headerValues, missingHeaders []string
	for _, header := range instance.Spec.MultiUserConfig.UserDefinedHeaders {
		val := credEnv[header.Key]
		if val != "" && ConfigurationOptionValueValid(header, credEnv) {
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

// entryMissingAdminConfig reports the admin-owned configuration the catalog entry still needs
// before this SessionManager will connect to it. Unlike EntryMissingAdminConfig, the connect path
// blocks only on a required secret-bound field that fails to resolve, so the two share the
// manifest resolution but not the binding check.
func (sm *SessionManager) entryMissingAdminConfig(ctx context.Context, entry v1.MCPServerCatalogEntry) (MissingCatalogEntryAdminConfig, error) {
	missing := MissingCatalogEntryAdminConfig{
		StaticOAuth: entryRequiresStaticOAuthCreds(entry),
	}

	refs, err := adminConfigManifestRefs(ctx, sm.storageClient, entry)
	if err != nil {
		return missing, err
	}

	for _, ref := range refs {
		var remote *types.RemoteRuntimeConfig
		if ref.manifest.RemoteConfig != nil {
			remote = &types.RemoteRuntimeConfig{Headers: ref.manifest.RemoteConfig.Headers}
		}

		resolved, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, ref.manifest.Env, remote, nil, sm.secretBindingAllowedLabel)
		if err != nil {
			return missing, err
		}

		for _, e := range ref.manifest.Env {
			if e.Required && e.SecretBinding != nil {
				if _, ok := resolved[e.Key]; !ok {
					missing.SecretBoundFields = append(missing.SecretBoundFields, secretBoundFieldLabel(ref.prefix, "env", e.MCPHeader))
				}
			}
		}

		if ref.manifest.RemoteConfig != nil {
			for _, h := range ref.manifest.RemoteConfig.Headers {
				if h.Required && h.SecretBinding != nil {
					if _, ok := resolved[h.Key]; !ok {
						missing.SecretBoundFields = append(missing.SecretBoundFields, secretBoundFieldLabel(ref.prefix, "header", h))
					}
				}
			}
		}
	}

	return missing, nil
}

func entryRequiresStaticOAuthCreds(entry v1.MCPServerCatalogEntry) bool {
	if entry.Spec.Manifest.RemoteConfig == nil || !entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		return false
	}
	return !entry.Status.OAuthCredentialConfigured
}

func secretBoundFieldLabel(prefix, kind string, h types.MCPHeader) string {
	key := h.Key
	if key == "" {
		key = h.Name
	}
	if key == "" {
		key = "<unknown>"
	}
	if prefix != "" {
		return fmt.Sprintf("component %s %s %s", prefix, kind, key)
	}
	return fmt.Sprintf("%s %s", kind, key)
}

func syncConnectServerRemoteConfigFromCatalogEntry(server *v1.MCPServer, entry v1.MCPServerCatalogEntry) bool {
	if server.Spec.Manifest.Runtime != types.RuntimeRemote || entry.Spec.Manifest.Runtime != types.RuntimeRemote || entry.Spec.Manifest.RemoteConfig == nil {
		return false
	}

	before := utils.Digest(server.Spec)
	entryRemote := entry.Spec.Manifest.RemoteConfig
	if server.Spec.Manifest.RemoteConfig == nil {
		server.Spec.Manifest.RemoteConfig = new(types.RemoteRuntimeConfig)
	}
	serverRemote := server.Spec.Manifest.RemoteConfig

	serverRemote.Headers = entryRemote.Headers
	serverRemote.StaticOAuthRequired = entryRemote.StaticOAuthRequired
	serverRemote.TunnelName = entryRemote.TunnelName
	switch {
	case entryRemote.Hostname != "":
		serverRemote.Hostname = entryRemote.Hostname
		serverRemote.IsTemplate = false
		serverRemote.URLTemplate = ""
		if serverRemote.URL == "" {
			server.Spec.NeedsURL = true
		} else if err := types.ValidateURLHostname(serverRemote.URL, entryRemote.Hostname); err != nil {
			server.Spec.NeedsURL = true
			server.Spec.PreviousURL = serverRemote.URL
			serverRemote.URL = ""
		} else {
			server.Spec.NeedsURL = false
			server.Spec.PreviousURL = ""
		}
	case entryRemote.URLTemplate != "":
		serverRemote.IsTemplate = true
		serverRemote.URLTemplate = entryRemote.URLTemplate
		serverRemote.Hostname = ""
		server.Spec.NeedsURL = serverRemote.URL == ""
		if !server.Spec.NeedsURL {
			server.Spec.PreviousURL = ""
		}
	}

	return before != utils.Digest(server.Spec)
}

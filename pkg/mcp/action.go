package mcp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var actionEnvVarRegex = regexp.MustCompile(`\${([^}]+)}`)

const requestTimeUpdateInterval = 15 * time.Minute

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
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: system.MCPServerInstancePrefix,
						Namespace:    server.Namespace,
					},
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
		addExtractedEnvVarsToCatalogEntry(&entry)

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
			if err := missingAdminConfig.err(id); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}

			allowMissingURL := catalogEntryRequiresUserURL(entry.Spec.Manifest)
			manifest, err := serverManifestFromCatalogEntryManifest(false, allowMissingURL, entry.Spec.Manifest, types.MCPServerManifest{})
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because it could not be converted to an MCP server: %v", id, err)
			}
			if err := ValidateServerManifest(ctx, manifest, false, ValidationOptions{
				RemoteMCPURLValidationConfig: sm.remoteURLValidationConfig,
				ResourceMaximums:             sm.resourceMaximums,
			}); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because its MCP server manifest is invalid: %v", id, err)
			}

			server := v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.MCPServerPrefix,
					Namespace:    system.DefaultNamespace,
				},
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
				server, err = sm.waitForCompositeReady(ctx, server, 30*time.Second)
				if err != nil {
					return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to wait for composite server to be ready: %w", err)
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

	addExtractedEnvVars(&server)

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

	tokenExchangeCred, err := sm.gatewayClient.RevealCredential(ctx, []string{server.Name}, server.Name)
	if err != nil {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to find token exchange credential: %w", err)
	}

	mergedEnv, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	serverConfig, missingConfig, err := ServerToServerConfig(server, instance.ValidConnectURLs(sm.baseURL), sm.baseURL, userID, scope, catalogName, mergedEnv, tokenExchangeCred.Secrets)
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

	addExtractedEnvVars(&server)

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
		tokenExchangeCred gatewaytypes.Credential
		tokenCredErr      error
	)
	if err = retry.OnError(kwait.Backoff{
		Steps:    10,
		Duration: 100 * time.Millisecond,
		Factor:   2.0,
		Jitter:   0.1,
	}, func(err error) bool {
		return errors.As(err, &gateway.CredentialNotFoundError{})
	}, func() error {
		tokenExchangeCred, tokenCredErr = sm.gatewayClient.RevealCredential(ctx, []string{server.Name}, server.Name)
		return tokenCredErr
	}); err != nil {
		return ServerConfig{}, nil, fmt.Errorf("failed to find token exchange credential: %w", tokenCredErr)
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

		serverConfig, missingConfig, err = CompositeServerToServerConfig(server, componentServers.Items, componentInstances.Items, server.ValidConnectURLs(sm.baseURL), sm.baseURL, userID, scope, catalogName, mergedEnv, tokenExchangeCred.Secrets)
		componentMissingConfig, componentErr := sm.compositeComponentsMissingConfig(ctx, userID, componentServers.Items, componentInstances.Items)
		if componentErr != nil {
			return ServerConfig{}, nil, componentErr
		}
		missingConfig = append(missingConfig, componentMissingConfig...)
	} else {
		serverConfig, missingConfig, err = ServerToServerConfig(server, server.ValidConnectURLs(sm.baseURL), sm.baseURL, userID, scope, catalogName, mergedEnv, tokenExchangeCred.Secrets)
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

func (sm *SessionManager) compositeComponentsMissingConfig(ctx context.Context, userID string, componentServers []v1.MCPServer, componentInstances []v1.MCPServerInstance) ([]string, error) {
	var missingConfig []string
	for _, component := range componentServers {
		_, componentMissingConfig, err := sm.serverConfigForAction(ctx, component, userID, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get config for component server %s: %w", component.Name, err)
		}
		for _, missing := range componentMissingConfig {
			missingConfig = append(missingConfig, fmt.Sprintf("%s: %s", component.Spec.MCPServerCatalogEntryName, missing))
		}
	}

	for _, instance := range componentInstances {
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
	if err := sm.storageClient.Status().Update(ctx, server); err != nil {
		log.Warnf("failed to update mcp server status: %v", err)
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
		if val != "" {
			headerNames = append(headerNames, header.Key)
			headerValues = append(headerValues, applyMCPServerInstanceHeaderPrefix(val, header.Prefix))
		} else if header.Required {
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

type missingCatalogEntryAdminConfig struct {
	SecretBoundFields []string
	StaticOAuth       bool
}

func (m missingCatalogEntryAdminConfig) err(entryID string) error {
	var parts []string
	if len(m.SecretBoundFields) > 0 {
		parts = append(parts, fmt.Sprintf("required Kubernetes Secret bindings are missing or empty for %s", strings.Join(m.SecretBoundFields, ", ")))
	}
	if m.StaticOAuth {
		parts = append(parts, "required static OAuth credentials have not been configured")
	}
	if len(parts) == 0 {
		return nil
	}
	return types.NewErrBadRequest("catalog entry %s cannot be connected because %s", entryID, strings.Join(parts, "; "))
}

func (sm *SessionManager) entryMissingAdminConfig(ctx context.Context, entry v1.MCPServerCatalogEntry) (missingCatalogEntryAdminConfig, error) {
	missing := missingCatalogEntryAdminConfig{
		StaticOAuth: entryRequiresStaticOAuthCreds(entry),
	}

	type manifestRef struct {
		prefix   string
		manifest types.MCPServerCatalogEntryManifest
	}

	m := entry.Spec.Manifest
	manifests := []manifestRef{{manifest: m}}
	if m.Runtime == types.RuntimeComposite {
		if m.CompositeConfig == nil {
			return missing, nil
		}
		manifests = nil
		for _, comp := range m.CompositeConfig.ComponentServers {
			if comp.MCPServerID != "" {
				continue
			}
			manifests = append(manifests, manifestRef{
				prefix:   comp.ComponentID(),
				manifest: comp.Manifest,
			})
		}
	}

	for _, ref := range manifests {
		cm := ref.manifest
		var remote *types.RemoteRuntimeConfig
		if cm.RemoteConfig != nil {
			remote = &types.RemoteRuntimeConfig{Headers: cm.RemoteConfig.Headers}
		}

		resolved, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, cm.Env, remote, nil, sm.secretBindingAllowedLabel)
		if err != nil {
			return missing, err
		}

		for _, e := range cm.Env {
			if e.Required && e.SecretBinding != nil {
				if _, ok := resolved[e.Key]; !ok {
					missing.SecretBoundFields = append(missing.SecretBoundFields, secretBoundFieldLabel(ref.prefix, "env", e.MCPHeader))
				}
			}
		}

		if cm.RemoteConfig != nil {
			for _, h := range cm.RemoteConfig.Headers {
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

func catalogEntryRequiresUserURL(manifest types.MCPServerCatalogEntryManifest) bool {
	if manifest.Runtime == types.RuntimeRemote &&
		manifest.RemoteConfig != nil &&
		(manifest.RemoteConfig.Hostname != "" || manifest.RemoteConfig.URLTemplate != "") {
		return true
	}
	if manifest.Runtime != types.RuntimeComposite || manifest.CompositeConfig == nil {
		return false
	}
	for _, component := range manifest.CompositeConfig.ComponentServers {
		if component.MCPServerID != "" {
			continue
		}
		if catalogEntryRequiresUserURL(component.Manifest) {
			return true
		}
	}
	return false
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

func serverManifestFromCatalogEntryManifest(isAdmin, disableHostnameValidation bool, entry types.MCPServerCatalogEntryManifest, input types.MCPServerManifest) (types.MCPServerManifest, error) {
	var result types.MCPServerManifest

	if entry.Runtime == types.RuntimeComposite {
		if entry.CompositeConfig == nil {
			return result, fmt.Errorf("composite config is required for composite runtime")
		}

		result = types.MCPServerManifest{
			Name:             entry.Name,
			Icon:             entry.Icon,
			ShortDescription: entry.ShortDescription,
			Description:      entry.Description,
			Metadata:         entry.Metadata,
			Runtime:          types.RuntimeComposite,
			ToolPreview:      entry.ToolPreview,
			Resources:        entry.Resources,
			CompositeConfig: &types.CompositeRuntimeConfig{
				ComponentServers: make([]types.ComponentServer, 0, len(entry.CompositeConfig.ComponentServers)),
			},
		}

		var inputConfig types.CompositeRuntimeConfig
		if input.CompositeConfig != nil {
			inputConfig = *input.CompositeConfig
		}

		inputComponents := make(map[string]types.ComponentServer, len(inputConfig.ComponentServers))
		for _, componentServer := range inputConfig.ComponentServers {
			if id := componentServer.ComponentID(); id != "" {
				inputComponents[id] = componentServer
			}
		}

		for _, entryComponent := range entry.CompositeConfig.ComponentServers {
			var (
				inputComponent = inputComponents[entryComponent.ComponentID()]
				userURL        string
			)

			entryHasStaticOAuth := entryComponent.Manifest.Runtime == types.RuntimeRemote &&
				entryComponent.Manifest.RemoteConfig != nil &&
				entryComponent.Manifest.RemoteConfig.StaticOAuthRequired
			inputHasStaticOAuth := inputComponent.Manifest.Runtime == types.RuntimeRemote &&
				inputComponent.Manifest.RemoteConfig != nil &&
				inputComponent.Manifest.RemoteConfig.StaticOAuthRequired
			if entryHasStaticOAuth && !inputHasStaticOAuth {
				return types.MCPServerManifest{}, types.NewErrBadRequest(
					"cannot update composite server: component %s has been updated to require static OAuth, which is not allowed in composite servers",
					entryComponent.ComponentID(),
				)
			}

			if entryComponent.Manifest.Runtime == types.RuntimeRemote &&
				entryComponent.Manifest.RemoteConfig != nil &&
				entryComponent.Manifest.RemoteConfig.Hostname != "" &&
				inputComponent.Manifest.RemoteConfig != nil {
				if url := inputComponent.Manifest.RemoteConfig.URL; url != "" && !strings.HasPrefix(url, "http") {
					inputComponent.Manifest.RemoteConfig.URL = "https://" + url
				}
				userURL = inputComponent.Manifest.RemoteConfig.URL
			}

			resultComponentManifest, err := types.MapCatalogEntryToServer(entryComponent.Manifest, userURL, inputComponent.Disabled || disableHostnameValidation)
			if err != nil {
				return types.MCPServerManifest{}, fmt.Errorf("failed to convert component manifest: %w", err)
			}

			result.CompositeConfig.ComponentServers = append(result.CompositeConfig.ComponentServers, types.ComponentServer{
				MCPServerID:    entryComponent.MCPServerID,
				CatalogEntryID: entryComponent.CatalogEntryID,
				ToolOverrides:  entryComponent.ToolOverrides,
				ToolPrefix:     entryComponent.ToolPrefix,
				Disabled:       inputComponent.Disabled,
				Manifest:       resultComponentManifest,
			})
		}
	} else {
		var userURL string
		if entry.Runtime == types.RuntimeRemote &&
			entry.RemoteConfig != nil &&
			entry.RemoteConfig.Hostname != "" &&
			input.RemoteConfig != nil {
			userURL = input.RemoteConfig.URL
		}

		var err error
		result, err = types.MapCatalogEntryToServer(entry, userURL, disableHostnameValidation)
		if err != nil {
			return types.MCPServerManifest{}, err
		}
	}

	if isAdmin {
		result = mergeMCPServerManifests(result, input)
	}

	return *result.DeepCopy(), nil
}

func mergeMCPServerManifests(existing, override types.MCPServerManifest) types.MCPServerManifest {
	if override.Name != "" {
		existing.Name = override.Name
	}
	if override.ShortDescription != "" {
		existing.ShortDescription = override.ShortDescription
	}
	if override.Description != "" {
		existing.Description = override.Description
	}
	if override.Icon != "" {
		existing.Icon = override.Icon
	}
	if len(override.Env) > 0 {
		existing.Env = override.Env
	}
	if override.Resources != nil {
		existing.Resources = override.Resources
	}
	if override.Runtime != "" {
		existing.Runtime = override.Runtime
	}
	if override.UVXConfig != nil {
		existing.UVXConfig = override.UVXConfig
	}
	if override.NPXConfig != nil {
		existing.NPXConfig = override.NPXConfig
	}
	if override.ContainerizedConfig != nil {
		existing.ContainerizedConfig = override.ContainerizedConfig
	}
	if override.RemoteConfig != nil {
		if existing.RemoteConfig == nil {
			existing.RemoteConfig = override.RemoteConfig
		} else {
			if override.RemoteConfig.URL != "" {
				existing.RemoteConfig.URL = override.RemoteConfig.URL
			}
			if len(override.RemoteConfig.Headers) > 0 {
				existing.RemoteConfig.Headers = override.RemoteConfig.Headers
			}
		}
	}

	return existing
}

func (sm *SessionManager) waitForCompositeReady(ctx context.Context, compositeServer v1.MCPServer, timeout time.Duration) (v1.MCPServer, error) {
	latest, err := wait.For(
		ctx,
		sm.storageClient,
		&compositeServer,
		func(cs *v1.MCPServer) (bool, error) {
			return cs.Spec.Manifest.CompositeConfig != nil &&
				len(cs.Spec.Manifest.CompositeConfig.ComponentServers) > 0 &&
				utils.Digest(cs.Spec.Manifest) == cs.Status.ObservedCompositeManifestHash, nil
		},
		wait.Option{
			Timeout: timeout,
		},
	)
	if err != nil {
		return compositeServer, err
	}

	return *latest, nil
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
	for _, env := range server.Spec.Manifest.Env {
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
				server.Spec.Manifest.Env = append(server.Spec.Manifest.Env, types.MCPEnv{
					MCPHeader: types.MCPHeader{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   true,
						Required:    true,
					},
				})
			}
		}
	}
}

func addExtractedEnvVarsToCatalogEntry(entry *v1.MCPServerCatalogEntry) {
	addExtractedEnvVarsToCatalogEntryManifest(&entry.Spec.Manifest)
}

func addExtractedEnvVarsToCatalogEntryManifest(manifest *types.MCPServerCatalogEntryManifest) {
	if manifest == nil {
		return
	}
	if manifest.Runtime == types.RuntimeComposite && manifest.CompositeConfig != nil {
		for i := range manifest.CompositeConfig.ComponentServers {
			addExtractedEnvVarsToCatalogEntryManifest(&manifest.CompositeConfig.ComponentServers[i].Manifest)
		}
		return
	}

	existing := make(map[string]struct{})
	for _, env := range manifest.Env {
		existing[env.Key] = struct{}{}
	}

	var toExtract []string
	switch manifest.Runtime {
	case types.RuntimeUVX:
		if manifest.UVXConfig != nil {
			toExtract = append(toExtract, manifest.UVXConfig.Command)
			if len(manifest.UVXConfig.Args) > 0 {
				toExtract = append(toExtract, manifest.UVXConfig.Args...)
			}
		}
	case types.RuntimeNPX:
		if manifest.NPXConfig != nil && len(manifest.NPXConfig.Args) > 0 {
			toExtract = append(toExtract, manifest.NPXConfig.Args...)
		}
	case types.RuntimeContainerized:
		if manifest.ContainerizedConfig != nil {
			toExtract = append(toExtract, manifest.ContainerizedConfig.Command)
			if len(manifest.ContainerizedConfig.Args) > 0 {
				toExtract = append(toExtract, manifest.ContainerizedConfig.Args...)
			}
		}
	case types.RuntimeRemote:
		if manifest.RemoteConfig != nil {
			for _, header := range manifest.RemoteConfig.Headers {
				existing[header.Key] = struct{}{}
			}
			toExtract = append(toExtract, manifest.RemoteConfig.URLTemplate)
		}
	}

	for _, v := range toExtract {
		for _, env := range extractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				if manifest.Runtime != types.RuntimeRemote {
					manifest.Env = append(manifest.Env, types.MCPEnv{
						MCPHeader: types.MCPHeader{
							Name:        env,
							Key:         env,
							Description: "Automatically detected variable",
							Sensitive:   true,
							Required:    true,
						},
					})
				} else if manifest.RemoteConfig != nil {
					manifest.RemoteConfig.Headers = append(manifest.RemoteConfig.Headers, types.MCPHeader{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   false,
						Required:    true,
					})
				}
			}
		}
	}
}

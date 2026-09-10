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
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	requestTimeUpdateInterval = 15 * time.Minute
)

var (
	actionEnvVarRegex = regexp.MustCompile(`\${([^}]+)}`)
)

type missingCatalogEntryAdminConfig struct {
	SecretBoundFields []string
	StaticOAuth       bool
}

// IDAndAudienceFromConnectURL returns the MCP server or instance name and audience based on the provided connect URL.
// The connect URL could have a vMCP ID, MCP server ID, server instance ID, or MCP catalog entry ID.
func (sm *SessionManager) IDAndAudienceFromConnectURL(ctx context.Context, id, userID string) (string, string, error) {
	if vmcp, _, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, id, userID); err != nil {
		return "", "", err
	} else if vmcp != nil {
		return id, id, nil
	}

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
	if vmcp, instance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, id, userID); err != nil {
		return "", v1.MCPServer{}, ServerConfig{}, nil, err
	} else if vmcp != nil {
		server, config, err := sm.serverForVMCPAction(ctx, id, userID, vmcp, instance)
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

func (sm *SessionManager) ServerForAction(ctx context.Context, id, userID string) (v1.MCPServer, ServerConfig, error) {
	if system.IsMCPServerInstanceID(id) {
		_, server, config, _, err := sm.serverForActionWithConnectID(ctx, id, userID, false)
		return server, config, err
	}
	if vmcp, instance, err := vmcpaccess.ResolveConnectID(ctx, sm.storageClient, id, userID); err != nil {
		return v1.MCPServer{}, ServerConfig{}, err
	} else if vmcp != nil {
		return sm.serverForVMCPAction(ctx, id, userID, vmcp, instance)
	}

	var server v1.MCPServer
	if err := sm.storageClient.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: id}, &server); err != nil {
		return server, ServerConfig{}, err
	}

	serverConfig, _, err := sm.serverConfigForAction(ctx, server, userID, false)
	return server, serverConfig, err
}

func (sm *SessionManager) serverForVMCPAction(ctx context.Context, id, userID string, vmcp *v1.VMCP, instance *v1.VMCPInstance) (v1.MCPServer, ServerConfig, error) {
	config, err := sm.serverConfigForVMCP(ctx, vmcp, instance, userID)
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
				instance := v1.MCPServerInstance{
					GenerateName: system.MCPServerInstancePrefix,
					Namespace:    server.Namespace,
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:             id,
						MCPCatalogName:            server.Spec.MCPCatalogID,
						MCPServerCatalogEntryName: server.Spec.MCPServerCatalogEntryName,
						PowerUserWorkspaceID:      server.Spec.PowerUserWorkspaceID,
						UserID:                    userID,
						Config:                    server.Spec.Manifest.UserConfig(),
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
		servers.Items = slices.DeleteFunc(servers.Items, func(server v1.MCPServer) bool {
			return server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != ""
		})
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
			resourceMaximums, err := sm.EffectiveKubernetesResourceMaximums(ctx, sm.storageClient)
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}
			if err := ValidateServerManifest(ctx, manifest, false, ValidationOptions{
				AllowMissingURL:              allowMissingURL,
				RemoteMCPURLValidationConfig: sm.remoteURLValidationConfig,
				ResourceMaximums:             resourceMaximums,
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

	mergedEnv, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, server.Spec.Manifest.Config, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return server, ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	serverConfig, missingConfig, err := ServerToServerConfig(server, instance.ValidConnectURLs(sm.baseURL), userID, scope, catalogName, mergedEnv)
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

	mergedEnv, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, server.Spec.Manifest.Config, cred.Secrets, sm.secretBindingAllowedLabel)
	if err != nil {
		return ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	catalogName, err := sm.catalogNameForServer(ctx, server, false)
	if err != nil {
		return ServerConfig{}, nil, err
	}

	serverConfig, missingConfig, err := ServerToServerConfig(server, server.ValidConnectURLs(sm.baseURL), userID, scope, catalogName, mergedEnv)
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
	if serverConfig.ComponentMCPServer || serverConfig.SystemMCPServer || serverConfig.IsAgentServer() || sm.webhookHelper == nil {
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

	manifest := entry.Spec.Manifest
	resolved, err := MergeBoundCreds(ctx, sm.localK8sClient, sm.obotNamespace, manifest.Config, nil, sm.secretBindingAllowedLabel)
	if err != nil {
		return missing, err
	}
	for _, config := range manifest.Config {
		if config.Required && config.SecretBinding != nil {
			if _, ok := resolved[config.Key]; !ok {
				kind := "env"
				if config.Usage == types.Header {
					kind = "header"
				}
				missing.SecretBoundFields = append(missing.SecretBoundFields, secretBoundFieldLabel("", kind, config.ToHeader()))
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

	server.Spec.Manifest.Config = entry.Spec.Manifest.Config
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

func serverManifestFromCatalogEntryManifest(isAdmin, disableHostnameValidation bool, entry types.MCPServerCatalogEntryManifest, input types.MCPServerManifest) (types.MCPServerManifest, error) {
	var userURL string
	if entry.Runtime == types.RuntimeRemote &&
		entry.RemoteConfig != nil &&
		entry.RemoteConfig.Hostname != "" &&
		input.RemoteConfig != nil {
		userURL = input.RemoteConfig.URL
	}

	result, err := types.MapCatalogEntryToServer(entry, userURL, disableHostnameValidation)
	if err != nil {
		return types.MCPServerManifest{}, err
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
	if len(override.Config) > 0 {
		existing.Config = override.Config
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
		}
	}

	return existing
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

func addExtractedEnvVarsToCatalogEntry(entry *v1.MCPServerCatalogEntry) {
	addExtractedEnvVarsToCatalogEntryManifest(&entry.Spec.Manifest)
}

func addExtractedEnvVarsToCatalogEntryManifest(manifest *types.MCPServerCatalogEntryManifest) {
	if manifest == nil {
		return
	}

	existing := make(map[string]struct{})
	for _, config := range manifest.Config {
		existing[config.Key] = struct{}{}
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
			toExtract = append(toExtract, manifest.RemoteConfig.URLTemplate)
		}
	}

	for _, v := range toExtract {
		for _, env := range extractEnvVars(v) {
			if _, exists := existing[env]; !exists {
				usage := types.Env
				sensitive := true
				if manifest.Runtime == types.RuntimeRemote {
					usage = types.Header
					sensitive = false
				}
				manifest.Config = append(manifest.Config, types.MCPConfig{
					Name:        env,
					Key:         env,
					Description: "Automatically detected variable",
					Sensitive:   sensitive,
					Required:    true,
					Usage:       usage,
				})
				existing[env] = struct{}{}
			}
		}
	}
}

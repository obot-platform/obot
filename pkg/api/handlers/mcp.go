package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	nahbackend "github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/wait"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	envVarRegex = regexp.MustCompile(`\${([^}]+)}`)
)

// MCPOAuthChecker will check the OAuth status for an MCP server. This interface breaks an import cycle.
type MCPOAuthChecker interface {
	CheckForMCPAuth(req api.Context, server v1.MCPServer, config mcp.ServerConfig, userID, mcpID, oauthAppAuthRequestID string) (string, error)
}

type MCPHandler struct {
	mcpSessionManager         *mcp.SessionManager
	mcpOAuthChecker           MCPOAuthChecker
	acrHelper                 *accesscontrolrule.Helper
	controllerBackend         nahbackend.Trigger
	mcpImagePullSecrets       []string
	mcpRuntimeBackend         string
	serverURL                 string
	secretBindingAllowedLabel string
	forceDynamicClient        bool
}

type urlTemplateConfigurationError struct {
	key string
}

func NewMCPHandler(mcpLoader *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, mcpOAuthChecker MCPOAuthChecker, controllerBackend nahbackend.Trigger, mcpImagePullSecrets []string, serverURL, secretBindingAllowedLabel string, forceDynamicClient bool) *MCPHandler {
	return &MCPHandler{
		mcpSessionManager:         mcpLoader,
		mcpOAuthChecker:           mcpOAuthChecker,
		acrHelper:                 acrHelper,
		controllerBackend:         controllerBackend,
		mcpImagePullSecrets:       mcpImagePullSecrets,
		mcpRuntimeBackend:         mcpLoader.MCPRuntimeBackend(),
		serverURL:                 serverURL,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
		forceDynamicClient:        forceDynamicClient,
	}
}

func validationOptions(remoteValidationConfig mcp.RemoteMCPURLValidationConfig) mcp.ValidationOptions {
	return mcp.ValidationOptions{
		RemoteMCPURLValidationConfig: remoteValidationConfig,
	}
}

// ValidationOptionsWithResourceMaximums builds MCP manifest validation options from startup and persisted settings.
func ValidationOptionsWithResourceMaximums(req api.Context, sessionManager *mcp.SessionManager) (mcp.ValidationOptions, error) {
	if sessionManager == nil {
		return mcp.ValidationOptions{}, nil
	}
	options := validationOptions(sessionManager.RemoteMCPURLValidationConfig())
	maximums, err := sessionManager.EffectiveKubernetesResourceMaximums(req.Context(), req.Storage)
	if err != nil {
		return mcp.ValidationOptions{}, err
	}
	options.ResourceMaximums = maximums
	return options, nil
}

func validateCatalogEntryManifestWithResourceMaximums(req api.Context, manifest types.MCPServerCatalogEntryManifest, gitManaged bool, sessionManager *mcp.SessionManager) error {
	options, err := ValidationOptionsWithResourceMaximums(req, sessionManager)
	if err != nil {
		return err
	}
	return mcp.ValidateCatalogEntryManifest(req.Context(), manifest, gitManaged, options)
}

func (m *MCPHandler) currentImagePullSecretNames(req api.Context) ([]string, error) {
	return mcp.CurrentImagePullSecretNames(req.Context(), req.Storage, m.mcpRuntimeBackend, m.mcpImagePullSecrets)
}

func (m *MCPHandler) currentK8sSettingsHash(req api.Context, settings v1.K8sSettingsSpec, mcpServer v1.MCPServer) (string, error) {
	imagePullSecretNames, err := m.currentImagePullSecretNames(req)
	if err != nil {
		return "", err
	}
	return m.currentK8sSettingsHashWithImagePullSecrets(settings, mcpServer, imagePullSecretNames)
}

func (m *MCPHandler) currentK8sSettingsHashWithImagePullSecrets(settings v1.K8sSettingsSpec, mcpServer v1.MCPServer, imagePullSecretNames []string) (string, error) {
	resources, err := mcp.CoreResourceRequirements(mcpServer.Spec.Manifest.Resources)
	if err != nil {
		return "", fmt.Errorf("failed to compute core resource requirements: %w", err)
	}
	return mcp.ComputeK8sSettingsHash(
		settings,
		resources,
		mcpServer.Spec.Manifest.Runtime,
		imagePullSecretNames,
	), nil
}

func (m *MCPHandler) GetEntryFromAllSources(req api.Context) error {
	var (
		entry v1.MCPServerCatalogEntry
		id    = req.PathValue("entry_id")
	)

	if err := req.Get(&entry, id); err != nil {
		return err
	}

	// Check if entry is from default catalog or workspace
	if entry.Spec.MCPCatalogName != system.DefaultCatalog && entry.Spec.PowerUserWorkspaceID == "" {
		return types.NewErrNotFound("MCP catalog entry not found")
	}

	return req.Write(ConvertMCPServerCatalogEntryWithWorkspace(entry, entry.Spec.PowerUserWorkspaceID, ""))
}

func (m *MCPHandler) ListEntriesFromAllSources(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}
	minimal, _ := strconv.ParseBool(req.URL.Query().Get("minimal"))

	convertEntry := func(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
		return convertMCPServerCatalogEntryForList(entry, entry.Spec.PowerUserWorkspaceID, "", minimal)
	}

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		entries := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
		for _, entry := range list.Items {
			entries = append(entries, convertEntry(entry))
		}
		return req.Write(types.MCPServerCatalogEntryList{Items: entries})
	}

	// Apply ACR filtering for regular users and for admins without ?all=true
	entries := make([]types.MCPServerCatalogEntry, 0, len(list.Items))
	for _, entry := range list.Items {
		var (
			err       error
			hasAccess bool
		)

		if entry.Spec.MCPCatalogName != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, entry.Name, entry.Spec.MCPCatalogName)
		} else if entry.Spec.PowerUserWorkspaceID != "" {
			hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, entry.Name, entry.Spec.PowerUserWorkspaceID)
		}
		if err != nil {
			return err
		}

		if hasAccess {
			// Hide entries that require OAuth credentials that haven't been configured (non-admins only).
			// Workspace owners can always see their own entries (they need to configure the OAuth credentials).
			if !req.UserIsAdmin() && entryRequiresStaticOAuthCreds(entry) {
				// Check if this is a workspace entry owned by the current user
				if entry.Spec.PowerUserWorkspaceID != system.GetPowerUserWorkspaceID(req.User.GetUID()) {
					// Either the entry is not in a workspace, or it's in a workspace not owned by the user. Omit it.
					continue
				}
			}
			entries = append(entries, convertEntry(entry))
		}
	}

	return req.Write(types.MCPServerCatalogEntryList{Items: entries})
}

func ConvertMCPServerCatalogEntry(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
	return ConvertMCPServerCatalogEntryWithWorkspace(entry, "", "")
}

func ConvertMCPServerCatalogEntryWithWorkspace(entry v1.MCPServerCatalogEntry, powerUserWorkspaceID, powerUserID string) types.MCPServerCatalogEntry {
	// Add extracted env vars directly to the entry
	addExtractedEnvVarsToCatalogEntry(&entry)

	return types.MCPServerCatalogEntry{
		Metadata:                  MetadataFrom(&entry),
		Manifest:                  entry.Spec.Manifest,
		Editable:                  entry.Spec.Editable,
		Detached:                  entry.Spec.Detached,
		CatalogName:               entry.Spec.MCPCatalogName,
		SourceURL:                 entry.Spec.SourceURL,
		UserCount:                 entry.Status.UserCount,
		LastUpdated:               v1.NewTime(entry.Status.LastUpdated),
		ToolPreviewsLastGenerated: v1.NewTime(entry.Status.ToolPreviewsLastGenerated),
		PowerUserWorkspaceID:      powerUserWorkspaceID,
		PowerUserID:               powerUserID,
		NeedsUpdate:               entry.Status.NeedsUpdate,
		OAuthCredentialConfigured: entry.Status.OAuthCredentialConfigured,
	}
}

func convertMCPServerCatalogEntryForList(entry v1.MCPServerCatalogEntry, powerUserWorkspaceID, powerUserID string, minimal bool) types.MCPServerCatalogEntry {
	if minimal {
		minimizeMCPServerCatalogEntryManifest(&entry.Spec.Manifest)
	}
	return ConvertMCPServerCatalogEntryWithWorkspace(entry, powerUserWorkspaceID, powerUserID)
}

func minimizeMCPServerCatalogEntryManifest(manifest *types.MCPServerCatalogEntryManifest) {
	manifest.Description = ""
	manifest.ToolPreview = nil
	manifest.RepoURL = ""
}

func (m *MCPHandler) ListServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var fieldSelector kclient.MatchingFields
	if catalogID != "" {
		fieldSelector = kclient.MatchingFields{
			"spec.mcpCatalogID": catalogID,
		}
	} else if workspaceID != "" {
		fieldSelector = kclient.MatchingFields{
			"spec.powerUserWorkspaceID": workspaceID,
		}
	} else {
		// List servers scoped to the user.
		fieldSelector = kclient.MatchingFields{
			"spec.userID": req.User.GetUID(),
		}
	}

	var servers v1.MCPServerList
	if err := req.List(&servers, fieldSelector); err != nil {
		return fmt.Errorf("failed to list MCP servers: %w", err)
	}

	credCtxs := make([]string, 0, len(servers.Items))
	for _, server := range servers.Items {
		credCtxs = append(credCtxs, server.CredentialContext(req.User.GetUID()))
	}

	creds, err := req.GatewayClient.ListCredentials(req.Context(), gateway.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		if _, ok := credMap[cred.Name]; !ok {
			c, err := req.GatewayClient.RevealCredential(req.Context(), []string{cred.Context}, cred.Name)
			if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			}
			credMap[cred.Name] = c.Secrets
		}
	}

	items := make([]types.MCPServer, 0, len(servers.Items))

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	bypassACRCheck := (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true"

	for _, server := range servers.Items {
		if server.Spec.Template || server.Spec.CompositeName != "" {
			continue
		}

		var (
			hasAccess bool
			err       error
		)

		if bypassACRCheck {
			// Admins/auditors with ?all=true can see all servers
			hasAccess = true
		} else if server.Spec.UserID == req.User.GetUID() {
			// If the server is owned by the current user, they have access to it
			hasAccess = true
		} else {
			// Apply ACR filtering for regular users and for admins without ?all=true
			if server.Spec.IsCatalogServer() {
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, server.Name, server.Spec.MCPCatalogID)
				if err != nil {
					return fmt.Errorf("failed to check access: %w", err)
				}
			} else if server.Spec.IsPowerUserWorkspaceServer() {
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, server.Name, server.Spec.PowerUserWorkspaceID)
				if err != nil {
					return fmt.Errorf("failed to check access: %w", err)
				}
			}
		}

		if !hasAccess {
			continue
		}

		// Add extracted env vars to the server definition
		addExtractedEnvVars(&server)

		if err := mcp.RefreshSecretBindingStatus(req.Context(), req.LocalK8sClient, req.ObotNamespace, &server, m.secretBindingAllowedLabel); err != nil {
			return fmt.Errorf("failed to resolve secret bindings for server %s: %w", server.Name, err)
		}
		converted := ConvertMCPServer(server, credMap[server.Name])
		items = append(items, converted)
	}

	return req.Write(types.MCPServerList{Items: items})
}

func (m *MCPHandler) GetServer(req api.Context) error {
	var (
		server      v1.MCPServer
		id          = req.PathValue("mcp_server_id")
		catalogID   = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(req.User.GetUID())}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	if err := mcp.RefreshSecretBindingStatus(req.Context(), req.LocalK8sClient, req.ObotNamespace, &server, m.secretBindingAllowedLabel); err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}
	converted := ConvertMCPServer(server, cred.Secrets)
	return req.Write(converted)
}

func (m *MCPHandler) LaunchServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if server.Spec.Manifest.Runtime == types.RuntimeVMCP {
		componentServers, err := m.aggregateComponentServersForAction(req, server, serverConfig)
		if err != nil {
			return err
		}

		for i, component := range componentServers {
			_, config, err := m.mcpSessionManager.ServerForAction(req.Context(), serverConfig.Components[i].ConnectID(), req.User)
			if err != nil {
				return fmt.Errorf("failed to get config for component server %s: %w", component.Name, err)
			}

			if config.Runtime != types.RuntimeRemote {
				_, err = m.mcpSessionManager.ListTools(req.Context(), config)
			}
			if err != nil {
				if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("Component MCP server %s is not healthy, check configuration for errors: %v", component.Name, err))
				}
				if errors.Is(err, mcp.ErrInsufficientCapacity) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, "Insufficient capacity to deploy MCP server. Please contact your administrator.")
				}
				if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
					return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
				}

				return fmt.Errorf("failed to launch component MCP server %s: %w", component.Name, err)
			}
		}

		return nil
	}

	if server.Spec.Manifest.Runtime != types.RuntimeRemote {
		_, err = m.mcpSessionManager.ListTools(req.Context(), serverConfig)
		if err != nil {
			if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
				return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
			}
			if errors.Is(err, mcp.ErrInsufficientCapacity) {
				return types.NewErrHTTP(http.StatusServiceUnavailable, "Insufficient capacity to deploy MCP server. Please contact your administrator.")
			}
			if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
				return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
			}
			return fmt.Errorf("failed to launch MCP server: %w", err)
		}
	}

	return nil
}

func (m *MCPHandler) CheckOAuth(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	needsOAuth, err := m.serverNeedsOAuth(req.Context(), &server, serverConfig)
	if err != nil {
		return err
	}
	if !needsOAuth && server.Spec.Manifest.Runtime == types.RuntimeVMCP {
		componentServers, err := m.aggregateComponentServersForAction(req, server, serverConfig)
		if err != nil {
			return err
		}
		for i := range componentServers {
			component := &componentServers[i]
			if component.Spec.Manifest.Runtime != types.RuntimeRemote {
				continue
			}
			_, componentConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), serverConfig.Components[i].ConnectID(), req.User)
			if err != nil {
				return fmt.Errorf("failed to load vMCP component server %s: %w", component.Name, err)
			}
			needsOAuth, err = m.serverNeedsOAuth(req.Context(), component, componentConfig)
			if err != nil {
				return err
			}
			if needsOAuth {
				break
			}
		}
	}
	if needsOAuth {
		req.WriteHeader(http.StatusPreconditionFailed)
	}

	return nil
}

func (m *MCPHandler) serverNeedsOAuth(ctx context.Context, server *v1.MCPServer, serverConfig mcp.ServerConfig) (bool, error) {
	if mcp.RequiresStaticOAuth(*server) {
		return true, nil
	}
	if serverConfig.Runtime != types.RuntimeRemote {
		return false, nil
	}

	if err := m.mcpSessionManager.PingServer(ctx, serverConfig); err != nil {
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return true, nil
		}
		return false, fmt.Errorf("failed to ping MCP server %s: %w", server.Name, err)
	}
	return false, nil
}

func (m *MCPHandler) GetOAuthURL(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	u, err := m.mcpOAuthChecker.CheckForMCPAuth(req, server, serverConfig, req.User.GetUID(), server.Name, "")
	if err != nil {
		return fmt.Errorf("failed to get OAuth URL: %w", err)
	}

	return req.Write(map[string]string{"oauthURL": u})
}

func (m *MCPHandler) GetTools(req api.Context) error {
	server, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return err
	}

	if caps.Tools == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
	}

	tools, err := toolsForServer(req.Context(), m.mcpSessionManager, server, serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return fmt.Errorf("failed to list tools: %w", err)
	}

	return req.Write(tools)
}

func (m *MCPHandler) GetResources(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	resources, err := m.mcpSessionManager.ListResources(req.Context(), serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list resources: %w", err)
	}

	return req.Write(resources)
}

func (m *MCPHandler) ReadResource(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	contents, err := m.mcpSessionManager.ReadResource(req.Context(), serverConfig, req.PathValue("resource_uri"))
	if err != nil {
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list resources: %w", err)
	}

	return req.Write(contents)
}

func (m *MCPHandler) GetPrompts(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return err
	}

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	prompts, err := m.mcpSessionManager.ListPrompts(req.Context(), serverConfig)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list prompts: %w", err)
	}

	return req.Write(prompts)
}

func (m *MCPHandler) GetPrompt(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return err
	}

	if caps.Prompts == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
	}

	var args map[string]string
	if err = req.Read(&args); err != nil {
		return fmt.Errorf("failed to read args: %w", err)
	}

	messages, description, err := m.mcpSessionManager.GetPrompt(req.Context(), serverConfig, req.PathValue("prompt_name"), args)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		}
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	return req.Write(map[string]any{
		"messages":    messages,
		"description": description,
	})
}

// validateServerScope checks that the catalog_id or workspace_id in the request URL matches the server.
// This prevents catalog- or workspace-scoped routes from operating on servers in a different scope.
func validateServerScope(req api.Context, server v1.MCPServer) error {
	if catalogID := req.PathValue("catalog_id"); catalogID != "" && server.Spec.MCPCatalogID != catalogID {
		return types.NewErrNotFound("MCP server %s not found", server.Name)
	}
	if workspaceID := req.PathValue("workspace_id"); workspaceID != "" && server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server %s not found", server.Name)
	}
	return nil
}

// mcpActionID lets MCPServer and vMCP routes share action handlers without sharing authorization.
func mcpActionID(req api.Context) string {
	return cmp.Or(req.PathValue("vmcp_id"), req.PathValue("mcp_server_id"))
}

func serverForActionWithCapabilities(req api.Context, mcpSessionManager *mcp.SessionManager) (v1.MCPServer, mcp.ServerConfig, *gomcp.ServerCapabilities, error) {
	server, serverConfig, err := mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User)
	if err != nil {
		return server, serverConfig, nil, err
	}

	caps, err := mcpSessionManager.ServerCapabilities(req.Context(), serverConfig)
	return server, serverConfig, caps, err
}

// aggregateComponentServersForAction resolves the component MCPServers used by
// a vMCP action. Component names come from the cached ServerConfig produced
// for the vMCP instance.
func (m *MCPHandler) aggregateComponentServersForAction(req api.Context, server v1.MCPServer, serverConfig mcp.ServerConfig) ([]v1.MCPServer, error) {
	if server.Spec.Manifest.Runtime == types.RuntimeVMCP {
		components := make([]v1.MCPServer, 0, len(serverConfig.Components))
		for _, component := range serverConfig.Components {
			if component.Name == "" {
				return nil, fmt.Errorf("vMCP %s contains a component without an MCP server", server.Name)
			}

			var componentServer v1.MCPServer
			if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
				Namespace: server.Namespace,
				Name:      component.Name,
			}, &componentServer); err != nil {
				return nil, fmt.Errorf("failed to get vMCP component server %s: %w", component.Name, err)
			}
			components = append(components, componentServer)
		}

		return components, nil
	}
	return nil, nil
}

func (m *MCPHandler) triggerMCPServerControllers(ctx context.Context, serverName string) error {
	if m.controllerBackend == nil {
		return fmt.Errorf("MCP server controller backend is not configured")
	}
	return m.controllerBackend.Trigger(ctx, v1.SchemeGroupVersion.WithKind("MCPServer"), serverName, 0)
}

func (e *urlTemplateConfigurationError) Error() string {
	return fmt.Sprintf("configuration value %q referenced by remoteConfig.urlTemplate is required", e.key)
}

// validateConfiguredOptions validates submitted env and header selections against their catalog-defined options.
func validateConfiguredOptions(config []types.MCPConfig, configured map[string]string) error {
	config = slices.DeleteFunc(slices.Clone(config), func(field types.MCPConfig) bool { return field.UserAllowed })
	missing, err := mcp.ValidateConfiguredOptions(config, configured)
	if err != nil {
		return err
	}
	if len(missing) > 0 {
		return fmt.Errorf("configuration %q requires a selection", missing[0])
	}
	return nil
}

// applyURLTemplate resolves submitted and static manifest values into a URL template.
func applyURLTemplate(templateStr string, envs []types.MCPConfig, configured map[string]string) (string, error) {
	values := make(map[string]string, len(configured)+len(envs))
	maps.Copy(values, configured)
	for _, env := range envs {
		if env.Value != "" {
			values[env.Key] = env.Value
		}
	}
	for _, key := range extractEnvVars(templateStr) {
		if values[key] == "" {
			return "", &urlTemplateConfigurationError{key: key}
		}
	}

	result := templateStr
	for key, value := range values {
		result = strings.ReplaceAll(result, fmt.Sprintf("${%s}", key), value)
	}
	return result, nil
}

// applyRemoteURLTemplate renders and validates a remote URL template before the server is used.
func applyRemoteURLTemplate(ctx context.Context, manifest *types.MCPServerManifest, envVars map[string]string, isMultiUser bool, options mcp.ValidationOptions) error {
	if manifest.Runtime != types.RuntimeRemote || manifest.RemoteConfig == nil || manifest.RemoteConfig.URLTemplate == "" {
		return nil
	}

	finalURL, err := applyURLTemplate(manifest.RemoteConfig.URLTemplate, manifest.Config, envVars)
	if err != nil {
		return fmt.Errorf("failed to apply URL template: %w", err)
	}

	manifest.RemoteConfig.URL = finalURL
	if err := mcp.ValidateServerManifest(ctx, *manifest, isMultiUser, options); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	return nil
}

func toolsForServer(ctx context.Context, mcpSessionManager *mcp.SessionManager, server v1.MCPServer, serverConfig mcp.ServerConfig) ([]types.MCPServerTool, error) {
	gTools, err := mcpSessionManager.ListTools(ctx, serverConfig)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return nil, types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
		} else if _, ok := errors.AsType[*mmmcp.AuthorizationError](err); ok {
			return nil, types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return nil, err
	}

	return mcp.ConvertTools(gTools, server.Spec.UnsupportedTools)
}

func extractEnvVars(text string) []string {
	if text == "" {
		return nil
	}

	matches := envVarRegex.FindAllStringSubmatch(text, -1)

	vars := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			vars = append(vars, match[1])
		}
	}

	return vars
}

// addExtractedEnvVars extracts and adds environment variables to the server definition
func addExtractedEnvVars(server *v1.MCPServer) {
	// Keep track of existing env vars in the spec to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range server.Spec.Manifest.Config {
		existing[env.Key] = struct{}{}
	}

	// Extract variables based on runtime type
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
					Name:        env,
					Key:         env,
					Description: "Automatically detected variable",
					Sensitive:   true,
					Required:    true,
					Usage:       types.Env,
				})
			}
		}
	}
}

// addExtractedEnvVarsToCatalogEntry extracts and adds environment variables to the catalog entry manifest
func addExtractedEnvVarsToCatalogEntry(entry *v1.MCPServerCatalogEntry) {
	addExtractedEnvVarsToCatalogEntryManifest(&entry.Spec.Manifest)
}

func addExtractedEnvVarsToCatalogEntryManifest(manifest *types.MCPServerCatalogEntryManifest) {
	if manifest == nil {
		return
	}
	// Keep track of existing env vars in the manifest to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range manifest.Config {
		existing[env.Key] = struct{}{}
	}

	// Extract variables based on runtime type
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
				if manifest.Runtime != types.RuntimeRemote {
					manifest.Config = append(manifest.Config, types.MCPConfig{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   true,
						Required:    true,
						Usage:       types.Env,
					})
				} else if manifest.RemoteConfig != nil {
					manifest.Config = append(manifest.Config, types.MCPConfig{
						Name:        env,
						Key:         env,
						Description: "Automatically detected variable",
						Sensitive:   false,
						Required:    true,
						Usage:       types.Header,
					})
				}
			}
		}
	}
}

func ConvertMCPServer(server v1.MCPServer, credEnv map[string]string) types.MCPServer {
	var missingEnvVars, missingHeaders []string

	for _, field := range server.Spec.Manifest.Config {
		if field.UserAllowed {
			continue
		}
		var missingRequired, invalidSelection bool
		if field.SecretBinding != nil {
			// The controller records bindings it cannot resolve. Callers refresh a status the
			// controller has not caught up with by calling mcp.RefreshSecretBindingStatus.
			missingRequired = field.Required && !field.Static && slices.Contains(server.Status.UnresolvedSecretBindings, field.Key)
		} else {
			configuredValue := credEnv[field.Key]
			missingRequired = field.Required && !field.Static && configuredValue == ""
			invalidSelection = configuredValue != "" && !mcp.ConfigurationOptionValueValid(field.ToHeader(), credEnv)
		}
		if missingRequired || invalidSelection {
			if field.Usage == types.Header {
				missingHeaders = append(missingHeaders, field.Key)
			} else {
				missingEnvVars = append(missingEnvVars, field.Key)
			}
		}
	}

	// Check if OAuth credentials are required but missing
	missingOAuth := false
	if server.Spec.Manifest.RemoteConfig != nil &&
		server.Spec.Manifest.RemoteConfig.StaticOAuthRequired {
		// Use the status field populated by the controller
		missingOAuth = !server.Status.OAuthCredentialConfigured
	}

	conditions := make([]types.DeploymentCondition, 0, len(server.Status.DeploymentConditions))
	for _, cond := range server.Status.DeploymentConditions {
		conditions = append(conditions, types.DeploymentCondition{
			Type:               string(cond.Type),
			Status:             string(cond.Status),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: *types.NewTime(cond.LastTransitionTime.Time),
			LastUpdateTime:     *types.NewTime(cond.LastUpdateTime.Time),
		})
	}

	converted := types.MCPServer{
		Metadata:                    MetadataFrom(&server),
		Alias:                       server.Spec.Alias,
		MissingRequiredEnvVars:      missingEnvVars,
		MissingRequiredHeaders:      missingHeaders,
		MissingOAuthCredentials:     missingOAuth,
		UserID:                      server.Spec.UserID,
		Configured:                  len(missingEnvVars) == 0 && len(missingHeaders) == 0 && !server.Spec.NeedsURL && !missingOAuth,
		MCPServerManifest:           server.Spec.Manifest,
		CatalogEntryID:              server.Spec.MCPServerCatalogEntryName,
		PowerUserWorkspaceID:        server.Spec.PowerUserWorkspaceID,
		MCPCatalogID:                server.Spec.MCPCatalogID,
		NeedsUpdate:                 server.Status.NeedsUpdate,
		NeedsK8sUpdate:              server.Status.NeedsK8sUpdate,
		NeedsURL:                    server.Spec.NeedsURL,
		PreviousURL:                 server.Spec.PreviousURL,
		MCPServerInstanceUserCount:  server.Status.MCPServerInstanceUserCount,
		DeploymentStatus:            server.Status.DeploymentStatus,
		DeploymentAvailableReplicas: server.Status.DeploymentAvailableReplicas,
		DeploymentReadyReplicas:     server.Status.DeploymentReadyReplicas,
		DeploymentReplicas:          server.Status.DeploymentReplicas,
		DeploymentConditions:        conditions,
		OAuthMetadata:               convertOAuthMetadata(server.Status.OAuthMetadata),
		K8sSettingsHash:             server.Status.K8sSettingsHash,
		Template:                    server.Spec.Template,
		CompositeName:               server.Spec.CompositeName,
		VMCPID:                      server.Spec.VMCPID,
		VMCPInstanceID:              server.Spec.VMCPInstanceID,
		VMCPComponentID:             server.Spec.VMCPComponentID,
	}

	if server.Spec.IsSingleUser() {
		converted.ServerUserType = types.ServerUserTypeSingleUser
	} else {
		converted.ServerUserType = types.ServerUserTypeMultiUser
	}

	return converted
}

func credentialEnvForMCPServer(req api.Context, server v1.MCPServer) (map[string]string, error) {
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return nil, fmt.Errorf("failed to find credential: %w", err)
	}

	return cred.Secrets, nil
}

func convertOAuthMetadata(metadata *v1.OAuthMetadata) *types.OAuthMetadata {
	if metadata == nil {
		return nil
	}

	registration := metadata.ClientRegistration.Raw
	if metadata.ClientIDMetadataDocumentSupported {
		registration = nil
	}

	return &types.OAuthMetadata{
		ProtectedResourceURL:              metadata.ProtectedResourceURL,
		AuthorizationServerURL:            metadata.AuthorizationServerURL,
		ProtectedResourceMetadata:         metadata.ProtectedResourceMetadata.Raw,
		AuthorizationServerMetadata:       metadata.AuthorizationServerMetadata.Raw,
		DynamicClientRegistration:         metadata.DynamicClientRegistration,
		ClientRegistration:                registration,
		ClientIDMetadataDocumentSupported: metadata.ClientIDMetadataDocumentSupported,
	}
}

func (m *MCPHandler) ListServersFromAllSources(req api.Context) error {
	var list v1.MCPServerList
	if err := req.List(&list, kclient.InNamespace(system.DefaultNamespace)); err != nil {
		return err
	}

	var allowedServers []v1.MCPServer

	// Allow admins/auditors to bypass ACR filtering with ?all=true
	if (req.UserIsAdmin() || req.UserIsAuditor()) && req.URL.Query().Get("all") == "true" {
		allowedServers = list.Items
	} else {
		// Apply ACR filtering for regular users and for admins without ?all=true
		for _, server := range list.Items {
			var (
				err       error
				hasAccess bool
			)

			if server.Spec.MCPCatalogID != "" {
				// Check default catalog servers
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerInCatalog(req.User, server.Name, server.Spec.MCPCatalogID)
			} else if server.Spec.PowerUserWorkspaceID != "" {
				// Check workspace-scoped servers
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerInWorkspace(req.User, server.Name, server.Spec.PowerUserWorkspaceID, server.Spec.UserID)
			}
			if err != nil {
				return err
			}

			if hasAccess {
				allowedServers = append(allowedServers, server)
			}
		}
	}

	var credCtxs []string
	for _, server := range allowedServers {
		credCtxs = append(credCtxs, server.CredentialContext(server.Spec.UserID))
	}

	creds, err := req.GatewayClient.ListCredentials(req.Context(), gateway.ListCredentialsOptions{
		CredentialContexts: credCtxs,
	})
	if err != nil {
		return fmt.Errorf("failed to list credentials: %w", err)
	}

	credMap := make(map[string]map[string]string, len(creds))
	for _, cred := range creds {
		if _, ok := credMap[cred.Name]; !ok {
			c, err := req.GatewayClient.RevealCredential(req.Context(), []string{cred.Context}, cred.Name)
			if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
				return fmt.Errorf("failed to find credential: %w", err)
			}
			credMap[cred.Name] = c.Secrets
		}
	}

	// Load catalog entries to enrich servers with tool previews
	var catalogEntries v1.MCPServerCatalogEntryList
	if err := req.List(&catalogEntries); err != nil {
		// Don't fail if we can't load catalog entries, just continue without previews
		slog.Error("failed to load catalog entries", "error", err)
	}

	catalogEntryMap := make(map[string]v1.MCPServerCatalogEntry, len(catalogEntries.Items))
	for _, entry := range catalogEntries.Items {
		catalogEntryMap[entry.Name] = entry
	}

	mcpServers := make([]types.MCPServer, 0, len(allowedServers))

	for _, server := range allowedServers {
		addExtractedEnvVars(&server)
		// Enrich with tool preview data if catalog entry exists
		if server.Spec.MCPServerCatalogEntryName != "" {
			entry := catalogEntryMap[server.Spec.MCPServerCatalogEntryName]
			// Add tool preview from catalog entry to server manifest
			server.Spec.Manifest.ToolPreview = entry.Spec.Manifest.ToolPreview
		}

		if err := mcp.RefreshSecretBindingStatus(req.Context(), req.LocalK8sClient, req.ObotNamespace, &server, m.secretBindingAllowedLabel); err != nil {
			return fmt.Errorf("failed to resolve secret bindings for server %s: %w", server.Name, err)
		}
		parent := ConvertMCPServer(server, credMap[server.Name])
		mcpServers = append(mcpServers, parent)
	}

	return req.Write(types.MCPServerList{Items: mcpServers})
}

func (m *MCPHandler) GetServerFromAllSources(req api.Context) error {
	var (
		server v1.MCPServer
		id     = req.PathValue("mcp_server_id")
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	if server.Spec.IsSingleUser() {
		return types.NewErrNotFound("MCP server not found")
	}

	// Get credential context based on server scoping
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	addExtractedEnvVars(&server)

	// Enrich with tool preview data if catalog entry exists
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err == nil {
			// Add tool preview from catalog entry to server manifest
			if entry.Spec.Manifest.ToolPreview != nil {
				server.Spec.Manifest.ToolPreview = entry.Spec.Manifest.ToolPreview
			}
		}
		// Don't fail if catalog entry is missing, just continue without preview
	}

	if err := mcp.RefreshSecretBindingStatus(req.Context(), req.LocalK8sClient, req.ObotNamespace, &server, m.secretBindingAllowedLabel); err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}
	return req.Write(ConvertMCPServer(server, cred.Secrets))
}

func (m *MCPHandler) ClearOAuthCredentials(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	mcpServerID := mcpActionID(req)

	if system.IsVMCPID(mcpServerID) {
		server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpServerID, req.User)
		if err != nil {
			return err
		}

		// vMCPs are synthetic MCP servers and intentionally have no catalog or
		// workspace scope. Keep the same scope check as the legacy endpoint so
		// scoped routes cannot address an unrelated vMCP.
		if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server not found")
		}

		componentServers, err := m.aggregateComponentServersForAction(req, server, serverConfig)
		if err != nil {
			return err
		}

		for i, component := range componentServers {
			if component.Spec.Manifest.Runtime != types.RuntimeRemote ||
				component.Spec.Manifest.RemoteConfig == nil {
				continue
			}

			componentServer, componentConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), serverConfig.Components[i].ConnectID(), req.User)
			if err != nil {
				return fmt.Errorf("failed to get config for vMCP component server %s: %w", component.Name, err)
			}
			if componentConfig.Runtime != types.RuntimeRemote {
				continue
			}
			componentURL := componentConfig.URL
			if componentURL == "" {
				componentURL = component.Spec.Manifest.RemoteConfig.URL
			}

			if err := req.GatewayClient.DeleteMCPOAuthTokenForURL(req.Context(), req.User.GetUID(), serverConfig.Components[i].ConnectID(), componentURL); err != nil {
				return fmt.Errorf("failed to delete OAuth credentials: %v", err)
			}

			if err := m.triggerMCPServerControllers(req.Context(), componentServer.Name); err != nil {
				return fmt.Errorf("failed to trigger MCP server reconciliation: %w", err)
			}
		}

		req.WriteHeader(http.StatusNoContent)
		return nil
	}

	var server v1.MCPServer
	if err := req.Get(&server, mcpServerID); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if server.Spec.Manifest.RemoteConfig != nil {
		if err := req.GatewayClient.DeleteMCPOAuthTokenForURL(req.Context(), req.User.GetUID(), server.Name, server.Spec.Manifest.RemoteConfig.URL); err != nil {
			return fmt.Errorf("failed to delete OAuth credentials: %v", err)
		}
	}

	if err := m.triggerMCPServerControllers(req.Context(), server.Name); err != nil {
		return fmt.Errorf("failed to trigger MCP server reconciliation: %w", err)
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

func (m *MCPHandler) GetServerDetails(req api.Context) error {
	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User)
	if err != nil {
		return err
	}

	if server.Spec.Template {
		return types.NewErrNotFound("MCP server not found")
	}

	if err := validateServerScope(req, server); err != nil {
		return err
	}

	if server.Spec.Manifest.Runtime == types.RuntimeRemote || server.Spec.Manifest.Runtime == types.RuntimeVMCP {
		return types.NewErrBadRequest("MCP server %s has runtime %s, which does not support details retrieval", server.Name, server.Spec.Manifest.Runtime)
	}

	if !req.UserIsAdmin() && !req.UserIsAuditor() {
		workspaceID := req.PathValue("workspace_id")
		if workspaceID == "" {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		} else if server.Spec.PowerUserWorkspaceID != "" && workspaceID != server.Spec.PowerUserWorkspaceID {
			return types.NewErrNotFound("MCP server %s not found", server.Name)
		} else if server.Spec.PowerUserWorkspaceID == "" {
			if server.Spec.MCPServerCatalogEntryName == "" {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}

			// In this case, the server should correspond to a workspace catalog entry.
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
				return fmt.Errorf("failed to get MCP server catalog entry: %v", err)
			}

			if entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}
		}
	}

	// Use the user ID from the server rather than from the request.
	serverConfig.UserID = server.Spec.UserID

	details, err := m.mcpSessionManager.GetServerDetails(req.Context(), serverConfig)
	if err != nil {
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	return req.Write(details)
}

func (m *MCPHandler) RestartServerDeployment(req api.Context) error {
	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User)
	if err != nil {
		return err
	}

	if err := validateServerScope(req, server); err != nil {
		return err
	}

	if server.Spec.Manifest.Runtime == types.RuntimeRemote || server.Spec.Manifest.Runtime == types.RuntimeVMCP {
		return types.NewErrBadRequest("MCP server %s has runtime %s, which does not support restart", server.Name, server.Spec.Manifest.Runtime)
	}

	if !req.UserIsAdmin() {
		// Allow users to restart their own single-user servers.
		userOwnsServer := server.Spec.IsOwnedBy(req.User.GetUID()) && server.Spec.IsSingleUser()
		if !userOwnsServer {
			// Fall back to workspace-based authorization
			workspaceID := req.PathValue("workspace_id")
			if workspaceID == "" {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			} else if server.Spec.PowerUserWorkspaceID != "" && workspaceID != server.Spec.PowerUserWorkspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			} else if server.Spec.PowerUserWorkspaceID == "" {
				if server.Spec.MCPServerCatalogEntryName == "" {
					return types.NewErrNotFound("MCP server %s not found", server.Name)
				}

				// In this case, the server should correspond to a workspace catalog entry.
				var entry v1.MCPServerCatalogEntry
				if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
					return fmt.Errorf("failed to get MCP server catalog entry: %v", err)
				}

				if entry.Spec.PowerUserWorkspaceID != workspaceID {
					return types.NewErrNotFound("MCP server %s not found", server.Name)
				}
			}
		}
	}

	if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	req.WriteHeader(http.StatusNoContent)
	return nil
}

// CheckK8sSettingsStatus checks if a server needs redeployment with new K8s settings
func (m *MCPHandler) CheckK8sSettingsStatus(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryID := req.PathValue("entry_id")

	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// Validate catalog/workspace membership
	// If entry_id is in the path, validate the server was created from that entry
	if entryID != "" {
		if server.Spec.MCPServerCatalogEntryName != entryID {
			return types.NewErrNotFound("MCP server not found")
		}

		// Get the entry and validate it's in the correct catalog/workspace
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, entryID); err != nil {
			return types.NewErrNotFound("MCP server not found")
		}

		// Validate the entry is in the correct catalog or workspace
		if entry.Spec.MCPCatalogName != catalogID || entry.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server not found")
		}
	} else if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		// Multi-user server was not in the specified catalog or workspace
		return types.NewErrNotFound("MCP server not found")
	}

	// Check if server has K8sSettingsHash in Status (only populated for Kubernetes runtime)
	deployedHash := server.Status.K8sSettingsHash
	if deployedHash == "" {
		return types.NewErrBadRequest("K8s settings check is only supported for Kubernetes runtime")
	}

	// Get current K8s settings
	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return err
	}

	currentHash, err := m.currentK8sSettingsHash(req, k8sSettings.Spec, server)
	if err != nil {
		return err
	}

	// Compare deployed hash with current hash
	needsUpdate := deployedHash != currentHash

	currentSettings, err := convertK8sSettings(k8sSettings)
	if err != nil {
		return err
	}

	status := types.K8sSettingsStatus{
		NeedsK8sUpdate:       needsUpdate,
		CurrentSettings:      &currentSettings,
		DeployedSettingsHash: deployedHash,
	}

	return req.Write(status)
}

// RedeployWithK8sSettings redeploys a server with the current K8s settings
func (m *MCPHandler) RedeployWithK8sSettings(req api.Context) error {
	if !mcp.IsKubernetesBackend(m.mcpRuntimeBackend) {
		return types.NewErrBadRequest("Redeployment with K8s settings is only supported for Kubernetes backend")
	}

	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	entryID := req.PathValue("entry_id")

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User)
	if err != nil {
		return err
	}

	// Validate catalog/workspace membership
	// If entry_id is in the path, validate the server was created from that entry
	if entryID != "" {
		if server.Spec.MCPServerCatalogEntryName != entryID {
			return types.NewErrNotFound("MCP server not found")
		}

		// Get the entry and validate it's in the correct catalog/workspace
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, entryID); err != nil {
			return types.NewErrNotFound("MCP server not found")
		}

		// Validate the entry is in the correct catalog or workspace
		if entry.Spec.MCPCatalogName != catalogID || entry.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server not found")
		}
	} else if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		// Multi-user server was not in the specified catalog or workspace
		return types.NewErrNotFound("MCP server not found")
	}

	// Check if server has K8sSettingsHash in Status
	deployedHash := server.Status.K8sSettingsHash

	// Get current K8s settings to compute current hash
	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return err
	}

	currentHash, err := m.currentK8sSettingsHash(req, k8sSettings.Spec, server)
	if err != nil {
		return err
	}
	hashDrift := deployedHash != currentHash

	// Trigger restart if hash drift OR if the server needs K8s update (e.g., PSA compliance)
	if hashDrift || server.Status.NeedsK8sUpdate {
		// Trigger restart to force redeployment with new settings
		if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
			if _, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
				return types.NewErrBadRequest("Restart is not supported by the current backend")
			}
			return fmt.Errorf("failed to redeploy server: %w", err)
		}

		// Wait for the redeployment to complete
		_, err := wait.For(req.Context(), req.Storage, &server, func(s *v1.MCPServer) (bool, error) {
			server = *s
			return !s.Status.NeedsK8sUpdate, nil
		})
		if err != nil {
			return fmt.Errorf("failed to wait for redeployment: %w", err)
		}
	}

	// Get credential for server
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	if err := mcp.RefreshSecretBindingStatus(req.Context(), req.LocalK8sClient, req.ObotNamespace, &server, m.secretBindingAllowedLabel); err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	// Return updated server
	return req.Write(ConvertMCPServer(server, cred.Secrets))
}

// ListServersNeedingK8sUpdateInCatalog lists all servers in a catalog that need redeployment with new K8s settings
func (m *MCPHandler) ListServersNeedingK8sUpdateInCatalog(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	if catalogID == "" {
		return types.NewErrBadRequest("catalog_id is required")
	}

	// Get current K8s settings to compute current hash
	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return fmt.Errorf("failed to get K8s settings: %w", err)
	}

	imagePullSecretNames, err := m.currentImagePullSecretNames(req)
	if err != nil {
		return err
	}

	// List all servers in the catalog
	var servers v1.MCPServerList
	if err := req.List(&servers, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	// Filter servers that need K8s updates and build lightweight response
	var serversNeedingUpdate []types.MCPServerNeedingK8sUpdate
	for _, server := range servers.Items {
		serverCatalogID := server.Spec.MCPCatalogID
		if serverCatalogID == "" && server.Spec.MCPServerCatalogEntryName != "" {
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err == nil {
				serverCatalogID = entry.Spec.MCPCatalogName
			}
		}

		if serverCatalogID != catalogID {
			continue
		}

		// Skip servers without K8s settings hash (non-K8s runtimes)
		if server.Status.K8sSettingsHash == "" {
			continue
		}

		// Check if hash differs from current settings
		currentHash, err := m.currentK8sSettingsHashWithImagePullSecrets(k8sSettings.Spec, server, imagePullSecretNames)
		if err != nil {
			return err
		}

		if server.Status.K8sSettingsHash != currentHash {
			serversNeedingUpdate = append(serversNeedingUpdate, types.MCPServerNeedingK8sUpdate{
				MCPServerID:             server.Name,
				MCPServerCatalogEntryID: server.Spec.MCPServerCatalogEntryName,
				PowerUserWorkspaceID:    server.Spec.PowerUserWorkspaceID,
			})
		}
	}

	return req.Write(types.MCPServersNeedingK8sUpdateList{Items: serversNeedingUpdate})
}

// ListServersNeedingK8sUpdateAcrossWorkspaces lists all servers across ALL workspaces that need redeployment with new K8s settings
func (m *MCPHandler) ListServersNeedingK8sUpdateAcrossWorkspaces(req api.Context) error {
	// Get current K8s settings to compute current hash
	var k8sSettings v1.K8sSettings
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.K8sSettingsName,
	}, &k8sSettings); err != nil {
		return fmt.Errorf("failed to get K8s settings: %w", err)
	}

	imagePullSecretNames, err := m.currentImagePullSecretNames(req)
	if err != nil {
		return err
	}

	// List all MCPServers (we'll filter for workspace servers below)
	var servers v1.MCPServerList
	if err := req.List(&servers, &kclient.ListOptions{
		Namespace: req.Namespace(),
	}); err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	// Filter servers that need K8s updates and build lightweight response
	var serversNeedingUpdate []types.MCPServerNeedingK8sUpdate
	for _, server := range servers.Items {
		// Determine workspace ID - check both server and its catalog entry
		workspaceID := server.Spec.PowerUserWorkspaceID

		// If server doesn't have workspace ID directly, check if it was created from a workspace catalog entry
		if workspaceID == "" && server.Spec.MCPServerCatalogEntryName != "" {
			var entry v1.MCPServerCatalogEntry
			if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err == nil {
				workspaceID = entry.Spec.PowerUserWorkspaceID
			}
			// Ignore error - entry might not exist or might not be accessible
		}

		// Only include servers that belong to a workspace (directly or via catalog entry)
		if workspaceID == "" {
			continue
		}

		// Skip servers without K8s settings hash (non-K8s runtimes)
		if server.Status.K8sSettingsHash == "" {
			continue
		}

		// Check if hash differs from current settings
		currentHash, err := m.currentK8sSettingsHashWithImagePullSecrets(k8sSettings.Spec, server, imagePullSecretNames)
		if err != nil {
			return err
		}

		if server.Status.K8sSettingsHash != currentHash {
			serversNeedingUpdate = append(serversNeedingUpdate, types.MCPServerNeedingK8sUpdate{
				MCPServerID:             server.Name,
				MCPServerCatalogEntryID: server.Spec.MCPServerCatalogEntryName,
				PowerUserWorkspaceID:    workspaceID,
			})
		}
	}

	return req.Write(types.MCPServersNeedingK8sUpdateList{Items: serversNeedingUpdate})
}

func (m *MCPHandler) StreamServerLogs(req api.Context) error {
	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User)
	if err != nil {
		return err
	}

	if err := validateServerScope(req, server); err != nil {
		return err
	}

	if serverConfig.Runtime == types.RuntimeRemote || serverConfig.Runtime == types.RuntimeVMCP {
		return types.NewErrBadRequest("MCP server %s has runtime %s, which does not support log retrieval", server.Name, serverConfig.Runtime)
	}

	// If this is a single-user MCP server that belongs to the user, then let them access the logs.
	if !server.Spec.IsOwnedBy(req.User.GetUID()) || !server.Spec.IsSingleUser() {
		// If the user doesn't own the server and is not an admin or auditor, check if they have access to the workspace.
		if !req.UserIsAdmin() && !req.UserIsAuditor() {
			workspaceID := req.PathValue("workspace_id")
			if workspaceID == "" {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			} else if server.Spec.PowerUserWorkspaceID != "" && workspaceID != server.Spec.PowerUserWorkspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			} else if server.Spec.PowerUserWorkspaceID == "" {
				if server.Spec.MCPServerCatalogEntryName == "" {
					return types.NewErrNotFound("MCP server %s not found", server.Name)
				}

				// In this case, the server should correspond to a workspace catalog entry.
				var entry v1.MCPServerCatalogEntry
				if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
					return fmt.Errorf("failed to get MCP server catalog entry: %v", err)
				}

				if entry.Spec.PowerUserWorkspaceID != workspaceID {
					return types.NewErrNotFound("MCP server %s not found", server.Name)
				}
			}
		}
	}

	// Use the user ID from the server rather than from the request.
	serverConfig.UserID = server.Spec.UserID

	logs, err := m.mcpSessionManager.StreamServerLogs(req.Context(), serverConfig)
	if err != nil {
		if nse, ok := errors.AsType[*mcp.ErrNotSupportedByBackend](err); ok {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	// Stream logs using the helper (handles SSE formatting, Docker header stripping, etc.)
	return StreamLogs(req.Context(), req.ResponseWriter, logs, StreamLogsOptions{
		SendKeepAlive:  true,
		SendDisconnect: true,
		SendEnded:      true,
	})
}

// ListServerInstances returns all instances for all servers within a specific catalog
func (m *MCPHandler) ListServerInstances(req api.Context) error {
	catalogID := req.PathValue("catalog_id")

	// Verify the catalog exists
	var catalog v1.MCPCatalog
	if err := req.Get(&catalog, catalogID); err != nil {
		return fmt.Errorf("failed to get catalog: %w", err)
	}

	// Get all servers in this catalog
	var serverList v1.MCPServerList
	if err := req.List(&serverList, kclient.MatchingFields{
		"spec.mcpCatalogID": catalogID,
	}); err != nil {
		return fmt.Errorf("failed to list servers in catalog: %w", err)
	}

	// Filter out template servers
	var catalogServers []v1.MCPServer
	for _, server := range serverList.Items {
		if !server.Spec.Template {
			catalogServers = append(catalogServers, server)
		}
	}

	// Get all instances for these catalog servers
	var allInstances v1.MCPServerInstanceList
	if err := req.List(&allInstances); err != nil {
		return fmt.Errorf("failed to list server instances: %w", err)
	}

	// Filter instances that belong to servers in this catalog
	var catalogServerNames = make(map[string]struct{})
	for _, server := range catalogServers {
		catalogServerNames[server.Name] = struct{}{}
	}

	var filteredInstances []v1.MCPServerInstance
	for _, instance := range allInstances.Items {
		if instance.Spec.Template || instance.Spec.CompositeName != "" {
			// Hide template and component instances
			continue
		}
		if _, exists := catalogServerNames[instance.Spec.MCPServerName]; exists {
			filteredInstances = append(filteredInstances, instance)
		}
	}

	// Convert instances to API types
	convertedInstances := make([]types.MCPServerInstance, 0, len(filteredInstances))
	for _, instance := range filteredInstances {
		credEnv, err := mcpServerInstanceCredEnv(req, instance)
		if err != nil {
			return err
		}

		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, credEnv))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

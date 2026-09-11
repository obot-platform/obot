package handlers

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/obot-platform/mmmcp"
	nahbackend "github.com/obot-platform/nah/pkg/backend"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	obottunnel "github.com/obot-platform/obot/pkg/tunnel"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/obot-platform/obot/pkg/wait"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	requestTimeUpdateInterval = 15 * time.Minute
	configURLKey              = "__url"
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

	// shutdownMCPServer is only injected for testing
	shutdownMCPServer func(string) error
}

type missingCatalogEntryAdminConfig struct {
	SecretBoundFields []string
	StaticOAuth       bool
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

func validateServerManifestWithResourceMaximums(req api.Context, manifest types.MCPServerManifest, isMultiUser bool, sessionManager *mcp.SessionManager) error {
	options, err := ValidationOptionsWithResourceMaximums(req, sessionManager)
	if err != nil {
		return err
	}
	return mcp.ValidateServerManifest(req.Context(), manifest, isMultiUser, options)
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
		mcpServer.Spec.NanobotAgentID != "",
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

	return req.Write(ConvertMCPServerCatalogEntryWithWorkspace(entry, entry.Spec.PowerUserWorkspaceID, "", m.serverURL))
}

func (m *MCPHandler) ListEntriesFromAllSources(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}
	minimal, _ := strconv.ParseBool(req.URL.Query().Get("minimal"))

	convertEntry := func(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
		return convertMCPServerCatalogEntryForList(entry, entry.Spec.PowerUserWorkspaceID, "", m.serverURL, minimal)
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

func ConvertMCPServerCatalogEntry(entry v1.MCPServerCatalogEntry, serverURL string) types.MCPServerCatalogEntry {
	return ConvertMCPServerCatalogEntryWithWorkspace(entry, "", "", serverURL)
}

func ConvertMCPServerCatalogEntryWithWorkspace(entry v1.MCPServerCatalogEntry, powerUserWorkspaceID, powerUserID, serverURL string) types.MCPServerCatalogEntry {
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
		ConnectURL:                defaultCatalogEntryConnectURL(serverURL, entry),
	}
}

func convertMCPServerCatalogEntryForList(entry v1.MCPServerCatalogEntry, powerUserWorkspaceID, powerUserID, serverURL string, minimal bool) types.MCPServerCatalogEntry {
	if minimal {
		minimizeMCPServerCatalogEntryManifest(&entry.Spec.Manifest)
	}
	return ConvertMCPServerCatalogEntryWithWorkspace(entry, powerUserWorkspaceID, powerUserID, serverURL)
}

func minimizeMCPServerCatalogEntryManifest(manifest *types.MCPServerCatalogEntryManifest) {
	manifest.Description = ""
	manifest.ToolPreview = nil
	manifest.RepoURL = ""
}

func defaultCatalogEntryConnectURL(serverURL string, entry v1.MCPServerCatalogEntry) string {
	if serverURL == "" {
		return ""
	}
	return system.MCPConnectURL(serverURL, entry.Name)
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

		slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
		if err != nil {
			return fmt.Errorf("failed to determine slug: %w", err)
		}

		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, credMap[server.Name], m.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings for server %s: %w", server.Name, err)
		}
		converted := ConvertMCPServer(server, mergedEnv, m.serverURL, slug)
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
	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	converted := ConvertMCPServer(server, mergedEnv, m.serverURL, slug)
	return req.Write(converted)
}

func (m *MCPHandler) DeleteServer(req api.Context) error {
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

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	// Prevent deletion of component servers that are part of a composite
	if server.Spec.CompositeName != "" {
		return types.NewErrForbidden(
			"cannot delete component of composite %q; delete the composite server instead",
			server.Spec.CompositeName,
		)
	}
	// Same for vMCPs
	if server.Spec.VMCPComponentID != "" {
		name := server.Spec.VMCPID
		if name == "" {
			name = server.Spec.VMCPInstanceID
		}
		return types.NewErrForbidden(
			"cannot delete component of vMCP %q; delete the vMCP server instead",
			name,
		)
	}

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(ConvertMCPServer(server, nil, m.serverURL, slug))
}

func (m *MCPHandler) LaunchServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User.GetUID())
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

		for _, component := range componentServers {
			_, config, err := m.mcpSessionManager.ServerForAction(req.Context(), component.Name, req.User.GetUID())
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

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User.GetUID())
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
			_, componentConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), component.Name, req.User.GetUID())
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

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User.GetUID())
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

func mcpServerOrInstanceFromConnectURL(req api.Context, id, secretBindingAllowedLabel string, validationOptions mcp.ValidationOptions) (v1.MCPServer, v1.MCPServerInstance, error) {
	switch {
	case system.IsMCPServerInstanceID(id):
		var instance v1.MCPServerInstance
		return v1.MCPServer{}, instance, req.Get(&instance, id)
	case system.IsMCPServerID(id):
		var server v1.MCPServer
		if err := req.Get(&server, id); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, err
		}

		if !server.Spec.IsSingleUser() {
			// This is a multi-user MCP server, and user is trying to connect to it.
			// List the MCP server instances, sort by creation time, and take the first one.
			var instances v1.MCPServerInstanceList
			if err := req.List(&instances, &kclient.ListOptions{
				FieldSelector: fields.SelectorFromSet(map[string]string{
					"spec.mcpServerName": id,
					"spec.userID":        req.User.GetUID(),
					"spec.template":      "false",
					"spec.compositeName": "",
				}),
			}); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}
			if len(instances.Items) == 0 {
				// If none exist, then create one for the user.
				instance := v1.MCPServerInstance{
					GenerateName: system.MCPServerInstancePrefix,
					Namespace:    server.Namespace,
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:             id,
						MCPCatalogName:            server.Spec.MCPCatalogID,
						MCPServerCatalogEntryName: server.Spec.MCPServerCatalogEntryName,
						PowerUserWorkspaceID:      server.Spec.PowerUserWorkspaceID,
						UserID:                    principal.ResourceOwnerID(req.User),
						Config:                    server.Spec.Manifest.UserConfig(),
					},
				}
				if err := req.Create(&instance); err != nil {
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
		// In this case, id refers to a catalog entry.
		// Get the catalog entry to make sure it's valid
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, id); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrNotFound("catalog entry %s not found", id)
		}
		addExtractedEnvVarsToCatalogEntry(&entry)

		// List the MCP servers for the user and take the first one.
		var servers v1.MCPServerList
		if err := req.List(&servers, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.mcpServerCatalogEntryName": id,
				"spec.userID":                    req.User.GetUID(),
				"spec.template":                  "false",
				"spec.compositeName":             "",
			}),
		}); err != nil {
			return v1.MCPServer{}, v1.MCPServerInstance{}, err
		}
		servers.Items = slices.DeleteFunc(servers.Items, func(server v1.MCPServer) bool {
			return server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != ""
		})
		if len(servers.Items) == 0 {
			// If the user has not configured an MCP server for the catalog entry, create a server for the user.
			missingAdminConfig, err := entryMissingAdminConfig(req.Context(), req.LocalK8sClient, req.ObotNamespace, entry, secretBindingAllowedLabel)
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to determine required admin configuration for catalog entry %s: %w", id, err)
			}
			if err := missingAdminConfig.err(id); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, err
			}

			// Convert the catalog entry manifest to a server manifest. Treat the user as non-admin always.
			allowMissingURL := catalogEntryRequiresUserURL(entry.Spec.Manifest)
			manifest, err := serverManifestFromCatalogEntryManifest(false, allowMissingURL, entry.Spec.Manifest, types.MCPServerManifest{})
			if err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because it could not be converted to an MCP server: %v", id, err)
			}
			if err := mcp.ValidateServerManifest(req.Context(), manifest, false, validationOptions); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because its MCP server manifest is invalid: %v", id, err)
			}
			if err := obottunnel.ValidateServerTunnelReferences(req.Context(), req.Storage, manifest); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because its tunnel configuration is invalid: %v", id, err)
			}

			// Create a new MCP server for the user.
			server := v1.MCPServer{
				GenerateName: system.MCPServerPrefix,
				Namespace:    req.Namespace(),
				Spec: v1.MCPServerSpec{
					Manifest:                  manifest,
					UnsupportedTools:          entry.Spec.UnsupportedTools,
					MCPServerCatalogEntryName: id,
					UserID:                    req.User.GetUID(),
					NeedsURL:                  allowMissingURL && (manifest.RemoteConfig == nil || manifest.RemoteConfig.URL == ""),
				},
			}
			if err := req.Create(&server); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to create MCP server for catalog entry %s: %w", id, err)
			}

			servers.Items = append(servers.Items, server)
		}

		slices.SortFunc(servers.Items, func(a, b v1.MCPServer) int {
			return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		})

		server := servers.Items[0]
		if syncConnectServerRemoteConfigFromCatalogEntry(&server, entry) {
			if err := req.Update(&server); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to update MCP server configuration from catalog entry %s: %w", id, err)
			}
		}

		return server, v1.MCPServerInstance{}, nil
	}
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

func entryMissingAdminConfig(ctx context.Context, client kclient.Client, obotNamespace string, entry v1.MCPServerCatalogEntry, secretBindingAllowedLabel string) (missingCatalogEntryAdminConfig, error) {
	missing := missingCatalogEntryAdminConfig{
		StaticOAuth: entryRequiresStaticOAuthCreds(entry),
	}

	type manifestRef struct {
		prefix   string
		manifest types.MCPServerCatalogEntryManifest
	}

	manifests := []manifestRef{{manifest: entry.Spec.Manifest}}
	for _, ref := range manifests {
		cm := ref.manifest
		missingBindings, err := mcp.MissingSecretBindings(ctx, client, obotNamespace, cm.Config, secretBindingAllowedLabel)
		if err != nil {
			return missing, err
		}
		for _, binding := range missingBindings {
			missing.SecretBoundFields = append(missing.SecretBoundFields, secretBoundFieldLabel(ref.prefix, binding.Kind, binding.Header))
		}
	}

	return missing, nil
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

	server.Spec.Manifest.Config = slices.Clone(entry.Spec.Manifest.Config)
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
	server, serverConfig, err := mcpSessionManager.ServerForAction(req.Context(), mcpActionID(req), req.User.GetUID())
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

// serverManifestFromCatalogEntryManifest converts a catalog entry manifest to a server manifest.
// If the user is an admin, they can override anything from the catalog entry.
func serverManifestFromCatalogEntryManifest(
	isAdmin bool,
	disableHostnameValidation bool,
	entry types.MCPServerCatalogEntryManifest,
	input types.MCPServerManifest,
) (types.MCPServerManifest, error) {
	var userURL string
	if entry.Runtime == types.RuntimeRemote && entry.RemoteConfig != nil && entry.RemoteConfig.Hostname != "" && input.RemoteConfig != nil {
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

	// Merge runtime-specific configurations
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

// applySecretBindingOverlay copies admin-selected secret bindings from the request
// onto matching template fields while preserving the template-owned runtime shape.
func applySecretBindingOverlay(manifest types.MCPServerManifest, overlay types.MCPServerManifest) types.MCPServerManifest {
	bindings := secretBindingsByConfig(overlay.Config, false)
	for i := range manifest.Config {
		if binding := bindings[manifest.Config[i].Key]; binding != nil {
			manifest.Config[i].SecretBinding = binding
			manifest.Config[i].Value = ""
		}
	}

	return manifest
}

func rejectCatalogSecretBindingOverrides(manifest types.MCPServerManifest, source *types.MCPServerCatalogEntryManifest, requirePinnedFields bool) *types.ErrHTTP {
	if source == nil {
		return nil
	}

	// Include nil bindings so a present field with no binding is treated as an
	// attempt to clear a catalog-owned binding. Omitted fields are allowed only
	// for partial deploy-time overlays, not full update payloads.
	bindings := secretBindingsByConfig(manifest.Config, true)
	for _, field := range source.Config {
		if field.SecretBinding == nil {
			continue
		}
		binding, ok := bindings[field.Key]
		if !ok {
			if requirePinnedFields {
				return types.NewErrBadRequest("%s %q: cannot omit catalog entry secretBinding", field.Usage, field.Key)
			}
			continue
		}
		if !sameSecretBinding(field.SecretBinding, binding) {
			return types.NewErrBadRequest("%s %q: cannot override catalog entry secretBinding", field.Usage, field.Key)
		}
	}

	return nil
}

// markAdminAddedSecretBindings derives server-owned AdminAdded metadata from the
// source catalog entry instead of trusting values supplied by UI or API clients.
func markAdminAddedSecretBindings(manifest *types.MCPServerManifest, source *types.MCPServerCatalogEntryManifest) {
	var sourceBindings map[string]*types.MCPSecretBinding
	if source != nil {
		sourceBindings = secretBindingsByConfig(source.Config, false)
	}
	for _, field := range manifest.Config {
		markAdminAddedSecretBinding(field.SecretBinding, sourceBindings[field.Key])
	}
}

func markAdminAddedSecretBinding(binding, sourceBinding *types.MCPSecretBinding) {
	if binding == nil {
		return
	}
	binding.AdminAdded = !sameSecretBinding(sourceBinding, binding)
}

func secretBindingsByConfig(fields []types.MCPConfig, includeNil bool) map[string]*types.MCPSecretBinding {
	bindings := make(map[string]*types.MCPSecretBinding, len(fields))
	for _, field := range fields {
		if includeNil || field.SecretBinding != nil {
			bindings[field.Key] = field.SecretBinding
		}
	}
	return bindings
}

func sameSecretBinding(a, b *types.MCPSecretBinding) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name && a.Key == b.Key
}

func (m *MCPHandler) CreateServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var input types.MCPServer
	if err := req.Read(&input); err != nil {
		return err
	}

	if input.MCPServerManifest.RemoteConfig != nil && !strings.HasPrefix(input.MCPServerManifest.RemoteConfig.URL, "http") {
		input.MCPServerManifest.RemoteConfig.URL = "https://" + input.MCPServerManifest.RemoteConfig.URL
	}

	server := v1.MCPServer{
		GenerateName: system.MCPServerPrefix,
		Namespace:    req.Namespace(),
		Finalizers:   []string{v1.MCPServerFinalizer},
		Spec: v1.MCPServerSpec{
			Alias:                     input.Alias,
			MCPServerCatalogEntryName: input.CatalogEntryID,
			UserID:                    req.User.GetUID(),
		},
	}

	if catalogID != "" {
		var catalog v1.MCPCatalog
		if err := req.Get(&catalog, catalogID); err != nil {
			return err
		}

		server.Spec.MCPCatalogID = catalogID
	} else if workspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := req.Get(&workspace, workspaceID); err != nil {
			return err
		}

		server.Spec.PowerUserWorkspaceID = workspaceID
	}

	var gitManagedEntry bool
	var sourceCatalogEntryManifest *types.MCPServerCatalogEntryManifest
	if input.CatalogEntryID != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, input.CatalogEntryID); err != nil {
			return err
		}
		sourceCatalogEntryManifest = catalogEntry.Spec.Manifest.DeepCopy()

		// Validate that the catalog entry type is compatible with the route used.
		if err := mcp.ValidateCatalogEntryForRoute(catalogEntry.Spec.Manifest, catalogID, workspaceID); err != nil {
			return types.NewErrBadRequest("%v", err)
		}

		// Verify the entry is visible from this route scope. Workspace routes can deploy
		// global catalog entries, so this intentionally uses visibility validation.
		if err := validateEntryVisibleFromScope(catalogEntry, catalogID, workspaceID); err != nil {
			return err
		}

		// POST /api/mcp-catalogs/{catalog_id}/servers is admin-only and skips per-entry ACR.
		// POST /api/mcp-servers and POST /api/workspaces/{workspace_id}/servers must check ACR
		// because the catalog entry ID comes from the request body and authz middleware cannot
		// validate per-entry permissions.
		if catalogID == "" {
			var (
				err       error
				hasAccess bool
			)

			if catalogEntry.Spec.MCPCatalogName != "" {
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInCatalog(req.User, catalogEntry.Name, catalogEntry.Spec.MCPCatalogName)
			} else if catalogEntry.Spec.PowerUserWorkspaceID != "" {
				hasAccess, err = m.acrHelper.UserHasAccessToMCPServerCatalogEntryInWorkspace(req.Context(), req.User, catalogEntry.Name, catalogEntry.Spec.PowerUserWorkspaceID)
			}
			if err != nil {
				return err
			}

			if !hasAccess {
				return types.NewErrForbidden("user does not have access to MCP server catalog entry")
			}
		}

		// Block server creation if OAuth is required but not configured
		if entryRequiresStaticOAuthCreds(catalogEntry) {
			return types.NewErrBadRequest("catalog entry requires OAuth configuration by an administrator before it can be used")
		}

		// Catalog entries no longer declare a sharing mode. They are mapped to the
		// legacy server manifest and administrators may still override that result.
		manifest, err := serverManifestFromCatalogEntryManifest(req.UserIsAdmin(), false, catalogEntry.Spec.Manifest, input.MCPServerManifest)
		if err != nil {
			return err
		}
		if err := mcp.ValidateCatalogConfigurationConstraints(manifest, catalogEntry.Spec.Manifest); err != nil {
			return types.NewErrBadRequest("invalid catalog configuration: %v", err)
		}

		server.Spec.Manifest = manifest
		server.Spec.UnsupportedTools = catalogEntry.Spec.UnsupportedTools
		gitManagedEntry = catalogEntry.IsGitManaged()
	} else if req.UserIsAdmin() || workspaceID != "" {
		// If the user is an admin, or if this server is being created in a workspace by a PowerUserPlus,
		// they can create a server with a manifest that is not in the catalog.
		server.Spec.Manifest = input.MCPServerManifest
		if mcp.ManifestHasConfigurationOptions(server.Spec.Manifest) {
			return types.NewErrBadRequest("configuration options may only be defined by a source catalog entry")
		}
	} else {
		return types.NewErrBadRequest("catalogEntryID is required")
	}

	if err := validateServerManifestWithResourceMaximums(req, server.Spec.Manifest, !server.Spec.IsSingleUser(), m.mcpSessionManager); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if err := obottunnel.ValidateServerTunnelReferences(req.Context(), req.Storage, server.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	adminManagedSecretBindings := req.UserIsAdmin() && server.Spec.IsCatalogServer()
	if adminManagedSecretBindings {
		markAdminAddedSecretBindings(&server.Spec.Manifest, sourceCatalogEntryManifest)
	}
	if err := mcp.ValidateSecretBindings(server.Spec.Manifest, gitManagedEntry, adminManagedSecretBindings, m.mcpRuntimeBackend); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	addExtractedEnvVars(&server)
	if adminManagedSecretBindings && !server.Spec.IsSingleUser() {
		if err := mcp.ValidateSecretBindingsAvailable(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, m.secretBindingAllowedLabel); err != nil {
			return types.NewErrBadRequest("validation failed: %v", err)
		}
	}
	// Run after extraction so auto-created Required=true entries cover any
	// template references the user did not pre-declare. This still catches the
	// case where the user pre-supplied a matching env entry with required=false
	// (which would otherwise ship a literal "${VAR}" string at runtime).
	if err := mcp.ValidateTemplateReferences(server.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if err := req.Create(&server); err != nil {
		return err
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(req.User.GetUID())}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.WriteCreated(ConvertMCPServer(server, mergedEnv, m.serverURL, slug))
}

// UpdateServer updates the manifest of an MCPServer.
// This can only be used by the admin (for things in the default catalog) and PowerUserPlusses, for things in their workspaces.
func (m *MCPHandler) UpdateServer(req api.Context) error {
	var (
		id          = req.PathValue("mcp_server_id")
		catalogID   = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		err         error
		updated     types.MCPServerManifest
		existing    v1.MCPServer
	)

	if err := req.Get(&existing, id); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if existing.Spec.MCPCatalogID != catalogID || existing.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if existing.Spec.VMCPComponentID != "" {
		return types.NewErrBadRequest("cannot update a server that is bound to a vMCP component")
	}

	if err = req.Read(&updated); err != nil {
		return err
	}
	if updated.RemoteConfig != nil && !strings.HasPrefix(updated.RemoteConfig.URL, "http") {
		updated.RemoteConfig.URL = "https://" + updated.RemoteConfig.URL
	}

	// Shutdown any server that is using the default credentials.
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{existing.CredentialContext(req.User.GetUID())}, existing.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	if err := validateServerManifestWithResourceMaximums(req, updated, !existing.Spec.IsSingleUser(), m.mcpSessionManager); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if existing.Spec.MCPServerCatalogEntryName != "" {
		if err := mcp.ValidateCatalogConfigurationConstraints(updated, existing.Spec.Manifest.ConvertToCatalogEntry()); err != nil {
			return types.NewErrBadRequest("invalid catalog configuration: %v", err)
		}
	} else if mcp.ManifestHasConfigurationOptions(updated) {
		return types.NewErrBadRequest("configuration options may only be defined by a source catalog entry")
	}
	if err := obottunnel.ValidateServerTunnelReferences(req.Context(), req.Storage, updated); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	var (
		gitManagedEntry            bool
		sourceCatalogEntryManifest *types.MCPServerCatalogEntryManifest
	)
	if existing.Spec.MCPServerCatalogEntryName != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, existing.Spec.MCPServerCatalogEntryName); err == nil {
			gitManagedEntry = catalogEntry.IsGitManaged()
			sourceCatalogEntryManifest = catalogEntry.Spec.Manifest.DeepCopy()
		}
	}
	adminManagedSecretBindings := req.UserIsAdmin() && existing.Spec.IsCatalogServer()
	if adminManagedSecretBindings {
		if err := rejectCatalogSecretBindingOverrides(updated, sourceCatalogEntryManifest, true); err != nil {
			return err
		}
		markAdminAddedSecretBindings(&updated, sourceCatalogEntryManifest)
	}
	if err := mcp.ValidateSecretBindings(updated, gitManagedEntry, adminManagedSecretBindings, m.mcpRuntimeBackend); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if err := mcp.ValidateTemplateReferences(updated); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if adminManagedSecretBindings && !existing.Spec.IsSingleUser() {
		updatedServer := existing
		updatedServer.Spec.Manifest = updated
		addExtractedEnvVars(&updatedServer)
		if err := mcp.ValidateSecretBindingsAvailable(req.Context(), req.LocalK8sClient, req.ObotNamespace, updatedServer.Spec.Manifest.Config, m.secretBindingAllowedLabel); err != nil {
			return types.NewErrBadRequest("validation failed: %v", err)
		}
	}

	// Shutdown the server only after the candidate configuration is known to be valid.
	if err := m.removeMCPServer(req.Context(), existing); err != nil {
		return err
	}

	// Use retry.RetryOnConflict because controllers (e.g. DetectK8sSettingsDrift,
	// UpdateMCPServerStatus) can update this MCPServer concurrently, bumping the
	// ResourceVersion between our read and write.
	if err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		if err := req.Get(&existing, id); err != nil {
			return err
		}

		// Re-validate catalog/workspace membership after re-fetch, since a controller
		// may have changed these fields between the initial check and this retry.
		if existing.Spec.MCPCatalogID != catalogID || existing.Spec.PowerUserWorkspaceID != workspaceID {
			return types.NewErrNotFound("MCP server not found")
		}

		existing.Spec.Manifest = updated
		addExtractedEnvVars(&existing)
		return req.Update(&existing)
	}); err != nil {
		return err
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, existing, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, existing.Spec.Manifest.Config, cred.Secrets, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	return req.Write(ConvertMCPServer(existing, mergedEnv, m.serverURL, slug))
}

func (m *MCPHandler) UpdateServerAlias(req api.Context) error {
	var (
		id          = req.PathValue("mcp_server_id")
		catalogID   = req.PathValue("catalog_id")
		workspaceID = req.PathValue("workspace_id")
		server      v1.MCPServer
	)

	if err := req.Get(&server, id); err != nil {
		return err
	}

	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	var input struct {
		Alias string `json:"alias,omitempty"`
	}
	if err := req.Read(&input); err != nil {
		return err
	}

	if input.Alias == server.Spec.Alias {
		// If the alias is the same, skip update.
		return nil
	}
	server.Spec.Alias = input.Alias

	if err := req.Update(&server); err != nil {
		return err
	}

	return nil
}

func (m *MCPHandler) ConfigureServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if mcpServer.Spec.MCPCatalogID != catalogID || mcpServer.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		return types.NewErrBadRequest("composite servers are no longer supported; use the migrated vMCP instance")
	}

	if mcpServer.Spec.VMCPComponentID != "" {
		return types.NewErrBadRequest("cannot configure a server associated to a vMCP component")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
	}
	if err := validateConfiguredOptions(mcpServer.Spec.Manifest.Config, envVars); err != nil {
		return types.NewErrBadRequest("invalid configuration: %v", err)
	}

	// Check if this server is from a catalog and has a URL template that needs to be processed.
	// URL templates may only reference user-supplied env vars. References to secret-bound env
	// vars are rejected to avoid resolving Secret-backed values during template expansion.
	if mcpServer.Spec.MCPServerCatalogEntryName != "" {
		var catalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&catalogEntry, mcpServer.Spec.MCPServerCatalogEntryName); err != nil {
			return fmt.Errorf("failed to get catalog entry %s: %w", mcpServer.Spec.MCPServerCatalogEntryName, err)
		}

		var updateServer bool
		if url := envVars[configURLKey]; url != "" {
			validationOptions, err := ValidationOptionsWithResourceMaximums(req, m.mcpSessionManager)
			if err != nil {
				return err
			}
			if err := updateMCPServerURLFromCatalogEntry(req.Context(), req.Storage, &mcpServer, catalogEntry, url, validationOptions); err != nil {
				return err
			}

			// The URL is part of user configuration, but it is stored on the MCPServer spec rather than in credentials.
			delete(envVars, configURLKey)
			updateServer = true
		}

		// Check if the catalog entry has a URL template for remote runtime
		// Templates use ${VARIABLE_NAME} syntax for variable substitution
		// Example: "https://${DATABRICKS_WORKSPACE_URL}/api/2.0/mcp/genie/${DATABRICKS_GENIE_SPACE_ID}"
		if catalogEntry.Spec.Manifest.Runtime == types.RuntimeRemote &&
			catalogEntry.Spec.Manifest.RemoteConfig != nil &&
			catalogEntry.Spec.Manifest.RemoteConfig.URLTemplate != "" {
			validationOptions, err := ValidationOptionsWithResourceMaximums(req, m.mcpSessionManager)
			if err != nil {
				return err
			}
			if err := applyRemoteURLTemplate(req.Context(), &mcpServer.Spec.Manifest, envVars, !mcpServer.Spec.IsSingleUser(), validationOptions); err != nil {
				if configErr, ok := errors.AsType[*urlTemplateConfigurationError](err); ok {
					return types.NewErrBadRequest("invalid configuration: %v", configErr)
				}
				return err
			}
			if err := obottunnel.ValidateServerTunnelReferences(req.Context(), req.Storage, mcpServer.Spec.Manifest); err != nil {
				return types.NewErrBadRequest("validation failed: %v", err)
			}

			updateServer = updateServer || mcpServer.Spec.NeedsURL || mcpServer.Spec.Manifest.RemoteConfig.URL != ""
			mcpServer.Spec.NeedsURL = false
			mcpServer.Spec.PreviousURL = ""
		}

		if updateServer {
			if err := req.Update(&mcpServer); err != nil {
				return fmt.Errorf("failed to update server configuration: %w", err)
			}
		}
	}

	credCtx := mcpServer.CredentialContext(req.User.GetUID())

	// Allow for updating credentials. The only way to update a credential is to delete the existing one and recreate it.
	if err := m.removeMCPServerAndCred(req.Context(), req.GatewayClient, mcpServer, []string{credCtx}); err != nil {
		return err
	}

	sanitizeConfig(envVars, mcpServer.Spec.Manifest)

	if err := req.GatewayClient.UpsertCredential(req.Context(), gatewaytypes.Credential{
		Context: credCtx,
		Name:    mcpServer.Name,
		Secrets: envVars,
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}
	if err := m.triggerMCPServerControllers(req.Context(), mcpServer.Name); err != nil {
		return fmt.Errorf("failed to trigger MCP server reconciliation: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, mcpServer.Spec.Manifest.Config, envVars, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	return req.Write(ConvertMCPServer(mcpServer, mergedEnv, m.serverURL, slug))
}

func (m *MCPHandler) triggerMCPServerControllers(ctx context.Context, serverName string) error {
	if m.controllerBackend == nil {
		return fmt.Errorf("MCP server controller backend is not configured")
	}
	return m.controllerBackend.Trigger(ctx, v1.SchemeGroupVersion.WithKind("MCPServer"), serverName, 0)
}

func sanitizeConfig(config map[string]string, manifest types.MCPServerManifest) {
	if config == nil {
		return
	}

	bound := map[string]struct{}{}
	for _, env := range manifest.Config {
		if env.SecretBinding != nil {
			bound[env.Key] = struct{}{}
		}
	}

	for key, val := range config {
		if val == "" {
			delete(config, key)
			continue
		}
		if _, ok := bound[key]; ok {
			delete(config, key)
		}
	}
}

func sanitizedConfigCopy(config map[string]string, manifest types.MCPServerManifest) map[string]string {
	if config == nil {
		return nil
	}
	result := make(map[string]string, len(config))
	maps.Copy(result, config)
	sanitizeConfig(result, manifest)
	return result
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

func (m *MCPHandler) DeconfigureServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if mcpServer.Spec.MCPCatalogID != catalogID || mcpServer.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		return types.NewErrBadRequest("composite servers are no longer supported; use the migrated vMCP instance")
	}

	if mcpServer.Spec.VMCPComponentID != "" {
		return types.NewErrBadRequest("cannot deconfigure server associated to vMCP component")
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	credCtx := mcpServer.CredentialContext(req.User.GetUID())

	if err := m.removeMCPServerAndCred(req.Context(), req.GatewayClient, mcpServer, []string{credCtx}); err != nil {
		return err
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(ConvertMCPServer(mcpServer, nil, m.serverURL, slug))
}

func (m *MCPHandler) Reveal(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if mcpServer.Spec.MCPCatalogID != catalogID || mcpServer.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	credCtx := mcpServer.CredentialContext(req.User.GetUID())

	// Return flat configuration
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{credCtx}, mcpServer.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(sanitizedConfigCopy(cred.Secrets, mcpServer.Spec.Manifest))
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
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

func (m *MCPHandler) removeMCPServer(ctx context.Context, mcpServer v1.MCPServer) error {
	if m.shutdownMCPServer != nil {
		return m.shutdownMCPServer(mcpServer.Name)
	}
	if err := m.mcpSessionManager.ShutdownServer(ctx, mcpServer.Name); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
}

func (m *MCPHandler) removeMCPServerAndCred(ctx context.Context, gatewayClient *gateway.Client, mcpServer v1.MCPServer, credCtx []string) error {
	// Delete credential if it exists
	if err := DeleteCredentialIfExists(ctx, gatewayClient, credCtx, mcpServer.Name); err != nil {
		return err
	}

	// Shutdown the server, even if there is no credential
	if err := m.removeMCPServer(ctx, mcpServer); err != nil {
		return fmt.Errorf("failed to shutdown server: %w", err)
	}

	return nil
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

func ConvertMCPServer(server v1.MCPServer, credEnv map[string]string, serverURL, slug string) types.MCPServer {
	var missingEnvVars, missingHeaders []string

	for _, field := range server.Spec.Manifest.Config {
		if field.UserAllowed {
			continue
		}
		configuredValue := credEnv[field.Key]
		missingRequired := field.Required && field.Value == "" && configuredValue == ""
		invalidSelection := configuredValue != "" && !mcp.ConfigurationOptionValueValid(field.ToHeader(), credEnv)
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

	var connectURL string
	if serverURL != "" {
		if server.Spec.IsSingleUser() {
			connectURL = system.MCPConnectURL(serverURL, slug)
		} else {
			// Multi-user servers expose a default connect URL that auto-provisions an instance on first use.
			connectURL = system.MCPConnectURL(serverURL, server.Name)
		}
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
		ConnectURL:                  connectURL,
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
		NanobotAgentID:              server.Spec.NanobotAgentID,
	}

	if server.Spec.IsSingleUser() {
		converted.ServerUserType = types.ServerUserTypeSingleUser
	} else {
		converted.ServerUserType = types.ServerUserTypeMultiUser
	}

	return converted
}

func ConfigurationTargetForConnectID(req api.Context, id, serverURL, secretBindingAllowedLabel string, validationOptions mcp.ValidationOptions) (*types.MCPServer, *types.MCPServerInstance, error) {
	server, instance, err := mcpServerOrInstanceFromConnectURL(req, id, secretBindingAllowedLabel, validationOptions)
	if err != nil {
		return nil, nil, err
	}

	if instance.Name != "" {
		credEnv, err := mcpServerInstanceCredEnv(req, instance)
		if err != nil {
			return nil, nil, err
		}
		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to determine MCP server instance slug: %w", err)
		}
		converted := ConvertMCPServerInstance(instance, credEnv, serverURL, slug)
		return nil, &converted, nil
	}

	credEnv, err := credentialEnvForMCPServer(req, server, secretBindingAllowedLabel)
	if err != nil {
		return nil, nil, err
	}
	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), server.Spec.MCPCatalogID, server.Spec.PowerUserWorkspaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to determine MCP server slug: %w", err)
	}

	converted := ConvertMCPServer(server, credEnv, serverURL, slug)
	return &converted, nil, nil
}

func credentialEnvForMCPServer(req api.Context, server v1.MCPServer, secretBindingAllowedLabel string) (map[string]string, error) {
	addExtractedEnvVars(&server)

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.CredentialContext(server.Spec.UserID)}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return nil, fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, secretBindingAllowedLabel)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	return mergedEnv, nil
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

func SlugForMCPServer(ctx context.Context, client kclient.Client, server v1.MCPServer, userID, catalogID, workspaceID string) (string, error) {
	if server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != "" {
		return server.Name, nil
	}
	var shouldHaveUnique bool
	if workspaceID == "" && catalogID == "" && server.Spec.MCPServerCatalogEntryName != "" {
		var serversWithEntryName v1.MCPServerList
		if err := client.List(ctx, &serversWithEntryName, &kclient.ListOptions{
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.mcpServerCatalogEntryName": server.Spec.MCPServerCatalogEntryName,
				"spec.userID":                    userID,
				"spec.template":                  "false",
				"spec.compositeName":             "",
			}),
		}); err != nil {
			return "", fmt.Errorf("failed to find MCP server catalog entry for server: %w", err)
		}
		serversWithEntryName.Items = slices.DeleteFunc(serversWithEntryName.Items, func(server v1.MCPServer) bool {
			return server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != ""
		})

		slices.SortFunc(serversWithEntryName.Items, func(a, b v1.MCPServer) int {
			return a.CreationTimestamp.Compare(b.CreationTimestamp.Time)
		})

		shouldHaveUnique = len(serversWithEntryName.Items) != 0 && serversWithEntryName.Items[0].Name != server.Name
	}

	slug := server.Spec.MCPServerCatalogEntryName
	if shouldHaveUnique || server.Spec.MCPServerCatalogEntryName == "" {
		slug = server.Name
	}

	return slug, nil
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

	var slug string
	for _, server := range allowedServers {
		addExtractedEnvVars(&server)
		// Enrich with tool preview data if catalog entry exists
		if server.Spec.MCPServerCatalogEntryName != "" {
			entry := catalogEntryMap[server.Spec.MCPServerCatalogEntryName]
			// Add tool preview from catalog entry to server manifest
			server.Spec.Manifest.ToolPreview = entry.Spec.Manifest.ToolPreview
		}

		slug, err = SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), system.DefaultCatalog, server.Spec.PowerUserWorkspaceID)
		if err != nil {
			return fmt.Errorf("failed to generate slug: %w", err)
		}

		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, credMap[server.Name], m.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings for server %s: %w", server.Name, err)
		}
		parent := ConvertMCPServer(server, mergedEnv, m.serverURL, slug)
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

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), server.Spec.MCPCatalogID, server.Spec.PowerUserWorkspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(ConvertMCPServer(server, mergedEnv, m.serverURL, slug))
}

func (m *MCPHandler) ClearOAuthCredentials(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")
	mcpServerID := mcpActionID(req)

	if system.IsVMCPID(mcpServerID) {
		server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), mcpServerID, req.User.GetUID())
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

		for _, component := range componentServers {
			if component.Spec.Manifest.Runtime != types.RuntimeRemote ||
				component.Spec.Manifest.RemoteConfig == nil {
				continue
			}

			componentServer, componentConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), component.Name, req.User.GetUID())
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

			if err := req.GatewayClient.DeleteMCPOAuthTokenForURL(req.Context(), req.User.GetUID(), componentServer.Name, componentURL); err != nil {
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
	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User.GetUID())
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
	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User.GetUID())
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

	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User.GetUID())
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

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Config, cred.Secrets, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	// Return updated server
	return req.Write(ConvertMCPServer(server, mergedEnv, m.serverURL, slug))
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
	server, serverConfig, err := m.mcpSessionManager.ServerForAction(req.Context(), req.PathValue("mcp_server_id"), req.User.GetUID())
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

func (m *MCPHandler) UpdateURL(req api.Context) error {
	var mcpServer v1.MCPServer
	if err := req.Get(&mcpServer, req.PathValue("mcp_server_id")); err != nil {
		return fmt.Errorf("failed to get server: %w", err)
	}

	if !mcpServer.Spec.IsSingleUser() {
		return types.NewErrBadRequest("cannot update the URL for a multi-user MCP server; use the UpdateServer endpoint instead")
	}

	if mcpServer.Spec.VMCPComponentID != "" {
		return types.NewErrBadRequest("cannot update the URL for a VMCP component")
	}

	if mcpServer.Spec.MCPServerCatalogEntryName == "" {
		return types.NewErrBadRequest("this server does not have a catalog entry")
	}

	if mcpServer.Spec.Manifest.Runtime != types.RuntimeRemote || mcpServer.Spec.Manifest.RemoteConfig == nil {
		return types.NewErrBadRequest("cannot update the URL for a non-remote MCP server")
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, mcpServer.Spec.MCPServerCatalogEntryName); err != nil {
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	if entry.Spec.Manifest.RemoteConfig == nil {
		return types.NewErrBadRequest("the catalog entry for this server does not have remote configuration")
	}

	if entry.Spec.Manifest.RemoteConfig.FixedURL != "" {
		return types.NewErrBadRequest("this server already has a fixed URL that cannot be updated")
	}

	if entry.Spec.Manifest.RemoteConfig.Hostname == "" {
		return types.NewErrBadRequest("the catalog entry for this server does not have a hostname")
	}

	var input struct {
		URL string `json:"url"`
	}
	if err := req.Read(&input); err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	validationOptions, err := ValidationOptionsWithResourceMaximums(req, m.mcpSessionManager)
	if err != nil {
		return err
	}
	if err := updateMCPServerURLFromCatalogEntry(req.Context(), req.Storage, &mcpServer, entry, input.URL, validationOptions); err != nil {
		return err
	}

	if err := req.Update(&mcpServer); err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(ConvertMCPServer(mcpServer, nil, m.serverURL, slug))
}

func updateMCPServerURLFromCatalogEntry(ctx context.Context, client kclient.Client, mcpServer *v1.MCPServer, entry v1.MCPServerCatalogEntry, inputURL string, opts mcp.ValidationOptions) error {
	if entry.Spec.Manifest.RemoteConfig == nil {
		return types.NewErrBadRequest("the catalog entry for this server does not have remote configuration")
	}
	if entry.Spec.Manifest.RemoteConfig.Hostname == "" {
		return types.NewErrBadRequest("the catalog entry for this server does not have a hostname")
	}

	if !strings.HasPrefix(inputURL, "http") {
		inputURL = "https://" + inputURL
	}

	if err := types.ValidateURLHostname(inputURL, entry.Spec.Manifest.RemoteConfig.Hostname); err != nil {
		return types.NewErrBadRequest("the hostname in the URL does not match the hostname in the catalog entry: %v", err)
	}

	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		return types.NewErrBadRequest("failed to parse input URL: %v", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return types.NewErrBadRequest("the URL must be HTTP or HTTPS")
	}

	if mcpServer.Spec.Manifest.RemoteConfig == nil {
		mcpServer.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{}
	}
	mcpServer.Spec.Manifest.RemoteConfig.URL = inputURL
	mcpServer.Spec.Manifest.RemoteConfig.Hostname = entry.Spec.Manifest.RemoteConfig.Hostname
	mcpServer.Spec.Manifest.RemoteConfig.TunnelName = entry.Spec.Manifest.RemoteConfig.TunnelName
	mcpServer.Spec.NeedsURL = false
	mcpServer.Spec.PreviousURL = ""

	if err := mcp.ValidateServerManifest(ctx, mcpServer.Spec.Manifest, !mcpServer.Spec.IsSingleUser(), opts); err != nil {
		return err
	}
	if err := obottunnel.ValidateServerTunnelReferences(ctx, client, mcpServer.Spec.Manifest); err != nil {
		return err
	}

	return nil
}

func (m *MCPHandler) TriggerUpdate(req api.Context) error {
	var (
		workspaceID = req.PathValue("workspace_id")
		server      v1.MCPServer
	)

	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return err
	}
	if server.Spec.VMCPID != "" || server.Spec.VMCPInstanceID != "" {
		return types.NewErrBadRequest("cannot trigger update for a vMCP component server; update the vMCP instead")
	}

	if !server.Spec.IsSingleUser() {
		// Multi-user servers deployed from catalog entries can be updated, but only
		// through catalog- or workspace-scoped routes.
		if server.Spec.MCPServerCatalogEntryName == "" {
			return types.NewErrBadRequest("cannot trigger update for a multi-user MCP server without a catalog entry; use the UpdateServer endpoint instead")
		}
		if err := validateServerScope(req, server); err != nil {
			return err
		}
		if !req.UserIsAdmin() {
			// Multi-user catalog entry deployments require PowerUserPlus access to the owning workspace.
			if !req.UserIsPowerUserPlus() || workspaceID == "" || server.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}
		}
	}

	// Reject component servers - must upgrade parent composite
	if server.Spec.CompositeName != "" {
		return types.NewErrBadRequest("cannot trigger update on a component server; upgrade the parent composite server instead")
	}

	if server.Spec.MCPServerCatalogEntryName == "" || !server.Status.NeedsUpdate {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
		return err
	}

	if !req.UserIsAdmin() && server.Spec.IsSingleUser() {
		// Allow users to upgrade their own single-user servers.
		if !server.Spec.IsOwnedBy(req.User.GetUID()) {
			// Workspace-based authorization for power user workspace entries
			if workspaceID == "" || entry.Spec.PowerUserWorkspaceID != workspaceID {
				return types.NewErrNotFound("MCP server %s not found", server.Name)
			}
		}
	}

	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return types.NewErrBadRequest("composite servers are no longer supported; update the migrated vMCP")
	}
	var configured map[string]string
	if entry.Spec.Manifest.Runtime == types.RuntimeRemote && entry.Spec.Manifest.RemoteConfig != nil && entry.Spec.Manifest.RemoteConfig.URLTemplate != "" {
		var err error
		configured, err = credentialEnvForMCPServer(req, server, m.secretBindingAllowedLabel)
		if err != nil {
			return err
		}
	}

	candidate := server.DeepCopy()
	if err := m.prepareCatalogServerUpdate(req, candidate, entry, configured); err != nil {
		return err
	}

	// Shutdown the server, even if there is no credential
	if err := m.removeMCPServer(req.Context(), server); err != nil {
		return err
	}

	// Use RetryOnConflict because catalog-entry updates cause controller-side
	// status writes (for example DetectDrift setting NeedsUpdate) that can race
	// with this spec update and bump the ResourceVersion.
	oldManifestHash := utils.Digest(server.Spec.Manifest)
	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latest v1.MCPServer
		if err := req.Get(&latest, server.Name); err != nil {
			return err
		}

		if utils.Digest(latest.Spec.Manifest) != oldManifestHash {
			return types.NewErrHTTP(http.StatusConflict, "manifest changed during update")
		}

		if err := m.prepareCatalogServerUpdate(req, &latest, entry, configured); err != nil {
			return err
		}
		return req.Update(&latest)
	}); err != nil {
		return err
	}

	return nil
}

func (m *MCPHandler) prepareCatalogServerUpdate(req api.Context, server *v1.MCPServer, entry v1.MCPServerCatalogEntry, configured map[string]string) error {
	updateServerFromCatalogEntry(server, entry)
	validationOptions, err := ValidationOptionsWithResourceMaximums(req, m.mcpSessionManager)
	if err != nil {
		return err
	}
	if server.Spec.Manifest.Runtime == types.RuntimeRemote && server.Spec.Manifest.RemoteConfig != nil && server.Spec.Manifest.RemoteConfig.URLTemplate != "" {
		if configErr := validateConfiguredOptions(server.Spec.Manifest.Config, configured); configErr == nil {
			if err := applyRemoteURLTemplate(req.Context(), &server.Spec.Manifest, configured, !server.Spec.IsSingleUser(), validationOptions); err != nil {
				if _, ok := errors.AsType[*urlTemplateConfigurationError](err); !ok {
					return err
				}
			}
		}
		// Invalid persisted selections intentionally leave the template unresolved so the API reports reconfiguration.
	}
	if err := mcp.ValidateServerManifest(req.Context(), server.Spec.Manifest, !server.Spec.IsSingleUser(), validationOptions); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if err := obottunnel.ValidateServerTunnelReferences(req.Context(), req.Storage, server.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	return nil
}

func updateServerFromCatalogEntry(server *v1.MCPServer, entry v1.MCPServerCatalogEntry) {
	// Update the server manifest with the latest from the catalog entry.
	server.Spec.Manifest.Metadata = entry.Spec.Manifest.Metadata
	server.Spec.Manifest.Name = entry.Spec.Manifest.Name
	server.Spec.Manifest.ShortDescription = entry.Spec.Manifest.ShortDescription
	server.Spec.Manifest.Description = entry.Spec.Manifest.Description
	server.Spec.Manifest.Icon = entry.Spec.Manifest.Icon
	server.Spec.Manifest.Config = slices.Clone(entry.Spec.Manifest.Config)
	server.Spec.Manifest.Resources = entry.Spec.Manifest.Resources
	server.Spec.Manifest.Runtime = entry.Spec.Manifest.Runtime
	server.Spec.Manifest.UVXConfig = entry.Spec.Manifest.UVXConfig
	server.Spec.Manifest.NPXConfig = entry.Spec.Manifest.NPXConfig
	server.Spec.Manifest.ContainerizedConfig = entry.Spec.Manifest.ContainerizedConfig

	// Handle remote runtime URL updates.
	if entry.Spec.Manifest.Runtime == types.RuntimeRemote && entry.Spec.Manifest.RemoteConfig != nil {
		if entry.Spec.Manifest.RemoteConfig.FixedURL != "" {
			// Use the fixed URL from catalog entry.
			server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
				URL:                 entry.Spec.Manifest.RemoteConfig.FixedURL,
				TunnelName:          entry.Spec.Manifest.RemoteConfig.TunnelName,
				StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
			}
		} else if entry.Spec.Manifest.RemoteConfig.Hostname != "" {
			// Check if the server's current URL matches the new hostname requirement.
			if server.Spec.Manifest.RemoteConfig != nil && server.Spec.Manifest.RemoteConfig.URL != "" {
				currentURL := server.Spec.Manifest.RemoteConfig.URL
				hostnameMismatchErr := types.ValidateURLHostname(currentURL, entry.Spec.Manifest.RemoteConfig.Hostname)

				server.Spec.NeedsURL = hostnameMismatchErr != nil
				if server.Spec.NeedsURL {
					server.Spec.PreviousURL = currentURL
					currentURL = ""
				} else {
					server.Spec.PreviousURL = ""
				}

				server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
					URL:                 currentURL,
					Hostname:            entry.Spec.Manifest.RemoteConfig.Hostname,
					TunnelName:          entry.Spec.Manifest.RemoteConfig.TunnelName,
					StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
				}
			} else {
				// No current URL, needs one.
				server.Spec.NeedsURL = true
				server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
					Hostname:            entry.Spec.Manifest.RemoteConfig.Hostname,
					TunnelName:          entry.Spec.Manifest.RemoteConfig.TunnelName,
					StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
				}
			}
		} else if entry.Spec.Manifest.RemoteConfig.URLTemplate != "" {
			server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
				IsTemplate:          true,
				URLTemplate:         entry.Spec.Manifest.RemoteConfig.URLTemplate,
				TunnelName:          entry.Spec.Manifest.RemoteConfig.TunnelName,
				StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
			}
		}
	} else {
		// For non-remote runtimes, clear the remote config.
		server.Spec.Manifest.RemoteConfig = nil
	}
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
		slug, err := SlugForMCPServerInstance(req.Context(), req.Storage, instance)
		if err != nil {
			return fmt.Errorf("failed to determine slug for instance %s: %w", instance.Name, err)
		}

		credEnv, err := mcpServerInstanceCredEnv(req, instance)
		if err != nil {
			return err
		}

		convertedInstances = append(convertedInstances, ConvertMCPServerInstance(instance, credEnv, m.serverURL, slug))
	}

	return req.Write(types.MCPServerInstanceList{
		Items: convertedInstances,
	})
}

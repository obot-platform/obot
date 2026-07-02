package handlers

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	nahbackend "github.com/obot-platform/nah/pkg/backend"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/obot-platform/obot/pkg/validation"
	"github.com/obot-platform/obot/pkg/wait"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var envVarRegex = regexp.MustCompile(`\${([^}]+)}`)

const (
	requestTimeUpdateInterval = 15 * time.Minute
	configURLKey              = "__url"
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

	// shutdownMCPServer is only injected for testing
	shutdownMCPServer func(string) error
}

func NewMCPHandler(mcpLoader *mcp.SessionManager, acrHelper *accesscontrolrule.Helper, mcpOAuthChecker MCPOAuthChecker, controllerBackend nahbackend.Trigger, mcpImagePullSecrets []string, serverURL, secretBindingAllowedLabel string) *MCPHandler {
	return &MCPHandler{
		mcpSessionManager:         mcpLoader,
		mcpOAuthChecker:           mcpOAuthChecker,
		acrHelper:                 acrHelper,
		controllerBackend:         controllerBackend,
		mcpImagePullSecrets:       mcpImagePullSecrets,
		mcpRuntimeBackend:         mcpLoader.MCPRuntimeBackend(),
		serverURL:                 serverURL,
		secretBindingAllowedLabel: secretBindingAllowedLabel,
	}
}

func validationOptions(remoteValidationConfig mcp.RemoteMCPURLValidationConfig) validation.Options {
	return validation.Options{
		RemoteMCPURLValidationConfig: remoteValidationConfig,
	}
}

// ValidationOptionsWithResourceMaximums builds MCP manifest validation options from the active MCP session manager.
func ValidationOptionsWithResourceMaximums(sessionManager *mcp.SessionManager) validation.Options {
	if sessionManager == nil {
		return validation.Options{}
	}
	options := validationOptions(sessionManager.RemoteMCPURLValidationConfig())
	options.ResourceMaximums = sessionManager.KubernetesResourceMaximums()
	return options
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
	return mcp.ComputeK8sSettingsHash(settings, resources, mcpServer.Spec.Manifest.Runtime, mcpServer.Spec.NanobotAgentID != "", m.mcpSessionManager.KubernetesResourceMaximums(), imagePullSecretNames), nil
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
	if HideMultiUserCatalogEntry(req, entry) {
		return types.NewErrNotFound("MCP catalog entry not found")
	}

	return req.Write(ConvertMCPServerCatalogEntryWithWorkspace(entry, entry.Spec.PowerUserWorkspaceID, "", m.serverURL))
}

func (m *MCPHandler) ListEntriesFromAllSources(req api.Context) error {
	var list v1.MCPServerCatalogEntryList
	if err := req.List(&list); err != nil {
		return err
	}

	convertEntry := func(entry v1.MCPServerCatalogEntry) types.MCPServerCatalogEntry {
		return ConvertMCPServerCatalogEntryWithWorkspace(entry, entry.Spec.PowerUserWorkspaceID, "", m.serverURL)
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
	var entries []types.MCPServerCatalogEntry
	for _, entry := range list.Items {
		if HideMultiUserCatalogEntry(req, entry) {
			continue
		}

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

// HideMultiUserCatalogEntry determines whether a user should be able to see a catalog entry based on
// its single-user or multi-user type.
func HideMultiUserCatalogEntry(req api.Context, entry v1.MCPServerCatalogEntry) bool {
	return !req.UserIsPowerUserPlus() && !entry.Spec.Manifest.ServerUserType.IsSingleUser()
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

func defaultCatalogEntryConnectURL(serverURL string, entry v1.MCPServerCatalogEntry) string {
	if serverURL == "" {
		return ""
	}
	if entry.Spec.Manifest.ServerUserType == types.ServerUserTypeMultiUser {
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
	if catalogID != "" {
		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", catalogID, server.Name))
		}
	} else if workspaceID != "" {
		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", workspaceID, server.Name))
		}
	} else {
		for _, server := range servers.Items {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name))
		}
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

		var components []types.MCPServer
		if server.Spec.Manifest.Runtime == types.RuntimeComposite {
			components, err = resolveCompositeComponents(req, server, m.secretBindingAllowedLabel)
			if err != nil {
				log.Warnf("failed to resolve composite components for server %s: %v", server.Name, err)
				return err
			}
		}
		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, credMap[server.Name], m.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings for server %s: %w", server.Name, err)
		}
		converted := ConvertMCPServer(server, mergedEnv, m.serverURL, slug, components...)
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

	var credCtxs []string
	if catalogID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", catalogID, server.Name)}
	} else if workspaceID != "" {
		credCtxs = []string{fmt.Sprintf("%s-%s", workspaceID, server.Name)}
	} else {
		credCtxs = []string{fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name)}
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}
	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, server, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	var components []types.MCPServer
	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		components, err = resolveCompositeComponents(req, server, m.secretBindingAllowedLabel)
		if err != nil {
			log.Warnf("failed to resolve composite components for server %s: %v", server.Name, err)
			return err
		}
	}
	converted := ConvertMCPServer(server, mergedEnv, m.serverURL, slug, components...)
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

	// Prevent deletion of multi-user servers that are referenced by running composite MCP servers or catalog entries.
	dependencies, err := listCompositeDeletionDependencies(req, server)
	if err != nil {
		return fmt.Errorf("failed to list composite deletion dependencies: %w", err)
	}
	if len(dependencies) > 0 {
		return req.WriteCode(map[string]any{
			"message":      "MCP server must be removed from all composite MCP servers before it can be deleted",
			"dependencies": dependencies,
		}, http.StatusConflict)
	}

	if err := req.Delete(&server); err != nil {
		return err
	}

	return req.Write(ConvertMCPServer(server, nil, m.serverURL, slug))
}

// compositeDeletionDependency represents a composite MCP server or catalog entry that depends
// on a given multi-user server and must be deleted before the multi-user server can be deleted.
type compositeDeletionDependency struct {
	// Name is the display name of the dependent composite MCP server.
	Name string `json:"name"`
	// Icon is the icon of the dependent composite MCP server.
	Icon string `json:"icon"`
	// MCPServerID is the ID of a running instance of a dependent composite MCP server.
	MCPServerID string `json:"mcpServerID,omitempty"`
	// CatalogEntryID is the catalog entry ID of the dependent composite MCP server.
	CatalogEntryID string `json:"catalogEntryID"`
}

// listCompositeDeletionDependencies lists the composite MCP servers and catalog entries that depend on the given multi-user server.
func listCompositeDeletionDependencies(req api.Context, server v1.MCPServer) ([]compositeDeletionDependency, error) {
	if server.Spec.IsSingleUser() {
		// Single-user servers cannot be composite components; skip dependency check.
		return nil, nil
	}

	var compositeServers v1.MCPServerList
	if err := req.List(&compositeServers,
		kclient.InNamespace(server.Namespace),
		kclient.MatchingFields{
			"spec.manifest.runtime": string(types.RuntimeComposite),
		},
	); err != nil {
		return nil, fmt.Errorf("failed to list composite servers: %w", err)
	}

	var compositeEntries v1.MCPServerCatalogEntryList
	if err := req.List(&compositeEntries,
		kclient.InNamespace(server.Namespace),
		kclient.MatchingFields{
			"spec.manifest.runtime": string(types.RuntimeComposite),
		},
	); err != nil {
		return nil, fmt.Errorf("failed to list composite catalog entries: %w", err)
	}

	var dependencies []compositeDeletionDependency
	for _, compositeServer := range compositeServers.Items {
		var compositeConfig types.CompositeRuntimeConfig
		if cfg := compositeServer.Spec.Manifest.CompositeConfig; cfg != nil {
			compositeConfig = *cfg
		}

		components := compositeConfig.ComponentServers
		for _, component := range components {
			if component.MCPServerID == server.Name {
				dependencies = append(dependencies, compositeDeletionDependency{
					Name:           compositeServer.Spec.Manifest.Name,
					Icon:           compositeServer.Spec.Manifest.Icon,
					MCPServerID:    compositeServer.Name,
					CatalogEntryID: compositeServer.Spec.MCPServerCatalogEntryName,
				})
				break
			}
		}
	}

	for _, compositeEntry := range compositeEntries.Items {
		var compositeConfig types.CompositeCatalogConfig
		if cfg := compositeEntry.Spec.Manifest.CompositeConfig; cfg != nil {
			compositeConfig = *cfg
		}

		components := compositeConfig.ComponentServers
		for _, component := range components {
			if component.MCPServerID == server.Name {
				dependencies = append(dependencies, compositeDeletionDependency{
					Name:           compositeEntry.Spec.Manifest.Name,
					Icon:           compositeEntry.Spec.Manifest.Icon,
					CatalogEntryID: compositeEntry.Name,
				})
				break
			}
		}
	}

	// Sort by catalog entry ID to ensure consistent ordering
	slices.SortFunc(dependencies, func(a, b compositeDeletionDependency) int {
		return strings.Compare(a.CatalogEntryID, b.CatalogEntryID)
	})

	return dependencies, nil
}

func (m *MCPHandler) LaunchServer(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		var componentServers v1.MCPServerList
		if err := req.List(&componentServers,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{
				"spec.compositeName": server.Name,
			},
		); err != nil {
			return fmt.Errorf("failed to list child servers: %w", err)
		}

		// Build disabled set from parent composite manifest; default is enabled
		var compositeConfig types.CompositeRuntimeConfig
		if server.Spec.Manifest.CompositeConfig != nil {
			compositeConfig = *server.Spec.Manifest.CompositeConfig
		}
		disabledComponents := make(map[string]bool, len(compositeConfig.ComponentServers))
		for _, comp := range compositeConfig.ComponentServers {
			disabledComponents[comp.CatalogEntryID] = comp.Disabled
		}

		for _, component := range componentServers.Items {
			// Skip if disabled in composite config
			if disabledComponents[component.Spec.MCPServerCatalogEntryName] {
				continue
			}

			config, _, err := serverConfigForAction(req, component, m.secretBindingAllowedLabel, false)
			if err != nil {
				return fmt.Errorf("failed to get config for component server %s: %w", component.Name, err)
			}

			if config.Runtime != types.RuntimeRemote {
				_, err = m.mcpSessionManager.ListTools(req.Context(), config)
			} else {
				// Don't use ListTools for remote MCP servers in case they need OAuth.
				_, err = m.mcpSessionManager.LaunchServer(req.Context(), config)
			}
			if err != nil {
				if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("Component MCP server %s is not healthy, check configuration for errors: %v", component.Name, err))
				}
				if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("No response from component MCP server %s, check configuration for errors", component.Name))
				}
				if errors.Is(err, mcp.ErrInsufficientCapacity) {
					return types.NewErrHTTP(http.StatusServiceUnavailable, "Insufficient capacity to deploy MCP server. Please contact your administrator.")
				}
				if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
					return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
				}

				return fmt.Errorf("failed to launch component MCP server %s: %w", component.Name, err)
			}
		}

		return nil
	}

	if server.Spec.Manifest.Runtime != types.RuntimeRemote {
		_, err = m.mcpSessionManager.ListTools(req.Context(), serverConfig)
	} else {
		// Don't use ListTools for remote MCP servers in case they need OAuth.
		_, err = m.mcpSessionManager.LaunchServer(req.Context(), serverConfig)
	}
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if errors.Is(err, mcp.ErrInsufficientCapacity) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "Insufficient capacity to deploy MCP server. Please contact your administrator.")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return fmt.Errorf("failed to launch MCP server: %w", err)
	}

	return nil
}

func (m *MCPHandler) CheckOAuth(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	// For servers that are in catalogs, this checks to make sure that a catalogID was provided and that it matches.
	// For servers that are in workspaces, this checks to make sure that a workspaceID was provided and that it matches.
	// For servers that are not in catalogs or workspaces, this checks to make sure that no catalogID or workspaceID was provided.
	if server.Spec.MCPCatalogID != catalogID || server.Spec.PowerUserWorkspaceID != workspaceID {
		return types.NewErrNotFound("MCP server not found")
	}

	if serverConfig.Runtime == types.RuntimeRemote {
		var are nmcp.AuthRequiredErr
		if _, err = m.mcpSessionManager.PingServer(req.Context(), serverConfig); err != nil {
			if !errors.As(err, &are) {
				return fmt.Errorf("failed to ping MCP server: %w", err)
			}
			req.WriteHeader(http.StatusPreconditionFailed)
		}
	}

	return nil
}

func (m *MCPHandler) GetOAuthURL(req api.Context) error {
	catalogID := req.PathValue("catalog_id")
	workspaceID := req.PathValue("workspace_id")

	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
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
	server, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager, m.secretBindingAllowedLabel)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
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
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return fmt.Errorf("failed to list tools: %w", err)
	}

	return req.Write(tools)
}

func (m *MCPHandler) GetResources(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager, m.secretBindingAllowedLabel)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
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
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list resources: %w", err)
	}

	return req.Write(resources)
}

func (m *MCPHandler) ReadResource(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager, m.secretBindingAllowedLabel)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		return err
	}

	if caps.Resources == nil {
		return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
	}

	contents, err := m.mcpSessionManager.ReadResource(req.Context(), serverConfig, req.PathValue("resource_uri"))
	if err != nil {
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support resources")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list resources: %w", err)
	}

	return req.Write(contents)
}

func (m *MCPHandler) GetPrompts(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager, m.secretBindingAllowedLabel)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
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
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}

		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to list prompts: %w", err)
	}

	return req.Write(prompts)
}

func (m *MCPHandler) GetPrompt(req api.Context) error {
	_, serverConfig, caps, err := serverForActionWithCapabilities(req, m.mcpSessionManager, m.secretBindingAllowedLabel)
	if err != nil {
		if errors.Is(err, mcp.ErrHealthCheckFailed) || errors.Is(err, mcp.ErrHealthCheckTimeout) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, fmt.Sprintf("MCP server is not healthy, check configuration for errors: %v", err))
		}
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
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
		if errors.Is(err, nmcp.ErrNoResult) || strings.HasSuffix(err.Error(), nmcp.ErrNoResult.Error()) {
			return types.NewErrHTTP(http.StatusServiceUnavailable, "No response from MCP server, check configuration for errors")
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support prompts")
		}
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrHTTP(http.StatusBadRequest, nse.Error())
		}
		var are nmcp.AuthRequiredErr
		if errors.As(err, &are) {
			return types.NewErrHTTP(http.StatusPreconditionFailed, "MCP server requires authentication")
		}
		return fmt.Errorf("failed to get prompt: %w", err)
	}

	return req.Write(map[string]any{
		"messages":    messages,
		"description": description,
	})
}

func mcpServerOrInstanceFromConnectURL(req api.Context, id, secretBindingAllowedLabel string, validationOptions validation.Options) (v1.MCPServer, v1.MCPServerInstance, error) {
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
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: system.MCPServerInstancePrefix,
						Namespace:    server.Namespace,
					},
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:             id,
						MCPCatalogName:            server.Spec.MCPCatalogID,
						MCPServerCatalogEntryName: server.Spec.MCPServerCatalogEntryName,
						PowerUserWorkspaceID:      server.Spec.PowerUserWorkspaceID,
						UserID:                    req.User.GetUID(),
						MultiUserConfig:           server.Spec.Manifest.MultiUserConfig,
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
			if err := validation.ValidateServerManifest(req.Context(), manifest, false, validationOptions); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, types.NewErrBadRequest("catalog entry %s cannot be connected because its MCP server manifest is invalid: %v", id, err)
			}

			// Create a new MCP server for the user.
			server := v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.MCPServerPrefix,
					Namespace:    req.Namespace(),
				},
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

			// The composite's component servers are created asynchronously by the controller
			// (EnsureCompositeComponents). The connect path builds the nanobot config by listing
			// components synchronously, so wait for the controller to reconcile them before
			// returning to avoid baking a config with missing components.
			if server.Spec.Manifest.Runtime == types.RuntimeComposite &&
				server.Spec.Manifest.CompositeConfig != nil &&
				len(server.Spec.Manifest.CompositeConfig.ComponentServers) > 0 {
				server, err = waitForCompositeReady(req, server, 30*time.Second)
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
			if err := req.Update(&server); err != nil {
				return v1.MCPServer{}, v1.MCPServerInstance{}, fmt.Errorf("failed to update MCP server configuration from catalog entry %s: %w", id, err)
			}
		}

		return server, v1.MCPServerInstance{}, nil
	}
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

func entryMissingAdminConfig(ctx context.Context, client kclient.Client, obotNamespace string, entry v1.MCPServerCatalogEntry, secretBindingAllowedLabel string) (missingCatalogEntryAdminConfig, error) {
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

		resolved, err := mcp.MergeBoundCreds(ctx, client, obotNamespace, cm.Env, remote, nil, secretBindingAllowedLabel)
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

// MCPIDAndAudienceFromConnectURL returns the MCP server or instance name and audience based on the provided connect URL.
// The connect URL could have an MCP server ID, server instance ID, or MCP catalog entry ID.
func MCPIDAndAudienceFromConnectURL(req api.Context, id, secretBindingAllowedLabel string, validationOptions validation.Options) (string, string, error) {
	server, instance, err := mcpServerOrInstanceFromConnectURL(req, id, secretBindingAllowedLabel, validationOptions)
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

func ServerForActionWithConnectID(req api.Context, id, secretBindingAllowedLabel string, validationOptions validation.Options) (string, v1.MCPServer, mcp.ServerConfig, error) {
	id, server, config, _, err := serverForActionWithConnectID(req, id, secretBindingAllowedLabel, false, validationOptions)
	return id, server, config, err
}

func ServerForActionWithConnectIDAllowMissingConfig(req api.Context, id, secretBindingAllowedLabel string, validationOptions validation.Options) (string, v1.MCPServer, mcp.ServerConfig, []string, error) {
	return serverForActionWithConnectID(req, id, secretBindingAllowedLabel, true, validationOptions)
}

func serverForActionWithConnectID(req api.Context, id, secretBindingAllowedLabel string, allowMissingConfig bool, validationOptions validation.Options) (string, v1.MCPServer, mcp.ServerConfig, []string, error) {
	server, instance, err := mcpServerOrInstanceFromConnectURL(req, id, secretBindingAllowedLabel, validationOptions)
	if err != nil {
		return "", v1.MCPServer{}, mcp.ServerConfig{}, nil, err
	}

	switch {
	case instance.Name != "":
		server, config, missingConfig, err := serverFromMCPServerInstance(req, instance, secretBindingAllowedLabel, allowMissingConfig)
		return instance.Name, server, config, missingConfig, err
	case server.Name != "":
		config, missingConfig, err := serverConfigForAction(req, server, secretBindingAllowedLabel, allowMissingConfig)
		return server.Name, server, config, missingConfig, err
	default:
		return "", v1.MCPServer{}, mcp.ServerConfig{}, nil, fmt.Errorf("unknown MCP server ID %s", id)
	}
}

func serverFromMCPServerInstance(req api.Context, instance v1.MCPServerInstance, secretBindingAllowedLabel string, allowMissingConfig bool) (v1.MCPServer, mcp.ServerConfig, []string, error) {
	var server v1.MCPServer
	if err := req.Get(&server, instance.Spec.MCPServerName); err != nil {
		return server, mcp.ServerConfig{}, nil, err
	}

	if server.Spec.NeedsURL {
		if allowMissingConfig {
			return server, mcp.ServerConfig{}, []string{"URL"}, nil
		}
		return server, mcp.ServerConfig{}, nil, fmt.Errorf("mcp server %s needs to update its URL", server.Name)
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

	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{credCtx}, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return server, mcp.ServerConfig{}, nil, fmt.Errorf("failed to find credential: %w", err)
	}

	catalogName := server.Spec.MCPCatalogID
	if catalogName == "" {
		catalogName = server.Status.MCPCatalogID
	}
	if catalogName == "" {
		// For multi-user servers in a workspace, use the workspace ID as the catalog name
		catalogName = server.Spec.PowerUserWorkspaceID
	}
	// Look up catalog entry for catalog name if needed
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err != nil {
			return server, mcp.ServerConfig{}, nil, fmt.Errorf("failed to get MCP server catalog entry: %w", err)
		}
		if catalogName == "" {
			catalogName = entry.Spec.MCPCatalogName
		}
		if catalogName == "" {
			catalogName = entry.Spec.PowerUserWorkspaceID
		}
	}

	tokenExchangeCred, err := req.GatewayClient.RevealCredential(req.Context(), []string{server.Name}, server.Name)
	if err != nil {
		return server, mcp.ServerConfig{}, nil, fmt.Errorf("failed to find token exchange credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, secretBindingAllowedLabel)
	if err != nil {
		return server, mcp.ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	serverConfig, missingConfig, err := mcp.ServerToServerConfig(server, instance.ValidConnectURLs(baseURL), baseURL, req.User.GetUID(), scope, catalogName, mergedEnv, tokenExchangeCred.Secrets)
	if err != nil {
		return server, mcp.ServerConfig{}, nil, err
	}

	instanceCredEnv, err := mcpServerInstanceCredEnv(req, instance)
	if err != nil {
		return server, mcp.ServerConfig{}, nil, err
	}

	var missingInstanceConfig []string
	serverConfig.PassthroughHeaderNames, serverConfig.PassthroughHeaderValues, missingInstanceConfig = mcpServerInstanceHeaders(instance, instanceCredEnv)
	missingConfig = append(missingConfig, missingInstanceConfig...)

	if len(missingConfig) > 0 {
		if allowMissingConfig {
			return server, serverConfig, missingConfig, nil
		}
		return server, mcp.ServerConfig{}, missingConfig, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	// Best effort to update the last request time.
	// Don't update on every request, only if it's been a while since the last update, to avoid excessive writes to storage.
	if time.Since(server.Status.LastRequestTime.Time) > requestTimeUpdateInterval {
		server.Status.LastRequestTime = metav1.Now()
		if err := req.Storage.Status().Update(req.Context(), &server); err != nil {
			log.Warnf("failed to update mcp server status: %v", err)
		}
	}

	return server, serverConfig, nil, nil
}

func ServerForAction(req api.Context, id, secretBindingAllowedLabel string) (v1.MCPServer, mcp.ServerConfig, error) {
	var server v1.MCPServer
	if err := req.Get(&server, id); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	serverConfig, _, err := serverConfigForAction(req, server, secretBindingAllowedLabel, false)
	return server, serverConfig, err
}

// serverConfigForAction handles both composite and non-composite server configuration.
// For composite servers, it uses the tokenService to create an ephemeral token and constructs
// a remote MCP server config pointing to the gateway. For non-composite servers, it retrieves
// credentials and builds the appropriate server configuration.
func serverConfigForAction(req api.Context, server v1.MCPServer, secretBindingAllowedLabel string, allowMissingConfig bool) (mcp.ServerConfig, []string, error) {
	if server.Spec.NeedsURL {
		if allowMissingConfig {
			return mcp.ServerConfig{}, []string{"URL"}, nil
		}
		return mcp.ServerConfig{}, nil, types.NewErrBadRequest("mcp server %s needs to update its URL", server.Name)
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

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&server)

	cred, err := req.GatewayClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return mcp.ServerConfig{}, nil, fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, secretBindingAllowedLabel)
	if err != nil {
		return mcp.ServerConfig{}, nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	catalogName := server.Spec.MCPCatalogID
	if catalogName == "" {
		catalogName = server.Status.MCPCatalogID
	}
	if catalogName == "" {
		// For multi-user servers in a workspace, use the workspace ID as the catalog name
		catalogName = server.Spec.PowerUserWorkspaceID
	}
	// Look up catalog entry for catalog name if needed
	if server.Spec.MCPServerCatalogEntryName != "" {
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, server.Spec.MCPServerCatalogEntryName); err == nil {
			if catalogName == "" {
				catalogName = entry.Spec.MCPCatalogName
			}
			if catalogName == "" {
				catalogName = entry.Spec.PowerUserWorkspaceID
			}
		} else if apierrors.IsNotFound(err) && server.Spec.CompositeName != "" {
			// For composite component's, this usually happens when the component's catalog entry
			// was deleted, but the component hasn't been removed from the composite catalog entry yet.
			// At the moment, composite MCP servers can only contain catalog entries from the default catalog,
			// so we can assume the deleted entry belongs to the default catalog for now.
			if catalogName == "" {
				catalogName = system.DefaultCatalog
			}
		} else {
			return mcp.ServerConfig{}, nil, fmt.Errorf("failed to get MCP server catalog entry: %w", err)
		}
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
		tokenExchangeCred, tokenCredErr = req.GatewayClient.RevealCredential(req.Context(), []string{server.Name}, server.Name)
		return tokenCredErr
	}); err != nil {
		return mcp.ServerConfig{}, nil, fmt.Errorf("failed to find token exchange credential: %w", tokenCredErr)
	}

	baseURL := strings.TrimSuffix(req.APIBaseURL, "/api")
	var (
		serverConfig  mcp.ServerConfig
		missingConfig []string
	)
	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		var componentServers v1.MCPServerList
		if err = req.List(&componentServers,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{"spec.compositeName": server.Name},
		); err != nil {
			return mcp.ServerConfig{}, nil, fmt.Errorf("failed to list component servers: %w", err)
		}

		var componentInstances v1.MCPServerInstanceList
		if err = req.List(&componentInstances,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{"spec.compositeName": server.Name},
		); err != nil {
			return mcp.ServerConfig{}, nil, fmt.Errorf("failed to list component servers instances: %w", err)
		}

		serverConfig, missingConfig, err = mcp.CompositeServerToServerConfig(server, componentServers.Items, componentInstances.Items, server.ValidConnectURLs(baseURL), baseURL, req.User.GetUID(), scope, catalogName, mergedEnv, tokenExchangeCred.Secrets)
		componentMissingConfig, componentErr := compositeComponentsMissingConfig(req, secretBindingAllowedLabel, componentServers.Items, componentInstances.Items)
		if componentErr != nil {
			return mcp.ServerConfig{}, nil, componentErr
		}
		missingConfig = append(missingConfig, componentMissingConfig...)
	} else {
		serverConfig, missingConfig, err = mcp.ServerToServerConfig(server, server.ValidConnectURLs(baseURL), baseURL, req.User.GetUID(), scope, catalogName, mergedEnv, tokenExchangeCred.Secrets)
	}
	if err != nil {
		return mcp.ServerConfig{}, nil, err
	}

	if len(missingConfig) > 0 {
		if allowMissingConfig {
			return serverConfig, missingConfig, nil
		}
		return mcp.ServerConfig{}, missingConfig, types.NewErrBadRequest("missing required config: %s", strings.Join(missingConfig, ", "))
	}

	// Best effort to update the last request time.
	// Don't update on every request, only if it's been a while since the last update, to avoid excessive writes to storage.
	if time.Since(server.Status.LastRequestTime.Time) > requestTimeUpdateInterval {
		server.Status.LastRequestTime = metav1.Now()
		if err := req.Storage.Status().Update(req.Context(), &server); err != nil {
			log.Warnf("failed to update mcp server status: %v", err)
		}
	}

	return serverConfig, nil, nil
}

func compositeComponentsMissingConfig(req api.Context, secretBindingAllowedLabel string, componentServers []v1.MCPServer, componentInstances []v1.MCPServerInstance) ([]string, error) {
	var missingConfig []string
	for _, component := range componentServers {
		_, componentMissingConfig, err := serverConfigForAction(req, component, secretBindingAllowedLabel, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get config for component server %s: %w", component.Name, err)
		}
		for _, missing := range componentMissingConfig {
			missingConfig = append(missingConfig, fmt.Sprintf("%s: %s", component.Spec.MCPServerCatalogEntryName, missing))
		}
	}

	for _, instance := range componentInstances {
		_, _, instanceMissingConfig, err := serverFromMCPServerInstance(req, instance, secretBindingAllowedLabel, true)
		if err != nil {
			return nil, fmt.Errorf("failed to get config for component server instance %s: %w", instance.Name, err)
		}
		for _, missing := range instanceMissingConfig {
			missingConfig = append(missingConfig, fmt.Sprintf("%s: %s", instance.Spec.MCPServerName, missing))
		}
	}

	return missingConfig, nil
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

func serverForAction(req api.Context, secretBindingAllowedLabel string) (v1.MCPServer, mcp.ServerConfig, error) {
	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
		return server, mcp.ServerConfig{}, err
	}

	serverConfig, _, err := serverConfigForAction(req, server, secretBindingAllowedLabel, false)
	return server, serverConfig, err
}

func serverForActionWithCapabilities(req api.Context, mcpSessionManager *mcp.SessionManager, secretBindingAllowedLabel string) (v1.MCPServer, mcp.ServerConfig, nmcp.ServerCapabilities, error) {
	server, serverConfig, err := serverForAction(req, secretBindingAllowedLabel)
	if err != nil {
		return server, serverConfig, nmcp.ServerCapabilities{}, err
	}

	caps, err := mcpSessionManager.ServerCapabilities(req.Context(), serverConfig)
	return server, serverConfig, caps, err
}

// serverManifestFromCatalogEntryManifest converts a catalog entry manifest to a server manifest.
// If the user is an admin, they can override anything from the catalog entry.
func serverManifestFromCatalogEntryManifest(
	isAdmin bool,
	disableHostnameValidation bool,
	entry types.MCPServerCatalogEntryManifest,
	input types.MCPServerManifest,
) (types.MCPServerManifest, error) {
	var result types.MCPServerManifest

	if entry.Runtime == types.RuntimeComposite {
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

			// Check if the component has gained static OAuth.
			// If so, reject the update - static OAuth components cannot be part of composites.
			entryHasStaticOAuth := entryComponent.Manifest.Runtime == types.RuntimeRemote &&
				entryComponent.Manifest.RemoteConfig != nil &&
				entryComponent.Manifest.RemoteConfig.StaticOAuthRequired
			inputHasStaticOAuth := inputComponent.Manifest.Runtime == types.RuntimeRemote &&
				inputComponent.Manifest.RemoteConfig != nil &&
				inputComponent.Manifest.RemoteConfig.StaticOAuthRequired

			if entryHasStaticOAuth && !inputHasStaticOAuth {
				// The component has gained static OAuth - reject the update.
				return types.MCPServerManifest{}, types.NewErrBadRequest(
					"cannot update composite server: component %s has been updated to require static OAuth, which is not allowed in composite servers",
					entryComponent.ComponentID(),
				)
			}

			if entryComponent.Manifest.Runtime == types.RuntimeRemote &&
				entryComponent.Manifest.RemoteConfig != nil &&
				entryComponent.Manifest.RemoteConfig.Hostname != "" &&
				inputComponent.Manifest.RemoteConfig != nil {
				// Add protocol prefix to the URL if it's missing
				if url := inputComponent.Manifest.RemoteConfig.URL; url != "" && !strings.HasPrefix(url, "http") {
					inputComponent.Manifest.RemoteConfig.URL = "https://" + url
				}
				userURL = inputComponent.Manifest.RemoteConfig.URL
			}

			// Map the catalog entry to a server manifest.
			// Pass the disabled field to bypass hostname validation for disabled remote components.
			// This is necessary because users don't need to provide required configuration for disabled components.
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
		// Non-composite: use the mapping function from types package to convert catalog entry to server manifest
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

	// If the user is an admin, they can override anything from the catalog entry.
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

			if len(override.RemoteConfig.Headers) > 0 {
				existing.RemoteConfig.Headers = override.RemoteConfig.Headers
			}
		}
	}

	return existing
}

// applySecretBindingOverlay copies admin-selected secret bindings from the request
// onto matching template fields while preserving the template-owned runtime shape.
func applySecretBindingOverlay(manifest types.MCPServerManifest, overlay types.MCPServerManifest) types.MCPServerManifest {
	bindingsByEnv := secretBindingsByEnv(overlay.Env, false)
	for i := range manifest.Env {
		if binding := bindingsByEnv[manifest.Env[i].Key]; binding != nil {
			manifest.Env[i].SecretBinding = binding
			manifest.Env[i].Value = ""
		}
	}

	if manifest.RemoteConfig != nil && overlay.RemoteConfig != nil {
		bindingsByHeader := secretBindingsByHeader(overlay.RemoteConfig.Headers, false)
		for i := range manifest.RemoteConfig.Headers {
			if binding := bindingsByHeader[manifest.RemoteConfig.Headers[i].Key]; binding != nil {
				manifest.RemoteConfig.Headers[i].SecretBinding = binding
				manifest.RemoteConfig.Headers[i].Value = ""
			}
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
	manifestBindingsByEnv := secretBindingsByEnv(manifest.Env, true)
	for _, field := range source.Env {
		if field.SecretBinding == nil {
			continue
		}
		binding, ok := manifestBindingsByEnv[field.Key]
		if !ok {
			if requirePinnedFields {
				return types.NewErrBadRequest("env %q: cannot omit catalog entry secretBinding", field.Key)
			}
			continue
		}
		if !sameSecretBinding(field.SecretBinding, binding) {
			return types.NewErrBadRequest("env %q: cannot override catalog entry secretBinding", field.Key)
		}
	}

	if source.RemoteConfig == nil || manifest.RemoteConfig == nil {
		return nil
	}
	// Include nil bindings so a present field with no binding is treated as an
	// attempt to clear a catalog-owned binding. Omitted fields are allowed only
	// for partial deploy-time overlays, not full update payloads.
	manifestBindingsByHeader := secretBindingsByHeader(manifest.RemoteConfig.Headers, true)
	for _, field := range source.RemoteConfig.Headers {
		if field.SecretBinding == nil {
			continue
		}
		binding, ok := manifestBindingsByHeader[field.Key]
		if !ok {
			if requirePinnedFields {
				return types.NewErrBadRequest("header %q: cannot omit catalog entry secretBinding", field.Key)
			}
			continue
		}
		if !sameSecretBinding(field.SecretBinding, binding) {
			return types.NewErrBadRequest("header %q: cannot override catalog entry secretBinding", field.Key)
		}
	}

	return nil
}

// markAdminAddedSecretBindings derives server-owned AdminAdded metadata from the
// source catalog entry instead of trusting values supplied by UI or API clients.
func markAdminAddedSecretBindings(manifest *types.MCPServerManifest, source *types.MCPServerCatalogEntryManifest) {
	var sourceEnv map[string]*types.MCPSecretBinding
	var sourceHeaders map[string]*types.MCPSecretBinding
	if source != nil {
		sourceEnv = secretBindingsByEnv(source.Env, false)
		if source.RemoteConfig != nil {
			sourceHeaders = secretBindingsByHeader(source.RemoteConfig.Headers, false)
		}
	}
	for i := range manifest.Env {
		markAdminAddedSecretBinding(manifest.Env[i].SecretBinding, sourceEnv[manifest.Env[i].Key])
	}
	if manifest.RemoteConfig != nil {
		for i := range manifest.RemoteConfig.Headers {
			header := manifest.RemoteConfig.Headers[i]
			markAdminAddedSecretBinding(header.SecretBinding, sourceHeaders[header.Key])
		}
	}
}

func markAdminAddedSecretBinding(binding, sourceBinding *types.MCPSecretBinding) {
	if binding == nil {
		return
	}
	binding.AdminAdded = !sameSecretBinding(sourceBinding, binding)
}

func secretBindingsByEnv(fields []types.MCPEnv, includeNil bool) map[string]*types.MCPSecretBinding {
	bindings := make(map[string]*types.MCPSecretBinding, len(fields))
	for _, field := range fields {
		if includeNil || field.SecretBinding != nil {
			bindings[field.Key] = field.SecretBinding
		}
	}
	return bindings
}

func secretBindingsByHeader(fields []types.MCPHeader, includeNil bool) map[string]*types.MCPSecretBinding {
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
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.MCPServerPrefix,
			Namespace:    req.Namespace(),
			Finalizers:   []string{v1.MCPServerFinalizer},
		},
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
		if err := validation.ValidateCatalogEntryForRoute(catalogEntry.Spec.Manifest, catalogID, workspaceID); err != nil {
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

		// For multi-user catalog entries, preserve the catalog entry's runtime shape.
		// Admins may override single-user catalog entry config.
		isAdminOverride := req.UserIsAdmin() && catalogEntry.Spec.Manifest.ServerUserType.IsSingleUser()
		manifest, err := serverManifestFromCatalogEntryManifest(isAdminOverride, false, catalogEntry.Spec.Manifest, input.MCPServerManifest)
		if err != nil {
			return err
		}
		if req.UserIsAdmin() && catalogID != "" && !catalogEntry.Spec.Manifest.ServerUserType.IsSingleUser() {
			if err := rejectCatalogSecretBindingOverrides(input.MCPServerManifest, &catalogEntry.Spec.Manifest, false); err != nil {
				return err
			}
			manifest = applySecretBindingOverlay(manifest, input.MCPServerManifest)
		}

		server.Spec.Manifest = manifest
		server.Spec.UnsupportedTools = catalogEntry.Spec.UnsupportedTools
		gitManagedEntry = catalogEntry.IsGitManaged()
	} else if req.UserIsAdmin() || workspaceID != "" {
		// If the user is an admin, or if this server is being created in a workspace by a PowerUserPlus,
		// they can create a server with a manifest that is not in the catalog.
		server.Spec.Manifest = input.MCPServerManifest
	} else {
		return types.NewErrBadRequest("catalogEntryID is required")
	}

	if err := validation.ValidateServerManifest(req.Context(), server.Spec.Manifest, !server.Spec.IsSingleUser(), ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	adminManagedSecretBindings := req.UserIsAdmin() && server.Spec.IsCatalogServer()
	if adminManagedSecretBindings {
		markAdminAddedSecretBindings(&server.Spec.Manifest, sourceCatalogEntryManifest)
	}
	if err := validation.ValidateSecretBindings(server.Spec.Manifest, gitManagedEntry, adminManagedSecretBindings, m.mcpRuntimeBackend); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	addExtractedEnvVars(&server)
	// Run after extraction so auto-created Required=true entries cover any
	// template references the user did not pre-declare. This still catches the
	// case where the user pre-supplied a matching env entry with required=false
	// (which would otherwise ship a literal "${VAR}" string at runtime).
	if err := validation.ValidateTemplateReferences(server.Spec.Manifest); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if err := req.Create(&server); err != nil {
		return err
	}

	var (
		cred gatewaytypes.Credential
		err  error
	)
	if catalogID != "" {
		cred, err = req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", catalogID, server.Name)}, server.Name)
	} else if workspaceID != "" {
		cred, err = req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", workspaceID, server.Name)}, server.Name)
	} else {
		cred, err = req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name)}, server.Name)
	}
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, m.secretBindingAllowedLabel)
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

	if err = req.Read(&updated); err != nil {
		return err
	}
	if updated.RemoteConfig != nil && !strings.HasPrefix(updated.RemoteConfig.URL, "http") {
		updated.RemoteConfig.URL = "https://" + updated.RemoteConfig.URL
	}

	// Shutdown any server that is using the default credentials.
	var cred gatewaytypes.Credential
	if catalogID != "" {
		cred, err = req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", catalogID, existing.Name)}, existing.Name)
	} else if workspaceID != "" {
		cred, err = req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", workspaceID, existing.Name)}, existing.Name)
	} else {
		cred, err = req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", req.User.GetUID(), existing.Name)}, existing.Name)
	}
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	if err := validation.ValidateServerManifest(req.Context(), updated, !existing.Spec.IsSingleUser(), ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	var gitManagedEntry bool
	var sourceCatalogEntryManifest *types.MCPServerCatalogEntryManifest
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
	if err := validation.ValidateSecretBindings(updated, gitManagedEntry, adminManagedSecretBindings, m.mcpRuntimeBackend); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}
	if err := validation.ValidateTemplateReferences(updated); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
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

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, existing.Spec.Manifest.Env, existing.Spec.Manifest.RemoteConfig, cred.Secrets, m.secretBindingAllowedLabel)
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

	// Handle composite server configuration differently
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		// Composite servers have nested env vars.
		// The keys are the catalog entry IDs and the values are the env vars for that component server.
		return m.configureCompositeServer(req, mcpServer)
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var envVars map[string]string
	if err := req.Read(&envVars); err != nil {
		return err
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
			if err := updateMCPServerURLFromCatalogEntry(req.Context(), &mcpServer, catalogEntry, url, ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
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
			// Apply the URL template with environment variables
			finalURL, err := applyURLTemplate(catalogEntry.Spec.Manifest.RemoteConfig.URLTemplate, envVars)
			if err != nil {
				return fmt.Errorf("failed to apply URL template: %w", err)
			}

			// Update the server's remote config URL with the processed template
			if mcpServer.Spec.Manifest.RemoteConfig == nil {
				mcpServer.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{}
			}
			mcpServer.Spec.Manifest.RemoteConfig.URL = finalURL

			if err := validation.ValidateServerManifest(req.Context(), mcpServer.Spec.Manifest, !mcpServer.Spec.IsSingleUser(), ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
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

	var credCtx string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
	} else if workspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", workspaceID, mcpServer.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
	}

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

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, mcpServer.Spec.Manifest.Env, mcpServer.Spec.Manifest.RemoteConfig, envVars, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	return req.Write(ConvertMCPServer(mcpServer, mergedEnv, m.serverURL, slug))
}

func (m *MCPHandler) configureCompositeServer(req api.Context, compositeServer v1.MCPServer) error {
	// Read configuration from request body
	var configRequest struct {
		ComponentConfigs map[string]struct {
			Config   map[string]string `json:"config"`
			URL      string            `json:"url"`
			Disabled bool              `json:"disabled"`
		} `json:"componentConfigs"`
	}
	if err := req.Read(&configRequest); err != nil {
		return types.NewErrBadRequest("failed to read configuration: %v", err)
	}
	if len(configRequest.ComponentConfigs) < 1 {
		return types.NewErrBadRequest("no component configurations provided")
	}

	// Wait for the composite server's child MCP servers and instances to match it's current runtime configuration.
	compositeServer, err := waitForCompositeReady(req, compositeServer, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to wait for composite server to be ready for configuration: %w", err)
	}

	// Query component servers
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	// Build an index of existing servers and their credential contexts
	// This lets us get/set the credential for each server
	existingServers := make(map[string]v1.MCPServer, len(componentServers.Items))
	for _, server := range componentServers.Items {
		if id := server.Spec.MCPServerCatalogEntryName; id != "" {
			existingServers[id] = server
		}
	}

	var componentInstances v1.MCPServerInstanceList
	if err := req.List(&componentInstances,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component instances: %w", err)
	}

	existingInstances := make(map[string]v1.MCPServerInstance, len(componentInstances.Items))
	for _, instance := range componentInstances.Items {
		if id := instance.Spec.MCPServerName; id != "" {
			existingInstances[id] = instance
		}
	}

	var (
		manifestChanged   bool
		oldManifestHash   = utils.Digest(compositeServer.Spec.Manifest)
		componentCreds    = make(map[string]gatewaytypes.Credential, len(existingServers))
		componentInstance = make(map[string]v1.MCPServerInstance, len(existingInstances))
	)
	for i, component := range compositeServer.Spec.Manifest.CompositeConfig.ComponentServers {
		componentID := component.ComponentID()
		if componentID == "" {
			continue
		}

		config, hasConfig := configRequest.ComponentConfigs[componentID]
		if !hasConfig {
			// Skip components we're not configuring
			continue
		}

		sanitizeConfig(config.Config, component.Manifest)

		if component.Disabled != config.Disabled {
			component.Disabled = config.Disabled
			manifestChanged = true
		}

		if instance, instanceExists := existingInstances[componentID]; instanceExists && !component.Disabled {
			componentCreds[componentID] = gatewaytypes.Credential{
				Context: MCPServerInstanceCredentialContext(instance),
				Name:    instance.Name,
				Secrets: config.Config,
			}
			componentInstance[componentID] = instance
		}

		if server, serverExists := existingServers[componentID]; serverExists && !component.Disabled {
			if runtime, remoteConfig := component.Manifest.Runtime, component.Manifest.RemoteConfig; runtime == types.RuntimeRemote && remoteConfig != nil {
				// Handle URL changes for templates and hostname constraints
				originalURL := remoteConfig.URL
				if remoteConfig.URLTemplate != "" {
					finalURL, err := applyURLTemplate(remoteConfig.URLTemplate, config.Config)
					if err != nil {
						return fmt.Errorf("failed to apply URL template %w", err)
					}
					remoteConfig.URL = finalURL
				} else if remoteConfig.Hostname != "" {
					remoteConfig.URL = config.URL
					if remoteConfig.URL != "" && !strings.HasPrefix(remoteConfig.URL, "http") {
						remoteConfig.URL = "https://" + remoteConfig.URL
					}
				}

				if remoteConfig.URL != originalURL {
					// Capture and validate the changes
					component.Manifest.RemoteConfig = remoteConfig
					if err := validation.ValidateServerManifest(req.Context(), component.Manifest, false, ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
						return fmt.Errorf("failed to validate server manifest %w", err)
					}
					server.Spec.Manifest = component.Manifest
					server.Spec.NeedsURL = false
					server.Spec.PreviousURL = ""
					if err := req.Update(&server); err != nil {
						return fmt.Errorf("failed to update component server URL configuration: %w", err)
					}
					existingServers[componentID] = server

					// Mark the composite manifest as changed
					manifestChanged = true
				}
			}

			// Capture the updated credential
			componentCreds[componentID] = gatewaytypes.Credential{
				Context: fmt.Sprintf("%s-%s", req.User.GetUID(), server.Name),
				Name:    server.Name,
				Secrets: config.Config,
			}
		}

		// Capture any changes made back to the composite server's manifest
		compositeServer.Spec.Manifest.CompositeConfig.ComponentServers[i] = component
	}

	// Create or update component server credentials
	// We do this in parallel because shutting down servers can take some time
	g, ctx := errgroup.WithContext(req.Context())
	for id, cred := range componentCreds {
		id, cred := id, cred // Rescope variables for closure
		g.Go(func() error {
			modified, err := ensureCredential(ctx, req.GatewayClient, cred)
			if err != nil {
				return fmt.Errorf("failed to ensure credential for component %s: %w", id, err)
			}

			if modified {
				if instance, ok := componentInstance[id]; ok {
					_, serverConfig, _, err := serverFromMCPServerInstance(req, instance, m.secretBindingAllowedLabel, false)
					if err == nil {
						// Best effort
						if err = m.mcpSessionManager.CloseClient(ctx, serverConfig, "default"); err != nil {
							log.Warnf("failed to close client for component %s: %v", instance.Name, err)
						}
					}
				} else {
					// Only remove the server if the credential was created or updated
					if err := m.removeMCPServer(ctx, existingServers[id]); err != nil {
						return fmt.Errorf("failed to remove MCP server: %w", err)
					}
				}
			}

			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("failed to save credentials for components: %w", err)
	}
	for id := range componentCreds {
		if server, ok := existingServers[id]; ok {
			if err := m.triggerMCPServerControllers(req.Context(), server.Name); err != nil {
				return fmt.Errorf("failed to trigger component MCP server reconciliation: %w", err)
			}
		}
	}

	if manifestChanged {
		// The composite MCP server's manifest has changed due to component configuration changes (e.g. Disabled or RemoteConfig fields)
		compositeServer, err = m.updateCompositeManifest(req, compositeServer.Name, oldManifestHash, compositeServer.Spec.Manifest)
		if err != nil {
			return fmt.Errorf("failed to update composite server manifest: %w", err)
		}

		// Wait for the update to be applied across all component servers
		compositeServer, err = waitForCompositeReady(req, compositeServer, 30*time.Second)
		if err != nil {
			return fmt.Errorf("failed to wait for composite server to be ready for configuration: %w", err)
		}
	}

	// Re-resolve the latest components to pick up latest config
	components, err := resolveCompositeComponents(req, compositeServer, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve component servers: %w", err)
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, compositeServer, req.User.GetUID(), "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, compositeServer.Spec.Manifest.Env, compositeServer.Spec.Manifest.RemoteConfig, nil, m.secretBindingAllowedLabel)
	if err != nil {
		return fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	return req.Write(ConvertMCPServer(compositeServer, mergedEnv, m.serverURL, slug, components...))
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
	for _, env := range manifest.Env {
		if env.SecretBinding != nil {
			bound[env.Key] = struct{}{}
		}
	}
	if manifest.RemoteConfig != nil {
		for _, header := range manifest.RemoteConfig.Headers {
			if header.SecretBinding != nil {
				bound[header.Key] = struct{}{}
			}
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

// applyURLTemplate applies a URL template with environment variables
// The template uses ${VARIABLE_NAME} syntax for variable substitution
func applyURLTemplate(templateStr string, envVars map[string]string) (string, error) {
	result := templateStr

	// Replace all ${VARIABLE_NAME} patterns with actual values
	for key, value := range envVars {
		placeholder := fmt.Sprintf("${%s}", key)
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result, nil
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
		return m.deconfigureCompositeServer(req, mcpServer)
	}

	// Add extracted env vars to the server definition
	addExtractedEnvVars(&mcpServer)

	var credCtx string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
	} else if workspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", workspaceID, mcpServer.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
	}

	if err := m.removeMCPServerAndCred(req.Context(), req.GatewayClient, mcpServer, []string{credCtx}); err != nil {
		return err
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, mcpServer, req.User.GetUID(), catalogID, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(ConvertMCPServer(mcpServer, nil, m.serverURL, slug))
}

func (m *MCPHandler) deconfigureCompositeServer(req api.Context, compositeServer v1.MCPServer) error {
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	for _, component := range componentServers.Items {
		addExtractedEnvVars(&component)

		if err := m.removeMCPServerAndCred(req.Context(), req.GatewayClient, component, []string{fmt.Sprintf("%s-%s", req.User.GetUID(), component.Name)}); err != nil {
			return err
		}
	}

	// Deconfigure any component MCPServerInstances created for this composite
	var componentInstances v1.MCPServerInstanceList
	if err := req.List(&componentInstances,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component instances: %w", err)
	}
	for _, instance := range componentInstances.Items {
		if err := DeleteCredentialIfExists(req.Context(), req.GatewayClient, []string{MCPServerInstanceCredentialContext(instance)}, instance.Name); err != nil {
			return fmt.Errorf("failed to delete component instance configuration %s: %w", instance.Name, err)
		}
		_, serverConfig, _, err := serverFromMCPServerInstance(req, instance, m.secretBindingAllowedLabel, false)
		if err == nil {
			// Best effort
			if err = m.mcpSessionManager.CloseClient(req.Context(), serverConfig, "default"); err != nil {
				log.Warnf("failed to close client for component %s: %v", instance.Name, err)
			}
		}
	}

	addExtractedEnvVars(&compositeServer)

	var (
		scope   = req.User.GetUID()
		credCtx = fmt.Sprintf("%s-%s", scope, compositeServer.Name)
	)
	if err := m.removeMCPServerAndCred(req.Context(), req.GatewayClient, compositeServer, []string{credCtx}); err != nil {
		return err
	}

	slug, err := SlugForMCPServer(req.Context(), req.Storage, compositeServer, scope, "", "")
	if err != nil {
		return fmt.Errorf("failed to generate slug: %w", err)
	}

	return req.Write(ConvertMCPServer(compositeServer, nil, m.serverURL, slug))
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

	// If this is a composite, return per-component configs
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		return m.revealCompositeServer(req, mcpServer)
	}

	var credCtx string
	if catalogID != "" {
		credCtx = fmt.Sprintf("%s-%s", catalogID, mcpServer.Name)
	} else if workspaceID != "" {
		credCtx = fmt.Sprintf("%s-%s", workspaceID, mcpServer.Name)
	} else {
		credCtx = fmt.Sprintf("%s-%s", req.User.GetUID(), mcpServer.Name)
	}

	// Non-composite: return flat env
	cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{credCtx}, mcpServer.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	} else if err == nil {
		return req.Write(sanitizedConfigCopy(cred.Secrets, mcpServer.Spec.Manifest))
	}

	return types.NewErrNotFound("no credential found for %q", mcpServer.Name)
}

// revealCompositeServer returns the per-component configuration values (env and URL) for a composite server
func (m *MCPHandler) revealCompositeServer(req api.Context, compositeServer v1.MCPServer) error {
	// List component servers for this composite
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	var componentInstances v1.MCPServerInstanceList
	if err := req.List(&componentInstances,
		kclient.InNamespace(compositeServer.Namespace),
		kclient.MatchingFields{"spec.compositeName": compositeServer.Name},
	); err != nil {
		return fmt.Errorf("failed to list component instances: %w", err)
	}

	var compositeConfig types.CompositeRuntimeConfig
	if compositeServer.Spec.Manifest.CompositeConfig != nil {
		compositeConfig = *compositeServer.Spec.Manifest.CompositeConfig
	}

	// Build disabled set from parent composite
	disabledComponents := make(map[string]bool, len(compositeConfig.ComponentServers))
	for _, comp := range compositeConfig.ComponentServers {
		if id := comp.ComponentID(); id != "" {
			disabledComponents[id] = comp.Disabled
		}
	}

	type componentConfig struct {
		Config   map[string]string `json:"config"`
		URL      string            `json:"url"`
		Disabled bool              `json:"disabled"`
	}
	result := make(map[string]componentConfig, len(disabledComponents))

	// For each component, reveal its credential context and URL
	for _, component := range componentServers.Items {
		cred, err := req.GatewayClient.RevealCredential(
			req.Context(),
			[]string{fmt.Sprintf("%s-%s", req.User.GetUID(), component.Name)},
			component.Name,
		)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential for component %s: %w", component.Name, err)
		}

		cfg := map[string]string{}
		for k, v := range sanitizedConfigCopy(cred.Secrets, component.Spec.Manifest) {
			if v != "" {
				cfg[k] = v
			}
		}

		url := ""
		if component.Spec.Manifest.RemoteConfig != nil {
			url = component.Spec.Manifest.RemoteConfig.URL
		}

		catalogEntryID := component.Spec.MCPServerCatalogEntryName
		result[catalogEntryID] = componentConfig{
			Config:   cfg,
			URL:      url,
			Disabled: disabledComponents[catalogEntryID],
		}
	}

	for _, instance := range componentInstances.Items {
		cred, err := req.GatewayClient.RevealCredential(
			req.Context(),
			[]string{MCPServerInstanceCredentialContext(instance)},
			instance.Name,
		)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return fmt.Errorf("failed to find credential for component instance %s: %w", instance.Name, err)
		}

		mcpServerID := instance.Spec.MCPServerName
		result[mcpServerID] = componentConfig{
			Config:   cred.Secrets,
			Disabled: disabledComponents[mcpServerID],
		}
	}

	// Include any components present only in the disabled set (e.g., multi-user components keyed by MCPServerID)
	for key, disabled := range disabledComponents {
		if _, exists := result[key]; exists {
			// If the component is already in the result, skip to preserve revealed values
			continue
		}
		result[key] = componentConfig{
			Disabled: disabled,
		}
	}

	return req.Write(map[string]any{"componentConfigs": result})
}

func toolsForServer(ctx context.Context, mcpSessionManager *mcp.SessionManager, server v1.MCPServer, serverConfig mcp.ServerConfig) ([]types.MCPServerTool, error) {
	gTools, err := mcpSessionManager.ListTools(ctx, serverConfig)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		if strings.HasSuffix(strings.ToLower(err.Error()), "method not found") {
			return nil, types.NewErrHTTP(http.StatusFailedDependency, "MCP server does not support tools")
		} else if _, ok := errors.AsType[nmcp.AuthRequiredErr](err); ok {
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
	for _, env := range server.Spec.Manifest.Env {
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

// addExtractedEnvVarsToCatalogEntry extracts and adds environment variables to the catalog entry manifest
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

	// Keep track of existing env vars in the manifest to avoid duplicates
	existing := make(map[string]struct{})
	for _, env := range manifest.Env {
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
			// Add the existing headers to the existing map.
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

func ConvertMCPServer(server v1.MCPServer, credEnv map[string]string, serverURL, slug string, components ...types.MCPServer) types.MCPServer {
	var missingEnvVars, missingHeaders []string

	// Check for missing required env vars. credEnv is expected to be the
	// merged map from mcp.MergeBoundCreds, so bound entries that resolved
	// are present here under their env.Key the same way user-supplied
	// values are.
	for _, env := range server.Spec.Manifest.Env {
		if !env.Required {
			continue
		}

		if _, ok := credEnv[env.Key]; !ok {
			missingEnvVars = append(missingEnvVars, env.Key)
		}
	}

	// Check for missing required headers (only for remote runtime).
	// Bound headers resolved via MergeBoundCreds are keyed by header.Key.
	if server.Spec.Manifest.Runtime == types.RuntimeRemote && server.Spec.Manifest.RemoteConfig != nil {
		for _, header := range server.Spec.Manifest.RemoteConfig.Headers {
			if !header.Required {
				continue
			}

			if _, ok := credEnv[header.Key]; !ok {
				missingHeaders = append(missingHeaders, header.Key)
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

	// For composite servers, also consider component configuration if provided
	if server.Spec.Manifest.Runtime == types.RuntimeComposite &&
		server.Spec.Manifest.CompositeConfig != nil && len(components) > 0 {
		var (
			componentServers   = server.Spec.Manifest.CompositeConfig.ComponentServers
			disabledComponents = make(map[string]bool, len(componentServers))
		)
		if credEnv != nil {
			converted.MissingRequiredEnvVars, converted.MissingRequiredHeaders = secretBoundMissingConfig(converted)
		} else {
			converted.MissingRequiredEnvVars = nil
			converted.MissingRequiredHeaders = nil
		}
		for _, comp := range componentServers {
			if id := comp.ComponentID(); id != "" {
				disabledComponents[id] = comp.Disabled
			}
		}

		for _, component := range components {
			if component.CatalogEntryID != "" && disabledComponents[component.CatalogEntryID] || component.Configured {
				continue
			}

			missingEnvVars, missingHeaders := secretBoundMissingConfig(component)
			converted.MissingRequiredEnvVars = append(converted.MissingRequiredEnvVars, missingEnvVars...)
			converted.MissingRequiredHeaders = append(converted.MissingRequiredHeaders, missingHeaders...)
			converted.Configured = false
		}
	}

	return converted
}

func ConfigurationTargetForConnectID(req api.Context, id, serverURL, secretBindingAllowedLabel string, validationOptions validation.Options) (*types.MCPServer, *types.MCPServerInstance, error) {
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

	var components []types.MCPServer
	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		components, err = resolveCompositeComponents(req, server, secretBindingAllowedLabel)
		if err != nil {
			return nil, nil, err
		}
	}

	converted := ConvertMCPServer(server, credEnv, serverURL, slug, components...)
	return &converted, nil, nil
}

func credentialEnvForMCPServer(req api.Context, server v1.MCPServer, secretBindingAllowedLabel string) (map[string]string, error) {
	var credCtxs []string
	switch {
	case server.Spec.MCPCatalogID != "":
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
	case server.Spec.PowerUserWorkspaceID != "":
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
	default:
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name))
	}

	addExtractedEnvVars(&server)

	cred, err := req.GatewayClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return nil, fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, secretBindingAllowedLabel)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve secret bindings: %w", err)
	}

	return mergedEnv, nil
}

func secretBoundMissingConfig(server types.MCPServer) (missingEnvVars, missingHeaders []string) {
	missingEnvKeys := make(map[string]struct{}, len(server.MissingRequiredEnvVars))
	for _, key := range server.MissingRequiredEnvVars {
		missingEnvKeys[key] = struct{}{}
	}
	for _, env := range server.MCPServerManifest.Env {
		if env.SecretBinding == nil {
			continue
		}
		if _, ok := missingEnvKeys[env.Key]; ok {
			missingEnvVars = append(missingEnvVars, env.Key)
		}
	}

	missingHeaderKeys := make(map[string]struct{}, len(server.MissingRequiredHeaders))
	for _, key := range server.MissingRequiredHeaders {
		missingHeaderKeys[key] = struct{}{}
	}
	if server.MCPServerManifest.RemoteConfig != nil {
		for _, header := range server.MCPServerManifest.RemoteConfig.Headers {
			if header.SecretBinding == nil {
				continue
			}
			if _, ok := missingHeaderKeys[header.Key]; ok {
				missingHeaders = append(missingHeaders, header.Key)
			}
		}
	}

	return missingEnvVars, missingHeaders
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

// resolveCompositeComponents lists components of a composite MCP server, reveals their credentials, and
// converts them to the public API type.
func resolveCompositeComponents(req api.Context, composite v1.MCPServer, secretBindingAllowedLabel string) ([]types.MCPServer, error) {
	var (
		componentServers    v1.MCPServerList
		componentInstances  v1.MCPServerInstanceList
		convertedComponents []types.MCPServer
	)

	if err := req.List(&componentServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", composite.Name),
		Namespace:     composite.Namespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list composite child servers: %w", err)
	}

	if err := req.List(&componentInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", composite.Name),
		Namespace:     composite.Namespace,
	}); err != nil {
		return nil, fmt.Errorf("failed to list composite child server instances: %w", err)
	}

	for _, component := range componentServers.Items {
		cred, err := req.GatewayClient.RevealCredential(req.Context(), []string{fmt.Sprintf("%s-%s", component.Spec.UserID, component.Name)}, component.Name)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return nil, fmt.Errorf("failed to reveal credential for component %s: %w", component.Name, err)
		}

		addExtractedEnvVars(&component)
		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, component.Spec.Manifest.Env, component.Spec.Manifest.RemoteConfig, cred.Secrets, secretBindingAllowedLabel)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve secret bindings for component %s: %w", component.Name, err)
		}
		// No slug/URL needed; only Configured/NeedsURL are used from the component
		convertedComponents = append(convertedComponents, ConvertMCPServer(component, mergedEnv, "", ""))
	}

	for _, instance := range componentInstances.Items {
		credEnv, err := mcpServerInstanceCredEnv(req, instance)
		if err != nil {
			return nil, fmt.Errorf("failed to reveal credential for component instance %s: %w", instance.Name, err)
		}

		_, _, missingHeaders := mcpServerInstanceHeaders(instance, credEnv)
		// No slug/URL needed; only CatalogEntryID and Configured are used from the component.
		convertedComponents = append(convertedComponents, types.MCPServer{
			CatalogEntryID:         instance.Spec.MCPServerName,
			Configured:             len(missingHeaders) == 0,
			MissingRequiredHeaders: missingHeaders,
		})
	}

	return convertedComponents, nil
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
		if server.Spec.IsCatalogServer() {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
		} else if server.Spec.IsPowerUserWorkspaceServer() {
			credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
		}
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
		log.Errorf("failed to load catalog entries: %v", err)
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

		// Resolve components via helper for composite servers
		var components []types.MCPServer
		if server.Spec.Manifest.Runtime == types.RuntimeComposite {
			components, err = resolveCompositeComponents(req, server, m.secretBindingAllowedLabel)
			if err != nil {
				log.Warnf("failed to resolve composite components for server %s: %v", server.Name, err)
				return err
			}
		}
		mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, credMap[server.Name], m.secretBindingAllowedLabel)
		if err != nil {
			return fmt.Errorf("failed to resolve secret bindings for server %s: %w", server.Name, err)
		}
		parent := ConvertMCPServer(server, mergedEnv, m.serverURL, slug, components...)
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
	var credCtxs []string
	if server.Spec.IsCatalogServer() {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)}
	} else if server.Spec.IsPowerUserWorkspaceServer() {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)}
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), credCtxs, server.Name)
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

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, m.secretBindingAllowedLabel)
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

	var server v1.MCPServer
	if err := req.Get(&server, req.PathValue("mcp_server_id")); err != nil {
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
	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	if err := validateServerScope(req, server); err != nil {
		return err
	}

	if server.Spec.Template {
		return types.NewErrNotFound("MCP server not found")
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
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
			return types.NewErrNotFound(nse.Error())
		}
		return err
	}

	return req.Write(details)
}

func (m *MCPHandler) RestartServerDeployment(req api.Context) error {
	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	if err := validateServerScope(req, server); err != nil {
		return err
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

	if server.Spec.Manifest.Runtime == types.RuntimeComposite {
		var compositeConfig types.CompositeRuntimeConfig
		if server.Spec.Manifest.CompositeConfig != nil {
			compositeConfig = *server.Spec.Manifest.CompositeConfig
		}
		disabledComponents := make(map[string]bool, len(compositeConfig.ComponentServers))
		for _, comp := range compositeConfig.ComponentServers {
			if comp.CatalogEntryID != "" {
				disabledComponents[comp.CatalogEntryID] = comp.Disabled
			}
		}

		// List child component servers
		var componentServers v1.MCPServerList
		if err := req.List(&componentServers,
			kclient.InNamespace(server.Namespace),
			kclient.MatchingFields{
				"spec.compositeName": server.Name,
			},
		); err != nil {
			return err
		}

		// Restart eligible component deployments (non-remote and not disabled)
		for _, component := range componentServers.Items {
			if disabledComponents[component.Spec.MCPServerCatalogEntryName] ||
				component.Spec.Manifest.Runtime == types.RuntimeRemote {
				continue
			}

			componentConfig, _, err := serverConfigForAction(req, component, m.secretBindingAllowedLabel, false)
			if err != nil {
				return err
			}

			if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), componentConfig); err != nil {
				if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
					return types.NewErrNotFound(nse.Error())
				}
				return err
			}
		}

		req.WriteHeader(http.StatusNoContent)
		return nil
	}

	if err := m.mcpSessionManager.RestartServerDeployment(req.Context(), serverConfig); err != nil {
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
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

	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
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
			if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
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
	var credCtxs []string
	if server.Spec.MCPCatalogID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name))
	} else if server.Spec.PowerUserWorkspaceID != "" {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name))
	} else {
		credCtxs = append(credCtxs, fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name))
	}

	cred, err := req.GatewayClient.RevealCredential(req.Context(), credCtxs, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to find credential: %w", err)
	}

	mergedEnv, err := mcp.MergeBoundCreds(req.Context(), req.LocalK8sClient, req.ObotNamespace, server.Spec.Manifest.Env, server.Spec.Manifest.RemoteConfig, cred.Secrets, m.secretBindingAllowedLabel)
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
	server, serverConfig, err := serverForAction(req, m.secretBindingAllowedLabel)
	if err != nil {
		return err
	}

	if err := validateServerScope(req, server); err != nil {
		return err
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
		if nse := (*mcp.ErrNotSupportedByBackend)(nil); errors.As(err, &nse) {
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

	if err := updateMCPServerURLFromCatalogEntry(req.Context(), &mcpServer, entry, input.URL, ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
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

func updateMCPServerURLFromCatalogEntry(ctx context.Context, mcpServer *v1.MCPServer, entry v1.MCPServerCatalogEntry, inputURL string, opts validation.Options) error {
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
	mcpServer.Spec.NeedsURL = false
	mcpServer.Spec.PreviousURL = ""

	if err := validation.ValidateServerManifest(ctx, mcpServer.Spec.Manifest, !mcpServer.Spec.IsSingleUser(), opts); err != nil {
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

	// Branch for composite servers
	if entry.Spec.Manifest.Runtime == types.RuntimeComposite {
		return m.triggerCompositeUpdate(req, server, entry)
	}

	candidate := server.DeepCopy()
	updateServerFromCatalogEntry(candidate, entry)
	if err := validation.ValidateServerManifest(req.Context(), candidate.Spec.Manifest, !candidate.Spec.IsSingleUser(), ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
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

		updateServerFromCatalogEntry(&latest, entry)

		// Validate again in case the catalog entry changed between retries.
		if err := validation.ValidateServerManifest(req.Context(), latest.Spec.Manifest, !latest.Spec.IsSingleUser(), ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
			return types.NewErrBadRequest("validation failed: %v", err)
		}
		return req.Update(&latest)
	}); err != nil {
		return err
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
	server.Spec.Manifest.Env = entry.Spec.Manifest.Env
	server.Spec.Manifest.Resources = entry.Spec.Manifest.Resources
	server.Spec.Manifest.Runtime = entry.Spec.Manifest.Runtime
	server.Spec.Manifest.UVXConfig = entry.Spec.Manifest.UVXConfig
	server.Spec.Manifest.NPXConfig = entry.Spec.Manifest.NPXConfig
	server.Spec.Manifest.ContainerizedConfig = entry.Spec.Manifest.ContainerizedConfig
	server.Spec.Manifest.MultiUserConfig = entry.Spec.Manifest.MultiUserConfig

	// Handle remote runtime URL updates.
	if entry.Spec.Manifest.Runtime == types.RuntimeRemote && entry.Spec.Manifest.RemoteConfig != nil {
		if entry.Spec.Manifest.RemoteConfig.FixedURL != "" {
			// Use the fixed URL from catalog entry.
			server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
				URL:                 entry.Spec.Manifest.RemoteConfig.FixedURL,
				Headers:             entry.Spec.Manifest.RemoteConfig.Headers,
				StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
			}
		} else if entry.Spec.Manifest.RemoteConfig.Hostname != "" {
			// Check if the server's current URL matches the new hostname requirement.
			if server.Spec.Manifest.RemoteConfig != nil && server.Spec.Manifest.RemoteConfig.URL != "" {
				hostnameMismatchErr := types.ValidateURLHostname(server.Spec.Manifest.RemoteConfig.URL, entry.Spec.Manifest.RemoteConfig.Hostname)

				server.Spec.NeedsURL = hostnameMismatchErr != nil
				if server.Spec.NeedsURL {
					server.Spec.PreviousURL = server.Spec.Manifest.RemoteConfig.URL
					server.Spec.Manifest.RemoteConfig.URL = ""
				}

				server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
					Headers:             entry.Spec.Manifest.RemoteConfig.Headers,
					Hostname:            entry.Spec.Manifest.RemoteConfig.Hostname,
					StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
				}
			} else {
				// No current URL, needs one.
				server.Spec.NeedsURL = true
				server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
					Headers:             entry.Spec.Manifest.RemoteConfig.Headers,
					StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
				}
			}
		} else if entry.Spec.Manifest.RemoteConfig.URLTemplate != "" {
			server.Spec.Manifest.RemoteConfig = &types.RemoteRuntimeConfig{
				Headers:             entry.Spec.Manifest.RemoteConfig.Headers,
				URLTemplate:         entry.Spec.Manifest.RemoteConfig.URLTemplate,
				StaticOAuthRequired: entry.Spec.Manifest.RemoteConfig.StaticOAuthRequired,
			}
		}
	} else {
		// For non-remote runtimes, clear the remote config.
		server.Spec.Manifest.RemoteConfig = nil
	}
}

// triggerCompositeUpdate upgrades a composite server and all its component servers from the latest catalog entry
func (m *MCPHandler) triggerCompositeUpdate(req api.Context, compositeServer v1.MCPServer, entry v1.MCPServerCatalogEntry) error {
	// Capture the hash of the initial server so we can compare changes on update.
	// This will let us abort an update if the server's manifest has changed before the update was applied.
	oldManifestHash := utils.Digest(compositeServer.Spec.Manifest)

	// Build fresh manifest with user URLs applied
	updatedManifest, err := serverManifestFromCatalogEntryManifest(
		req.UserIsAdmin(),
		true,
		entry.Spec.Manifest,
		compositeServer.Spec.Manifest,
	)
	if err != nil {
		return err
	}
	if err := validation.ValidateServerManifest(req.Context(), updatedManifest, !compositeServer.Spec.IsSingleUser(), ValidationOptionsWithResourceMaximums(m.mcpSessionManager)); err != nil {
		return types.NewErrBadRequest("validation failed: %v", err)
	}

	// Ensure the composite server's manifest is updated
	compositeServer, err = m.updateCompositeManifest(req, compositeServer.Name, oldManifestHash, updatedManifest)
	if err != nil {
		return err
	}

	// Wait for the composite server to apply the changes to all component servers
	if _, err := waitForCompositeReady(req, compositeServer, 30*time.Second); err != nil {
		return fmt.Errorf("failed to wait for component servers to sync: %w", err)
	}

	return nil
}

// updateCompositeManifest attempts to update a composite server to have the given manifest.
// This function will retry with backoff until the manifest is successfully applied, an
func (*MCPHandler) updateCompositeManifest(req api.Context, name, oldManifestHash string, manifest types.MCPServerManifest) (v1.MCPServer, error) {
	var compositeServer v1.MCPServer
	return compositeServer, kwait.ExponentialBackoffWithContext(
		req.Context(),
		retry.DefaultBackoff,
		func(context.Context) (bool, error) {
			var latest v1.MCPServer
			if err := req.Get(&latest, name); err != nil {
				return false, err
			}

			if utils.Digest(latest.Spec.Manifest) != oldManifestHash {
				return false, types.NewErrHTTP(http.StatusConflict, "manifest changed during update")
			}

			latest.Spec.Manifest = manifest
			if err := req.Update(&latest); apierrors.IsConflict(err) {
				return false, nil
			} else if err != nil {
				return false, err
			}

			compositeServer = latest

			return true, nil
		})
}

// waitForCompositeReady waits until the given timeout for the composite server's current manifest to be applied to its component servers
func waitForCompositeReady(req api.Context, compositeServer v1.MCPServer, timeout time.Duration) (v1.MCPServer, error) {
	latest, err := wait.For(
		req.Context(),
		req.Storage,
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

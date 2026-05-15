package mcpserver

import (
	"cmp"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/gptscript-ai/go-gptscript"
	"github.com/gptscript-ai/gptscript/pkg/hash"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	nmcp "github.com/obot-platform/nanobot/pkg/mcp"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

const oauthMetadataSyncInterval = time.Hour

type Handler struct {
	gptClient                    *gptscript.GPTScript
	mcpSessionManager            *mcp.SessionManager
	networkPolicyProviderEnabled bool
	defaultDenyAllEgress         bool
	singleUserIdleShutdownDelay  time.Duration
	multiUserIdleShutdownDelay   time.Duration
	agentIdleShutdownDelay       time.Duration
	baseURL                      string
}

func effectiveDenyAllEgress(v *bool, domains []string, defaultWhenEmpty bool) bool {
	if v != nil {
		return *v
	}
	return defaultWhenEmpty && len(domains) == 0
}

func New(gptClient *gptscript.GPTScript, mcpSessionManager *mcp.SessionManager, networkPolicyProviderEnabled, defaultDenyAllEgress bool, singleUserIdleShutdownDelay, multiUserIdleShutdownDelay, agentIdleShutdownDelay time.Duration, baseURL string) *Handler {
	return &Handler{
		gptClient:                    gptClient,
		mcpSessionManager:            mcpSessionManager,
		networkPolicyProviderEnabled: networkPolicyProviderEnabled,
		defaultDenyAllEgress:         defaultDenyAllEgress,
		singleUserIdleShutdownDelay:  singleUserIdleShutdownDelay,
		multiUserIdleShutdownDelay:   multiUserIdleShutdownDelay,
		agentIdleShutdownDelay:       agentIdleShutdownDelay,
		baseURL:                      baseURL,
	}
}

func (h *Handler) DetectDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" || server.Spec.CompositeName != "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	drifted, err := configurationHasDrifted(server.Spec.Manifest, entry.Spec.Manifest, h.defaultDenyAllEgress)
	if err != nil {
		return err
	}

	if server.Status.NeedsUpdate != drifted {
		log.Infof("MCP server catalog drift status changed: server=%s catalogEntry=%s needsUpdate=%v", server.Name, server.Spec.MCPServerCatalogEntryName, drifted)
		server.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

func (h *Handler) EnsureMCPNetworkPolicy(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if !h.networkPolicyProviderEnabled {
		return h.deleteMCPNetworkPolicy(req, server.Namespace, server.Name)
	}

	// Don't create an MCPNetworkPolicy if this is an agent pod
	if server.Spec.NanobotAgentID != "" {
		return nil
	}

	var egressDomains []string
	var denyAllEgress bool
	switch server.Spec.Manifest.Runtime {
	case types.RuntimeNPX:
		if server.Spec.Manifest.NPXConfig != nil {
			egressDomains = server.Spec.Manifest.NPXConfig.EgressDomains
			denyAllEgress = effectiveDenyAllEgress(server.Spec.Manifest.NPXConfig.DenyAllEgress, egressDomains, h.defaultDenyAllEgress)
		}
	case types.RuntimeUVX:
		if server.Spec.Manifest.UVXConfig != nil {
			egressDomains = server.Spec.Manifest.UVXConfig.EgressDomains
			denyAllEgress = effectiveDenyAllEgress(server.Spec.Manifest.UVXConfig.DenyAllEgress, egressDomains, h.defaultDenyAllEgress)
		}
	case types.RuntimeContainerized:
		if server.Spec.Manifest.ContainerizedConfig != nil {
			egressDomains = server.Spec.Manifest.ContainerizedConfig.EgressDomains
			denyAllEgress = effectiveDenyAllEgress(server.Spec.Manifest.ContainerizedConfig.DenyAllEgress, egressDomains, h.defaultDenyAllEgress)
		}
	default:
		return h.deleteMCPNetworkPolicy(req, server.Namespace, server.Name)
	}

	egressDomains = slices.Clone(egressDomains)
	slices.Sort(egressDomains)

	var policies v1.MCPNetworkPolicyList
	if err := req.List(&policies, &kclient.ListOptions{
		Namespace:     server.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", server.Name),
	}); err != nil {
		return err
	}

	if len(policies.Items) == 0 {
		return req.Client.Create(req.Ctx, &v1.MCPNetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: system.MCPNetworkPolicyPrefix,
				Namespace:    server.Namespace,
			},
			Spec: v1.MCPNetworkPolicySpec{
				MCPServerName: server.Name,
				PodSelector: map[string]string{
					"app": server.Name,
				},
				EgressDomains: egressDomains,
				DenyAllEgress: denyAllEgress,
			},
		})
	}

	slices.SortFunc(policies.Items, func(left, right v1.MCPNetworkPolicy) int {
		if c := left.CreationTimestamp.Compare(right.CreationTimestamp.Time); c != 0 {
			return c
		}
		return cmp.Compare(left.Name, right.Name)
	})

	policy := &policies.Items[0]
	for i := 1; i < len(policies.Items); i++ {
		if err := req.Client.Delete(req.Ctx, &policies.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	if policy.Spec.MCPServerName == server.Name &&
		maps.Equal(policy.Spec.PodSelector, map[string]string{"app": server.Name}) &&
		slices.Equal(sortedClone(policy.Spec.EgressDomains), egressDomains) &&
		policy.Spec.DenyAllEgress == denyAllEgress {
		return nil
	}

	policy.Spec.MCPServerName = server.Name
	policy.Spec.PodSelector = map[string]string{
		"app": server.Name,
	}
	policy.Spec.EgressDomains = egressDomains
	policy.Spec.DenyAllEgress = denyAllEgress
	return req.Client.Update(req.Ctx, policy)
}

func sortedClone(values []string) []string {
	cloned := slices.Clone(values)
	slices.Sort(cloned)
	return cloned
}

func (h *Handler) deleteMCPNetworkPolicy(req router.Request, namespace, name string) error {
	var policies v1.MCPNetworkPolicyList
	if err := req.List(&policies, &kclient.ListOptions{
		Namespace:     namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", name),
	}); err != nil {
		return err
	}

	for i := range policies.Items {
		if err := req.Client.Delete(req.Ctx, &policies.Items[i]); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// DetectK8sSettingsDrift detects when a server needs redeployment with new K8s settings
// Note: This handler only sets NeedsK8sUpdate based on K8sSettings hash drift.
// PSA compliance checking is handled separately in the deployment handler since it
// requires access to the actual Deployment object to inspect container security contexts.
func (h *Handler) DetectK8sSettingsDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	// Skip if server doesn't have K8s settings hash (not yet deployed)
	if server.Status.K8sSettingsHash == "" {
		return nil
	}

	// Get current K8s settings
	var k8sSettings v1.K8sSettings
	if err := req.Get(&k8sSettings, server.Namespace, system.K8sSettingsName); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get K8s settings: %w", err)
	}

	// Compute current K8s settings hash
	currentHash := mcp.ComputeK8sSettingsHash(k8sSettings.Spec)

	if server.Status.K8sSettingsHash != currentHash && !server.Status.NeedsK8sUpdate {
		log.Infof("MCP server requires K8s redeploy due to settings drift: server=%s previousHash=%s newHash=%s", server.Name, server.Status.K8sSettingsHash, currentHash)
		server.Status.NeedsK8sUpdate = true
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func configurationHasDrifted(serverManifest types.MCPServerManifest, entryManifest types.MCPServerCatalogEntryManifest, defaultDenyAllEgress bool) (bool, error) {
	// Check if runtime types differ
	if serverManifest.Runtime != entryManifest.Runtime {
		return true, nil
	}

	// Check runtime-specific configurations
	var drifted bool
	switch serverManifest.Runtime {
	case types.RuntimeUVX:
		drifted = uvxConfigHasDrifted(serverManifest.UVXConfig, entryManifest.UVXConfig, defaultDenyAllEgress)
	case types.RuntimeNPX:
		drifted = npxConfigHasDrifted(serverManifest.NPXConfig, entryManifest.NPXConfig, defaultDenyAllEgress)
	case types.RuntimeContainerized:
		drifted = containerizedConfigHasDrifted(serverManifest.ContainerizedConfig, entryManifest.ContainerizedConfig, defaultDenyAllEgress)
	case types.RuntimeRemote:
		drifted = remoteConfigHasDrifted(serverManifest.RemoteConfig, entryManifest.RemoteConfig)
	case types.RuntimeComposite:
		var err error
		drifted, err = compositeConfigHasDrifted(serverManifest.CompositeConfig, entryManifest.CompositeConfig, defaultDenyAllEgress)
		if err != nil {
			return false, err
		}
	default:
		return false, fmt.Errorf("unknown runtime type: %s", serverManifest.Runtime)
	}

	if drifted {
		return true, nil
	}

	// Check environment
	return !slicesEqualIgnoreOrderDeep(serverManifest.Env, entryManifest.Env), nil
}

// uvxConfigHasDrifted checks if UVX configuration has drifted
func uvxConfigHasDrifted(serverConfig, entryConfig *types.UVXRuntimeConfig, defaultDenyAllEgress bool) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	return serverConfig.Package != entryConfig.Package ||
		serverConfig.Command != entryConfig.Command ||
		!slices.Equal(serverConfig.Args, entryConfig.Args) ||
		!slices.Equal(serverConfig.EgressDomains, entryConfig.EgressDomains) ||
		effectiveDenyAllEgress(serverConfig.DenyAllEgress, serverConfig.EgressDomains, defaultDenyAllEgress) !=
			effectiveDenyAllEgress(entryConfig.DenyAllEgress, entryConfig.EgressDomains, defaultDenyAllEgress)
}

// npxConfigHasDrifted checks if NPX configuration has drifted
func npxConfigHasDrifted(serverConfig, entryConfig *types.NPXRuntimeConfig, defaultDenyAllEgress bool) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	return serverConfig.Package != entryConfig.Package ||
		!slices.Equal(serverConfig.Args, entryConfig.Args) ||
		!slices.Equal(serverConfig.EgressDomains, entryConfig.EgressDomains) ||
		effectiveDenyAllEgress(serverConfig.DenyAllEgress, serverConfig.EgressDomains, defaultDenyAllEgress) !=
			effectiveDenyAllEgress(entryConfig.DenyAllEgress, entryConfig.EgressDomains, defaultDenyAllEgress)
}

// containerizedConfigHasDrifted checks if containerized configuration has drifted
func containerizedConfigHasDrifted(serverConfig, entryConfig *types.ContainerizedRuntimeConfig, defaultDenyAllEgress bool) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	return serverConfig.Image != entryConfig.Image ||
		serverConfig.Command != entryConfig.Command ||
		serverConfig.Port != entryConfig.Port ||
		serverConfig.Path != entryConfig.Path ||
		!slices.Equal(serverConfig.Args, entryConfig.Args) ||
		!slices.Equal(serverConfig.EgressDomains, entryConfig.EgressDomains) ||
		effectiveDenyAllEgress(serverConfig.DenyAllEgress, serverConfig.EgressDomains, defaultDenyAllEgress) !=
			effectiveDenyAllEgress(entryConfig.DenyAllEgress, entryConfig.EgressDomains, defaultDenyAllEgress)
}

// remoteConfigHasDrifted checks if remote configuration has drifted
func remoteConfigHasDrifted(serverConfig *types.RemoteRuntimeConfig, entryConfig *types.RemoteCatalogConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	if entryConfig.Hostname != serverConfig.Hostname ||
		entryConfig.URLTemplate != serverConfig.URLTemplate {
		return true
	}

	// For remote runtime, we need to check if the server URL matches what the catalog entry expects
	if entryConfig.FixedURL != "" {
		// If catalog entry has a fixed URL, server URL should match exactly
		if serverConfig.URL != entryConfig.FixedURL {
			return true
		}
	}

	// Check if headers have drifted
	return !slicesEqualIgnoreOrderDeep(serverConfig.Headers, entryConfig.Headers)
}

func slicesEqualIgnoreOrderDeep[T any](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}

	used := make([]bool, len(b))
	for _, left := range a {
		found := false
		for i, right := range b {
			if used[i] || !reflect.DeepEqual(left, right) {
				continue
			}
			used[i] = true
			found = true
			break
		}
		if !found {
			return false
		}
	}

	return true
}

// compositeConfigHasDrifted checks if the composite configuration has drifted
func compositeConfigHasDrifted(serverConfig *types.CompositeRuntimeConfig, entryConfig *types.CompositeCatalogConfig, defaultDenyAllEgress bool) (bool, error) {
	if serverConfig == nil && entryConfig == nil {
		return false, nil
	}
	if serverConfig == nil || entryConfig == nil {
		return true, nil
	}

	// Fast length check
	if len(serverConfig.ComponentServers) != len(entryConfig.ComponentServers) {
		return true, nil
	}

	entryComponents := make(map[string]types.CatalogComponentServer, len(entryConfig.ComponentServers))
	for _, entryComponent := range entryConfig.ComponentServers {
		if id := entryComponent.ComponentID(); id != "" {
			entryComponents[id] = entryComponent
		}
	}

	for _, serverComponent := range serverConfig.ComponentServers {
		entryComponent, ok := entryComponents[serverComponent.ComponentID()]
		if !ok {
			return true, nil
		}

		// Compare tool prefix
		if serverComponent.ToolPrefix != entryComponent.ToolPrefix {
			return true, nil
		}

		// Compare tool overrides
		if hash.Digest(serverComponent.ToolOverrides) != hash.Digest(entryComponent.ToolOverrides) {
			return true, nil
		}

		// Compare manifests
		drifted, err := configurationHasDrifted(serverComponent.Manifest, entryComponent.Manifest, defaultDenyAllEgress)
		if err != nil || drifted {
			return drifted, err
		}
	}

	return false, nil
}

// EnsureMCPServerInstanceUserCount ensures that mcp server instance user count for multi-user MCP servers is up to date.
func (*Handler) EnsureMCPServerInstanceUserCount(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.IsSingleUser() {
		// Server is not multi-user, ensure we're not tracking the instance user count
		if server.Status.MCPServerInstanceUserCount == nil {
			return nil
		}

		// Corrupt state, drop the field to fix it
		server.Status.MCPServerInstanceUserCount = nil
		return req.Client.Status().Update(req.Ctx, server)
	}

	// Get the set of unique users with server instances pointing to this MCP server
	var mcpServerInstances v1.MCPServerInstanceList
	if err := req.List(&mcpServerInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.mcpServerName", server.Name),
		Namespace:     system.DefaultNamespace,
	}); err != nil {
		return fmt.Errorf("failed to list MCP server instances: %w", err)
	}

	uniqueUsers := make(map[string]struct{}, len(mcpServerInstances.Items))
	for _, instance := range mcpServerInstances.Items {
		if userID := instance.Spec.UserID; userID != "" && instance.DeletionTimestamp.IsZero() {
			uniqueUsers[userID] = struct{}{}
		}
	}

	if oldUserCount, newUserCount := server.Status.MCPServerInstanceUserCount, len(uniqueUsers); oldUserCount == nil || *oldUserCount != newUserCount {
		log.Infof("Updated MCP server instance user count: server=%s newCount=%d", server.Name, newUserCount)
		server.Status.MCPServerInstanceUserCount = &newUserCount
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) DeleteServersWithoutRuntime(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if string(server.Spec.Manifest.Runtime) == "" {
		log.Infof("Deleting MCP server with empty runtime: server=%s", server.Name)
		return req.Client.Delete(req.Ctx, server)
	}

	return nil
}

func (h *Handler) DeleteServersForAnonymousUser(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.UserID == "anonymous" {
		log.Infof("Deleting MCP server for anonymous user: server=%s", server.Name)
		return req.Client.Delete(req.Ctx, server)
	}

	return nil
}

func (h *Handler) EnsureMCPCatalogID(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if (server.Status.MCPCatalogID == "" || server.Status.MCPCatalogID == server.Spec.MCPServerCatalogEntryName) && server.Spec.MCPCatalogID == "" && server.Spec.MCPServerCatalogEntryName != "" {
		var mcpCatalogEntry v1.MCPServerCatalogEntry
		if err := req.Get(&mcpCatalogEntry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
			// Don't return an error here if the entry isn't found.
			// This will prevent the MCPServer from being requeued repeatedly when the catalog entry doesn't exist.
			return kclient.IgnoreNotFound(err)
		}

		server.Status.MCPCatalogID = mcpCatalogEntry.Spec.MCPCatalogName
		log.Infof("Resolved MCP catalog ID for server: server=%s catalogEntry=%s catalogID=%s", server.Name, server.Spec.MCPServerCatalogEntryName, server.Status.MCPCatalogID)
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) MigrateSharedWithinMCPCatalogName(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.SharedWithinMCPCatalogName != "" && server.Spec.MCPCatalogID == "" {
		server.Spec.MCPCatalogID = server.Spec.SharedWithinMCPCatalogName
		server.Spec.SharedWithinMCPCatalogName = ""
		log.Infof("Migrating MCP server shared catalog field to MCPCatalogID: server=%s catalogID=%s", server.Name, server.Spec.MCPCatalogID)
		return req.Client.Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) EnsureMCPServerSecretInfo(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	fieldSelector := fields.SelectorFromSet(map[string]string{
		"spec.mcpServerName": server.Name,
	})
	var oauthClients v1.OAuthClientList
	if err := req.List(&oauthClients, &kclient.ListOptions{
		Namespace:     req.Namespace,
		FieldSelector: fieldSelector,
	}); err != nil {
		return err
	}

	if len(oauthClients.Items) == 0 {
		// If listing with the cache doesn't return anything, double-check with the uncached listing
		if err := req.List(untriggered.UncachedList(&oauthClients), &kclient.ListOptions{
			Namespace:     req.Namespace,
			FieldSelector: fieldSelector,
		}); err != nil {
			return err
		}
	}

	if server.Status.AuditLogTokenHash != "" {
		cred, err := h.gptClient.RevealCredential(req.Ctx, []string{server.Name}, server.Name)
		if err != nil {
			return fmt.Errorf("failed to get credential: %w", err)
		}

		if server.Status.AuditLogTokenHash != hash.Digest(cred.Env["AUDIT_LOG_TOKEN"]) {
			log.Infof("Audit log token drift detected for MCP server, rotating credential: server=%s", server.Name)
			// Reset the audit log token hash to reset the credential.
			server.Status.AuditLogTokenHash = ""
		}
	}

	if len(oauthClients.Items) > 0 && (server.Status.AuditLogTokenHash != "" || server.Spec.CompositeName != "") {
		// Nothing else to do here.
		return nil
	}

	clientID := system.OAuthClientPrefix + strings.ToLower(rand.Text())
	clientSecret := strings.ToLower(rand.Text() + rand.Text())
	hashedClientSecretHash, err := bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash client secret: %w", err)
	}

	auditLogToken := strings.ToLower(rand.Text() + rand.Text())

	if err := h.gptClient.CreateCredential(req.Ctx, gptscript.Credential{
		Context:  server.Name,
		ToolName: server.Name,
		Type:     gptscript.CredentialTypeTool,
		Env: map[string]string{
			"TOKEN_EXCHANGE_CLIENT_ID":     fmt.Sprintf("%s:%s", req.Namespace, clientID),
			"TOKEN_EXCHANGE_CLIENT_SECRET": clientSecret,
			"AUDIT_LOG_TOKEN":              auditLogToken,
		},
	}); err != nil {
		return fmt.Errorf("failed to create credential: %w", err)
	}

	oauthClient := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:       clientID,
			Namespace:  req.Namespace,
			Finalizers: []string{v1.OAuthClientFinalizer},
		},
		Spec: v1.OAuthClientSpec{
			Manifest: types.OAuthClientManifest{
				GrantTypes: []string{"urn:ietf:params:oauth:grant-type:token-exchange"},
			},
			ClientSecretHash: hashedClientSecretHash,
			MCPServerName:    server.Name,
		},
	}

	if err := req.Client.Create(req.Ctx, &oauthClient); err != nil {
		return fmt.Errorf("failed to create OAuth client: %w", err)
	}

	server.Status.AuditLogTokenHash = hash.Digest(auditLogToken)
	log.Infof("Provisioned OAuth exchange credentials for MCP server: server=%s oauthClient=%s", server.Name, oauthClient.Name)

	return nil
}

// CleanupNestedCompositeServers removes component servers with composite runtimes from composite MCP servers.
// This handler cleans up servers that were created before API validation to prevent nested composite servers.
func (h *Handler) CleanupNestedCompositeServers(req router.Request, _ router.Response) error {
	var (
		server   = req.Object.(*v1.MCPServer)
		manifest = server.Spec.Manifest
	)

	if manifest.Runtime != types.RuntimeComposite ||
		manifest.CompositeConfig == nil {
		return nil
	}

	// Delete component servers with composite runtimes
	if server.Spec.CompositeName != "" {
		log.Infof("Deleting nested composite component server: server=%s parentComposite=%s", server.Name, server.Spec.CompositeName)
		return kclient.IgnoreNotFound(req.Client.Delete(req.Ctx, server))
	}
	// Remove all composite components from the server's manifest
	var (
		components    = manifest.CompositeConfig.ComponentServers
		numComponents = len(components)
	)
	components = slices.DeleteFunc(components, func(component types.ComponentServer) bool {
		return component.Manifest.Runtime == types.RuntimeComposite
	})

	if numComponents == len(components) {
		return nil
	}

	server.Spec.Manifest.CompositeConfig.ComponentServers = components
	log.Infof("Pruned nested composite components from MCP server manifest: server=%s removedComponents=%d", server.Name, numComponents-len(components))
	return kclient.IgnoreNotFound(req.Client.Update(req.Ctx, server))
}

func (h *Handler) EnsureCompositeComponents(req router.Request, _ router.Response) error {
	var (
		compositeServer = req.Object.(*v1.MCPServer)
		manifest        = compositeServer.Spec.Manifest
	)

	if manifest.Runtime != types.RuntimeComposite ||
		manifest.CompositeConfig == nil ||
		len(manifest.CompositeConfig.ComponentServers) < 1 {
		return nil
	}

	// Load all existing component servers
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return fmt.Errorf("failed to list component servers: %w", err)
	}

	// Load all existing component instances (for multi-user components)
	var componentInstances v1.MCPServerInstanceList
	if err := req.List(&componentInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return fmt.Errorf("failed to list component instances: %w", err)
	}

	// Create index of existing catalog entry components by ID
	existingServers := make(map[string]v1.MCPServer, len(componentServers.Items))
	for _, existing := range componentServers.Items {
		if id := existing.Spec.MCPServerCatalogEntryName; id != "" {
			existingServers[id] = existing
		}
	}

	// Create index of existing multi-user component instances by MCPServerID
	existingInstances := make(map[string]v1.MCPServerInstance, len(componentInstances.Items))
	for _, existing := range componentInstances.Items {
		if id := existing.Spec.MCPServerName; id != "" {
			existingInstances[existing.Spec.MCPServerName] = existing
		}
	}

	// withNeedsURL returns the given MCP server with a NeedsURL field set according to its hostname constraint and url.
	// If the server is not remote, or does not have a hostname constraint, it returns the unmodified server.
	withNeedsURL := func(server v1.MCPServer) v1.MCPServer {
		remoteConfig := compositeServer.Spec.Manifest.RemoteConfig
		if compositeServer.Spec.Manifest.Runtime != types.RuntimeRemote || remoteConfig == nil || remoteConfig.Hostname == "" {
			return server
		}

		server.Spec.NeedsURL = types.ValidateURLHostname(remoteConfig.URL, remoteConfig.Hostname) != nil
		return server
	}

	// Ensuring a composite server is up-to-date has 3 steps:
	// 1. Create new component servers and instances
	// 2. Update existing component servers (no-op on existing instances, since there's nothing to change)
	// 3. Delete removed component servers and instances
	for _, component := range manifest.CompositeConfig.ComponentServers {
		if component.MCPServerID != "" {
			// Multi-user component
			if _, exists := existingInstances[component.MCPServerID]; !exists {
				// New instance, create it
				var multiUserServer v1.MCPServer
				if err := req.Get(&multiUserServer, compositeServer.Namespace, component.MCPServerID); err != nil {
					return fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
				}

				if err := req.Client.Create(req.Ctx, &v1.MCPServerInstance{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: system.MCPServerInstancePrefix,
						Namespace:    compositeServer.Namespace,
						Finalizers:   []string{v1.MCPServerInstanceFinalizer},
					},
					Spec: v1.MCPServerInstanceSpec{
						MCPServerName:        component.MCPServerID,
						MCPCatalogName:       multiUserServer.Spec.MCPCatalogID,
						PowerUserWorkspaceID: multiUserServer.Spec.PowerUserWorkspaceID,
						MultiUserConfig:      multiUserServer.Spec.Manifest.MultiUserConfig,
						UserID:               compositeServer.Spec.UserID,
						CompositeName:        compositeServer.Name,
					},
				}); err != nil {
					return fmt.Errorf("failed to create instance for multi-user component: %w", err)
				}
				log.Infof("Created component MCPServerInstance for composite server: composite=%s componentServer=%s userID=%s", compositeServer.Name, component.MCPServerID, compositeServer.Spec.UserID)
			} else {
				existingInstance := existingInstances[component.MCPServerID]
				var multiUserServer v1.MCPServer
				if err := req.Get(&multiUserServer, compositeServer.Namespace, component.MCPServerID); err != nil {
					return fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
				}

				if hash.Digest(existingInstance.Spec.MultiUserConfig) != hash.Digest(multiUserServer.Spec.Manifest.MultiUserConfig) {
					existingInstance.Spec.MultiUserConfig = multiUserServer.Spec.Manifest.MultiUserConfig
					if err := req.Client.Update(req.Ctx, &existingInstance); err != nil {
						return fmt.Errorf("failed to update instance for multi-user component: %w", err)
					}
				}
			}

			// Remove the instance to build the list of existing instances to delete
			delete(existingInstances, component.MCPServerID)
			continue
		}

		// Catalog entry component
		if existingServer, exists := existingServers[component.CatalogEntryID]; !exists {
			// New server, create it
			newServer := withNeedsURL(v1.MCPServer{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: system.MCPServerPrefix,
					Namespace:    compositeServer.Namespace,
					Finalizers:   []string{v1.MCPServerFinalizer},
				},
				Spec: v1.MCPServerSpec{
					Manifest:                  component.Manifest,
					MCPServerCatalogEntryName: component.CatalogEntryID,
					UserID:                    compositeServer.Spec.UserID,
					CompositeName:             compositeServer.Name,
				},
			})

			if err := req.Client.Create(req.Ctx, &newServer); err != nil {
				return fmt.Errorf("failed to create new component server: %w", err)
			}
			log.Infof("Created component MCP server for composite server: composite=%s catalogEntry=%s", compositeServer.Name, component.CatalogEntryID)
		} else if hash.Digest(existingServer.Spec.Manifest) != hash.Digest(component.Manifest) {
			log.Infof("Updating component MCP server manifest for composite server: composite=%s componentServer=%s", compositeServer.Name, existingServer.Name)
			// Ensure the server is shut down before updating it
			if err := h.mcpSessionManager.ShutdownServer(req.Ctx, existingServer.Name); err != nil {
				return err
			}

			if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
				var latestServer v1.MCPServer
				if err := req.Get(&latestServer, compositeServer.Namespace, existingServer.Name); err != nil {
					return err
				}

				latestServer.Spec.Manifest = component.Manifest
				latestServer = withNeedsURL(latestServer)
				return req.Client.Update(req.Ctx, &latestServer)
			}); err != nil {
				return fmt.Errorf("failed to update existing component server: %w", err)
			}
		}

		// Remove the server to build the list of existing servers to delete
		delete(existingServers, component.CatalogEntryID)
	}

	// Delete existing instances that were not in the updated manifest
	for _, instance := range existingInstances {
		log.Infof("Deleting stale component MCPServerInstance: composite=%s instance=%s", compositeServer.Name, instance.Name)
		if err := req.Delete(&instance); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete instance %s: %w", instance.Name, err)
		}
	}

	// Delete existing servers that were not in the updated manifest
	for _, server := range existingServers {
		log.Infof("Deleting stale component MCP server: composite=%s server=%s", compositeServer.Name, server.Name)
		if err := req.Delete(&server); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete server %s: %w", server.Name, err)
		}
	}

	// All of the component MCP servers should now match the manifest of the composite.
	// Update the status hash to reflect the observed state.
	if manifestHash := hash.Digest(manifest); compositeServer.Status.ObservedCompositeManifestHash != manifestHash {
		compositeServer.Status.ObservedCompositeManifestHash = manifestHash
		log.Infof("Updated observed composite manifest hash: composite=%s hash=%s", compositeServer.Name, manifestHash)
		if err := req.Client.Status().Update(req.Ctx, compositeServer); err != nil {
			return fmt.Errorf("failed to update composite server status: %w", err)
		}
	}

	return nil
}

// SyncOAuthCredentialStatus syncs the OAuthCredentialConfigured status from the catalog entry.
// This replaces the push-based propagation logic with a pull-based approach where each MCP server
// is responsible for syncing its own status from its parent catalog entry.
func (h *Handler) SyncOAuthCredentialStatus(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	// Only relevant for servers created from catalog entries
	if server.Spec.MCPServerCatalogEntryName == "" {
		return clearOAuthStatusIfSet(req, server)
	}

	// Look up the catalog entry
	var catalogEntry v1.MCPServerCatalogEntry
	if err := req.Get(&catalogEntry, server.Namespace, server.Spec.MCPServerCatalogEntryName); err != nil {
		if apierrors.IsNotFound(err) {
			// Catalog entry deleted, this server itself will soon be cleaned up
			return nil
		}
		return fmt.Errorf("failed to get catalog entry: %w", err)
	}

	// Check if catalog entry requires static OAuth
	requiresStaticOAuth := catalogEntry.Spec.Manifest.Runtime == types.RuntimeRemote &&
		catalogEntry.Spec.Manifest.RemoteConfig != nil &&
		catalogEntry.Spec.Manifest.RemoteConfig.StaticOAuthRequired

	if !requiresStaticOAuth {
		return clearOAuthStatusIfSet(req, server)
	}

	// Sync status from catalog entry
	if server.Status.OAuthCredentialConfigured != catalogEntry.Status.OAuthCredentialConfigured {
		server.Status.OAuthCredentialConfigured = catalogEntry.Status.OAuthCredentialConfigured
		log.Infof("Updated MCP server OAuth credential status from catalog entry: server=%s catalogEntry=%s configured=%v", server.Name, catalogEntry.Name, server.Status.OAuthCredentialConfigured)
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) SyncOAuthMetadata(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Status.Idle {
		// Server is idle, don't do anything.
		return nil
	}

	if server.Spec.Manifest.Runtime != types.RuntimeRemote || server.Spec.Manifest.RemoteConfig == nil {
		return setOAuthMetadata(req, server, new(v1.OAuthMetadata), nil)
	}

	if !shouldSyncOAuthMetadata(server, time.Now()) {
		return nil
	}

	var credCtxs []string
	if server.Spec.IsCatalogServer() {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.MCPCatalogID, server.Name)}
	} else if server.Spec.IsPowerUserWorkspaceServer() {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.PowerUserWorkspaceID, server.Name)}
	} else {
		credCtxs = []string{fmt.Sprintf("%s-%s", server.Spec.UserID, server.Name)}
	}
	cred, err := h.gptClient.RevealCredential(req.Ctx, credCtxs, server.Name)
	if err != nil && !errors.As(err, &gptscript.ErrNotFound{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	serverConfig, missingConfig, err := mcp.ServerToServerConfig(*server, server.ValidConnectURLs(h.baseURL), h.baseURL, server.Spec.UserID, server.Name, server.Status.MCPCatalogID, cred.Env, nil)
	if err != nil {
		return fmt.Errorf("failed to convert MCP server to server config: %w", err)
	} else if len(missingConfig) > 0 {
		return nil
	}

	metadata, err := nmcp.GetOAuthMetadata(req.Ctx, nmcp.Server{
		BaseURL: serverConfig.URL,
		Headers: serverConfigHeaders(serverConfig),
	}, "Obot Test MCP OAuth Client", system.MCPOAuthCallbackURL(h.baseURL))
	if err != nil {
		return fmt.Errorf("failed to get OAuth metadata: %w", err)
	}

	statusMetadata := &v1.OAuthMetadata{
		ProtectedResourceURL:        metadata.ProtectedResourceMetadataURL,
		AuthorizationServerURL:      metadata.AuthorizationServerMetadataURL,
		ProtectedResourceMetadata:   metadata.ProtectedResourceMetadata,
		AuthorizationServerMetadata: metadata.AuthorizationServerMetadata,
		ClientRegistration:          metadata.ClientRegistration,
		DynamicClientRegistration:   metadata.DynamicClientRegistration,
	}

	syncTime := metav1.Now()
	return setOAuthMetadata(req, server, statusMetadata, &syncTime)
}

func shouldSyncOAuthMetadata(server *v1.MCPServer, now time.Time) bool {
	lastSync := server.Status.LastOAuthMetadataSync
	if server.Status.LastRequestTime.IsZero() || !server.Status.LastRequestTime.After(lastSync.Time) {
		return false
	}

	return lastSync.IsZero() || now.Sub(lastSync.Time) >= oauthMetadataSyncInterval
}

func setOAuthMetadata(req router.Request, server *v1.MCPServer, statusMetadata *v1.OAuthMetadata, syncTime *metav1.Time) error {
	metadataChanged := !reflect.DeepEqual(server.Status.OAuthMetadata, statusMetadata)
	syncTimeChanged := syncTime != nil && !server.Status.LastOAuthMetadataSync.Equal(syncTime)
	if metadataChanged || syncTimeChanged {
		server.Status.OAuthMetadata = statusMetadata
		if syncTime != nil {
			server.Status.LastOAuthMetadataSync = *syncTime
		}
		log.Infof("Updated MCP server OAuth metadata: server=%s", server.Name)
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func serverConfigHeaders(serverConfig mcp.ServerConfig) map[string]string {
	result := make(map[string]string, len(serverConfig.PassthroughHeaderNames)+len(serverConfig.Headers))
	for i, key := range serverConfig.PassthroughHeaderNames {
		if i < len(serverConfig.PassthroughHeaderValues) {
			result[key] = serverConfig.PassthroughHeaderValues[i]
		}
	}
	for _, header := range serverConfig.Headers {
		key, value, ok := strings.Cut(header, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func (h *Handler) ShutdownIdleServers(req router.Request, resp router.Response) error {
	mcpServer := req.Object.(*v1.MCPServer)
	if mcpServer.Status.LastRequestTime.IsZero() {
		if time.Since(mcpServer.CreationTimestamp.Time) > time.Minute {
			// Set the time if it is zero so we don't shutdown servers that were just created.
			mcpServer.Status.LastRequestTime = metav1.Now()
			return req.Client.Status().Update(req.Ctx, mcpServer)
		}

		// Give things some time to settle.
		resp.RetryAfter(time.Minute)
		return nil
	}

	idleInterval := time.Duration(mcpServer.Spec.Manifest.IdleShutdownIntervalHours) * time.Hour
	if idleInterval == 0 {
		idleInterval = h.singleUserIdleShutdownDelay
		if mcpServer.Spec.NanobotAgentID != "" {
			idleInterval = h.agentIdleShutdownDelay
		} else if !mcpServer.Spec.IsSingleUser() {
			idleInterval = h.multiUserIdleShutdownDelay
		}
	}

	if idleInterval < 0 {
		// If the idleInterval is negative, then shutdown is disabled for this server.
		if mcpServer.Status.Idle {
			mcpServer.Status.Idle = false
			if err := req.Client.Status().Update(req.Ctx, mcpServer); err != nil {
				return fmt.Errorf("failed to update idle status for server %s: %w", mcpServer.Name, err)
			}
		}
		return nil
	}

	if since := time.Since(mcpServer.Status.LastRequestTime.Time); since > idleInterval {
		if err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, mcpServer.Name); err != nil {
			return fmt.Errorf("failed to shutdown idle server %s: %w", mcpServer.Name, err)
		}

		if !mcpServer.Status.Idle {
			mcpServer.Status.Idle = true
			if err := req.Client.Status().Update(req.Ctx, mcpServer); err != nil {
				return fmt.Errorf("failed to update idle status for server %s: %w", mcpServer.Name, err)
			}
		}
	} else {
		if mcpServer.Status.Idle {
			mcpServer.Status.Idle = false
			if err := req.Client.Status().Update(req.Ctx, mcpServer); err != nil {
				return fmt.Errorf("failed to update idle status for server %s: %w", mcpServer.Name, err)
			}
		}

		if retry := idleInterval - since; retry < 10*time.Hour {
			// All objects are retried every 10 hours. If we should retry sooner, then trigger a retry.
			resp.RetryAfter(retry)
		}
	}

	return nil
}

// clearOAuthStatusIfSet clears the OAuthCredentialConfigured status if it is currently set.
func clearOAuthStatusIfSet(req router.Request, server *v1.MCPServer) error {
	if server.Status.OAuthCredentialConfigured {
		server.Status.OAuthCredentialConfigured = false
		log.Infof("Cleared MCP server OAuth credential status: server=%s", server.Name)
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

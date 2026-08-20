package mcpserver

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/tunnel"
	"github.com/obot-platform/obot/pkg/utils"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	oauthMetadataSyncInterval = time.Hour
)

type Handler struct {
	gatewayClient                *gateway.Client
	mcpSessionManager            *mcp.SessionManager
	tokenStore                   mcp.GlobalTokenStore
	networkPolicyProviderEnabled bool
	defaultDenyAllEgress         bool
	singleUserIdleShutdownDelay  time.Duration
	multiUserIdleShutdownDelay   time.Duration
	agentIdleShutdownDelay       time.Duration
	baseURL                      string
	mcpRuntimeBackend            string
	mcpImagePullSecrets          []string
}

type compositeChildren struct {
	servers            map[string]v1.MCPServer
	instances          map[string]v1.MCPServerInstance
	duplicateServers   []v1.MCPServer
	duplicateInstances []v1.MCPServerInstance
}

func effectiveDenyAllEgress(v *bool, domains []string, defaultWhenEmpty bool) bool {
	if v != nil {
		return *v
	}
	return defaultWhenEmpty && len(domains) == 0
}

func New(gatewayClient *gateway.Client, mcpSessionManager *mcp.SessionManager, tokenStore mcp.GlobalTokenStore, networkPolicyProviderEnabled, defaultDenyAllEgress bool, singleUserIdleShutdownDelay, multiUserIdleShutdownDelay, agentIdleShutdownDelay time.Duration, baseURL string, mcpRuntimeBackend string, mcpImagePullSecrets []string) *Handler {
	return &Handler{
		gatewayClient:                gatewayClient,
		mcpSessionManager:            mcpSessionManager,
		tokenStore:                   tokenStore,
		networkPolicyProviderEnabled: networkPolicyProviderEnabled,
		defaultDenyAllEgress:         defaultDenyAllEgress,
		singleUserIdleShutdownDelay:  singleUserIdleShutdownDelay,
		multiUserIdleShutdownDelay:   multiUserIdleShutdownDelay,
		agentIdleShutdownDelay:       agentIdleShutdownDelay,
		baseURL:                      baseURL,
		mcpRuntimeBackend:            mcpRuntimeBackend,
		mcpImagePullSecrets:          mcpImagePullSecrets,
	}
}

// DetectDrift reports whether this server is behind its own catalog entry. A component server is
// an ordinary server here, with no composite special case. A composite additionally rolls up its
// component servers' drift, since list responses drive the update badge but carry no per-component
// detail. A multi-user component's shared server is not rolled up: a composite update cannot clear
// its drift, so the badge would never go away.
func (h *Handler) DetectDrift(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.MCPServerCatalogEntryName == "" {
		return nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, server.Namespace, server.Spec.MCPServerCatalogEntryName); apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	drifted, err := ConfigurationHasDrifted(req.Ctx, h.gatewayClient, server, entry.Spec.Manifest, h.defaultDenyAllEgress)
	if err != nil {
		return err
	}

	if !drifted && server.Spec.Manifest.Runtime == types.RuntimeComposite {
		drifted, err = h.componentServersHaveDrifted(req, server)
		if err != nil {
			return err
		}
	}

	if server.Status.NeedsUpdate != drifted {
		slog.Info("MCP server catalog drift status changed", "server", server.Name, "catalogEntry", server.Spec.MCPServerCatalogEntryName, "needsUpdate", drifted)
		server.Status.NeedsUpdate = drifted
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

// componentServersHaveDrifted reports whether any of the composite's component servers is behind
// its own upstream. Listing here registers a watch trigger, so this handler is woken when one of
// them updates its status.
func (*Handler) componentServersHaveDrifted(req router.Request, compositeServer *v1.MCPServer) (bool, error) {
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return false, fmt.Errorf("failed to list component servers: %w", err)
	}

	return slices.ContainsFunc(componentServers.Items, func(component v1.MCPServer) bool {
		return component.Status.NeedsUpdate
	}), nil
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
			GenerateName: system.MCPNetworkPolicyPrefix,
			Namespace:    server.Namespace,
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

// DetectK8sSettingsDrift detects when a server needs redeployment with new
// K8s settings, including managed image pull secrets.
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

	imagePullSecretNames, err := mcp.CurrentImagePullSecretNames(req.Ctx, req.Client, h.mcpRuntimeBackend, h.mcpImagePullSecrets)
	if err != nil {
		return err
	}

	resources, err := mcp.CoreResourceRequirements(server.Spec.Manifest.Resources)
	if err != nil {
		return fmt.Errorf("failed to compute core resource requirements: %w", err)
	}

	currentHash := mcp.ComputeK8sSettingsHash(
		k8sSettings.Spec,
		resources,
		server.Spec.Manifest.Runtime,
		server.Spec.NanobotAgentID != "",
		imagePullSecretNames,
	)
	shouldSetNeedsK8sUpdate := server.Status.K8sSettingsHash != currentHash && !server.Status.NeedsK8sUpdate

	if shouldSetNeedsK8sUpdate {
		slog.Info("MCP server requires K8s redeploy due to K8s settings drift", "server", server.Name, "previousHash", server.Status.K8sSettingsHash, "newHash", currentHash)
		server.Status.NeedsK8sUpdate = true
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

// ConfigurationHasDrifted compares runtime config, env, resources, and multi-user config between a server
// and a catalog entry manifest. Static values omitted from the persisted server manifest are restored from
// its gateway credential before comparison.
func ConfigurationHasDrifted(ctx context.Context, gatewayClient *gateway.Client, server *v1.MCPServer, entryManifest types.MCPServerCatalogEntryManifest, defaultDenyAllEgress bool) (bool, error) {
	staticKeys := make(map[string]struct{})
	for _, env := range entryManifest.Env {
		if env.Value != "" {
			staticKeys[env.Key] = struct{}{}
		}
	}
	if entryManifest.RemoteConfig != nil {
		for _, header := range entryManifest.RemoteConfig.Headers {
			if header.Value != "" {
				staticKeys[header.Key] = struct{}{}
			}
		}
	}

	serverManifest := server.Spec.Manifest
	if len(staticKeys) > 0 {
		credentialContext := server.Spec.UserID
		if server.Spec.MCPCatalogID != "" {
			credentialContext = server.Spec.MCPCatalogID
		} else if server.Spec.PowerUserWorkspaceID != "" {
			credentialContext = server.Spec.PowerUserWorkspaceID
		}

		credential, err := gatewayClient.RevealCredential(ctx, []string{fmt.Sprintf("%s-%s", credentialContext, server.Name)}, server.Name)
		if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
			return false, err
		}

		serverManifest.Env = slices.Clone(serverManifest.Env)
		for i, env := range serverManifest.Env {
			if _, ok := staticKeys[env.Key]; ok && env.Value == "" {
				serverManifest.Env[i].Value = credential.Secrets[env.Key]
			}
		}

		if serverManifest.RemoteConfig != nil {
			remoteConfig := *serverManifest.RemoteConfig
			remoteConfig.Headers = slices.Clone(remoteConfig.Headers)
			serverManifest.RemoteConfig = &remoteConfig
			for i, header := range serverManifest.RemoteConfig.Headers {
				if _, ok := staticKeys[header.Key]; ok && header.Value == "" {
					serverManifest.RemoteConfig.Headers[i].Value = credential.Secrets[header.Key]
				}
			}
		}
	}

	return configurationHasDrifted(serverManifest, entryManifest, defaultDenyAllEgress)
}

// configurationHasDrifted compares only the fields common to MCPServerManifest and
// MCPServerCatalogEntryManifest. It is also used for nested composite components,
// which do not have their own MCPServer object or credential context.
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
		drifted = compositeConfigHasDrifted(serverManifest.CompositeConfig, entryManifest.CompositeConfig)
	default:
		return false, fmt.Errorf("unknown runtime type: %s", serverManifest.Runtime)
	}

	if drifted {
		return true, nil
	}

	// Check multi-user configuration
	if multiUserConfigHasDrifted(serverManifest.MultiUserConfig, entryManifest.MultiUserConfig) {
		return true, nil
	}

	// Check environment. Secret binding selections are deployment configuration,
	// not source catalog drift.
	serverEnv := withoutAdminManagedSecretBoundEnvFields(serverManifest.Env, entryManifest.Env)
	entryEnv := withoutAdminManagedSecretBoundEnvFields(entryManifest.Env, serverManifest.Env)
	if fieldSlicesHaveDrifted(serverEnv, entryEnv, mcpEnvMatchesCatalog) {
		return true, nil
	}

	return resourcesHasDrifted(serverManifest.Resources, entryManifest.Resources), nil
}

func resourcesHasDrifted(serverResources, entryResources *types.MCPResourceRequirements) bool {
	if serverResources == nil && entryResources == nil {
		return false
	}
	if serverResources == nil || entryResources == nil {
		return true
	}
	return !reflect.DeepEqual(serverResources, entryResources)
}

func multiUserConfigHasDrifted(serverConfig, entryConfig *types.MultiUserConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}
	return fieldSlicesHaveDrifted(serverConfig.UserDefinedHeaders, entryConfig.UserDefinedHeaders, mcpHeaderMatchesCatalog)
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
		entryConfig.URLTemplate != serverConfig.URLTemplate ||
		entryConfig.TunnelName != serverConfig.TunnelName {
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
	serverHeaders := withoutAdminManagedSecretBoundHeaderFields(serverConfig.Headers, entryConfig.Headers)
	entryHeaders := withoutAdminManagedSecretBoundHeaderFields(entryConfig.Headers, serverConfig.Headers)
	return fieldSlicesHaveDrifted(serverHeaders, entryHeaders, mcpHeaderMatchesCatalog)
}

// withoutAdminManagedSecretBoundEnvFields removes env fields that exist only on
// serverFields because an admin selected a deployment-level secret binding.
// Catalog-owned fields are kept so pinned catalog bindings still participate in
// drift detection.
func withoutAdminManagedSecretBoundEnvFields(serverFields, entryFields []types.MCPEnv) []types.MCPEnv {
	entryKeys := make(map[string]struct{}, len(entryFields))
	for _, field := range entryFields {
		entryKeys[field.Key] = struct{}{}
	}

	result := make([]types.MCPEnv, 0, len(serverFields))
	for _, field := range serverFields {
		_, entryHasField := entryKeys[field.Key]
		if !entryHasField && adminAddedSecretBinding(field.SecretBinding) {
			continue
		}
		result = append(result, field)
	}
	return result
}

// withoutAdminManagedSecretBoundHeaderFields removes header fields that exist
// only on serverFields because an admin selected a deployment-level secret
// binding. Catalog-owned fields are kept so pinned catalog bindings still
// participate in drift detection.
func withoutAdminManagedSecretBoundHeaderFields(serverFields, entryFields []types.MCPHeader) []types.MCPHeader {
	entryKeys := make(map[string]struct{}, len(entryFields))
	for _, field := range entryFields {
		entryKeys[field.Key] = struct{}{}
	}

	result := make([]types.MCPHeader, 0, len(serverFields))
	for _, field := range serverFields {
		_, entryHasField := entryKeys[field.Key]
		if !entryHasField && adminAddedSecretBinding(field.SecretBinding) {
			continue
		}
		result = append(result, field)
	}
	return result
}

func fieldSlicesHaveDrifted[T any](serverFields, entryFields []T, matches func(T, T) bool) bool {
	if len(serverFields) != len(entryFields) {
		return true
	}

	used := make([]bool, len(entryFields))
	for _, serverField := range serverFields {
		found := false
		for i, entryField := range entryFields {
			if used[i] || !matches(serverField, entryField) {
				continue
			}
			used[i] = true
			found = true
			break
		}
		if !found {
			return true
		}
	}

	return false
}

func mcpEnvMatchesCatalog(serverField, entryField types.MCPEnv) bool {
	if oneSidedAdminAddedSecretBinding(serverField.SecretBinding, entryField.SecretBinding) {
		serverField.SecretBinding = nil
		serverField.Value = entryField.Value
	}
	if oneSidedAdminAddedSecretBinding(entryField.SecretBinding, serverField.SecretBinding) {
		entryField.SecretBinding = nil
		entryField.Value = serverField.Value
	}

	return reflect.DeepEqual(serverField, entryField)
}

func mcpHeaderMatchesCatalog(serverField, entryField types.MCPHeader) bool {
	if oneSidedAdminAddedSecretBinding(serverField.SecretBinding, entryField.SecretBinding) {
		serverField.SecretBinding = nil
		serverField.Value = entryField.Value
	}
	if oneSidedAdminAddedSecretBinding(entryField.SecretBinding, serverField.SecretBinding) {
		entryField.SecretBinding = nil
		entryField.Value = serverField.Value
	}
	return reflect.DeepEqual(serverField, entryField)
}

func oneSidedAdminAddedSecretBinding(left, right *types.MCPSecretBinding) bool {
	// AdminAdded is derived metadata. Ignore it when it appears on only one side;
	// any real binding difference on the other side remains for DeepEqual below.
	return adminAddedSecretBinding(left) && !adminAddedSecretBinding(right)
}

func adminAddedSecretBinding(binding *types.MCPSecretBinding) bool {
	return binding != nil && binding.AdminAdded
}

// compositeConfigHasDrifted compares only what a composite owns against its catalog entry:
// membership, tool prefixes, and tool overrides. Disabled is per-deployment, has no counterpart on
// the entry, and is ignored.
func compositeConfigHasDrifted(serverConfig *types.CompositeRuntimeConfig, entryConfig *types.CompositeCatalogConfig) bool {
	if serverConfig == nil && entryConfig == nil {
		return false
	}
	if serverConfig == nil || entryConfig == nil {
		return true
	}

	if len(serverConfig.ComponentServers) != len(entryConfig.ComponentServers) {
		return true
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
			return true
		}

		if serverComponent.ToolPrefix != entryComponent.ToolPrefix {
			return true
		}

		if utils.Digest(serverComponent.ToolOverrides) != utils.Digest(entryComponent.ToolOverrides) {
			return true
		}
	}

	return false
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
		slog.Info("Updated MCP server instance user count", "server", server.Name, "newCount", newUserCount)
		server.Status.MCPServerInstanceUserCount = &newUserCount
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) DeleteServersWithoutRuntime(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if string(server.Spec.Manifest.Runtime) == "" {
		slog.Info("Deleting MCP server with empty runtime", "server", server.Name)
		return req.Client.Delete(req.Ctx, server)
	}

	return nil
}

func (h *Handler) DeleteServersForAnonymousUser(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.UserID == "anonymous" {
		slog.Info("Deleting MCP server for anonymous user", "server", server.Name)
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
		slog.Info("Resolved MCP catalog ID for server", "server", server.Name, "catalogEntry", server.Spec.MCPServerCatalogEntryName, "catalogID", server.Status.MCPCatalogID)
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
}

func (h *Handler) MigrateSharedWithinMCPCatalogName(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)

	if server.Spec.SharedWithinMCPCatalogName != "" && server.Spec.MCPCatalogID == "" {
		server.Spec.MCPCatalogID = server.Spec.SharedWithinMCPCatalogName
		server.Spec.SharedWithinMCPCatalogName = ""
		slog.Info("Migrating MCP server shared catalog field to MCPCatalogID", "server", server.Name, "catalogID", server.Spec.MCPCatalogID)
		return req.Client.Update(req.Ctx, server)
	}

	return nil
}

// EnsureCompositeComponents reconciles a composite's references with its child servers and instances.
func (h *Handler) EnsureCompositeComponents(req router.Request, _ router.Response) error {
	var (
		compositeServer = req.Object.(*v1.MCPServer)
		manifest        = compositeServer.Spec.Manifest
		syncRequest     = compositeServer.Annotations[v1.MCPServerCompositeSyncRequestedAtAnnotation]
	)

	if manifest.Runtime != types.RuntimeComposite || manifest.CompositeConfig == nil {
		return nil
	}

	children, err := h.listCompositeChildren(req, compositeServer)
	if err != nil {
		return err
	}

	componentErrors := make(map[string]string)
	for _, component := range manifest.CompositeConfig.ComponentServers {
		if err := h.reconcileComponent(req, compositeServer, component, children.servers, children.instances); err != nil {
			slog.Info("Composite component could not be reconciled", "composite", compositeServer.Name, "component", component.ComponentID(), "error", err)
			componentErrors[component.ComponentID()] = err.Error()
			continue
		}
	}

	// Unresolvable references are removed from children before cleanup so existing children survive.
	for _, instance := range slices.Concat(slices.Collect(maps.Values(children.instances)), children.duplicateInstances) {
		slog.Info("Deleting stale component MCPServerInstance", "composite", compositeServer.Name, "instance", instance.Name)
		if err := req.Delete(&instance); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete instance %s: %w", instance.Name, err)
		}
	}
	for _, server := range slices.Concat(slices.Collect(maps.Values(children.servers)), children.duplicateServers) {
		slog.Info("Deleting stale component MCP server", "composite", compositeServer.Name, "server", server.Name)
		if err := req.Delete(&server); kclient.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to delete server %s: %w", server.Name, err)
		}
	}

	if compositeServer.Status.ObservedCompositeGeneration == compositeServer.Generation &&
		compositeServer.Status.ObservedCompositeSyncRequest == syncRequest &&
		utils.Digest(compositeServer.Status.ComponentErrors) == utils.Digest(nonEmpty(componentErrors)) {
		return nil
	}

	compositeServer.Status.ObservedCompositeGeneration = compositeServer.Generation
	compositeServer.Status.ObservedCompositeSyncRequest = syncRequest
	compositeServer.Status.ComponentErrors = nonEmpty(componentErrors)
	return req.Client.Status().Update(req.Ctx, compositeServer)
}

func nonEmpty(errs map[string]string) map[string]string {
	if len(errs) == 0 {
		return nil
	}
	return errs
}

func (*Handler) listCompositeChildren(req router.Request, compositeServer *v1.MCPServer) (compositeChildren, error) {
	var componentServers v1.MCPServerList
	if err := req.List(&componentServers, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return compositeChildren{}, fmt.Errorf("failed to list component servers: %w", err)
	}

	var componentInstances v1.MCPServerInstanceList
	if err := req.List(&componentInstances, &kclient.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("spec.compositeName", compositeServer.Name),
		Namespace:     compositeServer.Namespace,
	}); err != nil {
		return compositeChildren{}, fmt.Errorf("failed to list component instances: %w", err)
	}

	children := compositeChildren{
		servers:   make(map[string]v1.MCPServer, len(componentServers.Items)),
		instances: make(map[string]v1.MCPServerInstance, len(componentInstances.Items)),
	}
	for _, existing := range componentServers.Items {
		id := existing.Spec.MCPServerCatalogEntryName
		if id == "" {
			continue
		}
		kept, ok := children.servers[id]
		if !ok {
			children.servers[id] = existing
			continue
		}
		if kept.CreationTimestamp.Before(&existing.CreationTimestamp) {
			children.duplicateServers = append(children.duplicateServers, existing)
			continue
		}
		children.duplicateServers = append(children.duplicateServers, kept)
		children.servers[id] = existing
	}

	for _, existing := range componentInstances.Items {
		id := existing.Spec.MCPServerName
		if id == "" {
			continue
		}
		kept, ok := children.instances[id]
		if !ok {
			children.instances[id] = existing
			continue
		}
		if kept.CreationTimestamp.Before(&existing.CreationTimestamp) {
			children.duplicateInstances = append(children.duplicateInstances, existing)
			continue
		}
		children.duplicateInstances = append(children.duplicateInstances, kept)
		children.instances[id] = existing
	}

	return children, nil
}

func (h *Handler) reconcileComponent(
	req router.Request,
	compositeServer *v1.MCPServer,
	component types.ComponentServer,
	componentServers map[string]v1.MCPServer,
	componentInstances map[string]v1.MCPServerInstance,
) error {
	if component.MCPServerID != "" {
		existingInstance, exists := componentInstances[component.MCPServerID]
		delete(componentInstances, component.MCPServerID)

		var multiUserServer v1.MCPServer
		if err := req.Get(&multiUserServer, compositeServer.Namespace, component.MCPServerID); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return fmt.Errorf("failed to get multi-user server %s: %w", component.MCPServerID, err)
		}

		if !exists {
			if err := req.Client.Create(req.Ctx, &v1.MCPServerInstance{
				GenerateName: system.MCPServerInstancePrefix,
				Namespace:    compositeServer.Namespace,
				Finalizers:   []string{v1.MCPServerInstanceFinalizer},

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
			slog.Info("Created component MCPServerInstance for composite server", "composite", compositeServer.Name, "componentServer", component.MCPServerID, "userID", compositeServer.Spec.UserID)
			return nil
		}

		if utils.Digest(existingInstance.Spec.MultiUserConfig) != utils.Digest(multiUserServer.Spec.Manifest.MultiUserConfig) {
			existingInstance.Spec.MultiUserConfig = multiUserServer.Spec.Manifest.MultiUserConfig
			if err := req.Client.Update(req.Ctx, &existingInstance); err != nil {
				return fmt.Errorf("failed to update instance for multi-user component: %w", err)
			}
		}

		return nil
	}

	existingServer, exists := componentServers[component.CatalogEntryID]
	delete(componentServers, component.CatalogEntryID)

	syncRequest := compositeServer.Annotations[v1.MCPServerCompositeSyncRequestedAtAnnotation]
	if exists && (syncRequest == "" || syncRequest == compositeServer.Status.ObservedCompositeSyncRequest) {
		return nil
	}

	// Router reads register a watch; a missing upstream leaves an existing child running.
	var entry v1.MCPServerCatalogEntry
	if err := req.Get(&entry, compositeServer.Namespace, component.CatalogEntryID); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get component catalog entry %s: %w", component.CatalogEntryID, err)
	}

	if !exists {
		return h.createCompositeChildServer(req, compositeServer, component, entry)
	}

	return h.updateCompositeChildServer(req, compositeServer, existingServer, entry)
}

func (h *Handler) createCompositeChildServer(req router.Request, compositeServer *v1.MCPServer, component types.ComponentServer, entry v1.MCPServerCatalogEntry) error {
	manifest, needsURL, err := h.buildComponentServerManifest(req, entry, "", component.Disabled)
	if err != nil {
		return err
	}

	newServer := v1.MCPServer{
		GenerateName: system.MCPServerPrefix,
		Namespace:    compositeServer.Namespace,
		Finalizers:   []string{v1.MCPServerFinalizer},

		Spec: v1.MCPServerSpec{
			Manifest:                  manifest,
			MCPServerCatalogEntryName: component.CatalogEntryID,
			UserID:                    compositeServer.Spec.UserID,
			CompositeName:             compositeServer.Name,
			NeedsURL:                  needsURL,
		},
	}

	if err := req.Client.Create(req.Ctx, &newServer); err != nil {
		return fmt.Errorf("failed to create new component server: %w", err)
	}
	slog.Info("Created component MCP server for composite server", "composite", compositeServer.Name, "catalogEntry", component.CatalogEntryID)

	return nil
}

// updateCompositeChildServer rebuilds a child while preserving its volumes.
func (h *Handler) updateCompositeChildServer(req router.Request, compositeServer *v1.MCPServer, existingServer v1.MCPServer, entry v1.MCPServerCatalogEntry) error {
	var currentURL string
	if existingServer.Spec.Manifest.RemoteConfig != nil {
		currentURL = existingServer.Spec.Manifest.RemoteConfig.URL
	}

	manifest, needsURL, err := h.buildComponentServerManifest(req, entry, currentURL, false)
	if err != nil {
		return err
	}

	if utils.Digest(existingServer.Spec.Manifest) == utils.Digest(manifest) && existingServer.Spec.NeedsURL == needsURL {
		return nil
	}

	slog.Info("Updating component MCP server manifest for composite server", "composite", compositeServer.Name, "componentServer", existingServer.Name)
	if err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, existingServer.Name); err != nil {
		return err
	}

	if err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		var latestServer v1.MCPServer
		if err := req.Get(&latestServer, compositeServer.Namespace, existingServer.Name); err != nil {
			return err
		}

		latestServer.Spec.Manifest = manifest
		latestServer.Spec.NeedsURL = needsURL
		return req.Client.Update(req.Ctx, &latestServer)
	}); err != nil {
		return fmt.Errorf("failed to update existing component server: %w", err)
	}

	return nil
}

// buildComponentServerManifest builds and validates a child manifest from its current catalog entry.
func (h *Handler) buildComponentServerManifest(req router.Request, entry v1.MCPServerCatalogEntry, userURL string, disabled bool) (types.MCPServerManifest, bool, error) {
	allowMissingURL := mcp.CatalogEntryRequiresUserURL(entry.Spec.Manifest)

	manifest, err := types.MapCatalogEntryToServer(entry.Spec.Manifest, userURL, disabled || allowMissingURL)
	if err != nil {
		return types.MCPServerManifest{}, false, fmt.Errorf("failed to convert component manifest: %w", err)
	}

	resourceMaximums, err := h.mcpSessionManager.EffectiveKubernetesResourceMaximums(req.Ctx, req.Client)
	if err != nil {
		return types.MCPServerManifest{}, false, fmt.Errorf("failed to resolve resource maximums: %w", err)
	}

	if err := mcp.ValidateServerManifest(req.Ctx, manifest, false, mcp.ValidationOptions{
		AllowMissingURL:              allowMissingURL,
		RemoteMCPURLValidationConfig: h.mcpSessionManager.RemoteMCPURLValidationConfig(),
		ResourceMaximums:             resourceMaximums,
	}); err != nil {
		return types.MCPServerManifest{}, false, fmt.Errorf("validation failed: %w", err)
	}

	if err := tunnel.ValidateServerTunnelReferences(req.Ctx, req.Client, manifest); err != nil {
		return types.MCPServerManifest{}, false, fmt.Errorf("validation failed: %w", err)
	}

	needsURL := allowMissingURL && (manifest.RemoteConfig == nil || manifest.RemoteConfig.URL == "")

	return manifest, needsURL, nil
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
		slog.Info("Updated MCP server OAuth credential status from catalog entry", "server", server.Name, "catalogEntry", catalogEntry.Name, "configured", server.Status.OAuthCredentialConfigured)
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

	blockingConfig := h.mcpSessionManager.RemoteMCPURLValidationConfig()

	if server.Spec.Manifest.RemoteConfig.TunnelName == "" {
		if err := mcp.ValidateRemoteMCPURL(req.Ctx, server.Spec.Manifest.RemoteConfig.URL, blockingConfig); err != nil {
			// If the URL doesn't pass validation, then don't do anything so that we sync as soon as the configuration is updated.
			slog.Info("Remote MCP URL validation failed, not checking OAuth metadata", "server", server.Name, "error", err)
			return nil
		}
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
	cred, err := h.gatewayClient.RevealCredential(req.Ctx, credCtxs, server.Name)
	if err != nil && !errors.As(err, &gateway.CredentialNotFoundError{}) {
		return fmt.Errorf("failed to reveal credential: %w", err)
	}

	serverConfig, missingConfig, err := mcp.ServerToServerConfig(*server, server.ValidConnectURLs(h.baseURL), server.Spec.UserID, server.Name, server.Status.MCPCatalogID, cred.Secrets)
	if err != nil {
		return fmt.Errorf("failed to convert MCP server to server config: %w", err)
	} else if len(missingConfig) > 0 {
		return nil
	}

	metadata, err := h.mcpSessionManager.GetOAuthMetadata(req.Ctx, serverConfig,
		"Obot Test MCP OAuth Client", system.MCPOAuthCallbackURL(h.baseURL), mcp.RequiresStaticOAuth(*server))
	if err != nil {
		return fmt.Errorf("failed to get OAuth metadata: %w", err)
	}

	statusMetadata := &v1.OAuthMetadata{
		ProtectedResourceURL:              metadata.ProtectedResourceMetadataURL,
		AuthorizationServerURL:            metadata.AuthorizationServerMetadataURL,
		ProtectedResourceMetadata:         runtime.RawExtension{Raw: metadata.ProtectedResourceMetadata},
		AuthorizationServerMetadata:       runtime.RawExtension{Raw: metadata.AuthorizationServerMetadata},
		ClientRegistration:                runtime.RawExtension{Raw: metadata.ClientRegistration},
		DynamicClientRegistration:         metadata.DynamicClientRegistration,
		ClientIDMetadataDocumentSupported: metadata.ClientIDMetadataDocumentSupported,
	}

	syncTime := metav1.Now()
	return setOAuthMetadata(req, server, statusMetadata, &syncTime)
}

func (h *Handler) SyncThirdPartyAuthStatus(req router.Request, _ router.Response) error {
	server := req.Object.(*v1.MCPServer)
	if server.Spec.Manifest.Runtime != types.RuntimeRemote || server.Spec.Manifest.RemoteConfig == nil {
		return nil
	}

	token, err := h.gatewayClient.GetMCPOAuthToken(req.Ctx, server.Spec.UserID, server.Name, server.Spec.Manifest.RemoteConfig.URL)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if hasAuthed := (token != nil && token.AccessToken != ""); server.Status.UserHasAuthenticated != hasAuthed {
		server.Status.UserHasAuthenticated = hasAuthed
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
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
		slog.Info("Updated MCP server OAuth metadata", "server", server.Name)
		return req.Client.Status().Update(req.Ctx, server)
	}

	return nil
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
		// If the server is already idle, then no need to shutdown.
		if !mcpServer.Status.Idle {
			if err := h.mcpSessionManager.ShutdownIdleServer(req.Ctx, mcpServer.Name); err != nil {
				return fmt.Errorf("failed to shutdown idle server %s: %w", mcpServer.Name, err)
			}

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

// SetNonDeployServerStatus sets the deployment status for servers that don't have a corresponding deployment.
func (h *Handler) SetNonDeployServerStatus(req router.Request, _ router.Response) error {
	mcpServer := req.Object.(*v1.MCPServer)
	if mcpServer.Spec.Manifest.Runtime == types.RuntimeRemote || mcpServer.Spec.Manifest.Runtime == types.RuntimeComposite {
		mcpServer.Status.DeploymentStatus = "Available"
		mcpServer.Status.DeploymentAvailableReplicas = nil
		mcpServer.Status.DeploymentReadyReplicas = nil
		mcpServer.Status.DeploymentReplicas = nil
		mcpServer.Status.DeploymentConditions = nil
	}

	return nil
}

// clearOAuthStatusIfSet clears the OAuthCredentialConfigured status if it is currently set.
func clearOAuthStatusIfSet(req router.Request, server *v1.MCPServer) error {
	if server.Status.OAuthCredentialConfigured {
		server.Status.OAuthCredentialConfigured = false
		slog.Info("Cleared MCP server OAuth credential status", "server", server.Name)
		return req.Client.Status().Update(req.Ctx, server)
	}
	return nil
}

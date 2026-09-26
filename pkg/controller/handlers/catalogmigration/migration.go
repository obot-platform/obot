// Package catalogmigration converts standalone legacy MCP resources to vMCPs
// before controllers and API traffic start.
package catalogmigration

import (
	"cmp"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	"github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// MigrationName is the durable gateway marker and part of the stable destination IDs.
	MigrationName = "standalone_mcp_to_vmcp"
)

type Handler struct {
	reveal    func(context.Context, []string, string) (gatewaytypes.Credential, error)
	upsert    func(context.Context, gatewaytypes.Credential) error
	copyOAuth func(context.Context, string, string, string) error
}

func New(client *gateway.Client) *Handler {
	return &Handler{
		reveal:    client.RevealCredential,
		upsert:    client.UpsertCredential,
		copyOAuth: client.CopyMCPOAuthTokens,
	}
}

func migrationName(prefix, namespace, source string) string {
	digest := sha256.Sum256([]byte(MigrationName + "/" + namespace + "/" + source))

	return fmt.Sprintf("%s%x", prefix, digest[:16])
}

// MigrateAll retains sources until the caller records successful completion.
// Stable destination IDs allow a failed startup to retry without duplicating objects.
func (h *Handler) MigrateAll(ctx context.Context, client kclient.Client) error {
	var (
		entries   v1.MCPServerCatalogEntryList
		servers   v1.MCPServerList
		instances v1.MCPServerInstanceList
		rules     v1.AccessControlRuleList
	)

	for _, list := range []kclient.ObjectList{&entries, &servers, &instances, &rules} {
		if err := client.List(ctx, list); err != nil {
			return err
		}
	}

	entryTargets := make(map[kclient.ObjectKey]v1.VMCP)
	serverTargets := make(map[kclient.ObjectKey]v1.VMCP)
	sharedEntries := make(map[kclient.ObjectKey]bool)

	for _, server := range servers.Items {
		if legacyStandaloneServer(server) && !legacySingleUser(server) {
			sharedEntries[kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.MCPServerCatalogEntryName}] = true
		}
	}

	entriesByName := make(map[kclient.ObjectKey]*v1.MCPServerCatalogEntry)

	for _, entry := range entries.Items {
		key := kclient.ObjectKeyFromObject(&entry)
		entriesByName[key] = &entry

		// Shared entries are templates; their configured deployments own the vMCPs.
		if !entry.DeletionTimestamp.IsZero() || entry.Spec.Manifest.Runtime == types.RuntimeComposite || sharedEntries[key] {
			continue
		}

		target, err := h.migrateEntry(ctx, client, entry, rules.Items)
		if err != nil {
			return err
		}

		if target != nil {
			entryTargets[key] = *target
		}
	}

	entryFilters := make(map[kclient.ObjectKey][]types.Resource)

	for _, server := range servers.Items {
		if !legacyStandaloneServer(server) || !server.DeletionTimestamp.IsZero() {
			continue
		}

		entryKey := kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.MCPServerCatalogEntryName}
		var target v1.VMCP
		if legacySingleUser(server) {
			target = entryTargets[entryKey]
			if err := h.migrateSingleUserServer(ctx, client, server, entriesByName[entryKey], &target); err != nil {
				return err
			}
		} else {
			var err error
			target, err = h.migrateSharedServer(ctx, client, server, rules.Items)
			if err != nil {
				return err
			}

			serverTargets[kclient.ObjectKeyFromObject(&server)] = target
		}

		if entryKey.Name != "" {
			resource := types.Resource{Type: types.ResourceTypeMCPServer, ID: target.Name}
			if !slices.Contains(entryFilters[entryKey], resource) {
				entryFilters[entryKey] = append(entryFilters[entryKey], resource)
			}
		}
	}

	for _, instance := range instances.Items {
		if !instance.DeletionTimestamp.IsZero() || instance.Spec.VMCPComponentID != "" || instance.Spec.VMCPInstanceID != "" || instance.Spec.CompositeName != "" {
			continue
		}

		if slices.ContainsFunc(servers.Items, func(server v1.MCPServer) bool {
			return server.Namespace == instance.Namespace && server.Name == instance.Spec.MCPServerName && server.Spec.NanobotAgentID != ""
		}) {
			continue
		}

		target, ok := serverTargets[kclient.ObjectKey{Namespace: instance.Namespace, Name: instance.Spec.MCPServerName}]
		if !ok {
			return fmt.Errorf("MCP server instance %s/%s has no migrated server %q", instance.Namespace, instance.Name, instance.Spec.MCPServerName)
		}

		if err := h.migrateInstance(ctx, client, instance, target); err != nil {
			return err
		}
	}

	// Replace each entry reference once, after every sibling destination exists.
	// This preserves all shared deployments and personal fallbacks on the filter.
	for entry, targets := range entryFilters {
		if err := MigrateFilters(ctx, client, entry.Namespace, targets, []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}}, nil); err != nil {
			return err
		}
	}

	return nil
}

func (h *Handler) migrateEntry(ctx context.Context, client kclient.Client, entry v1.MCPServerCatalogEntry, rules []v1.AccessControlRule) (*v1.VMCP, error) {
	profiles := matchingProfiles(rules, entry.Namespace, entry.Spec.MCPCatalogName, entry.Spec.PowerUserWorkspaceID, types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name})
	if len(profiles) == 0 {
		return nil, nil
	}

	target := newVMCP(entry.Namespace, entry.Name, entry.Spec.Manifest, profiles)
	component, static := newComponent(entry.Name, cmp.Or(entry.Spec.MCPCatalogName, entry.Spec.PowerUserWorkspaceID), entry.Name, entry.Spec.Manifest, nil, false)
	component.CatalogEntry.UnsupportedTools = slices.Clone(entry.Spec.UnsupportedTools)
	component.SourceDigest = utils.Digest(types.MCPServerCatalogEntrySnapshot{Manifest: entry.Spec.Manifest, UnsupportedTools: entry.Spec.UnsupportedTools})
	target.Spec.Manifest.Components = []types.VMCPComponent{component}

	if err := h.ensureVMCP(ctx, client, &target, static); err != nil {
		return nil, err
	}

	if err := migrateFilters(ctx, client, target, types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}, cmp.Or(entry.Spec.MCPCatalogName, entry.Spec.PowerUserWorkspaceID)); err != nil {
		return nil, err
	}

	return &target, nil
}

func (h *Handler) migrateSharedServer(ctx context.Context, client kclient.Client, server v1.MCPServer, rules []v1.AccessControlRule) (v1.VMCP, error) {
	profiles := matchingProfiles(rules, server.Namespace, server.Spec.MCPCatalogID, server.Spec.PowerUserWorkspaceID, types.Resource{Type: types.ResourceTypeMCPServer, ID: server.Name})
	if server.Spec.PowerUserWorkspaceID != "" {
		var workspace v1.PowerUserWorkspace
		if err := client.Get(ctx, kclient.ObjectKey{Namespace: server.Namespace, Name: server.Spec.PowerUserWorkspaceID}, &workspace); err != nil {
			return v1.VMCP{}, err
		}

		profiles = append(profiles, types.VMCPProfile{Name: "owner", Subjects: []types.Subject{{Type: types.SubjectTypeUser, ID: workspace.Spec.UserID}}, Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}})
	}

	manifest := server.Spec.Manifest.ConvertToCatalogEntry()
	// ConvertToCatalogEntry clears deployment-only UserAllowed flags.
	manifest.Config = slices.Clone(server.Spec.Manifest.Config)
	// Shared credentials belong to the catalog or workspace, not the creator.
	values, err := h.read(ctx, server.CredentialContext(""), server.Name)
	if err != nil {
		return v1.VMCP{}, err
	}

	target := newVMCP(server.Namespace, server.Name, manifest, profiles)
	target.Spec.Manifest.DisplayName = cmp.Or(server.Spec.Alias, target.Spec.Manifest.DisplayName)
	target.Spec.CreatorUserID = server.Spec.UserID
	component, static := newComponent(server.Name, cmp.Or(server.Spec.MCPCatalogID, server.Spec.PowerUserWorkspaceID), cmp.Or(server.Spec.MCPServerCatalogEntryName, server.Name), manifest, values, true)
	component.CatalogEntry.UnsupportedTools = slices.Clone(server.Spec.UnsupportedTools)
	component.SourceDigest = utils.Digest(types.MCPServerCatalogEntrySnapshot{Manifest: manifest, UnsupportedTools: server.Spec.UnsupportedTools})
	target.Spec.Manifest.Components = []types.VMCPComponent{component}

	if err := h.ensureVMCP(ctx, client, &target, static); err != nil {
		return v1.VMCP{}, err
	}

	if err := migrateFilters(ctx, client, target, types.Resource{Type: types.ResourceTypeMCPServer, ID: server.Name}, cmp.Or(server.Spec.MCPCatalogID, server.Spec.PowerUserWorkspaceID)); err != nil {
		return v1.VMCP{}, err
	}

	return target, nil
}

func (h *Handler) migrateSingleUserServer(ctx context.Context, client kclient.Client, server v1.MCPServer, entry *v1.MCPServerCatalogEntry, target *v1.VMCP) error {
	if target.Name == "" {
		// Preserve orphaned/private connections without creating a new shared grant.
		manifest := server.Spec.Manifest.ConvertToCatalogEntry()
		*target = newVMCP(server.Namespace, server.Name, manifest, nil)
		target.Spec.UserID = server.Spec.UserID
		for i := range manifest.Config {
			manifest.Config[i].Value = ""
		}

		catalog := server.Status.MCPCatalogID
		if entry != nil {
			catalog = cmp.Or(entry.Spec.MCPCatalogName, entry.Spec.PowerUserWorkspaceID)
		}

		component, _ := newComponent(server.Name, catalog, cmp.Or(server.Spec.MCPServerCatalogEntryName, server.Name), manifest, nil, false)
		component.CatalogEntry.UnsupportedTools = slices.Clone(server.Spec.UnsupportedTools)
		component.SourceDigest = utils.Digest(types.MCPServerCatalogEntrySnapshot{Manifest: server.Spec.Manifest.ConvertToCatalogEntry(), UnsupportedTools: server.Spec.UnsupportedTools})
		target.Spec.Manifest.Components = []types.VMCPComponent{component}

		if err := h.ensureVMCP(ctx, client, target, make(map[string]string)); err != nil {
			return err
		}
	}

	if len(target.Spec.Manifest.Components) != 1 || !target.DeletionTimestamp.IsZero() {
		return fmt.Errorf("migration target %q must have one component and not be deleting", target.Name)
	}

	// Direct connection filters must be replaced even when using a catalog vMCP.
	if err := migrateFilters(ctx, client, *target, types.Resource{Type: types.ResourceTypeMCPServer, ID: server.Name}, target.Spec.Manifest.Components[0].MCPCatalogID); err != nil {
		return err
	}

	values, err := h.read(ctx, server.CredentialContext(server.Spec.UserID), server.Name)
	if err != nil {
		return err
	}

	configuration := configValues(server.Spec.Manifest.Config)
	maps.Copy(configuration, values)

	component := *target.Spec.Manifest.Components[0].DeepCopy()
	component.SourceDigest = utils.Digest([]any{component, target.Spec.ComponentStaticConfigurationHashes[component.ID]})
	component.CatalogEntry.Manifest = server.Spec.Manifest.ConvertToCatalogEntry()
	component.CatalogEntry.UnsupportedTools = slices.Clone(server.Spec.UnsupportedTools)
	component.ForceSingleUser = true

	for i := range component.CatalogEntry.Manifest.Config {
		field := &component.CatalogEntry.Manifest.Config[i]
		field.Value = ""
		if !slices.ContainsFunc(component.Configuration, func(policy types.VMCPConfigurationPolicy) bool { return policy.Key == field.Key }) {
			component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{Key: field.Key, Policy: types.VMCPConfigurationPolicyUserAllowed})
		}
	}

	allowConfiguration(&component, configuration)

	return h.ensureInstance(ctx, client, *target, &server, server.Spec.UserID, component, configuration, server.Name)
}

func (h *Handler) migrateInstance(ctx context.Context, client kclient.Client, instance v1.MCPServerInstance, target v1.VMCP) error {
	if target.Spec.LegacySlug != instance.Spec.MCPServerName || len(target.Spec.Manifest.Components) != 1 || !target.DeletionTimestamp.IsZero() {
		return fmt.Errorf("invalid migration target %q for MCP server %q", target.Name, instance.Spec.MCPServerName)
	}

	values, err := h.read(ctx, instance.Spec.UserID+"-"+instance.Name, instance.Name)
	if err != nil {
		return err
	}

	configuration := configValues(instance.Spec.Config)
	maps.Copy(configuration, values)

	component := *target.Spec.Manifest.Components[0].DeepCopy()
	component.SourceDigest = utils.Digest([]any{component, target.Spec.ComponentStaticConfigurationHashes[component.ID]})
	allowConfiguration(&component, configuration)

	return h.ensureInstance(ctx, client, target, &instance, instance.Spec.UserID, component, configuration, instance.Spec.MCPServerName)
}

// Legacy servers predate vMCP ownership, so classify them by their original scope.
func legacySingleUser(server v1.MCPServer) bool {
	return server.Spec.MCPCatalogID == "" && server.Spec.PowerUserWorkspaceID == ""
}

func legacyStandaloneServer(server v1.MCPServer) bool {
	return server.Spec.NanobotAgentID == "" && server.Spec.VMCPID == "" && server.Spec.VMCPInstanceID == "" && server.Spec.VMCPComponentID == "" && server.Spec.CompositeName == "" && server.Spec.Manifest.Runtime != types.RuntimeComposite
}

func newVMCP(namespace, source string, manifest types.MCPServerCatalogEntryManifest, profiles []types.VMCPProfile) v1.VMCP {
	return v1.VMCP{
		Name:       migrationName(system.VMCPPrefix, namespace, source),
		Namespace:  namespace,
		Finalizers: []string{v1.VMCPFinalizer},
		Spec: v1.VMCPSpec{
			LegacySlug: source,
			Manifest: types.VMCPManifest{
				DisplayName: cmp.Or(manifest.Name, source),
				Description: manifest.Description,
				Icon:        manifest.Icon,
				Profiles:    profiles,
			},
		},
	}
}

func matchingProfiles(rules []v1.AccessControlRule, namespace, catalog, workspace string, resource types.Resource) []types.VMCPProfile {
	profiles := []types.VMCPProfile{}
	for _, rule := range rules {
		if rule.Namespace != namespace || !rule.DeletionTimestamp.IsZero() || rule.Spec.MCPCatalogID != catalog || rule.Spec.PowerUserWorkspaceID != workspace {
			continue
		}

		if slices.Contains(rule.Spec.Manifest.Resources, resource) || slices.Contains(rule.Spec.Manifest.Resources, types.Resource{Type: types.ResourceTypeSelector, ID: "*"}) {
			profiles = append(profiles, types.VMCPProfile{Name: rule.Name, Subjects: slices.Clone(rule.Spec.Manifest.Subjects), Permissions: types.VMCPProfilePermissions{AllowAllComponents: true}})
		}
	}

	return profiles
}

func newComponent(id, catalog, entry string, manifest types.MCPServerCatalogEntryManifest, values map[string]string, shared bool) (types.VMCPComponent, map[string]string) {
	component := types.VMCPComponent{
		ID:                      id,
		Name:                    cmp.Or(manifest.Name, id),
		MCPCatalogID:            catalog,
		MCPServerCatalogEntryID: entry,
		CatalogEntry:            types.MCPServerCatalogEntrySnapshot{Manifest: *manifest.DeepCopy()},
		ForceSingleUser:         !shared,
		OAuthCredentialID:       vmcp.StaticOAuthCredentialReference(manifest, entry),
	}

	static := make(map[string]string)

	for i := range component.CatalogEntry.Manifest.Config {
		config := &component.CatalogEntry.Manifest.Config[i]
		policy := types.VMCPConfigurationPolicyUserAllowed
		if shared && !config.UserAllowed || !shared && config.Value != "" {
			policy = types.VMCPConfigurationPolicyFixed
			value := config.Value
			if v, ok := values[config.Key]; ok {
				value = v
			}

			static[vmcp.ConfigurationKey(id, config.Key)] = value
		}

		config.Value, config.UserAllowed = "", false
		component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{Key: config.Key, Policy: policy})
	}

	return component, static
}

func configValues(config []types.MCPConfig) map[string]string {
	values := make(map[string]string)

	for _, field := range config {
		if field.Value != "" {
			values[field.Key] = field.Value
		}
	}

	return values
}

func allowConfiguration(component *types.VMCPComponent, values map[string]string) {
	for key := range values {
		index := slices.IndexFunc(component.Configuration, func(p types.VMCPConfigurationPolicy) bool { return p.Key == key })
		if index < 0 {
			component.Configuration = append(component.Configuration, types.VMCPConfigurationPolicy{Key: key, Policy: types.VMCPConfigurationPolicyUserAllowed})
		} else {
			component.Configuration[index].Policy = types.VMCPConfigurationPolicyUserAllowed
		}
	}
}

func (h *Handler) ensureVMCP(ctx context.Context, client kclient.Client, target *v1.VMCP, static map[string]string) error {
	var existing v1.VMCP
	if err := client.Get(ctx, kclient.ObjectKeyFromObject(target), &existing); err == nil {
		if existing.Spec.LegacySlug != target.Spec.LegacySlug || existing.Spec.UserID != target.Spec.UserID || !existing.DeletionTimestamp.IsZero() {
			return fmt.Errorf("vMCP migration target %q has different ownership or is deleting", target.Name)
		}

		*target = existing
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	if err := target.Spec.Manifest.Validate(); err != nil {
		return fmt.Errorf("migrate %q: %w", target.Spec.LegacySlug, err)
	}

	// Store fixed values under the vMCP before creating its object.
	vmcp.SetStaticConfigurationHashes(target, static)
	if err := h.save(ctx, vmcp.StaticConfigurationCredentialContext(target.Name), static); err != nil {
		return err
	}

	return client.Create(ctx, target)
}

func (h *Handler) ensureInstance(ctx context.Context, client kclient.Client, target v1.VMCP, source kclient.Object, userID string, component types.VMCPComponent, values map[string]string, oauthSource string) error {
	instance := v1.VMCPInstance{
		Name:       migrationName(system.VMCPInstancePrefix, source.GetNamespace(), source.GetName()),
		Namespace:  source.GetNamespace(),
		Finalizers: []string{v1.VMCPInstanceFinalizer},
		Spec: v1.VMCPInstanceSpec{
			LegacySlug:       source.GetName(),
			UserID:           userID,
			Manifest:         types.VMCPInstanceManifest{VMCPID: target.Name},
			LegacyComponents: []types.VMCPComponent{component},
		},
	}

	created := source.GetCreationTimestamp()
	instance.Spec.LegacyCreatedAt = &created

	var existing v1.VMCPInstance
	if err := client.Get(ctx, kclient.ObjectKeyFromObject(&instance), &existing); err == nil {
		if existing.Spec.LegacySlug != instance.Spec.LegacySlug || existing.Spec.UserID != userID || existing.Spec.Manifest.VMCPID != target.Name || !existing.DeletionTimestamp.IsZero() {
			return fmt.Errorf("vMCP instance migration target %q has different ownership or is deleting", instance.Name)
		}

		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	configuration := make(map[string]string)

	for key, value := range values {
		configuration[vmcp.ConfigurationKey(component.ID, key)] = value
	}

	if err := h.save(ctx, vmcp.InstanceConfigurationCredentialContext(instance.Name), configuration); err != nil {
		return err
	}

	if h.copyOAuth != nil {
		owner := instance.Name
		if vmcp.IsMultiUser(component) {
			owner = target.Name
		}

		if err := h.copyOAuth(ctx, userID, oauthSource, name.SafeConcatName(system.MCPServerPrefix+owner, component.ID)); err != nil {
			return err
		}
	}

	instance.Status.UserConfigurationHash = utils.Digest(configuration)
	return client.Create(ctx, &instance)
}

func (h *Handler) read(ctx context.Context, scope, name string) (map[string]string, error) {
	credential, err := h.reveal(ctx, []string{scope}, name)
	if errors.As(err, &gateway.CredentialNotFoundError{}) {
		return make(map[string]string), nil
	}

	return credential.Secrets, err
}

func (h *Handler) save(ctx context.Context, scope string, values map[string]string) error {
	return h.upsert(ctx, gatewaytypes.Credential{Context: scope, Name: vmcp.ConfigurationCredentialName(), Secrets: values})
}

func migrateFilters(ctx context.Context, client kclient.Client, target v1.VMCP, source types.Resource, catalog string) error {
	return MigrateFilters(ctx, client, target.Namespace, []types.Resource{{Type: types.ResourceTypeMCPServer, ID: target.Name}}, []types.Resource{source}, []string{catalog})
}

// MigrateFilters replaces direct legacy references while retaining catalog and
// wildcard scopes, which still apply to other resources. Retries also remove
// sources from filters that already contain a destination.
func MigrateFilters(ctx context.Context, client kclient.Client, namespace string, targets, sources []types.Resource, catalogs []string) error {
	var filters v1.MCPWebhookValidationList
	if err := client.List(ctx, &filters, kclient.InNamespace(namespace)); err != nil {
		return err
	}

	for i := range filters.Items {
		filter := &filters.Items[i]
		resources := filter.Spec.Manifest.Resources
		if !filter.DeletionTimestamp.IsZero() {
			continue
		}

		matches := slices.ContainsFunc(resources, func(resource types.Resource) bool {
			return slices.Contains(sources, resource) || resource.Type == types.ResourceTypeMcpCatalog && resource.ID != "" && slices.Contains(catalogs, resource.ID)
		})
		if !matches {
			continue
		}

		updated := slices.DeleteFunc(slices.Clone(resources), func(resource types.Resource) bool { return slices.Contains(sources, resource) })
		if !slices.Contains(updated, types.Resource{Type: types.ResourceTypeSelector, ID: "*"}) {
			for _, target := range targets {
				if !slices.Contains(updated, target) {
					updated = append(updated, target)
				}
			}
		}

		if slices.Equal(resources, updated) {
			continue
		}

		filter.Spec.Manifest.Resources = updated
		if err := client.Update(ctx, filter); err != nil {
			return err
		}
	}

	return nil
}

// Cleanup runs on every startup after the one-time migration has succeeded.
// Keep finalizers so normal controller cleanup can retire credentials/runtimes.
func Cleanup(ctx context.Context, client kclient.Client) error {
	var servers v1.MCPServerList
	if err := client.List(ctx, &servers); err != nil {
		return err
	}

	var instances v1.MCPServerInstanceList
	if err := client.List(ctx, &instances); err != nil {
		return err
	}

	for i := range instances.Items {
		instance := &instances.Items[i]
		if instance.Spec.VMCPComponentID == "" && instance.Spec.VMCPInstanceID == "" && instance.DeletionTimestamp.IsZero() {
			if slices.ContainsFunc(servers.Items, func(server v1.MCPServer) bool {
				return server.Namespace == instance.Namespace && server.Name == instance.Spec.MCPServerName && server.Spec.NanobotAgentID != ""
			}) {
				continue
			}

			if err := kclient.IgnoreNotFound(client.Delete(ctx, instance)); err != nil {
				return err
			}
		}
	}

	for i := range servers.Items {
		server := &servers.Items[i]
		if server.Spec.NanobotAgentID == "" && server.Spec.VMCPComponentID == "" && server.Spec.VMCPID == "" && server.Spec.VMCPInstanceID == "" && server.DeletionTimestamp.IsZero() {
			if err := kclient.IgnoreNotFound(client.Delete(ctx, server)); err != nil {
				return err
			}
		}
	}

	return nil
}

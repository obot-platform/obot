package mcpcatalog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/gitcredential"
	"github.com/obot-platform/obot/pkg/mcp"
	catalogvalidation "github.com/obot-platform/obot/pkg/mcpcatalog"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// VMCPCatalogCredentialContext isolates vMCP source tokens from MCP catalogs with the same name.
func VMCPCatalogCredentialContext(catalogName string) string {
	return "vmcp-catalog-" + catalogName
}

func (*Handler) SetUpDefaultVMCPCatalog(ctx context.Context, c kclient.Client, paths string) error {
	var existing v1.VMCPCatalog
	if err := c.Get(ctx, router.Key(system.DefaultNamespace, system.DefaultCatalog), &existing); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}
	var sources []string
	for path := range strings.SplitSeq(paths, ",") {
		if path = strings.TrimSpace(path); path != "" {
			sources = append(sources, path)
		}
	}
	return c.Create(ctx, &v1.VMCPCatalog{
		Name:       system.DefaultCatalog,
		Namespace:  system.DefaultNamespace,
		Finalizers: []string{v1.VMCPCatalogFinalizer},
		Spec:       v1.VMCPCatalogSpec{DisplayName: "Default", SourceURLs: sources},
	})
}

func (h *Handler) SyncVMCP(req router.Request, resp router.Response) error {
	catalog := req.Object.(*v1.VMCPCatalog)

	interval := time.Hour
	if len(catalog.Status.SyncErrors) > 0 {
		interval = time.Minute
	}

	force := catalog.Annotations[v1.VMCPCatalogSyncAnnotation] == "true" || catalog.Annotations[forceSyncStartupAnnotation] != startupSyncGeneration
	if elapsed := time.Since(catalog.Status.LastSyncTime.Time); !force && elapsed < interval {
		resp.RetryAfter(interval - elapsed)
		return nil
	}

	catalog.Status.IsSyncing = true
	if err := req.Client.Status().Update(req.Ctx, catalog); err != nil {
		return err
	}
	defer func() {
		var current v1.VMCPCatalog
		if err := req.Client.Get(req.Ctx, kclient.ObjectKeyFromObject(catalog), &current); err != nil {
			slog.Error("failed to get vMCP catalog after sync", "error", err)
			return
		}

		current.Status.IsSyncing = false
		if err := req.Client.Status().Update(req.Ctx, &current); err != nil {
			slog.Error("failed to clear vMCP catalog syncing status", "error", err)
		}
	}()

	var entries v1.MCPServerCatalogEntryList
	if err := req.Client.List(req.Ctx, &entries, kclient.InNamespace(catalog.Namespace)); err != nil {
		return fmt.Errorf("list catalog entries for vMCP sync: %w", err)
	}

	catalog.Status.SyncErrors = map[string]string{}
	var desired []kclient.Object
	seen := map[string]struct{}{}
	for _, source := range catalog.Spec.SourceURLs {
		token, err := gitcredential.ResolveOrReveal(req.Ctx, req.Client, h.gatewayClient, catalog.Namespace, catalog.Spec.SourceURLGitCredentialIDs[source], source, VMCPCatalogCredentialContext(catalog.Name), CatalogCredentialToolName)
		if errors.Is(err, gitcredential.ErrLegacyCredential) {
			slog.Error("failed to retrieve legacy vMCP catalog credential, continuing without authentication", "source", source, "error", err)
		} else if err != nil {
			addSyncError(catalog.Status.SyncErrors, source, err.Error())
			continue
		}

		definitions, err := readCatalogManifests[types.VMCPSourceManifest](req.Ctx, h.httpClient, source, token)
		if err != nil {
			addSyncError(catalog.Status.SyncErrors, source, err.Error())
		}

		for _, definition := range definitions {
			id := vmcpName(catalog.Name, source, definition.DisplayName)
			if _, ok := seen[id]; ok {
				addSyncError(catalog.Status.SyncErrors, source, fmt.Sprintf("duplicate vMCP display name %q", definition.DisplayName))
				continue
			}

			seen[id] = struct{}{}
			vmcp, err := h.syncVMCPDefinition(req.Ctx, req.Client, catalog, source, id, definition, entries.Items)
			if err != nil {
				addSyncError(catalog.Status.SyncErrors, source, fmt.Sprintf("vMCP %q: %v", definition.DisplayName, err))
				continue
			}

			desired = append(desired, vmcp)
		}
	}

	// A failed read or unresolved reference is not evidence of upstream removal.
	if len(catalog.Status.SyncErrors) == 0 {
		if err := reconcileRemovedVMCPs(req.Ctx, req.Client, catalog, desired); err != nil {
			return err
		}
	}

	catalog.Status.LastSyncTime = metav1.Now()
	if err := req.Client.Status().Update(req.Ctx, catalog); err != nil {
		return err
	}

	delete(catalog.Annotations, v1.VMCPCatalogSyncAnnotation)
	if catalog.Annotations == nil {
		catalog.Annotations = make(map[string]string, 1)
	}
	catalog.Annotations[forceSyncStartupAnnotation] = startupSyncGeneration
	if err := req.Client.Update(req.Ctx, catalog); err != nil {
		return err
	}

	interval = time.Hour
	if len(catalog.Status.SyncErrors) > 0 {
		interval = time.Minute
	}

	resp.RetryAfter(interval)
	return nil
}

func vmcpName(catalogName, source, displayName string) string {
	return name.SafeHashConcatName(system.VMCPPrefix, catalogName, utils.Digest(mcp.SourceIDForURL(source))[:8], catalogvalidation.SanitizeName(displayName), utils.Digest(displayName)[:8])
}

func (h *Handler) RemoveVMCPCatalog(req router.Request, _ router.Response) error {
	catalog := req.Object.(*v1.VMCPCatalog)
	if err := reconcileRemovedVMCPs(req.Ctx, req.Client, catalog, nil); err != nil {
		return err
	}

	_, err := h.gatewayClient.DeleteCredential(req.Ctx, VMCPCatalogCredentialContext(catalog.Name), CatalogCredentialToolName)
	return err
}

func (h *Handler) syncVMCPDefinition(ctx context.Context, c kclient.Client, catalog *v1.VMCPCatalog, source, id string, definition types.VMCPSourceManifest, entries []v1.MCPServerCatalogEntry) (*v1.VMCP, error) {
	manifest, err := resolveVMCPDefinition(definition, entries)
	if err != nil {
		return nil, err
	}

	digest := utils.Digest(definition)
	configuration := vmcpconfig.ExtractStaticConfiguration(&manifest, nil)
	credentialContext := vmcpconfig.StaticConfigurationCredentialContext(id)
	credentialName := vmcpconfig.ConfigurationCredentialName()
	var (
		current     v1.VMCP
		rollbackErr error
		attemptErr  error
	)
	err = retry.OnError(retry.DefaultBackoff, func(err error) bool {
		attemptErr = err
		return rollbackErr == nil && (apierrors.IsConflict(err) || apierrors.IsAlreadyExists(err))
	}, func() error {
		attemptErr = nil
		current = v1.VMCP{}
		err := c.Get(ctx, router.Key(catalog.Namespace, id), &current)
		exists := err == nil
		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		if exists && (current.Spec.VMCPCatalogName != catalog.Name || mcp.SourceIDForURL(current.Spec.SourceURL) != mcp.SourceIDForURL(source)) {
			return fmt.Errorf("vMCP %q conflicts with an existing definition", id)
		}
		if exists && !current.DeletionTimestamp.IsZero() {
			return fmt.Errorf("vMCP %q is being deleted", id)
		}

		// Still resolve references above so unavailable sources surface as sync errors,
		// but catalog drift alone must never deploy new snapshots or credentials.
		if exists && !current.Spec.Detached && current.Spec.SourceDigest == digest {
			return nil
		}

		previous, err := h.gatewayClient.RevealCredential(ctx, []string{credentialContext}, credentialName)
		credentialExists := err == nil
		if err != nil && !errors.As(err, &gatewayclient.CredentialNotFoundError{}) {
			return fmt.Errorf("read vMCP static configuration before sync: %w", err)
		}

		if !exists {
			current = v1.VMCP{
				Name:       id,
				Namespace:  catalog.Namespace,
				Finalizers: []string{v1.VMCPFinalizer},
			}
		}
		current.Spec.Manifest = manifest
		current.Spec.VMCPCatalogName = catalog.Name
		current.Spec.SourceURL = source
		current.Spec.SourceDigest = digest
		current.Spec.Detached = false
		vmcpconfig.SetStaticConfigurationHashes(&current, configuration)

		if err := h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: credentialContext,
			Name:    credentialName,
			Secrets: configuration,
		}); err != nil {
			return fmt.Errorf("store vMCP static configuration: %w", err)
		}

		if exists {
			err = c.Update(ctx, &current)
		} else {
			err = c.Create(ctx, &current)
		}
		if err != nil {
			// Roll back even if the resource write failed because sync was canceled.
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if credentialExists {
				rollbackErr = h.gatewayClient.UpsertCredential(rollbackCtx, previous)
			} else {
				_, rollbackErr = h.gatewayClient.DeleteCredential(rollbackCtx, credentialContext, credentialName)
			}
			if rollbackErr != nil {
				rollbackErr = fmt.Errorf("roll back vMCP static configuration: %w", rollbackErr)
			}
		}
		return err
	})

	// retry.OnError can discard cancellation errors from a non-retriable attempt.
	if err == nil {
		err = attemptErr
	}
	return &current, errors.Join(err, rollbackErr)
}

func resolveVMCPDefinition(source types.VMCPSourceManifest, entries []v1.MCPServerCatalogEntry) (types.VMCPManifest, error) {
	manifest := types.VMCPManifest{
		DisplayName: source.DisplayName,
		Description: source.Description,
		Icon:        source.Icon,
		Profiles:    source.Profiles,
	}
	for _, component := range source.Components {
		refSource, key, hasSep, valid := parseSourceRef("", component.EntryKey)
		if !valid || !hasSep {
			return manifest, fmt.Errorf("component %q entryKey must have the form sourceURLOrPath::entryKey", component.Name)
		}
		var resolved *v1.MCPServerCatalogEntry
		for i := range entries {
			entry := &entries[i]
			if !entry.IsGitManaged() || !entry.DeletionTimestamp.IsZero() || entry.Spec.PowerUserWorkspaceID != "" || entry.Spec.Manifest.EntryKey != key || mcp.SourceIDForURL(entry.Spec.SourceURL) != mcp.SourceIDForURL(refSource) {
				continue
			}
			if resolved != nil {
				return manifest, fmt.Errorf("component %q entryKey %q is ambiguous", component.Name, component.EntryKey)
			}
			resolved = entry
		}
		if resolved == nil {
			return manifest, fmt.Errorf("component %q entryKey %q cannot be resolved", component.Name, component.EntryKey)
		}
		if component.Name == "" {
			component.Name = resolved.Spec.Manifest.Name
			if component.Name == "" {
				component.Name = resolved.Name
			}
		}
		snapshot := types.MCPServerCatalogEntrySnapshot{
			Manifest:         resolved.Spec.Manifest,
			UnsupportedTools: resolved.Spec.UnsupportedTools,
		}
		manifest.Components = append(manifest.Components, types.VMCPComponent{
			ID:                      utils.Digest(component.EntryKey),
			Name:                    component.Name,
			MCPCatalogID:            resolved.Spec.MCPCatalogName,
			MCPServerCatalogEntryID: resolved.Name,
			CatalogEntry:            snapshot,
			SourceDigest:            utils.Digest(snapshot),
			OAuthCredentialID:       vmcpconfig.StaticOAuthCredentialReference(snapshot.Manifest, resolved.Name),
			Configuration:           component.Configuration,
			ForceSingleUser:         component.ForceSingleUser,
			AllowedTools:            component.AllowedTools,
			ToolPrefix:              component.ToolPrefix,
			ToolOverrides:           component.ToolOverrides,
		})
	}
	ids := make(map[string]string, len(manifest.Components))
	for i, component := range manifest.Components {
		ids[source.Components[i].EntryKey] = component.ID
	}
	// Copy before filtering policies and translating profile entry keys so the source stays unchanged.
	manifest = *manifest.DeepCopy()
	for i := range manifest.Components {
		component := &manifest.Components[i]
		static := map[string]bool{}
		for _, field := range component.CatalogEntry.Manifest.Config {
			static[field.Key] = field.Value != "" || field.SecretBinding != nil
		}

		component.Configuration = slices.DeleteFunc(component.Configuration, func(policy types.VMCPConfigurationPolicy) bool {
			return static[policy.Key]
		})
	}

	for i := range manifest.Profiles {
		profile := &manifest.Profiles[i]
		if profile.Permissions.AllowedComponents == nil {
			continue
		}
		components := map[string]types.VMCPComponentSet{}
		for entryKey, component := range profile.Permissions.AllowedComponents {
			id, ok := ids[entryKey]
			if !ok {
				return manifest, fmt.Errorf("profile %q references unknown component entryKey %q", profile.Name, entryKey)
			}
			components[id] = component
		}
		profile.Permissions.AllowedComponents = components
	}
	// Synced definitions have no creating user, so only configuration policies are
	// defaulted. Access comes from the profiles the source declares.
	manifest.DefaultConfigurationPolicies()
	return manifest, manifest.ValidateForSync()
}

// reconcileRemovedVMCPs detaches referenced definitions and deletes unused ones.
// Detached definitions belong to the API and are no longer subject to pruning.
func reconcileRemovedVMCPs(ctx context.Context, c kclient.Client, catalog *v1.VMCPCatalog, desired []kclient.Object) error {
	keep := make(map[string]struct{}, len(desired))
	for _, obj := range desired {
		keep[obj.GetName()] = struct{}{}
	}
	var definitions v1.VMCPList
	if err := c.List(ctx, &definitions, kclient.InNamespace(catalog.Namespace)); err != nil {
		return err
	}
	for _, definition := range definitions.Items {
		if definition.Spec.VMCPCatalogName != catalog.Name || !definition.IsGitManaged() {
			continue
		}
		if _, ok := keep[definition.Name]; ok {
			continue
		}
		if err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			var current v1.VMCP
			if err := c.Get(ctx, kclient.ObjectKeyFromObject(&definition), &current); err != nil {
				return kclient.IgnoreNotFound(err)
			}
			if !current.IsGitManaged() || current.Spec.VMCPCatalogName != catalog.Name {
				return nil
			}
			var instances v1.VMCPInstanceList
			if err := c.List(ctx, &instances, kclient.InNamespace(catalog.Namespace), kclient.MatchingFields{"spec.manifest.vmcpID": current.Name}); err != nil {
				return err
			}
			if len(instances.Items) == 0 {
				return kclient.IgnoreNotFound(c.Delete(ctx, &current, kclient.Preconditions{UID: &current.UID, ResourceVersion: &current.ResourceVersion}))
			}
			current.Spec.Detached = true
			return c.Update(ctx, &current)
		}); err != nil {
			return fmt.Errorf("reconcile removed vMCP %q: %w", definition.Name, err)
		}
	}
	return nil
}

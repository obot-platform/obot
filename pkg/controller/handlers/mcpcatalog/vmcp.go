package mcpcatalog

import (
	"context"
	"errors"
	"fmt"

	"github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *Handler) prepareCatalogVMCPs(ctx context.Context, c kclient.Client, catalog *v1.MCPCatalog, objs []kclient.Object) ([]kclient.Object, map[string]string, error) {
	entries := make([]kclient.Object, 0, len(objs))
	vmcps := make([]*v1.VMCP, 0)
	entriesByRef := make(map[string]*v1.MCPServerCatalogEntry)
	for _, obj := range objs {
		if entry, ok := obj.(*v1.MCPServerCatalogEntry); ok {
			entries = append(entries, entry)
			if entry.Spec.Manifest.EntryKey != "" {
				ref := sourceRef(mcp.SourceIDForURL(entry.Spec.SourceURL), entry.Spec.Manifest.EntryKey)
				entriesByRef[ref] = entry
			}
		} else if vmcp, ok := obj.(*v1.VMCP); ok {
			vmcps = append(vmcps, vmcp)
		}
	}
	if len(vmcps) == 0 {
		return objs, nil, nil
	}

	var existingList v1.VMCPList
	if err := c.List(ctx, &existingList, kclient.InNamespace(catalog.Namespace)); err != nil {
		return nil, nil, fmt.Errorf("failed to list vMCPs: %w", err)
	}
	existingByName := make(map[string]*v1.VMCP, len(existingList.Items))
	for index := range existingList.Items {
		existing := &existingList.Items[index]
		// Older vMCPs predate retained source references. Recover them before
		// renamed catalog entries are removed by this sync.
		if existing.Spec.ComponentCatalogReferences == nil {
			existing.Spec.ComponentCatalogReferences = make(map[string]string)
		}
		for _, component := range existing.Spec.Manifest.Components {
			if existing.Spec.ComponentCatalogReferences[component.ID] != "" || component.MCPServerCatalogEntryID == "" {
				continue
			}
			var entry v1.MCPServerCatalogEntry
			if err := c.Get(ctx, kclient.ObjectKey{Namespace: existing.Namespace, Name: component.MCPServerCatalogEntryID}, &entry); apierrors.IsNotFound(err) {
				continue
			} else if err != nil {
				return nil, nil, fmt.Errorf("failed to recover vMCP component catalog reference: %w", err)
			}
			if entry.Spec.SourceURL != "" && entry.Spec.Manifest.EntryKey != "" {
				existing.Spec.ComponentCatalogReferences[component.ID] = sourceRef(mcp.SourceIDForURL(entry.Spec.SourceURL), entry.Spec.Manifest.EntryKey)
			}
		}

		existingByName[existing.Name] = existing
	}

	syncErrors := make(map[string]string)
	seenNames := make(map[string]struct{}, len(vmcps))
	for _, vmcp := range vmcps {
		sourceURL := vmcp.Spec.SourceURL
		existing := existingByName[vmcp.Name]
		if existing != nil {
			adopting := existing.Spec.Adopted != nil && !*existing.Spec.Adopted
			if !catalogOwnsVMCP(existing, catalog.Name) && !adopting {
				addSyncError(syncErrors, sourceURL, fmt.Sprintf("vMCP %q conflicts with an Obot-managed vMCP", vmcp.Spec.Manifest.DisplayName))
				continue
			}
			vmcp.Name = existing.Name
			vmcp.Spec.Adopted = existing.Spec.Adopted
			if adopting {
				vmcp.Spec.Adopted = new(true)
			}
			vmcp.Spec.LegacySlug = existing.Spec.LegacySlug
			vmcp.Spec.CreatorUserID = existing.Spec.CreatorUserID
			vmcp.Spec.UserID = existing.Spec.UserID
		}
		if _, duplicate := seenNames[vmcp.Name]; duplicate {
			addSyncError(syncErrors, sourceURL, fmt.Sprintf("duplicate vMCP identity %q", vmcp.Name))
			continue
		}

		if err := resolveCatalogVMCPComponents(vmcp, existing, sourceURL, entriesByRef); err != nil {
			addSyncError(syncErrors, sourceURL, fmt.Sprintf("vMCP %q: %v", vmcp.Spec.Manifest.DisplayName, err))
			continue
		}
		if err := vmcp.Spec.Manifest.Validate(); err != nil {
			addSyncError(syncErrors, sourceURL, fmt.Sprintf("vMCP %q: %v", vmcp.Spec.Manifest.DisplayName, err))
			continue
		}

		var previousConfiguration map[string]string
		if existing != nil {
			credential, err := h.gatewayClient.RevealCredential(ctx,
				[]string{vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name)},
				vmcpconfig.StaticConfigurationCredentialName(existing),
			)
			if err != nil {
				if _, notFound := errors.AsType[gclient.CredentialNotFoundError](err); !notFound {
					addSyncError(syncErrors, sourceURL, fmt.Sprintf("failed to read vMCP %q configuration: %v", vmcp.Name, err))
					continue
				}
			}
			previousConfiguration = credential.Secrets
		}

		staticConfiguration := vmcpconfig.ExtractStaticConfiguration(&vmcp.Spec.Manifest, previousConfiguration)
		vmcpconfig.VersionStaticConfiguration(vmcp, staticConfiguration)
		if err := h.gatewayClient.UpsertCredential(ctx, gatewaytypes.Credential{
			Context: vmcpconfig.StaticConfigurationCredentialContext(vmcp.Name),
			Name:    vmcpconfig.StaticConfigurationCredentialName(vmcp),
			Secrets: staticConfiguration,
		}); err != nil {
			addSyncError(syncErrors, sourceURL, fmt.Sprintf("failed to store vMCP %q configuration: %v", vmcp.Name, err))
			continue
		}

		seenNames[vmcp.Name] = struct{}{}
		entries = append(entries, vmcp)
	}
	return entries, syncErrors, nil
}

func resolveCatalogVMCPComponents(vmcp *v1.VMCP, existing *v1.VMCP, sourceURL string, entriesByRef map[string]*v1.MCPServerCatalogEntry) error {
	sourceID := mcp.SourceIDForURL(sourceURL)
	vmcp.Spec.ComponentCatalogReferences = make(map[string]string)
	for index := range vmcp.Spec.Manifest.Components {
		component := &vmcp.Spec.Manifest.Components[index]
		refSourceID, entryKey, _, valid := parseSourceRef(sourceID, component.MCPServerCatalogEntryID)
		if !valid {
			return fmt.Errorf("invalid component catalog entry reference %q", component.MCPServerCatalogEntryID)
		}
		reference := sourceRef(refSourceID, entryKey)
		entry := entriesByRef[reference]
		if entry == nil {
			return fmt.Errorf("component catalog entry %q was not found", component.MCPServerCatalogEntryID)
		}

		component.MCPServerCatalogEntryID = entry.Name
		component.MCPCatalogID = entry.Spec.MCPCatalogName
		component.CatalogEntry = types.MCPServerCatalogEntrySnapshot{
			Manifest:         entry.Spec.Manifest,
			UnsupportedTools: entry.Spec.UnsupportedTools,
		}
		component.SourceDigest = utils.Digest(component.CatalogEntry)
		component.OAuthCredentialID = vmcpconfig.StaticOAuthCredentialReference(entry.Spec.Manifest, entry.Name)
		if component.Name == "" {
			component.Name = entry.Spec.Manifest.Name
		}
		// Prefer an explicit stable ID; otherwise match the referenced entry.
		// Retain migrated single-user runtimes so sync does not replace their OAuth identities.
		var (
			matches       []types.VMCPComponent
			named         *types.VMCPComponent
			explicitMatch bool
		)
		for _, previous := range existingComponents(existing) {
			if component.ID != "" && previous.ID == component.ID {
				component.ForceSingleUser = component.ForceSingleUser || previous.ForceSingleUser
				explicitMatch = true
				break
			}
			previousReference := existing.Spec.ComponentCatalogReferences[previous.ID]
			if previousReference != "" && previousReference != reference || previousReference == "" && previous.MCPServerCatalogEntryID != component.MCPServerCatalogEntryID {
				continue
			}
			matches = append(matches, previous)
			if previous.Name == component.Name {
				named = &previous
			}
		}

		if explicitMatch {
			vmcp.Spec.ComponentCatalogReferences[component.ID] = reference
			continue
		}

		if named != nil {
			if component.ID != "" && component.ID != named.ID {
				return fmt.Errorf("component %q cannot change ID", component.Name)
			}
			component.ID = named.ID
			component.ForceSingleUser = component.ForceSingleUser || named.ForceSingleUser
		} else if component.ID == "" {
			switch len(matches) {
			case 0:
				component.ID = utils.Digest([]string{reference, component.Name})[:32]
			case 1:
				component.ID = matches[0].ID
				component.ForceSingleUser = component.ForceSingleUser || matches[0].ForceSingleUser
			default:
				return fmt.Errorf("component %q matches multiple existing components; specify an id", component.Name)
			}
		}
		vmcp.Spec.ComponentCatalogReferences[component.ID] = reference
	}
	return nil
}

func existingComponents(vmcp *v1.VMCP) []types.VMCPComponent {
	if vmcp == nil {
		return nil
	}
	return vmcp.Spec.Manifest.Components
}

func catalogOwnsVMCP(vmcp *v1.VMCP, catalogName string) bool {
	for _, owner := range vmcp.OwnerReferences {
		if owner.APIVersion == v1.SchemeGroupVersion.String() && owner.Kind == "MCPCatalog" && owner.Name == catalogName {
			return true
		}
	}
	return false
}

func reconcileRemovedVMCPs(ctx context.Context, c kclient.Client, catalog *v1.MCPCatalog, desired []kclient.Object) error {
	desiredNames := make(map[string]struct{})
	for _, obj := range desired {
		if vmcp, ok := obj.(*v1.VMCP); ok {
			desiredNames[vmcp.Name] = struct{}{}
		}
	}

	var existing v1.VMCPList
	if err := c.List(ctx, &existing, kclient.InNamespace(catalog.Namespace)); err != nil {
		return fmt.Errorf("failed to list catalog vMCPs: %w", err)
	}
	for index := range existing.Items {
		vmcp := &existing.Items[index]
		if !catalogOwnsVMCP(vmcp, catalog.Name) {
			continue
		}
		if _, wanted := desiredNames[vmcp.Name]; wanted {
			continue
		}
		if err := c.Delete(ctx, vmcp); err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete removed catalog vMCP %q: %w", vmcp.Name, err)
		}
	}
	return nil
}

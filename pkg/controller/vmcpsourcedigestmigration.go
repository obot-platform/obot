package controller

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	vmcpSourceDigestMigrationName = "vmcp_component_source_digest"
)

// legacySourceDigests returns every form a stored source digest of the snapshot may take: the
// current form, followed by the legacy forms.
func legacySourceDigests(snapshot types.MCPServerCatalogEntrySnapshot) []string {
	return append([]string{vmcpconfig.SourceDigest(snapshot)}, vmcpconfig.LegacySourceDigests(snapshot)...)
}

// legacyNeedsUpdate reports whether a component whose source digest may be stored in a legacy
// form has drifted from its source.
func legacyNeedsUpdate(component types.VMCPComponent, current types.MCPServerCatalogEntrySnapshot) bool {
	digest := component.SourceDigest
	if digest == "" || slices.Contains(legacySourceDigests(component.CatalogEntry), digest) {
		// The snapshot is an exact copy of its source, so the digest can be recomputed.
		digest = vmcpconfig.SourceDigest(component.CatalogEntry)
	}
	// Migrations that strip fixed values from the snapshot may have stored a legacy digest form,
	// which still matches an unchanged source.
	return !slices.Contains(legacySourceDigests(current), digest)
}

// migrateVMCPSourceDigests stores every vMCP component's source digest in the current form, so
// drift detection compares a single digest. It must run after the static configuration migration,
// which judges drift against legacy digest forms.
func migrateVMCPSourceDigests(ctx context.Context, client kclient.Client) error {
	var (
		vmcps     v1.VMCPList
		instances v1.VMCPInstanceList
	)
	for _, list := range []kclient.ObjectList{&vmcps, &instances} {
		if err := client.List(ctx, list); err != nil {
			return err
		}
	}

	for i := range vmcps.Items {
		vmcp := &vmcps.Items[i]
		var changed [][2]types.VMCPComponent
		for j := range vmcp.Spec.Manifest.Components {
			component := &vmcp.Spec.Manifest.Components[j]
			digest, err := currentSourceDigest(ctx, client, vmcp.Namespace, *component)
			if err != nil {
				return fmt.Errorf("failed to compute source digest of vMCP %q component %q: %w", vmcp.Name, component.ID, err)
			}
			if digest != component.SourceDigest {
				previous := *component.DeepCopy()
				component.SourceDigest = digest
				changed = append(changed, [2]types.VMCPComponent{previous, *component})
			}
		}
		if len(changed) == 0 {
			continue
		}

		// Legacy components are rebound before the vMCP is updated, so a retry after a failed
		// update finds them bound to the component it computes again.
		for _, components := range changed {
			if err := retainLegacyComponents(ctx, client, *vmcp, components[0], components[1], instances.Items); err != nil {
				return err
			}
		}
		if err := client.Update(ctx, vmcp); err != nil {
			return fmt.Errorf("failed to update vMCP %q: %w", vmcp.Name, err)
		}
	}
	return nil
}

// currentSourceDigest returns a component's source digest in the current form. A digest that
// cannot be recomputed from the snapshot, because a migration stripped values from it, is
// recomputed from its source while it still matches; otherwise the source has drifted and the
// digest is kept.
func currentSourceDigest(ctx context.Context, client kclient.Client, namespace string, component types.VMCPComponent) (string, error) {
	if component.SourceDigest == "" || slices.Contains(legacySourceDigests(component.CatalogEntry), component.SourceDigest) {
		return vmcpconfig.SourceDigest(component.CatalogEntry), nil
	}
	if component.MCPServerCatalogEntryID == "" {
		return component.SourceDigest, nil
	}

	var entry v1.MCPServerCatalogEntry
	if err := client.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: component.MCPServerCatalogEntryID}, &entry); apierrors.IsNotFound(err) {
		return component.SourceDigest, nil
	} else if err != nil {
		return "", err
	}
	if snapshot := entrySnapshot(entry); slices.Contains(legacySourceDigests(snapshot), component.SourceDigest) {
		return vmcpconfig.SourceDigest(snapshot), nil
	}
	return component.SourceDigest, nil
}

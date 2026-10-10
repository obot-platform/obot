package vmcp

import (
	"fmt"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// DetectDrift reports source changes without changing the deployed snapshots. Migrated
// connections may retain older snapshots than a current component, and updating the vMCP is how
// they are released, so those are reported as well.
func DetectDrift(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)

	var instances v1.VMCPInstanceList
	if err := req.List(&instances, &kclient.ListOptions{Namespace: vmcp.Namespace, FieldSelector: fields.OneTermEqualSelector("spec.manifest.vmcpID", vmcp.Name)}); err != nil {
		return fmt.Errorf("list VMCP instances: %w", err)
	}

	statuses := make([]v1.VMCPComponentStatus, 0, len(vmcp.Spec.Manifest.Components))
	for _, component := range vmcp.Spec.Manifest.Components {
		status := v1.VMCPComponentStatus{Name: component.Name}
		for _, previous := range vmcp.Status.Components {
			if previous.Name == component.Name {
				status = previous
				break
			}
		}
		status.SourceMissing = false
		status.NeedsUpdate = false
		status.ConnectionSnapshot = nil
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, vmcp.Namespace, component.MCPServerCatalogEntryID); apierrors.IsNotFound(err) {
			status.SourceMissing = true
		} else if err != nil {
			return fmt.Errorf("get VMCP component source %q: %w", component.MCPServerCatalogEntryID, err)
		} else {
			status.NeedsUpdate = vmcpconfig.NeedsUpdate(component, types.MCPServerCatalogEntrySnapshot{
				Manifest:         entry.Spec.Manifest,
				UnsupportedTools: entry.Spec.UnsupportedTools,
			})
			if !status.NeedsUpdate {
				status.ConnectionSnapshot = outdatedConnectionSnapshot(*vmcp, component, instances.Items)
				status.NeedsUpdate = status.ConnectionSnapshot != nil
			}
		}
		statuses = append(statuses, status)
	}
	if equality.Semantic.DeepEqual(vmcp.Status.Components, statuses) {
		return nil
	}
	vmcp.Status.Components = statuses
	return req.Client.Status().Update(req.Ctx, vmcp)
}

// outdatedConnectionSnapshot returns the first outdated snapshot a connection retains for the
// component, if any.
func outdatedConnectionSnapshot(vmcp v1.VMCP, component types.VMCPComponent, instances []v1.VMCPInstance) *types.MCPServerCatalogEntrySnapshot {
	for _, instance := range instances {
		if !instance.DeletionTimestamp.IsZero() {
			continue
		}
		if legacy, ok := vmcpconfig.LegacyComponent(vmcp, instance, component); ok && vmcpconfig.LegacyComponentNeedsUpdate(legacy, component) {
			snapshot := vmcpconfig.LegacySnapshot(legacy, component)
			return &snapshot
		}
	}
	return nil
}

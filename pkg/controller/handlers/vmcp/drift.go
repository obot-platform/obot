package vmcp

import (
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// DetectDrift reports source changes without changing the deployed snapshots.
func DetectDrift(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
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
		var entry v1.MCPServerCatalogEntry
		if err := req.Get(&entry, vmcp.Namespace, component.MCPServerCatalogEntryID); apierrors.IsNotFound(err) {
			status.SourceMissing = true
		} else if err != nil {
			return fmt.Errorf("get VMCP component source %q: %w", component.MCPServerCatalogEntryID, err)
		} else {
			current := types.MCPServerCatalogEntrySnapshot{
				Manifest:         entry.Spec.Manifest,
				UnsupportedTools: entry.Spec.UnsupportedTools,
			}
			status.NeedsUpdate = utils.Digest(component.CatalogEntry) != utils.Digest(current)
		}
		statuses = append(statuses, status)
	}
	if slices.Equal(vmcp.Status.Components, statuses) {
		return nil
	}
	vmcp.Status.Components = statuses
	return req.Client.Status().Update(req.Ctx, vmcp)
}

package vmcp

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// ReconcileProfiles keeps the vMCP's profiles identified and their stored data in step with the manifest. It gives
// each profile stored before profiles had IDs its derived ID, once; deletes the data stored for profiles that the
// manifest no longer has, by comparing the profile IDs last recorded in status with the manifest's; and reports
// whether agent selection is ambiguous.
func (h *Handler) ReconcileProfiles(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if !vmcp.DeletionTimestamp.IsZero() {
		// The finalizer deletes the data of every profile.
		return nil
	}

	// The API and catalog sync assign IDs when they save a vMCP, so only a vMCP stored before profiles had IDs lacks
	// them. Its profiles get the IDs catalog sync derives, so a catalog-synced vMCP keeps them on its next sync.
	if vmcpconfig.AssignDerivedProfileIDs(vmcp.Name, &vmcp.Spec.Manifest) {
		return req.Client.Update(req.Ctx, vmcp)
	}

	current := vmcpconfig.ProfileIDs(vmcp.Spec.Manifest)
	removed := slices.DeleteFunc(slices.Clone(vmcp.Status.ProfileIDs), func(id string) bool {
		return slices.Contains(current, id)
	})
	// Configuration is only stored for a profile recorded in ProfileIDs, but its record in status names the profile
	// too, so a profile with one is cleaned up even if it was somehow never recorded.
	for _, id := range slices.Sorted(maps.Keys(vmcp.Status.ProfileConfigurationRevisions)) {
		if !slices.Contains(current, id) && !slices.Contains(removed, id) {
			removed = append(removed, id)
		}
	}
	if len(removed) > 0 {
		if err := h.deleteProfileData(req, vmcp.Name, removed); err != nil {
			return err
		}
		slog.Info("Deleted the data of removed vMCP profiles", "vmcp", vmcp.Name, "profiles", removed)
	}

	changed := !slices.Equal(vmcp.Status.ProfileIDs, current)
	vmcp.Status.ProfileIDs = current
	for _, id := range removed {
		if _, ok := vmcp.Status.ProfileConfigurationRevisions[id]; ok {
			delete(vmcp.Status.ProfileConfigurationRevisions, id)
			changed = true
		}
	}
	if meta.SetStatusCondition(&vmcp.Status.Conditions, agentSelectionCondition(vmcp.Spec.Manifest)) {
		changed = true
	}
	if !changed {
		return nil
	}
	return req.Client.Status().Update(req.Ctx, vmcp)
}

// RemoveProfileData deletes the data stored for every profile of a vMCP being deleted: those in its manifest and those
// last recorded in its status, which include profiles removed just before the deletion.
func (h *Handler) RemoveProfileData(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	ids := vmcpconfig.ProfileIDs(vmcp.Spec.Manifest)
	for _, id := range slices.Concat(vmcp.Status.ProfileIDs, slices.Sorted(maps.Keys(vmcp.Status.ProfileConfigurationRevisions))) {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return h.deleteProfileData(req, vmcp.Name, ids)
}

// CollectProfileConfiguration deletes the superseded configuration revisions of the vMCP's profiles once no agent's
// instance records that its configuration was copied from them. Only then is a revision unused: an instance keeps
// recording the revision it was last resolved from until the vMCP instance controller copies the current one.
//
// Instances are listed only while a revision waits to be deleted, and listing them watches them, so an instance that
// moves to the current revision rechecks the vMCP. A vMCP whose profiles carry no configuration never watches its
// instances.
func (h *Handler) CollectProfileConfiguration(req router.Request, _ router.Response) error {
	vmcp := req.Object.(*v1.VMCP)
	if !vmcp.DeletionTimestamp.IsZero() || h.profileConfiguration == nil {
		// The finalizer deletes every revision of a vMCP being deleted.
		return nil
	}
	if !slices.ContainsFunc(slices.Collect(maps.Values(vmcp.Status.ProfileConfigurationRevisions)), func(revision v1.VMCPProfileConfigurationRevision) bool {
		return len(revision.Superseded) > 0
	}) {
		return nil
	}

	var instances v1.VMCPInstanceList
	if err := req.List(&instances, &kclient.ListOptions{
		Namespace:     vmcp.Namespace,
		FieldSelector: fields.OneTermEqualSelector("spec.manifest.vmcpID", vmcp.Name),
	}); err != nil {
		return err
	}
	changed, err := h.profileConfiguration.CollectSuperseded(req.Ctx, vmcp, instances.Items)
	if changed {
		// Revisions already deleted are recorded as deleted even if a later one failed.
		if updateErr := req.Client.Status().Update(req.Ctx, vmcp); updateErr != nil {
			return errors.Join(err, updateErr)
		}
	}
	return err
}

func (h *Handler) deleteProfileData(req router.Request, vmcpID string, profileIDs []string) error {
	if h.profileData == nil {
		return nil
	}
	if err := h.profileData.DeleteProfileData(req.Ctx, vmcpID, profileIDs); err != nil {
		return fmt.Errorf("failed to delete the data of vMCP %q profiles %v: %w", vmcpID, profileIDs, err)
	}
	return nil
}

// agentSelectionCondition reports whether more than one profile names every agent. Such a vMCP predates the wildcard
// rule across profiles, so it still validates, but the profile an agent falls back to is ambiguous: an agent without
// its own profile cannot connect, and one with its own profile cannot connect while it needs setup from the fallback.
func agentSelectionCondition(manifest types.VMCPManifest) metav1.Condition {
	profiles := manifest.AgentWildcardProfiles()
	if len(profiles) < 2 {
		return metav1.Condition{
			Type:    types.VMCPConditionAgentSelectionAmbiguous,
			Status:  metav1.ConditionFalse,
			Reason:  "AtMostOneAgentWildcardProfile",
			Message: "At most one profile names every agent.",
		}
	}
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Name)
	}
	return metav1.Condition{
		Type:   types.VMCPConditionAgentSelectionAmbiguous,
		Status: metav1.ConditionTrue,
		Reason: "MultipleAgentWildcardProfiles",
		Message: fmt.Sprintf("Profiles %q all name every agent, with the agent wildcard or the selector wildcard (*), so agents without their own profile cannot connect, "+
			"and agents with their own profile cannot use components that need configuration or OAuth from the fallback. "+
			"Users are unaffected. Leave at most one such profile to let agents connect.", names),
	}
}

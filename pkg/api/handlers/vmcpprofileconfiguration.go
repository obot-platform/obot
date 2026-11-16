package handlers

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/obot-platform/obot/pkg/wait"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// profileConfigurationCollectionTimeout bounds how long a reset waits for the vMCP controller to delete the
	// revisions of the configuration it cleared.
	profileConfigurationCollectionTimeout = 15 * time.Second
)

// VMCPProfileConfigurationHandler serves the routes that set the configuration a vMCP profile gives agents, in place of
// the values a user enters on their own connection. They exist only for agents, so they answer 501 while
// authentication is disabled, as the agent admin API does.
//
// The routes are admin-only, which the authorizer enforces: they are under the administrators' /api/vmcps/ rule, no
// other role or group is granted them, and they are not on the agents' allowlist. They report key names and whether
// each is set, never a value. The status of a profile is read through the readiness API.
type VMCPProfileConfigurationHandler struct {
	authEnabled bool
	store       *vmcpconfig.ProfileConfigurationStore
	// collectionTimeout bounds how long a reset waits for its revisions to be collected.
	collectionTimeout time.Duration
}

func NewVMCPProfileConfigurationHandler(authEnabled bool, store *vmcpconfig.ProfileConfigurationStore) *VMCPProfileConfigurationHandler {
	return &VMCPProfileConfigurationHandler{
		authEnabled:       authEnabled,
		store:             store,
		collectionTimeout: profileConfigurationCollectionTimeout,
	}
}

// RegisterRoutes registers the routes with handle.
func (h *VMCPProfileConfigurationHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	handle("PUT /api/vmcps/{vmcp_id}/profiles/{profile_id}/configuration", h.Update)
	handle("POST /api/vmcps/{vmcp_id}/profiles/{profile_id}/configuration/reset", h.Reset)
}

// Update changes a profile's configuration (types.VMCPProfileConfigurationUpdate). Saving configuration that still
// lacks required values is allowed: agents that need them are refused until they are set.
func (h *VMCPProfileConfigurationHandler) Update(req api.Context) error {
	if err := agentIdentitiesAvailable(h.authEnabled); err != nil {
		return err
	}
	var update types.VMCPProfileConfigurationUpdate
	if err := req.Read(&update); err != nil {
		return types.NewErrBadRequest("failed to read profile configuration: %v", err)
	}
	profileID := req.PathValue("profile_id")
	previous := currentProfileConfigurationRevision(req, req.PathValue("vmcp_id"), profileID)
	vmcp, err := h.store.Update(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, profileID, update.Components)
	if err != nil {
		return err
	}
	// Saving what the profile already has publishes no revision, and so changes nothing to record.
	if current := vmcp.Status.ProfileConfigurationRevisions[profileID]; current.Name != previous.Name {
		var set, cleared []string
		for componentID, changes := range update.Components {
			for key, value := range changes {
				if value == nil || strings.TrimSpace(*value) == "" {
					cleared = append(cleared, vmcpconfig.ConfigurationKey(componentID, key))
				} else {
					set = append(set, vmcpconfig.ConfigurationKey(componentID, key))
				}
			}
		}
		slices.Sort(set)
		slices.Sort(cleared)
		// Key names only. The values are secrets, or may be, and are never read here beyond whether they are blank.
		recordAdminAudit(req, gclient.AdminAuditEventInput{
			Action:       gtypes.AdminAuditActionVMCPProfileConfigurationUpdate,
			ResourceKind: gtypes.AdminAuditResourceVMCPProfile,
			ResourceID:   profileID,
			ParentID:     vmcp.Name,
			Details:      map[string]any{"set": set, "cleared": cleared, "revision": current.Name, "keys": current.Keys},
		})
	}
	return req.Write(profileStatus(*vmcp, req.PathValue("profile_id")))
}

// Reset clears a profile's configuration. Agents stop being given its values as soon as it is published, but its
// revisions stay stored until no agent's instance uses them and the vMCP controller deletes them; until then the
// profile still counts as having data (vmcpconfig.ProfileDataStore.ProfilesWithData), so the guard against replacing
// it by name and the wildcard rule across profiles still apply. So a reset that cleared configuration waits, for a
// bounded time, until the controller has collected it. If that takes longer, the reset still succeeds, and the status
// it returns reports the revisions still to be collected (PendingCollection).
func (h *VMCPProfileConfigurationHandler) Reset(req api.Context) error {
	if err := agentIdentitiesAvailable(h.authEnabled); err != nil {
		return err
	}
	profileID := req.PathValue("profile_id")
	previous := currentProfileConfigurationRevision(req, req.PathValue("vmcp_id"), profileID)
	vmcp, cleared, err := h.store.Reset(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, profileID)
	if err != nil {
		return err
	}
	if cleared {
		recordAdminAudit(req, gclient.AdminAuditEventInput{
			Action:       gtypes.AdminAuditActionVMCPProfileConfigurationReset,
			ResourceKind: gtypes.AdminAuditResourceVMCPProfile,
			ResourceID:   profileID,
			ParentID:     vmcp.Name,
			Details:      map[string]any{"cleared": previous.Keys, "revision": previous.Name},
		})
		ctx, cancel := context.WithTimeout(req.Context(), h.collectionTimeout)
		defer cancel()
		collected, err := wait.For(ctx, req.Storage, vmcp, func(current *v1.VMCP) (bool, error) {
			_, pending := current.Status.ProfileConfigurationRevisions[profileID]
			return !pending, nil
		})
		if err == nil {
			vmcp = collected
		} else {
			slog.Debug("Profile configuration not yet collected after reset", "vmcp", vmcp.Name, "profile", profileID, "error", err)
		}
	}
	return req.Write(profileStatus(*vmcp, profileID))
}

// currentProfileConfigurationRevision returns the revision a profile's configuration is at, which is empty if the
// profile has none or the vMCP cannot be read: the change that follows reports a vMCP that is missing.
func currentProfileConfigurationRevision(req api.Context, vmcpID, profileID string) v1.VMCPProfileConfigurationRevision {
	var vmcp v1.VMCP
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: req.Namespace(), Name: vmcpID}, &vmcp); err != nil {
		return v1.VMCPProfileConfigurationRevision{}
	}
	return vmcp.Status.ProfileConfigurationRevisions[profileID]
}

// profileStatus reports the configuration keys a profile of vmcp sets, which the store has checked it has.
func profileStatus(vmcp v1.VMCP, profileID string) types.VMCPProfileStatus {
	profile, ok := vmcp.Spec.Manifest.Profile(profileID)
	if !ok {
		return types.VMCPProfileStatus{ProfileID: profileID, Components: []types.VMCPProfileComponentStatus{}}
	}
	return vmcpconfig.ProfileStatus(vmcp, profile)
}

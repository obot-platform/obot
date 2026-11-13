package handlers

import (
	"context"
	"log/slog"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
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
// the values a user enters on their own connection. They exist only for agents, so they answer 404 while agent
// identities are disabled, as the agent admin API does.
//
// The routes are admin-only, which the authorizer enforces: they are under the administrators' /api/vmcps/ rule, no
// other role or group is granted them, and they are not on the agents' allowlist. They report key names and whether
// each is set, never a value. The status of a profile is read through the readiness API.
type VMCPProfileConfigurationHandler struct {
	enabled     bool
	authEnabled bool
	store       *vmcpconfig.ProfileConfigurationStore
	// collectionTimeout bounds how long a reset waits for its revisions to be collected.
	collectionTimeout time.Duration
}

func NewVMCPProfileConfigurationHandler(enabled, authEnabled bool, store *vmcpconfig.ProfileConfigurationStore) *VMCPProfileConfigurationHandler {
	return &VMCPProfileConfigurationHandler{
		enabled:           enabled,
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
	if err := agentIdentitiesAvailable(h.enabled, h.authEnabled); err != nil {
		return err
	}
	var update types.VMCPProfileConfigurationUpdate
	if err := req.Read(&update); err != nil {
		return types.NewErrBadRequest("failed to read profile configuration: %v", err)
	}
	vmcp, err := h.store.Update(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, req.PathValue("profile_id"), update.Components)
	if err != nil {
		return err
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
	if err := agentIdentitiesAvailable(h.enabled, h.authEnabled); err != nil {
		return err
	}
	profileID := req.PathValue("profile_id")
	vmcp, cleared, err := h.store.Reset(req.Context(), req.Storage, kclient.ObjectKey{Namespace: req.Namespace(), Name: req.PathValue("vmcp_id")}, profileID)
	if err != nil {
		return err
	}
	if cleared {
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

// profileStatus reports the configuration keys a profile of vmcp sets, which the store has checked it has.
func profileStatus(vmcp v1.VMCP, profileID string) types.VMCPProfileStatus {
	profile, ok := vmcp.Spec.Manifest.Profile(profileID)
	if !ok {
		return types.VMCPProfileStatus{ProfileID: profileID, Components: []types.VMCPProfileComponentStatus{}}
	}
	return vmcpconfig.ProfileStatus(vmcp, profile)
}

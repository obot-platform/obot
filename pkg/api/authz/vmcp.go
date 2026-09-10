package authz

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

// CheckVMCPComponentAccess restricts personal vMCP components to accessible entries.
func CheckVMCPComponentAccess(ctx context.Context, u kuser.Info, ownerID string, entry *v1.MCPServerCatalogEntry, helper *accesscontrolrule.Helper, userInfo func(context.Context, uint) (kuser.Info, error)) error {
	if ownerID == "" {
		return nil
	}
	if ownerID != u.GetUID() {
		// Administrators can manage another user's personal vMCP, but its catalog
		// access must follow the owner, not the administrator performing the edit.
		id, err := strconv.ParseUint(ownerID, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid VMCP owner ID %q: %w", ownerID, err)
		}
		u, err = userInfo(ctx, uint(id))
		if err != nil {
			return fmt.Errorf("get VMCP owner %q: %w", ownerID, err)
		}
	}
	allowed, err := UserCanReadCatalogEntry(ctx, u, entry, helper)
	if err != nil {
		return err
	}
	if !allowed {
		return types.NewErrForbidden("access denied to catalog entry %q", entry.Name)
	}
	return nil
}

func IsVMCPAdministrator(u kuser.Info) bool {
	return slices.Contains(u.GetGroups(), types.GroupAdmin)
}

// CheckVMCPForceSingleUser permits only administrators to change the override.
// Personal owners can still edit other fields while preserving an existing value.
func CheckVMCPForceSingleUser(u kuser.Info, current, desired bool) error {
	if current != desired && !IsVMCPAdministrator(u) {
		return types.NewErrForbidden("only administrators can change forceSingleUser")
	}
	return nil
}

// ValidateVMCPToolSelection rejects explicit selections outside the profile union.
func ValidateVMCPToolSelection(u kuser.Info, vmcp *v1.VMCP, selection types.VMCPToolSet) error {
	if err := vmcp.Spec.Manifest.ValidateToolSet(selection); err != nil {
		return types.NewErrBadRequest("invalid tool selection: %v", err)
	}
	grant := vmcpaccess.AllowedTools(u, vmcp.Spec.Manifest.Profiles, nil)
	for _, tool := range selection.References() {
		if (vmcp.Spec.UserID != "" && vmcp.Spec.UserID != u.GetUID()) || !vmcpaccess.ToolGranted(grant, tool) {
			return types.NewErrBadRequest("tool %q on component %q is not granted by the VMCP profiles", tool.Name, tool.ComponentID)
		}
	}
	return nil
}

// UserCanReadVMCP applies the VMCP visibility model. A personal VMCP is only
// visible to its owner. A shared VMCP is visible to users matching at least
// one profile; catalog access is intentionally irrelevant.
func UserCanReadVMCP(u kuser.Info, vmcp *v1.VMCP) bool {
	if vmcp.Spec.UserID != "" {
		return vmcp.Spec.UserID == u.GetUID()
	}
	return userMatchesVMCPProfile(u, vmcp.Spec.Manifest.Profiles)
}

func UserCanConnectVMCP(u kuser.Info, vmcp *v1.VMCP) bool {
	return len(vmcp.Spec.Manifest.Components) > 0 && UserCanReadVMCP(u, vmcp)
}

func UserCanManageVMCP(u kuser.Info, vmcp *v1.VMCP) bool {
	return IsVMCPAdministrator(u) || (vmcp.Spec.UserID != "" && vmcp.Spec.UserID == u.GetUID())
}

func UserCanReadVMCPInstance(u kuser.Info, instance *v1.VMCPInstance, vmcp *v1.VMCP) bool {
	return IsVMCPAdministrator(u) || (instance.Spec.UserID == u.GetUID() && UserCanReadVMCP(u, vmcp))
}

func userMatchesVMCPProfile(u kuser.Info, profiles []types.VMCPProfile) bool {
	return len(vmcpaccess.MatchingProfiles(u, profiles)) > 0
}

func (a *Authorizer) checkVMCP(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.VMCPID == "" {
		return true, nil
	}

	var vmcp v1.VMCP
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.VMCPID), &vmcp); err != nil {
		return false, err
	}

	// Launch and per-user OAuth actions consume the vMCP; they do not manage its definition.
	if req.Method == http.MethodGet ||
		(req.Method == http.MethodPost && (req.URL.Path == "/api/vmcps/"+resources.VMCPID+"/launch" || req.URL.Path == "/api/vmcps/"+resources.VMCPID+"/check-oauth")) ||
		(req.Method == http.MethodDelete && req.URL.Path == "/api/vmcps/"+resources.VMCPID+"/oauth") {
		if UserCanReadVMCP(u, &vmcp) {
			resources.Authorizated.VMCP = &vmcp
			return true, nil
		}
		return false, nil
	}
	if UserCanManageVMCP(u, &vmcp) {
		resources.Authorizated.VMCP = &vmcp
		return true, nil
	}
	return false, nil
}

func (a *Authorizer) checkVMCPInstance(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.VMCPInstanceID == "" {
		return true, nil
	}

	var instance v1.VMCPInstance
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.VMCPInstanceID), &instance); err != nil {
		return false, err
	}

	if IsVMCPAdministrator(u) {
		resources.Authorizated.VMCPInstance = &instance
		return true, nil
	}
	if instance.Spec.UserID != u.GetUID() {
		return false, nil
	}

	var vmcp v1.VMCP
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, instance.Spec.Manifest.VMCPID), &vmcp); err != nil {
		return false, err
	}
	if !UserCanReadVMCPInstance(u, &instance, &vmcp) {
		return false, nil
	}

	resources.Authorizated.VMCPInstance = &instance
	return true, nil
}

func (a *Authorizer) checkVMCPComponent(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.VMCPComponentMCPID == "" {
		return true, nil
	}

	vmcp, instance := resources.Authorizated.VMCP, resources.Authorizated.VMCPInstance
	if vmcp == nil || instance == nil {
		return false, nil
	}

	var component v1.MCPServer
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.VMCPComponentMCPID), &component); err != nil {
		return false, err
	}
	belongs := component.Spec.VMCPInstanceID == instance.Name && component.Spec.UserID == u.GetUID()
	if vmcpaccess.IsMultiUser(vmcp.Spec.Manifest) {
		belongs = component.Spec.VMCPID == vmcp.Name && component.Spec.VMCPInstanceID == ""
	}
	return belongs && slices.ContainsFunc(vmcpaccess.ComponentsForInstance(*vmcp, *instance), func(candidate types.VMCPComponent) bool {
		return candidate.ID == component.Spec.VMCPComponentID
	}), nil
}

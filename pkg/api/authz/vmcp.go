package authz

import (
	"net/http"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

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

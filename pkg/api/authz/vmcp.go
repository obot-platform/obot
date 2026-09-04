package authz

import (
	"net/http"
	"slices"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

func IsVMCPAdministrator(u kuser.Info) bool {
	return slices.Contains(u.GetGroups(), types.GroupAdmin)
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
	groups := make(map[string]struct{}, len(u.GetGroups()))
	for _, group := range u.GetGroups() {
		groups[group] = struct{}{}
	}
	for _, extraName := range []string{"obot_groups", "auth_provider_groups"} {
		for _, group := range u.GetExtra()[extraName] {
			groups[group] = struct{}{}
		}
	}

	for _, profile := range profiles {
		for _, subject := range profile.Subjects {
			switch subject.Type {
			case types.SubjectTypeSelector:
				if subject.ID == "*" {
					return true
				}
			case types.SubjectTypeUser:
				if subject.ID == u.GetUID() {
					return true
				}
			case types.SubjectTypeGroup:
				if _, ok := groups[subject.ID]; ok {
					return true
				}
			}
		}
	}
	return false
}

func (a *Authorizer) checkVMCP(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.VMCPID == "" {
		return true, nil
	}

	var vmcp v1.VMCP
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.VMCPID), &vmcp); err != nil {
		return false, err
	}

	if req.Method == http.MethodGet {
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

package authz

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	vmcpaccess "github.com/obot-platform/obot/pkg/vmcp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
)

var (
	// delegatedSetupInstanceRoutes are the routes that name one vMCP instance (refusesDelegatedSetupInstance).
	delegatedSetupInstanceRoutes = newPathMatcher(
		"/api/vmcp-instances/{vmcp_instance_id}",
		"/api/vmcp-instances/{vmcp_instance_id}/{rest...}",
	)
)

// CheckVMCPComponentAccess restricts personal vMCP components to accessible entries.
func CheckVMCPComponentAccess(ctx context.Context, u kuser.Info, ownerID string, entry *v1.MCPServerCatalogEntry, helper *accesscontrolrule.Helper, userInfo func(context.Context, uint) (kuser.Info, error)) error {
	if ownerID == "" {
		return nil
	}
	if ownerID != u.GetUID() {
		// Administrators can manage another user's personal vMCP, but its catalog
		// access must follow the owner, not the administrator performing the edit.
		owner, err := principal.Parse(ownerID)
		if err != nil {
			return fmt.Errorf("invalid VMCP owner ID %q: %w", ownerID, err)
		}
		userID, ok := owner.UserID()
		if !ok {
			return fmt.Errorf("invalid VMCP owner ID %q: personal VMCPs are owned by users", ownerID)
		}
		u, err = userInfo(ctx, userID)
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

// ValidateVMCPToolSelection rejects explicit selections outside the profile union.
func ValidateVMCPToolSelection(u kuser.Info, vmcp *v1.VMCP, selection map[string]types.VMCPComponentSet) error {
	if err := vmcp.Spec.Manifest.ValidateComponents(selection); err != nil {
		return types.NewErrBadRequest("invalid tool selection: %v", err)
	}
	if vmcp.Spec.UserID != "" && vmcp.Spec.UserID == u.GetUID() {
		return nil
	}
	grant := vmcpaccess.AllowedTools(u, *vmcp, nil)
	for _, tool := range types.ComponentToolReferences(selection) {
		if vmcp.Spec.UserID != "" || !vmcpaccess.ToolGranted(grant, tool) {
			return types.NewErrBadRequest("tool %q on component %q is not granted by the VMCP profiles", tool.Name, tool.ComponentID)
		}
	}
	return nil
}

// UserCanReadVMCP applies the VMCP visibility model. A personal VMCP is only
// visible to its owner, who is always a user: an agent never owns or reads one.
// A shared VMCP is visible to users and agents matching at least one profile;
// catalog access is intentionally irrelevant. An agent acting on behalf of a user
// sees it as the agent does: when at least one profile names the agent
// (vmcpaccess.CallerProfiles).
func UserCanReadVMCP(u kuser.Info, vmcp *v1.VMCP) bool {
	if vmcp.Spec.UserID != "" {
		return !principal.ClaimsAgent(u) && vmcp.Spec.UserID == u.GetUID()
	}
	return len(vmcpaccess.CallerProfiles(u, *vmcp)) > 0
}

func UserCanConnectVMCP(u kuser.Info, vmcp *v1.VMCP) bool {
	return len(vmcp.Spec.Manifest.Components) > 0 && UserCanReadVMCP(u, vmcp)
}

func UserCanManageVMCP(u kuser.Info, vmcp *v1.VMCP) bool {
	return IsVMCPAdministrator(u) || (vmcp.Spec.UserID != "" && vmcp.Spec.UserID == u.GetUID())
}

// UserCanReadVMCPInstance reports whether u may read instance, an instance of vmcp. Nobody reads the instance of what a
// user set up through an agent this way, an administrator included: only that user, through the setup routes.
func UserCanReadVMCPInstance(u kuser.Info, instance *v1.VMCPInstance, vmcp *v1.VMCP) bool {
	if principal.IsDelegatedOwner(instance.Spec.UserID) {
		return false
	}
	return IsVMCPAdministrator(u) || (instance.Spec.UserID == u.GetUID() && UserCanReadVMCP(u, vmcp))
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
		(req.Method == http.MethodDelete && (req.URL.Path == "/api/vmcps/"+resources.VMCPID+"/oauth" || isVMCPComponentOAuthPath(req.URL.Path, resources.VMCPID))) {
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

// refusesDelegatedSetupInstance reports whether req names, on a vMCP instance route, the instance of what a user set up
// through an agent (principal.DelegatedOwner), which no instance route reaches, for anyone. Only that user reaches it,
// through the setup routes of the agent's request or delegation, which never return a value or a token. This is
// checked ahead of every rule, because the administrators' rules grant every instance route without looking at the
// instance, so that no administrator reads, configures, reveals or deletes it. A request whose instance cannot be read
// is refused.
func (a *Authorizer) refusesDelegatedSetupInstance(req *http.Request) bool {
	vars, ok := delegatedSetupInstanceRoutes.Match(req)
	if !ok {
		return false
	}
	var instance v1.VMCPInstance
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, vars("vmcp_instance_id")), &instance); apierrors.IsNotFound(err) {
		return false
	} else if err != nil {
		return true
	}
	return principal.IsDelegatedOwner(instance.Spec.UserID)
}

// isVMCPComponentOAuthPath reports whether path is the per-user OAuth reset
// for one component of the vMCP: /api/vmcps/{vmcp_id}/components/{component_id}/oauth.
func isVMCPComponentOAuthPath(path, vmcpID string) bool {
	componentID, ok := strings.CutPrefix(path, "/api/vmcps/"+vmcpID+"/components/")
	if !ok {
		return false
	}
	componentID, ok = strings.CutSuffix(componentID, "/oauth")
	return ok && componentID != "" && !strings.Contains(componentID, "/")
}

func (a *Authorizer) checkVMCPInstance(req *http.Request, resources *Resources, u User) (bool, error) {
	if resources.VMCPInstanceID == "" {
		return true, nil
	}

	var instance v1.VMCPInstance
	if err := a.get(req.Context(), router.Key(system.DefaultNamespace, resources.VMCPInstanceID), &instance); err != nil {
		return false, err
	}

	if req.Method == http.MethodPost && req.URL.Path == "/api/vmcp-instances/"+resources.VMCPInstanceID+"/tester/chat" {
		return userCanConnectToMCP(req.Context(), a.uncached, a.acrHelper, u, resources.VMCPInstanceID, resources)
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
	// Component servers are owned by the encoded principal of their instance's owner, so compare encodings on both
	// sides: an agent never matches a user's server with its numeric ID, or the other way around.
	owner, err := principal.CallerOwner(u)
	if err != nil {
		return false, nil
	}
	return slices.ContainsFunc(vmcpaccess.ComponentsForInstance(*vmcp, *instance), func(candidate types.VMCPComponent) bool {
		if candidate.ID != component.Spec.VMCPComponentID {
			return false
		}
		if vmcpaccess.IsMultiUser(candidate) {
			return component.Spec.VMCPID == vmcp.Name && component.Spec.VMCPInstanceID == ""
		}
		return component.Spec.VMCPInstanceID == instance.Name && component.Spec.UserID == owner
	}), nil
}

package handlers

import (
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/agentprofile"
	"github.com/obot-platform/obot/pkg/api"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gtypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/mcp"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/subjects"
	"github.com/obot-platform/obot/pkg/system"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	// agentDelegationManagementPath is the UI page at which a user manages what they set up through one of their
	// delegations. The callback of an upstream account connected there returns the browser to it.
	agentDelegationManagementPath = "/agents/delegations/"
)

// DelegatedSetupOAuthStarter starts the authorization of a user's upstream account through an agent. The MCP OAuth
// handler factory implements it; this interface breaks an import cycle.
type DelegatedSetupOAuthStarter interface {
	StartDelegatedSetupOAuth(req api.Context, vmcp *v1.VMCP, pair *v1.VMCPInstance, componentID, returnPath string) (types.AgentDelegationSetupOAuth, error)
}

// AgentDelegationSetupHandler serves what a user sets up for vMCPs through an agent (adr/2026-11-20-agent-on-behalf-of-
// users.md, "Approval and tokens", step 2, and "Revocation and lifecycle"): the values they enter and the upstream
// accounts they connect, owned by the pair of the agent and the user (principal.DelegatedOwner), apart from the user's
// own setup, the agent's shared setup and every other user's. It is the user setup code (a vMCP instance, its
// configuration credential validated by vmcpconfig.ValidateAndEncodeUserConfiguration, and the user component OAuth
// flow) with the pair as the owner.
//
// Two sets of routes share it, and one set of rules:
//
//   - /api/agent-delegation-requests/{request_id}/setup…, while the signed-in user approves a pending request. The
//     pair is the request's agent and the user.
//   - /api/me/agent-delegations/{delegation_id}/setup…, afterwards. The pair is the delegation's agent and user.
//
// Who may call them is the authorizer's (pkg/api/authz): the approval routes are a signed-in user's browser session's,
// from Obot's own pages, for a request that is pending and that they may approve; the /api/me routes are the
// delegation's own user's. The handler reads the pair from what the authorizer admitted, and checks only what the
// request's body and path name: that the agent is granted the vMCP and the component, and may act for the user.
//
// Responses report values as set or unset, and upstream accounts as connected or not, and never carry a value or a
// token. No administrator route returns them.
type AgentDelegationSetupHandler struct {
	grants *vmcpconfig.PreconfiguredOAuthStore
	oauth  DelegatedSetupOAuthStarter
}

// setupPair is the pair of an agent and a user whose setup a request names.
type setupPair struct {
	owner principal.DelegatedOwner
	// matched reports that a profile of the agent matches the user now. Without one, the agent's tokens for the user
	// reach nothing, so the user sets up nothing through it.
	matched bool
	// returnPath is the page the callback of an upstream account returns the browser to.
	returnPath string
}

func NewAgentDelegationSetupHandler(grants *vmcpconfig.PreconfiguredOAuthStore, oauth DelegatedSetupOAuthStarter) *AgentDelegationSetupHandler {
	return &AgentDelegationSetupHandler{
		grants: grants,
		oauth:  oauth,
	}
}

// RegisterRoutes registers the routes with handle. The vMCP's path value is setup_vmcp_id, not vmcp_id, so that the
// authorizer does not take it for a vMCP the caller manages or reads themselves: the user reaches it through the
// agent, whatever they may reach on their own.
func (h *AgentDelegationSetupHandler) RegisterRoutes(handle func(pattern string, f api.HandlerFunc)) {
	for _, prefix := range []string{"/api/agent-delegation-requests/{request_id}", "/api/me/agent-delegations/{delegation_id}"} {
		handle("GET "+prefix+"/setup", h.Get)
		handle("PUT "+prefix+"/setup/vmcps/{setup_vmcp_id}/configuration", h.Configure)
		handle("POST "+prefix+"/setup/vmcps/{setup_vmcp_id}/components/{component_id}/oauth", h.StartOAuth)
		handle("DELETE "+prefix+"/setup/vmcps/{setup_vmcp_id}/components/{component_id}/oauth", h.DisconnectOAuth)
	}
}

// profiles returns the profiles of vmcp that grant the agent acting on behalf of the user, as they do for the agent's
// tokens for the user (vmcpconfig.DelegatedProfiles).
func (p setupPair) profiles(vmcp v1.VMCP) []types.VMCPProfile {
	if !p.matched {
		return nil
	}
	return vmcpconfig.DelegatedProfiles(p.owner.AgentID, vmcp)
}

// Get returns the setup of every vMCP that the agent may use for the user (types.AgentDelegationSetup).
func (h *AgentDelegationSetupHandler) Get(req api.Context) error {
	pair, err := h.pair(req)
	if err != nil {
		return err
	}
	vmcps, err := grantedVMCPs(req, pair)
	if err != nil {
		return err
	}
	setup := types.AgentDelegationSetup{VMCPs: make([]types.AgentDelegationSetupVMCP, 0, len(vmcps))}
	for i := range vmcps {
		selection, err := h.selection(req, pair, vmcps[i])
		if err != nil {
			return err
		}
		vmcpSetup, err := h.vmcpSetup(req, pair, &vmcps[i], selection, nil)
		if err != nil {
			return err
		}
		setup.VMCPs = append(setup.VMCPs, vmcpSetup)
	}
	return req.Write(setup)
}

// Configure sets and clears the values the user enters for a vMCP through the agent (types.AgentDelegationSetupConfiguration),
// with the validation of a user's own configuration, and returns the vMCP's setup. Keys that are left out are left as
// they are.
func (h *AgentDelegationSetupHandler) Configure(req api.Context) error {
	pair, err := h.pair(req)
	if err != nil {
		return err
	}
	vmcp, selection, err := h.grantedVMCP(req, pair)
	if err != nil {
		return err
	}
	var body types.AgentDelegationSetupConfiguration
	if err := req.Read(&body); err != nil {
		return types.NewErrBadRequest("failed to read the configuration: %v", err)
	}

	set := types.VMCPConfiguration{Components: map[string]map[string]string{}}
	var cleared []string
	for componentID, values := range body.Components {
		index := slices.IndexFunc(selection.Components, func(component types.VMCPComponent) bool { return component.ID == componentID })
		if index < 0 {
			return types.NewErrBadRequest("the agent can't use component %q of vMCP %q for you", componentID, vmcp.Name)
		}
		allowed := vmcpconfig.UserConfiguration(selection.Components[index])
		for key, value := range values {
			if value != nil {
				if set.Components[componentID] == nil {
					set.Components[componentID] = map[string]string{}
				}
				set.Components[componentID][key] = *value
				continue
			}
			if !slices.ContainsFunc(allowed, func(field types.MCPConfig) bool { return field.Key == key }) {
				return types.NewErrBadRequest("configuration %q for component %q is not user-allowed", key, componentID)
			}
			cleared = append(cleared, vmcpconfig.ConfigurationKey(componentID, key))
		}
	}
	// The values are validated as a user's own are, against the components as the pair's instance has them.
	manifest := vmcp.Spec.Manifest
	manifest.Components = selection.Components
	encoded, err := vmcpconfig.ValidateAndEncodeUserConfiguration(manifest, set)
	if err != nil {
		return types.NewErrBadRequest("invalid configuration: %v", err)
	}

	instance, err := ensurePairInstance(req, vmcp, pair.owner)
	if err != nil {
		return err
	}
	credentialContext := vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)
	values, err := revealPairConfiguration(req, instance)
	if err != nil {
		return err
	}
	for componentID, keys := range set.Components {
		for key := range keys {
			// An empty value is no value, as for a user's own configuration: setting it clears it.
			cleared = append(cleared, vmcpconfig.ConfigurationKey(componentID, key))
		}
	}
	for _, key := range cleared {
		delete(values, key)
	}
	maps.Copy(values, encoded)
	// A URL template value that changes the template's host is refused now, as VMCPInstanceHandler.Configure refuses
	// it for a user's own instance, rather than stored for the connection to fail later. The check sees every value the
	// pair's instance will hold, not only those set here, since this request updates the configuration rather than
	// replacing it. The vMCP's fixed values are not read, so a template that uses one resolves as missing and is skipped
	// here; the vMCP controller and the runtime check it with every value before anything connects.
	for _, component := range selection.Components {
		remote := component.CatalogEntry.Manifest.RemoteConfig
		if remote == nil || remote.URLTemplate == "" {
			continue
		}
		if _, _, err := mcp.ResolveURLTemplate(remote.URLTemplate, vmcpconfig.ComponentConfig(component), vmcpconfig.ComponentServerConfiguration(component, nil, values), nil); err != nil {
			return types.NewErrBadRequest("invalid configuration for component %q: %v", component.ID, err)
		}
	}

	if len(values) == 0 {
		if _, err := req.GatewayClient.DeleteCredential(req.Context(), credentialContext, vmcpconfig.ConfigurationCredentialName()); err != nil {
			return fmt.Errorf("failed to clear the configuration: %w", err)
		}
	} else if err := req.GatewayClient.UpsertCredential(req.Context(), gtypes.Credential{
		Context: credentialContext,
		Name:    vmcpconfig.ConfigurationCredentialName(),
		Secrets: values,
	}); err != nil {
		return fmt.Errorf("failed to store the configuration: %w", err)
	}
	// The controllers copy the values to the pair's component servers and connections, as for a user's own instance.
	if err := syncInstanceConfigurationHash(req, instance, values); err != nil {
		return err
	}

	vmcpSetup, err := h.vmcpSetup(req, pair, vmcp, selection, values)
	if err != nil {
		return err
	}
	return req.Write(vmcpSetup)
}

// StartOAuth starts the authorization of the user's upstream account for a component through the agent, and returns
// the URL the user opens (types.AgentDelegationSetupOAuth). Only the user who started it can complete it, and the
// callback returns their browser to the page that started it.
func (h *AgentDelegationSetupHandler) StartOAuth(req api.Context) error {
	pair, err := h.pair(req)
	if err != nil {
		return err
	}
	vmcp, selection, err := h.grantedVMCP(req, pair)
	if err != nil {
		return err
	}
	componentID := req.PathValue("component_id")
	index := slices.IndexFunc(selection.Components, func(component types.VMCPComponent) bool { return component.ID == componentID })
	switch {
	case index < 0:
		return types.NewErrNotFound("the agent can't use component %q of vMCP %q for you", componentID, vmcp.Name)
	case !vmcpconfig.ComponentUsesOAuth(selection.Components[index]):
		return types.NewErrBadRequest("component %q is not a remote server, so it does not use OAuth", componentID)
	case selection.Grants[index].Held():
		return types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("the vMCP's preconfigured OAuth covers component %q for everyone, so it is used in place of your account; an administrator manages it", componentID))
	}

	instance, err := ensurePairInstance(req, vmcp, pair.owner)
	if err != nil {
		return err
	}
	authorization, err := h.oauth.StartDelegatedSetupOAuth(req, vmcp, instance, componentID, pair.returnPath)
	if err != nil {
		return err
	}
	return req.Write(authorization)
}

// DisconnectOAuth deletes the user's upstream account for a component through the agent, and returns the vMCP's setup.
// The agent then needs the user's approval again for the component, if it needs OAuth.
func (h *AgentDelegationSetupHandler) DisconnectOAuth(req api.Context) error {
	pair, err := h.pair(req)
	if err != nil {
		return err
	}
	vmcp, selection, err := h.grantedVMCP(req, pair)
	if err != nil {
		return err
	}
	componentID := req.PathValue("component_id")
	index := slices.IndexFunc(selection.Components, func(component types.VMCPComponent) bool { return component.ID == componentID })
	if index < 0 {
		return types.NewErrNotFound("the agent can't use component %q of vMCP %q for you", componentID, vmcp.Name)
	}
	instance, err := vmcpconfig.FindInstance(req.Context(), req.Storage, vmcp.Namespace, vmcp.Name, pair.owner.String())
	if err != nil {
		return err
	}
	if instance != nil && vmcpconfig.ComponentUsesOAuth(selection.Components[index]) {
		if err := req.GatewayClient.DeleteMCPOAuthTokens(req.Context(), pair.owner.String(), vmcpconfig.PairConnectID(instance.Name, selection.Components[index])); err != nil {
			return fmt.Errorf("failed to disconnect the upstream account: %w", err)
		}
	}
	vmcpSetup, err := h.vmcpSetup(req, pair, vmcp, selection, nil)
	if err != nil {
		return err
	}
	return req.Write(vmcpSetup)
}

// pair returns the pair whose setup the request names: the agent of the pending request the signed-in user approves,
// or of the user's delegation, and the user. The authorizer admitted the caller to the request or the delegation
// (authz authorizeAgentDelegationRequestSetup and checkAgentDelegation), so pair only reads them.
func (h *AgentDelegationSetupHandler) pair(req api.Context) (setupPair, error) {
	var (
		agent      *gtypes.Agent
		user       subjects.Caller
		matched    bool
		returnPath string
	)
	if requestID := req.PathValue("request_id"); requestID != "" {
		approver, err := delegationApprover(req)
		if err != nil {
			return setupPair{}, err
		}
		approval, err := req.GatewayClient.GetAgentDelegationApproval(req.Context(), requestID, approver)
		if err != nil {
			return setupPair{}, agentDelegationApprovalError(err)
		}
		// Approval is only for a user whom a profile of the agent matches.
		agent, user, matched = approval.Agent, approver, true
		returnPath = agentApprovalPath + url.PathEscape(approval.Request.ID)
	} else {
		// The authorizer admitted only the delegation's own user (authz checkAgentDelegation).
		caller, ok := subjects.CallerFromUserInfo(req.User)
		if !ok {
			return setupPair{}, types.NewErrHTTP(http.StatusInternalServerError, "the caller is not a user")
		}
		delegation, err := req.GatewayClient.GetAgentDelegation(req.Context(), req.PathValue("delegation_id"))
		if errors.Is(err, gclient.ErrAgentDelegationNotFound) {
			return setupPair{}, types.NewErrNotFound("agent delegation not found")
		} else if err != nil {
			return setupPair{}, err
		}
		if agent, err = req.GatewayClient.GetAgent(req.Context(), delegation.AgentID); errors.Is(err, gorm.ErrRecordNotFound) {
			return setupPair{}, types.NewErrNotFound("agent delegation not found")
		} else if err != nil {
			return setupPair{}, err
		}
		// Whether the user may set up anything through the agent follows whether a profile of the agent matches them
		// now, with their current groups, as the agent's tokens do.
		user, matched = caller, agentprofile.Matches(agent.Profiles, caller)
		returnPath = agentDelegationManagementPath + url.PathEscape(delegation.ID)
	}

	userID, ok := user.Principal.UserID()
	if !ok {
		return setupPair{}, types.NewErrHTTP(http.StatusInternalServerError, "the caller is not a user")
	}
	return setupPair{
		owner:      principal.DelegatedOwner{AgentID: agent.ID, UserID: userID},
		matched:    matched,
		returnPath: returnPath,
	}, nil
}

// grantedVMCP returns the vMCP the request's path names, if the agent may use it for the user, and what the agent is
// granted of it.
func (h *AgentDelegationSetupHandler) grantedVMCP(req api.Context, pair setupPair) (*v1.VMCP, vmcpconfig.DelegatedSelection, error) {
	vmcpID := req.PathValue("setup_vmcp_id")
	var vmcp v1.VMCP
	if err := req.Get(&vmcp, vmcpID); apierrors.IsNotFound(err) || err == nil && !vmcpGranted(pair, vmcp) {
		return nil, vmcpconfig.DelegatedSelection{}, types.NewErrNotFound("the agent can't use vMCP %q for you", vmcpID)
	} else if err != nil {
		return nil, vmcpconfig.DelegatedSelection{}, err
	}
	selection, err := h.selection(req, pair, vmcp)
	if err != nil {
		return nil, vmcpconfig.DelegatedSelection{}, err
	}
	return &vmcp, selection, nil
}

// selection returns what the agent acting on behalf of the user is granted of vmcp, as a connection of the agent on
// behalf of the user selects it: from the components of the agent's own instance, which it connects through.
func (h *AgentDelegationSetupHandler) selection(req api.Context, pair setupPair, vmcp v1.VMCP) (vmcpconfig.DelegatedSelection, error) {
	agentInstance, err := vmcpconfig.FindInstance(req.Context(), req.Storage, vmcp.Namespace, vmcp.Name, principal.Agent(pair.owner.AgentID).String())
	if err != nil {
		return vmcpconfig.DelegatedSelection{}, err
	}
	return vmcpconfig.SelectDelegated(req.Context(), h.grants, vmcp, agentInstance, pair.profiles(vmcp))
}

// vmcpSetup returns the setup of vmcp through the agent: what the agent is granted of it (selection), with the
// pair's values, read now when values is nil, and upstream accounts. The values are only compared, and never returned.
func (h *AgentDelegationSetupHandler) vmcpSetup(req api.Context, pair setupPair, vmcp *v1.VMCP, selection vmcpconfig.DelegatedSelection, values map[string]string) (types.AgentDelegationSetupVMCP, error) {
	instance, err := vmcpconfig.FindInstance(req.Context(), req.Storage, vmcp.Namespace, vmcp.Name, pair.owner.String())
	if err != nil {
		return types.AgentDelegationSetupVMCP{}, err
	}
	if values == nil {
		if values, err = revealPairConfiguration(req, instance); err != nil {
			return types.AgentDelegationSetupVMCP{}, err
		}
	}
	connected, err := h.grants.PairConnections(req.Context(), instance, selection.Components)
	if err != nil {
		return types.AgentDelegationSetupVMCP{}, err
	}
	readiness := selection.Readiness(vmcpconfig.PairSetup{Instance: instance, Values: values, Connected: connected})

	setup := types.AgentDelegationSetupVMCP{
		VMCPID:      vmcp.Name,
		DisplayName: vmcp.Spec.Manifest.DisplayName,
		Ready:       readiness.Ready(),
		Components:  make([]types.AgentDelegationSetupComponent, 0, len(selection.Components)),
	}
	for i, component := range selection.Components {
		setup.Components = append(setup.Components, setupComponent(component, selection.Grants[i], connected[component.ID], values, readiness))
	}
	return setup, nil
}

// setupComponent reports the setup of component through the agent, where grant is the vMCP's preconfigured OAuth for it,
// connected reports that the user connected an upstream account for it, values are the pair's values, and readiness
// is the agent's readiness on behalf of the user. It never carries a value.
func setupComponent(component types.VMCPComponent, grant vmcpconfig.ComponentOAuthGrant, connected bool, values map[string]string, readiness vmcpconfig.Readiness) types.AgentDelegationSetupComponent {
	result := types.AgentDelegationSetupComponent{
		ComponentID:   component.ID,
		Name:          component.Name,
		Configuration: []types.AgentDelegationSetupField{},
		OAuth:         types.AgentDelegationSetupOAuthNotRequired,
	}
	for _, field := range vmcpconfig.UserConfiguration(component) {
		result.Configuration = append(result.Configuration, types.AgentDelegationSetupField{
			Key:         field.Key,
			Name:        field.Name,
			Description: field.Description,
			Sensitive:   field.Sensitive,
			Required:    field.Required,
			Usage:       field.Usage,
			Options:     field.Options,
			Set:         values[vmcpconfig.ConfigurationKey(component.ID, field.Key)] != "",
		})
	}
	switch {
	case !vmcpconfig.ComponentUsesOAuth(component):
	case grant.Held():
		result.OAuth = types.AgentDelegationSetupOAuthPreconfigured
	case connected:
		result.OAuth = types.AgentDelegationSetupOAuthConnected
	case grant.Required:
		result.OAuth = types.AgentDelegationSetupOAuthNeedsAuthorization
	}
	for _, reason := range readiness.Reasons {
		if reason.ComponentID == component.ID {
			result.Reasons = append(result.Reasons, reason)
		}
	}
	result.Ready = len(result.Reasons) == 0

	if len(result.Configuration) == 0 && (result.OAuth == types.AgentDelegationSetupOAuthNotRequired || result.OAuth == types.AgentDelegationSetupOAuthPreconfigured) {
		result.Covered = true
		switch {
		case result.OAuth == types.AgentDelegationSetupOAuthPreconfigured:
			result.CoveredBy = types.AgentDelegationSetupCoveredPreconfiguredOAuth
		case slices.ContainsFunc(vmcpconfig.ComponentConfig(component), func(field types.MCPConfig) bool { return !field.UserAllowed }):
			result.CoveredBy = types.AgentDelegationSetupCoveredFixed
		default:
			result.CoveredBy = types.AgentDelegationSetupCoveredNothing
		}
	}
	return result
}

// grantedVMCPs returns the shared vMCPs that the agent may use for the user, by ID: those of which the agent acting on
// behalf of the user is granted at least one profile (vmcpconfig.DelegatedProfiles).
func grantedVMCPs(req api.Context, pair setupPair) ([]v1.VMCP, error) {
	vmcps, err := listSharedVMCPs(req)
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(vmcps, func(vmcp v1.VMCP) bool {
		return !vmcpGranted(pair, vmcp)
	}), nil
}

// vmcpGranted reports whether the agent may use vmcp for the user of pair, as it does for a connection
// of the agent on behalf of the user (authz.UserCanConnectVMCP).
func vmcpGranted(pair setupPair, vmcp v1.VMCP) bool {
	return vmcp.DeletionTimestamp.IsZero() && vmcp.Spec.UserID == "" && len(vmcp.Spec.Manifest.Components) > 0 && len(pair.profiles(vmcp)) > 0
}

// ensurePairInstance returns the instance of what the user set up for vmcp through the agent, owned by owner, and
// creates it if there is none yet.
func ensurePairInstance(req api.Context, vmcp *v1.VMCP, owner principal.DelegatedOwner) (*v1.VMCPInstance, error) {
	instance, err := vmcpconfig.FindInstance(req.Context(), req.Storage, vmcp.Namespace, vmcp.Name, owner.String())
	if err != nil || instance != nil {
		return instance, err
	}
	instance = &v1.VMCPInstance{
		Finalizers:   []string{v1.VMCPInstanceFinalizer},
		GenerateName: system.VMCPInstancePrefix,
		Namespace:    vmcp.Namespace,
		Spec: v1.VMCPInstanceSpec{
			Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
			UserID:   owner.String(),
		},
	}
	if err := req.Create(instance); err != nil {
		return nil, fmt.Errorf("failed to create the setup of vMCP %q through the agent: %w", vmcp.Name, err)
	}
	return instance, nil
}

// revealPairConfiguration returns the values in the configuration credential of instance, the pair's instance, or none
// when it is nil or has none. They never leave the server.
func revealPairConfiguration(req api.Context, instance *v1.VMCPInstance) (map[string]string, error) {
	values := map[string]string{}
	if instance == nil {
		return values, nil
	}
	credential, err := req.GatewayClient.RevealCredential(req.Context(), []string{vmcpconfig.InstanceConfigurationCredentialContext(instance.Name)}, vmcpconfig.ConfigurationCredentialName())
	if _, notFound := errors.AsType[gclient.CredentialNotFoundError](err); notFound {
		return values, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to read the configuration: %w", err)
	}
	maps.Copy(values, credential.Secrets)
	return values, nil
}

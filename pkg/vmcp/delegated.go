package vmcp

import (
	"context"
	"fmt"
	"slices"

	"github.com/obot-platform/nah/pkg/name"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
)

// What a user sets up for a vMCP through an agent (adr/2026-11-20-agent-on-behalf-of-users.md, "Identity and
// storage") is owned by the pair of the agent and the user (principal.DelegatedOwner), so that it is never the user's
// own setup, the agent's shared setup, or another user's. It is held as a user's own setup is, by the same code, with
// the pair as the owner:
//
//   - The pair's instance of the vMCP: a VMCPInstance whose UserID is the pair. Nobody connects through it; an agent
//     acting on behalf of the user connects through the agent's own instance, and uses the pair's component servers and
//     connections for the components that need the user (UsesPairSetup).
//   - Its configuration credential (InstanceConfigurationCredentialContext), with the values the user entered through
//     the agent, which the controllers copy to the pair's component servers and connections, as they do a user's.
//   - The upstream OAuth tokens of the pair's component connections (PairConnectID), keyed by the pair, as a user's
//     are keyed by the user.
//
// A pair's instance carries the components that may need the user (PairInstanceComponents), whatever the vMCP profiles
// grant, so that setup that no longer applies, because no profile of the vMCP names the agent or no agent profile
// matches the user any more, stays until the delegation is revoked or the vMCP, the agent or the user is deleted.

// PairSetup is what the pair of an agent and a user has set up for a vMCP, as the connect path and the setup API read
// it. It never holds a value or a token.
type PairSetup struct {
	// Instance is the pair's instance of the vMCP, or nil when it has none, after the VMCPInstance controller recorded
	// which of its required values are missing (MissingRequiredConfiguration).
	Instance *v1.VMCPInstance
	// Values are the values in the pair's configuration credential, by ConfigurationKey, when the caller read them,
	// so that what was just written counts before the controller records it. They are only compared with what is
	// required, and never leave the server. Without them, the missing values recorded on Instance are used.
	Values map[string]string
	// Connected reports, by component ID, whether the pair holds an upstream OAuth token for the component.
	Connected map[string]bool
}

// DelegatedSelection is the components of a vMCP that an agent acting on behalf of a user is granted through the vMCP
// profiles that name the agent (EnabledComponents), with the vMCP's preconfigured OAuth for each, and where each
// runs (UsesPairSetup), in the order of EnabledComponents.
type DelegatedSelection struct {
	Components []types.VMCPComponent
	// Grants are the vMCP's preconfigured OAuth grants, one for each component (SelectGrants), checked against the URL agents
	// reach.
	Grants []ComponentOAuthGrant
	// UsesPair reports, for each component, whether it runs on the pair's component server or connection.
	UsesPair []bool
}

// PairOwner returns the pair that owns instance, and false for an instance that is not a pair's.
func PairOwner(instance v1.VMCPInstance) (principal.DelegatedOwner, bool) {
	return principal.ParseDelegatedOwner(instance.Spec.UserID)
}

// ComponentServerName is the name of the component server that an instance with instanceName has for a component
// that runs a server for each instance (one that is not IsMultiUser).
func ComponentServerName(instanceName, componentID string) string {
	return name.SafeConcatName(system.MCPServerPrefix+instanceName, componentID)
}

// ComponentConnectionName is the name of the connection that an instance with instanceName has to the shared server
// of a component that IsMultiUser.
func ComponentConnectionName(instanceName, componentID string) string {
	return name.SafeConcatName(system.MCPServerInstancePrefix+instanceName, componentID)
}

// PairConnectID is the connection through which the pair whose instance has instanceName reaches component: its own
// server for a component with a server per instance, or else its connection to the component's shared server. The
// pair's upstream OAuth token for the component is keyed by it, as a user's is by their connection.
func PairConnectID(instanceName string, component types.VMCPComponent) string {
	if IsMultiUser(component) {
		return ComponentConnectionName(instanceName, component.ID)
	}
	return ComponentServerName(instanceName, component.ID)
}

// UserConfiguration returns the configuration of component that a user enters (a userAllowed policy), with the
// schema the user configuration form shows, and never a value: what ValidateAndEncodeUserConfiguration accepts for it.
// A value that the catalog entry supplies itself, as a literal or from a Secret, is never the user's, even when a policy
// marks it userAllowed.
func UserConfiguration(component types.VMCPComponent) []types.MCPConfig {
	var fields []types.MCPConfig
	for _, field := range ComponentConfig(component) {
		if !field.UserAllowed || field.Static || field.SecretBinding != nil {
			continue
		}
		field.Value = ""
		fields = append(fields, field)
	}
	return fields
}

// PairInstanceComponents returns, of components, those that a pair's instance carries: those that may need something
// from the user, a value they enter (UserConfiguration) or an upstream OAuth account (a remote component, which may ask
// for OAuth). It depends on neither the vMCP profiles, the agent profiles nor the vMCP's preconfigured OAuth grants, so that
// none of them changing removes what the user set up.
func PairInstanceComponents(components []types.VMCPComponent) []types.VMCPComponent {
	return slices.DeleteFunc(components, func(component types.VMCPComponent) bool {
		return len(UserConfiguration(component)) == 0 && !ComponentUsesOAuth(component)
	})
}

// UsesPairSetup reports whether an agent acting on behalf of a user reaches component, with grant as the vMCP's
// preconfigured OAuth grant for it (SelectGrants), through the pair's component server or connection rather than the
// agent's own:
//
//   - A component that takes a value from the user does, and carries the pair's values. Values the vMCP or the catalog
//     entry fixes apply to it as they do to everyone's.
//   - A remote component that the vMCP holds no preconfigured OAuth for does, so that it authorizes with the pair's
//     upstream token, if the user connected one. The vMCP's grant, when it holds one, is used in place of the pair's
//     token wherever the component runs.
//
// Every other component needs nothing from the user: it runs on the agent's own instance, which every user the agent
// acts for shares, with only the values fixed for everyone and the vMCP's grant.
func UsesPairSetup(component types.VMCPComponent, grant ComponentOAuthGrant) bool {
	return len(UserConfiguration(component)) > 0 || ComponentUsesOAuth(component) && !grant.Held()
}

// SelectDelegated returns what an agent acting on behalf of a user is granted of vmcp through instance, the agent's own
// instance, or nil before the agent connected (DelegatedSelection), where profiles are the vMCP profiles that name the
// agent (DelegatedProfiles, as CallerProfiles returns them for the agent acting on behalf of the user).
// The connect path and the setup API both start from it, so they agree on which components need the user.
func SelectDelegated(ctx context.Context, grants *PreconfiguredOAuthStore, vmcp v1.VMCP, instance *v1.VMCPInstance, profiles []types.VMCPProfile) (DelegatedSelection, error) {
	if vmcp.Spec.UserID != "" {
		return DelegatedSelection{}, fmt.Errorf("VMCP %q is personal, and agents only use shared VMCPs", vmcp.Name)
	}
	if grants == nil {
		return DelegatedSelection{}, fmt.Errorf("a delegated selection needs the store of vMCP preconfigured OAuth grants")
	}
	var current v1.VMCPInstance
	if instance != nil {
		current = *instance
	}
	selection := DelegatedSelection{Components: ProfilesGrantedComponents(profiles, ComponentsForInstance(vmcp, current))}
	var err error
	if selection.Grants, err = grants.SelectGrants(ctx, vmcp, selection.Components); err != nil {
		return DelegatedSelection{}, err
	}
	selection.UsesPair = make([]bool, len(selection.Components))
	for i, component := range selection.Components {
		// The MCP gateway refuses a grant for another URL than the one the component reaches; readiness says so first.
		if err := grants.checkGrantTarget(ctx, vmcp, component, &selection.Grants[i]); err != nil {
			return DelegatedSelection{}, err
		}
		selection.UsesPair[i] = UsesPairSetup(component, selection.Grants[i])
	}
	return selection, nil
}

// NeedsPair reports whether any component of the selection runs on the pair's instance.
func (s DelegatedSelection) NeedsPair() bool {
	return slices.Contains(s.UsesPair, true)
}

// Readiness returns whether the agent can use the vMCP on behalf of the user, with pair as what the user set up through
// the agent. A component is not ready while:
//
//   - a value the user enters that it requires is not set in the pair's configuration
//     (types.AgentReadinessConfigurationRequired);
//   - the vMCP's preconfigured OAuth for it no longer works (types.AgentReadinessOAuthInvalid), which only an
//     administrator fixes: the grant is used in place of anything the user could connect; or
//   - the vMCP holds no grant for it, it is known to need OAuth (ComponentOAuthGrant.Required), and the pair holds no
//     upstream token for it (types.AgentReadinessOAuthRequired).
//
// AgentReadinessFailure.ForUser reports what the user can set up as needing their approval. It never reads a value or
// a token.
func (s DelegatedSelection) Readiness(pair PairSetup) Readiness {
	var readiness Readiness
	for i, component := range s.Components {
		readiness.Reasons = append(readiness.Reasons, pairConfigurationReasons(component, pair)...)
		switch grant := s.Grants[i]; {
		case grant.Held():
			readiness.Reasons = append(readiness.Reasons, grant.ReadinessReasons(component)...)
		case ComponentUsesOAuth(component) && grant.Required && !pair.Connected[component.ID]:
			readiness.Reasons = append(readiness.Reasons, ReadinessReason{ComponentID: component.ID, Code: types.AgentReadinessOAuthRequired})
		}
	}
	return readiness
}

// pairConfigurationReasons returns the required values of component that the user enters and has not set through the
// agent: those missing from pair's values when it has them, or else as recorded on the pair's instance, or every one
// of them when the pair has neither.
func pairConfigurationReasons(component types.VMCPComponent, pair PairSetup) []ReadinessReason {
	var missing []string
	if pair.Values == nil && pair.Instance != nil {
		missing = pair.Instance.Status.MissingRequiredConfiguration
	} else {
		missing = MissingRequiredConfiguration(component, pair.Values, true)
	}
	var fields []string
	for _, key := range missing {
		if componentID, field, ok := ParseConfigurationKey(key); ok && componentID == component.ID {
			fields = append(fields, field)
		}
	}
	return ConfigurationRequiredReasons(component.ID, fields)
}

// PairConnections reports, by component ID, which of components the pair that owns instance, the pair's instance,
// holds an upstream OAuth token for, from the tokens' metadata: no token is read. A pair without an instance holds
// none.
func (s *PreconfiguredOAuthStore) PairConnections(ctx context.Context, instance *v1.VMCPInstance, components []types.VMCPComponent) (map[string]bool, error) {
	connected := map[string]bool{}
	if instance == nil {
		return connected, nil
	}
	if _, ok := PairOwner(*instance); !ok {
		return nil, fmt.Errorf("VMCP instance %q is not owned by a user's setup through an agent", instance.Name)
	}
	byConnection := map[string]string{}
	for _, component := range components {
		if ComponentUsesOAuth(component) {
			byConnection[PairConnectID(instance.Name, component)] = component.ID
		}
	}
	if len(byConnection) == 0 {
		return connected, nil
	}
	mcpIDs := make([]string, 0, len(byConnection))
	for mcpID := range byConnection {
		mcpIDs = append(mcpIDs, mcpID)
	}
	slices.Sort(mcpIDs)
	stored, err := s.gateway.MCPOAuthGrantMetadata(ctx, []string{instance.Spec.UserID}, mcpIDs)
	if err != nil {
		return nil, fmt.Errorf("read the upstream OAuth tokens of VMCP instance %q: %w", instance.Name, err)
	}
	for _, token := range stored {
		if componentID, ok := byConnection[token.MCPID]; ok && token.UserID == instance.Spec.UserID {
			connected[componentID] = true
		}
	}
	return connected, nil
}

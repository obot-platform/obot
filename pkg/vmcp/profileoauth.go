package vmcp

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	_ ProfileDataStore        = (*ProfileOAuthGrantStore)(nil)
	_ ComponentReadinessCheck = ProfileOAuthGrantCheck{}
)

// ProfileOAuthGrantGateway is the storage of profile OAuth grants: MCP OAuth tokens whose owner is a
// principal.ProfileGrant. The gateway client implements it.
type ProfileOAuthGrantGateway interface {
	MCPOAuthGrantMetadata(ctx context.Context, owners, mcpIDs []string) ([]gatewaytypes.MCPOAuthGrantMetadata, error)
	MCPOAuthGrantOwners(ctx context.Context, owners []string) ([]string, error)
	DeleteMCPOAuthGrants(ctx context.Context, owner, mcpID string, grantIDs []string) error
	DeleteMCPOAuthGrantsForOwners(ctx context.Context, owners []string) error
}

// ProfileOAuthGrantStore holds the OAuth grants that administrators authorize on agent-capable vMCP profiles. An
// administrator authorizes each OAuth component of a profile once, and every agent that resolves to the profile uses
// the grant headlessly, on every replica.
//
// A grant is an MCP OAuth token, stored as a user's is, with a different key: its owner (UserID) is
// principal.ProfileGrant(vmcpID, profileID), which no one can authenticate as, and its MCPID is
// ProfileOAuthGrantMCPID(vmcpID, componentID), which names no MCPServer, so no user's or agent's token is ever read as
// a grant or deleted with one, and the reverse. The token records the upstream URL it was authorized for, and is used
// only for that URL. Nothing about a grant is in the manifest, so catalog-synced vMCPs keep their grants across syncs.
//
// It is a ProfileDataStore: the vMCP controller deletes a profile's grants when the profile or its vMCP is deleted,
// and a component's grants when the component leaves the manifest (DeleteRemovedComponentGrants). Deleting an agent
// deletes no grant: grants belong to profiles.
type ProfileOAuthGrantStore struct {
	gateway ProfileOAuthGrantGateway
}

// AgentOAuthGrant is the grant an agent uses for one component, selected from the agent's profiles
// (ProfileOAuthGrantStore.SelectAgentGrants).
type AgentOAuthGrant struct {
	ComponentID string
	// ProfileID is the profile whose grant the agent uses. It is empty when no profile of the agent holds one.
	ProfileID string
	// Owner and MCPID are the key of the grant's token: principal.ProfileGrant and ProfileOAuthGrantMCPID.
	Owner string
	MCPID string
	// Fallback reports that the grant is the wildcard profile's, which the agent falls back to because its explicit
	// profile, if any, holds none for the component.
	Fallback bool
	// AmbiguousFallback reports that the agent would fall back for the component, but the profile it would fall back
	// to is ambiguous (AgentSelection.Ambiguous), so it has no grant.
	AmbiguousFallback bool
	// State is types.VMCPProfileOAuthAuthorized or types.VMCPProfileOAuthInvalid when the agent has a grant.
	State string
	// InvalidReason says why an invalid grant is invalid.
	InvalidReason string
	// URL is the upstream URL the grant was authorized for.
	URL string
}

// ProfileOAuthGrantCheck is the OAuth source of agent readiness (AgentReadinessChecks.OAuth). It selects the grant an
// agent uses for each component as the MCP gateway does when it connects the agent (SelectAgentGrants), and reports:
//
//   - ReadinessOAuthInvalid, fixed in the profile that holds it, for a grant that no longer works. The agent never
//     falls back from a broken grant, so a broken explicit grant blocks the component even when the wildcard profile
//     holds a working one.
//   - ReadinessOAuthRequired, fixed in the agent's explicit profile if it has one and otherwise in the wildcard, for a
//     component that is known to need OAuth (ComponentRequiresOAuth) when no profile of the agent holds a grant.
//   - AmbiguousFallbackReason for such a component when the agent's explicit profile holds none and the fallback is
//     ambiguous.
//
// A remote component that only asks for OAuth when it is called is caught then: the gateway answers the agent with a
// 409 that names it.
type ProfileOAuthGrantCheck struct {
	Grants *ProfileOAuthGrantStore
}

func NewProfileOAuthGrantStore(gateway ProfileOAuthGrantGateway) *ProfileOAuthGrantStore {
	return &ProfileOAuthGrantStore{gateway: gateway}
}

// ProfileOAuthGrantMCPID is the MCPID of the token of a profile's grant for a component. Grants are per component, not
// per connection: every agent of the profile uses the same one, whatever connection it reaches the component through.
func ProfileOAuthGrantMCPID(vmcpID, componentID string) string {
	return vmcpID + "/" + componentID
}

// ComponentUsesOAuth reports whether a component can hold a profile OAuth grant: only a remote server authorizes its
// callers with OAuth.
func ComponentUsesOAuth(component types.VMCPComponent) bool {
	return component.CatalogEntry.Manifest.Runtime == types.RuntimeRemote
}

// ComponentRequiresOAuth reports whether a component is known to need OAuth before it is called: a remote server that
// requires administrator static OAuth.
func ComponentRequiresOAuth(component types.VMCPComponent) bool {
	return ComponentOAuthCredentialReference(component) != ""
}

// StaticComponentTargetURL returns the upstream URL that every connection to a component reaches, when the
// component's definition fixes it, so that a grant can be checked against it without resolving anyone's
// configuration. The MCP gateway checks the grant against the URL it actually connects to on every use either way.
func StaticComponentTargetURL(component types.VMCPComponent) (string, bool) {
	remote := component.CatalogEntry.Manifest.RemoteConfig
	if !ComponentUsesOAuth(component) || remote == nil || remote.FixedURL == "" {
		return "", false
	}
	return remote.FixedURL, true
}

// ProfileOAuthGrantDefinitionHash identifies what a component's grant is authorized against: the upstream it reaches
// and the client it authorizes with. A pending authorization records it when it starts, and its callback stores the
// grant only if the component still has it.
func ProfileOAuthGrantDefinitionHash(component types.VMCPComponent) string {
	return utils.Digest([]any{
		"vmcp-profile-oauth-grant/v1",
		component.ID,
		component.MCPServerCatalogEntryID,
		ComponentOAuthCredentialReference(component),
		component.CatalogEntry.Manifest.Runtime,
		component.CatalogEntry.Manifest.RemoteConfig,
	})
}

// ProfilesWithData returns the profiles, among profileIDs, that hold any grant, with one query.
func (s *ProfileOAuthGrantStore) ProfilesWithData(ctx context.Context, vmcpID string, profileIDs []string) ([]string, error) {
	if len(profileIDs) == 0 {
		return nil, nil
	}
	owners := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		owners = append(owners, principal.ProfileGrant(vmcpID, id))
	}
	found, err := s.gateway.MCPOAuthGrantOwners(ctx, owners)
	if err != nil {
		return nil, fmt.Errorf("find vMCP %q profiles with OAuth grants: %w", vmcpID, err)
	}
	var result []string
	for _, id := range profileIDs {
		if slices.Contains(found, principal.ProfileGrant(vmcpID, id)) && !slices.Contains(result, id) {
			result = append(result, id)
		}
	}
	return result, nil
}

// DeleteProfileData deletes every grant of the profiles, with one statement. Deleting what is already gone succeeds,
// so it can be called again after any failure. The vMCP controller removes the profiles' records from status after
// it succeeds.
func (s *ProfileOAuthGrantStore) DeleteProfileData(ctx context.Context, vmcpID string, profileIDs []string) error {
	owners := make([]string, 0, len(profileIDs))
	for _, id := range profileIDs {
		owners = append(owners, principal.ProfileGrant(vmcpID, id))
	}
	if err := s.gateway.DeleteMCPOAuthGrantsForOwners(ctx, owners); err != nil {
		return fmt.Errorf("delete OAuth grants of vMCP %q profiles: %w", vmcpID, err)
	}
	return nil
}

// SelectAgentGrants selects the grant an agent uses for each component, from the agent's profiles (selection), with
// one query. It is the selection that agent readiness and the MCP gateway both use:
//
//   - The explicit profile comes first. If it holds a grant for the component, that is the agent's grant, whether it
//     works or not: a grant that was authorized and then broke never falls back.
//   - Only if the explicit profile holds none, or the agent has no explicit profile, is the wildcard profile's grant
//     used, unless the wildcard is ambiguous (AmbiguousFallback).
//
// A component that cannot hold a grant (ComponentUsesOAuth) has none. No token or client credential is read.
func (s *ProfileOAuthGrantStore) SelectAgentGrants(ctx context.Context, vmcp v1.VMCP, selection AgentSelection, components []types.VMCPComponent) ([]AgentOAuthGrant, error) {
	var owners, mcpIDs []string
	for _, profile := range []*types.VMCPProfile{selection.Explicit, selection.Wildcard} {
		if profile != nil && profile.ID != "" {
			owners = append(owners, principal.ProfileGrant(vmcp.Name, profile.ID))
		}
	}
	for _, component := range components {
		if ComponentUsesOAuth(component) {
			mcpIDs = append(mcpIDs, ProfileOAuthGrantMCPID(vmcp.Name, component.ID))
		}
	}
	var stored []gatewaytypes.MCPOAuthGrantMetadata
	if len(owners) > 0 && len(mcpIDs) > 0 {
		var err error
		if stored, err = s.gateway.MCPOAuthGrantMetadata(ctx, owners, mcpIDs); err != nil {
			return nil, fmt.Errorf("read OAuth grants of vMCP %q: %w", vmcp.Name, err)
		}
	}
	find := func(profile *types.VMCPProfile, mcpID string) *gatewaytypes.MCPOAuthGrantMetadata {
		if profile == nil || profile.ID == "" {
			return nil
		}
		owner := principal.ProfileGrant(vmcp.Name, profile.ID)
		for i := range stored {
			if stored[i].UserID == owner && stored[i].MCPID == mcpID {
				return &stored[i]
			}
		}
		return nil
	}

	result := make([]AgentOAuthGrant, 0, len(components))
	for _, component := range components {
		grant := AgentOAuthGrant{ComponentID: component.ID}
		if ComponentUsesOAuth(component) {
			mcpID := ProfileOAuthGrantMCPID(vmcp.Name, component.ID)
			var (
				profile *types.VMCPProfile
				found   *gatewaytypes.MCPOAuthGrantMetadata
			)
			switch {
			case find(selection.Explicit, mcpID) != nil:
				profile, found = selection.Explicit, find(selection.Explicit, mcpID)
			case selection.Ambiguous:
				grant.AmbiguousFallback = true
			case find(selection.Wildcard, mcpID) != nil:
				profile, found = selection.Wildcard, find(selection.Wildcard, mcpID)
				grant.Fallback = true
			}
			if found != nil {
				grant.ProfileID = profile.ID
				grant.Owner = found.UserID
				grant.MCPID = found.MCPID
				grant.URL = found.URL
				grant.State, grant.InvalidReason = grantState(*found, component)
			}
		}
		result = append(result, grant)
	}
	return result, nil
}

// grantState is whether a stored grant works for component, from what is stored about it: it is invalid once marked
// so, or when the component's definition fixes a target other than the one the grant was authorized for.
func grantState(grant gatewaytypes.MCPOAuthGrantMetadata, component types.VMCPComponent) (string, string) {
	if grant.InvalidatedAt != nil {
		return types.VMCPProfileOAuthInvalid, grant.InvalidReason
	}
	if target, ok := StaticComponentTargetURL(component); ok && target != grant.URL {
		return types.VMCPProfileOAuthInvalid, "the component's upstream URL changed since the grant was authorized"
	}
	return types.VMCPProfileOAuthAuthorized, ""
}

// Usable reports whether the agent has a grant for the component that works.
func (g AgentOAuthGrant) Usable() bool {
	return g.ProfileID != "" && g.State == types.VMCPProfileOAuthAuthorized
}

// ReadinessReasons returns why the grant keeps the agent from using component (ProfileOAuthGrantCheck), where
// selection is how the agent's profiles apply to it.
func (g AgentOAuthGrant) ReadinessReasons(component types.VMCPComponent, selection AgentSelection) []ReadinessReason {
	switch {
	case g.ProfileID != "" && g.State != types.VMCPProfileOAuthAuthorized:
		return []ReadinessReason{{ComponentID: component.ID, Code: ReadinessOAuthInvalid, ProfileID: g.ProfileID}}
	case g.ProfileID != "" || !ComponentRequiresOAuth(component):
		return nil
	case g.AmbiguousFallback:
		return []ReadinessReason{AmbiguousFallbackReason(component.ID)}
	default:
		return []ReadinessReason{{ComponentID: component.ID, Code: ReadinessOAuthRequired, ProfileID: selection.fixProfileID()}}
	}
}

// FallbackGrantProfileIDs returns the profiles whose grants an agent uses as the profile it falls back to.
func FallbackGrantProfileIDs(grants []AgentOAuthGrant) []string {
	var ids []string
	for _, grant := range grants {
		if grant.Fallback && grant.ProfileID != "" && !slices.Contains(ids, grant.ProfileID) {
			ids = append(ids, grant.ProfileID)
		}
	}
	return ids
}

func (c ProfileOAuthGrantCheck) ComponentReadiness(ctx context.Context, in AgentReadinessInput, component types.VMCPComponent) ([]ReadinessReason, error) {
	if c.Grants == nil {
		return UnsupportedAgentOAuth{}.ComponentReadiness(ctx, in, component)
	}
	grants, err := c.Grants.SelectAgentGrants(ctx, in.VMCP, in.Selection, []types.VMCPComponent{component})
	if err != nil {
		return nil, err
	}
	return grants[0].ReadinessReasons(component, in.Selection), nil
}

// ProfileOAuthStatus reports the grant a profile of vmcp holds for each component that can hold one, by component ID,
// with one query. It describes the profile alone, with no fallback, and never reads a token.
func (s *ProfileOAuthGrantStore) ProfileOAuthStatus(ctx context.Context, vmcp v1.VMCP, profileID string) (map[string]types.VMCPProfileOAuthStatus, error) {
	components := ComponentsForInstance(vmcp, v1.VMCPInstance{})
	var mcpIDs []string
	for _, component := range components {
		if ComponentUsesOAuth(component) {
			mcpIDs = append(mcpIDs, ProfileOAuthGrantMCPID(vmcp.Name, component.ID))
		}
	}
	stored, err := s.gateway.MCPOAuthGrantMetadata(ctx, []string{principal.ProfileGrant(vmcp.Name, profileID)}, mcpIDs)
	if err != nil {
		return nil, fmt.Errorf("read OAuth grants of vMCP %q profile %q: %w", vmcp.Name, profileID, err)
	}
	result := map[string]types.VMCPProfileOAuthStatus{}
	for _, component := range components {
		if !ComponentUsesOAuth(component) {
			continue
		}
		status := types.VMCPProfileOAuthStatus{Required: ComponentRequiresOAuth(component), State: types.VMCPProfileOAuthMissing}
		mcpID := ProfileOAuthGrantMCPID(vmcp.Name, component.ID)
		for _, grant := range stored {
			if grant.MCPID != mcpID {
				continue
			}
			status.State, status.InvalidReason = grantState(grant, component)
			status.Target = grant.URL
			status.Scopes = strings.Fields(grant.Scopes)
			if p, err := principal.Parse(grant.AuthorizedBy); err == nil {
				status.AuthorizedBy = p.ID
			}
			if grant.AuthorizedAt != nil {
				status.AuthorizedAt = types.NewTime(*grant.AuthorizedAt)
			}
		}
		result[component.ID] = status
	}
	return result, nil
}

// AddOAuthStatus adds to a profile's status the OAuth grants ProfileOAuthStatus reported for it.
func AddOAuthStatus(status *types.VMCPProfileStatus, oauth map[string]types.VMCPProfileOAuthStatus) {
	for i := range status.Components {
		if grant, ok := oauth[status.Components[i].ComponentID]; ok {
			status.Components[i].OAuth = &grant
		}
	}
}

// ProfileOAuthGrantTarget returns the profile and component of vmcp that an administrator authorizes a grant for, or
// the error that refuses it. The profile must be agent-capable, agent selection on the vMCP must not be ambiguous,
// since writing a grant puts the vMCP under the wildcard rule for every later edit, and the component must be a remote
// one, the only kind that authorizes with OAuth.
func ProfileOAuthGrantTarget(vmcp *v1.VMCP, profileID, componentID string) (types.VMCPProfile, types.VMCPComponent, error) {
	profile, err := writableProfile(vmcp, profileID)
	if err != nil {
		return types.VMCPProfile{}, types.VMCPComponent{}, err
	}
	if err := checkProfileSetup(vmcp, profile, "OAuth grants"); err != nil {
		return types.VMCPProfile{}, types.VMCPComponent{}, err
	}
	components := ComponentsForInstance(*vmcp, v1.VMCPInstance{})
	componentIndex := slices.IndexFunc(components, func(component types.VMCPComponent) bool { return component.ID == componentID })
	if componentID == "" || componentIndex < 0 {
		return types.VMCPProfile{}, types.VMCPComponent{}, types.NewErrNotFound("vMCP %q has no component %q", vmcp.Name, componentID)
	}
	component := components[componentIndex]
	if !ComponentUsesOAuth(component) {
		return types.VMCPProfile{}, types.VMCPComponent{}, types.NewErrBadRequest("component %q is not a remote server, so it does not use OAuth", component.ID)
	}
	return profile, component, nil
}

// newGrantID returns the ID of a new grant.
func newGrantID() string {
	return rand.Text()
}

// Authorize stores the grant an administrator authorized for a profile's component, and returns the vMCP with it
// published. check is called with the vMCP as stored, its profile and its component on every attempt, and refuses the
// grant if they are no longer what the administrator authorized; ProfileOAuthGrantTarget refuses it first if they
// cannot hold one. write stores the grant's token under owner and mcpID, with grantID.
//
// It writes the grant as writeProfileData writes any profile data, which keeps every grant deleted with its profile,
// and also with its component: the grant's ID is recorded with the profile's before the grant is stored, and the vMCP
// controller deletes every recorded grant whose profile or component leaves the spec. The grant replaces any grant
// stored for the profile's component before, and is published as the only one recorded for the component. An
// unpublished grant is deleted by its ID only.
func (s *ProfileOAuthGrantStore) Authorize(ctx context.Context, client kclient.Client, key kclient.ObjectKey, profileID, componentID string, check func(*v1.VMCP, types.VMCPProfile, types.VMCPComponent) error, write func(ctx context.Context, owner, mcpID, grantID string) error) (*v1.VMCP, error) {
	owner := principal.ProfileGrant(key.Name, profileID)
	mcpID := ProfileOAuthGrantMCPID(key.Name, componentID)
	return writeProfileData(ctx, client, key, profileID, func(vmcp *v1.VMCP) (*profileDataWrite, error) {
		profile, component, err := ProfileOAuthGrantTarget(vmcp, profileID, componentID)
		if err != nil {
			return nil, err
		}
		if err := check(vmcp, profile, component); err != nil {
			return nil, err
		}

		grantID := newGrantID()
		return &profileDataWrite{
			record: func(vmcp *v1.VMCP) bool {
				return recordGrantIDs(vmcp, profileID, componentID, slices.Concat(recordedGrantIDs(vmcp, profileID, componentID), []string{grantID}))
			},
			store: func(ctx context.Context) error {
				if err := write(ctx, owner, mcpID, grantID); err != nil {
					return fmt.Errorf("store OAuth grant of vMCP %q profile %q for component %q: %w", vmcp.Name, profileID, componentID, err)
				}
				return nil
			},
			publish: func(vmcp *v1.VMCP) {
				recordGrantIDs(vmcp, profileID, componentID, []string{grantID})
			},
			discard: func(ctx context.Context) {
				if err := s.gateway.DeleteMCPOAuthGrants(ctx, owner, mcpID, []string{grantID}); err != nil {
					slog.Warn("Failed to delete an unpublished vMCP profile OAuth grant", "vmcp", vmcp.Name, "profile", profileID, "component", componentID, "error", err)
				}
			},
		}, nil
	})
}

// Reset deletes the grant a profile holds for a component, and reports whether it held one. Unlike Authorize, it is
// allowed on a profile that is not agent-capable, on a vMCP whose agent selection is ambiguous, and for a component
// the vMCP no longer has: removing a grant can never give an agent the wrong one.
//
// It lists the grants that may be stored for the component, those recorded in status and the one stored, then
// publishes the removal of their record in a status update at the resource version it listed them at, and only then
// deletes them, by their IDs. A grant authorized after the listing has another ID and is kept; one recorded before
// the publication makes it conflict, and the reset is retried from a fresh read that lists it.
func (s *ProfileOAuthGrantStore) Reset(ctx context.Context, client kclient.Client, key kclient.ObjectKey, profileID, componentID string) (*v1.VMCP, bool, error) {
	owner := principal.ProfileGrant(key.Name, profileID)
	mcpID := ProfileOAuthGrantMCPID(key.Name, componentID)
	var (
		result  *v1.VMCP
		cleared bool
	)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		vmcp, err := readVMCP(ctx, client, key)
		if err != nil {
			return err
		}
		if _, err := profileOf(vmcp, profileID); err != nil {
			return err
		}

		listed := slices.Clone(recordedGrantIDs(vmcp, profileID, componentID))
		stored, err := s.gateway.MCPOAuthGrantMetadata(ctx, []string{owner}, []string{mcpID})
		if err != nil {
			return fmt.Errorf("read OAuth grant of vMCP %q profile %q for component %q: %w", vmcp.Name, profileID, componentID, err)
		}
		for _, grant := range stored {
			if !slices.Contains(listed, grant.GrantID) {
				listed = append(listed, grant.GrantID)
			}
		}
		cleared = len(stored) > 0
		if len(listed) == 0 {
			result = vmcp
			return nil
		}

		if recordGrantIDs(vmcp, profileID, componentID, nil) {
			if err := client.Status().Update(ctx, vmcp); err != nil {
				return err
			}
		}
		if err := s.gateway.DeleteMCPOAuthGrants(ctx, owner, mcpID, listed); err != nil {
			return fmt.Errorf("delete OAuth grant of vMCP %q profile %q for component %q: %w", vmcp.Name, profileID, componentID, err)
		}
		result = vmcp
		return nil
	})
	return result, cleared, err
}

// DeleteRemovedComponentGrants deletes the grants recorded for components that vmcp's manifest no longer has, by their
// recorded IDs, and removes their records from vmcp's status. It reports whether it changed the status, which the
// caller saves. Profiles that the manifest no longer has are handled with the rest of their data (DeleteProfileData).
//
// Grants are deleted before their records, so a failure at any point is retried with the records still in place. A
// grant is deleted only by an ID recorded for it: if vmcp is stale, and a component with the same ID was added back and
// authorized again since, that grant has an ID vmcp does not record, and is kept; saving the stale status then fails
// with a conflict.
func (s *ProfileOAuthGrantStore) DeleteRemovedComponentGrants(ctx context.Context, vmcp *v1.VMCP) (bool, error) {
	var components []string
	for _, component := range ComponentsForInstance(*vmcp, v1.VMCPInstance{}) {
		components = append(components, component.ID)
	}
	profiles := ProfileIDs(vmcp.Spec.Manifest)
	var kept []v1.VMCPProfileOAuthGrant
	for _, record := range vmcp.Status.ProfileOAuthGrants {
		if slices.Contains(components, record.ComponentID) || !slices.Contains(profiles, record.ProfileID) {
			kept = append(kept, record)
			continue
		}
		owner := principal.ProfileGrant(vmcp.Name, record.ProfileID)
		if err := s.gateway.DeleteMCPOAuthGrants(ctx, owner, ProfileOAuthGrantMCPID(vmcp.Name, record.ComponentID), record.GrantIDs); err != nil {
			return false, fmt.Errorf("delete OAuth grants of removed component %q of vMCP %q profile %q: %w", record.ComponentID, vmcp.Name, record.ProfileID, err)
		}
		slog.Info("Deleted the OAuth grants of a removed vMCP component", "vmcp", vmcp.Name, "profile", record.ProfileID, "component", record.ComponentID)
	}
	if len(kept) == len(vmcp.Status.ProfileOAuthGrants) {
		return false, nil
	}
	vmcp.Status.ProfileOAuthGrants = kept
	return true, nil
}

// ProfileOAuthGrantProfileIDs returns the profiles that vmcp's status records grants for, which the vMCP controller
// deletes the data of along with the profiles recorded in v1.VMCPStatus.ProfileIDs.
func ProfileOAuthGrantProfileIDs(vmcp v1.VMCP) []string {
	var ids []string
	for _, record := range vmcp.Status.ProfileOAuthGrants {
		if !slices.Contains(ids, record.ProfileID) {
			ids = append(ids, record.ProfileID)
		}
	}
	return ids
}

// RemoveProfileOAuthGrantRecords removes the grant records of profiles from vmcp's status, after their grants were
// deleted, and reports whether it removed any.
func RemoveProfileOAuthGrantRecords(vmcp *v1.VMCP, profileIDs []string) bool {
	before := len(vmcp.Status.ProfileOAuthGrants)
	vmcp.Status.ProfileOAuthGrants = slices.DeleteFunc(vmcp.Status.ProfileOAuthGrants, func(record v1.VMCPProfileOAuthGrant) bool {
		return slices.Contains(profileIDs, record.ProfileID)
	})
	if len(vmcp.Status.ProfileOAuthGrants) == 0 {
		vmcp.Status.ProfileOAuthGrants = nil
	}
	return len(vmcp.Status.ProfileOAuthGrants) != before
}

// recordedGrantIDs returns the grant IDs vmcp's status records for a profile's component.
func recordedGrantIDs(vmcp *v1.VMCP, profileID, componentID string) []string {
	for _, record := range vmcp.Status.ProfileOAuthGrants {
		if record.ProfileID == profileID && record.ComponentID == componentID {
			return record.GrantIDs
		}
	}
	return nil
}

// recordGrantIDs records grantIDs as those that may be stored for a profile's component, removing the record when
// there are none, and reports whether that changed the status.
func recordGrantIDs(vmcp *v1.VMCP, profileID, componentID string, grantIDs []string) bool {
	index := slices.IndexFunc(vmcp.Status.ProfileOAuthGrants, func(record v1.VMCPProfileOAuthGrant) bool {
		return record.ProfileID == profileID && record.ComponentID == componentID
	})
	switch {
	case index < 0 && len(grantIDs) == 0:
		return false
	case index < 0:
		vmcp.Status.ProfileOAuthGrants = append(vmcp.Status.ProfileOAuthGrants, v1.VMCPProfileOAuthGrant{ProfileID: profileID, ComponentID: componentID, GrantIDs: grantIDs})
		return true
	case len(grantIDs) == 0:
		vmcp.Status.ProfileOAuthGrants = slices.Delete(vmcp.Status.ProfileOAuthGrants, index, index+1)
		if len(vmcp.Status.ProfileOAuthGrants) == 0 {
			vmcp.Status.ProfileOAuthGrants = nil
		}
		return true
	case slices.Equal(vmcp.Status.ProfileOAuthGrants[index].GrantIDs, grantIDs):
		return false
	default:
		vmcp.Status.ProfileOAuthGrants[index].GrantIDs = grantIDs
		return true
	}
}

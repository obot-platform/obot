package vmcp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/util/retry"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// GrantTargetMismatch explains a grant that was authorized for another URL than the one a connection to its
	// component reaches, without either URL.
	GrantTargetMismatch = "the grant was authorized for another upstream URL than the one the vMCP now configures"
)

// PreconfiguredOAuthGateway is the storage of preconfigured OAuth grants: MCP OAuth tokens whose owner is a
// principal.PreconfiguredOAuthOwner. The gateway client implements it.
type PreconfiguredOAuthGateway interface {
	MCPOAuthGrantMetadata(ctx context.Context, owners, mcpIDs []string) ([]gatewaytypes.MCPOAuthGrantMetadata, error)
	DeleteMCPOAuthGrants(ctx context.Context, owner, mcpID string, grantIDs []string) error
	DeleteMCPOAuthGrantsForOwner(ctx context.Context, owner string) error
}

// PreconfiguredOAuthStore holds the OAuth grants that administrators preconfigure for the remote components of shared
// vMCPs. A vMCP holds at most one grant per component. An administrator authorizes it once, and everyone who reaches
// the component through the vMCP uses it, on every replica: every agent that a profile naming it grants the component,
// headlessly, and every user connecting to the vMCP themselves, who is then never asked to authorize the component
// with an account of their own, and whose own token for it, if they have one, is not used. Profiles hold nothing: they
// decide which components, and so which grants, a caller reaches, as they decide which tools it gets.
//
// A grant is an MCP OAuth token, stored as a user's is, with a different key: its owner (UserID) is
// principal.PreconfiguredOAuthOwner(vmcpID), which no one can authenticate as, and its MCPID is
// PreconfiguredOAuthMCPID(vmcpID, componentID), which names no MCPServer, so no user's or agent's token is ever read as
// a grant or deleted with one, and the reverse. The token records the upstream URL it was authorized for, and is used
// only for that URL. Nothing about a grant is in the manifest, so catalog-synced vMCPs keep their grants across syncs.
//
// The vMCP controller deletes a component's grant when the component leaves the manifest
// (DeleteRemovedComponentGrants), and every grant of a vMCP being deleted (DeleteVMCPGrants). Changing a profile, or
// deleting a user or an agent, deletes no grant.
type PreconfiguredOAuthStore struct {
	gateway PreconfiguredOAuthGateway
	// targets resolves the URL a connection to a component reaches from the values the vMCP and the catalog entry fix,
	// for components whose definition does not fix it. Without it, only a URL the component's definition fixes is
	// compared with its grant's.
	targets ComponentTargetResolver
	// removed records that Obot deleted a grant on its own (GrantRemovalRecorder). It may be nil.
	removed GrantRemovalRecorder
}

// GrantRemovalRecorder records that a vMCP's grant for a component was deleted by Obot rather than by an administrator,
// because the component or the vMCP was removed. Recording is best effort and must not fail the deletion it records.
type GrantRemovalRecorder func(ctx context.Context, vmcpID, componentID, reason string)

// ComponentTarget is the upstream URL that a component's preconfigured OAuth grant is bound to: the URL a connection to
// the component reaches with the values fixed for everyone by the vMCP and the catalog entry, the only values an agent
// is given, and the only ones an administrator can authorize a grant for.
type ComponentTarget struct {
	// URL is the URL. It is empty when a value it needs is missing.
	URL string
	// Missing are the keys the URL needs that neither the vMCP nor the catalog entry fixes.
	Missing []string
}

// ComponentTargetResolver resolves the upstream URL that a component's preconfigured OAuth grant is bound to
// (ComponentTarget), from the same values as an agent's component server is given. The MCP session manager implements
// it.
type ComponentTargetResolver interface {
	ComponentTarget(ctx context.Context, vmcp v1.VMCP, component types.VMCPComponent) (ComponentTarget, error)
}

// ComponentOAuthGrant is the preconfigured OAuth grant a vMCP holds for one component, as SelectGrants found it, which
// everyone who reaches the component through the vMCP uses.
type ComponentOAuthGrant struct {
	ComponentID string
	// Owner and MCPID are the key of the grant's token: principal.PreconfiguredOAuthOwner and PreconfiguredOAuthMCPID.
	// They are empty when the vMCP holds no grant for the component (Held).
	Owner string
	MCPID string
	// Required reports that the component is known to need OAuth (requiresOAuth): it requires static OAuth, the vMCP
	// holds a grant for it, working or not, or the vMCP controller's probe of the component found that it asks for
	// OAuth.
	Required bool
	// State is types.VMCPPreconfiguredOAuthAuthorized or types.VMCPPreconfiguredOAuthInvalid when the vMCP holds a
	// grant.
	State string
	// InvalidReason says why an invalid grant is invalid.
	InvalidReason string
	// URL is the upstream URL the grant was authorized for.
	URL string

	// stored is what is stored about the grant, without its tokens, for the grant's status (Status).
	stored gatewaytypes.MCPOAuthGrantMetadata
}

func NewPreconfiguredOAuthStore(gateway PreconfiguredOAuthGateway) *PreconfiguredOAuthStore {
	return &PreconfiguredOAuthStore{gateway: gateway}
}

// SetGrantRemovalRecorder sets the recorder of the grants the store deletes as the vMCP controller removes a component
// or a vMCP, which no administrator asked for one at a time. Set it before the store is used.
func (s *PreconfiguredOAuthStore) SetGrantRemovalRecorder(recorder GrantRemovalRecorder) {
	s.removed = recorder
}

// SetComponentTargets sets the resolver of the URLs that grants are bound to for components whose definition does not
// fix the URL (ComponentTargetResolver). Set it before the store is used.
func (s *PreconfiguredOAuthStore) SetComponentTargets(targets ComponentTargetResolver) {
	s.targets = targets
}

// PreconfiguredOAuthMCPID is the MCPID of the token of a vMCP's grant for a component. Grants are per component, not
// per connection: everyone who reaches the component uses the same one, whatever connection they reach it through.
func PreconfiguredOAuthMCPID(vmcpID, componentID string) string {
	return vmcpID + "/" + componentID
}

// ComponentUsesOAuth reports whether a component can hold a preconfigured OAuth grant: only a remote server authorizes
// its callers with OAuth.
func ComponentUsesOAuth(component types.VMCPComponent) bool {
	return component.CatalogEntry.Manifest.Runtime == types.RuntimeRemote
}

// requiresOAuth reports whether component of vmcp, which can hold a grant (ComponentUsesOAuth), is known to need OAuth
// before it is called, where stored are the grants vmcp holds: when it requires administrator static OAuth; when vmcp
// holds a grant for it among stored, working or not, since a grant can only be authorized for a server that asked for
// OAuth; or else when the vMCP controller's probe of the component found that it asks for OAuth
// (ProbedOAuthRequirement). A probe that failed counts as not finding it.
func requiresOAuth(vmcp v1.VMCP, component types.VMCPComponent, stored []gatewaytypes.MCPOAuthGrantMetadata) bool {
	mcpID := PreconfiguredOAuthMCPID(vmcp.Name, component.ID)
	return ComponentOAuthCredentialReference(component) != "" ||
		slices.ContainsFunc(stored, func(grant gatewaytypes.MCPOAuthGrantMetadata) bool { return grant.MCPID == mcpID }) ||
		ProbedOAuthRequirement(vmcp, component) == v1.VMCPComponentOAuthRequired
}

// StaticComponentTargetURL returns the upstream URL that every connection to a component reaches, when the
// component's definition fixes it, so that a grant can be checked against it without reading any configuration.
// Otherwise the URL comes from the vMCP's fixed configuration (ComponentTargetResolver). The MCP gateway checks the grant
// against the URL it actually connects to on every use either way.
func StaticComponentTargetURL(component types.VMCPComponent) (string, bool) {
	remote := component.CatalogEntry.Manifest.RemoteConfig
	if !ComponentUsesOAuth(component) || remote == nil || remote.FixedURL == "" {
		return "", false
	}
	return remote.FixedURL, true
}

// PreconfiguredOAuthDefinitionHash identifies what a component's grant is authorized against: the upstream it reaches
// and the client it authorizes with. A pending authorization records it when it starts, and its callback stores the
// grant only if the component still has it.
func PreconfiguredOAuthDefinitionHash(component types.VMCPComponent) string {
	return utils.Digest([]any{
		"vmcp-preconfigured-oauth/v1",
		component.ID,
		component.MCPServerCatalogEntryID,
		ComponentOAuthCredentialReference(component),
		component.CatalogEntry.Manifest.Runtime,
		component.CatalogEntry.Manifest.RemoteConfig,
	})
}

// PreconfigurableComponents returns the remote components of a shared vMCP (ComponentUsesOAuth), in manifest order:
// the components an administrator can preconfigure OAuth for, and the only ones the vMCP controller probes for OAuth.
// Any of them can be preconfigured, whichever profiles grant it, to users or to agents. A personal vMCP has none: its
// owner authorizes their own connection.
func PreconfigurableComponents(vmcp v1.VMCP) []types.VMCPComponent {
	if vmcp.Spec.UserID != "" {
		return nil
	}
	return slices.DeleteFunc(ComponentsForInstance(vmcp, v1.VMCPInstance{}), func(component types.VMCPComponent) bool {
		return !ComponentUsesOAuth(component)
	})
}

// SelectGrants returns the grant vmcp holds for each of components, in their order, with one query. It is the selection
// that agent readiness, the preconfigured OAuth API and the MCP gateway, for users and agents alike, all use. A
// component that cannot hold a grant (ComponentUsesOAuth) has none. No token or client credential is read.
//
// It does not decide who uses a grant: the caller passes only the components the caller reaches (EnabledComponents),
// so no one uses the grant of a component that no profile naming them grants.
func (s *PreconfiguredOAuthStore) SelectGrants(ctx context.Context, vmcp v1.VMCP, components []types.VMCPComponent) ([]ComponentOAuthGrant, error) {
	owner := principal.PreconfiguredOAuthOwner(vmcp.Name)
	var mcpIDs []string
	for _, component := range components {
		if ComponentUsesOAuth(component) {
			mcpIDs = append(mcpIDs, PreconfiguredOAuthMCPID(vmcp.Name, component.ID))
		}
	}
	var stored []gatewaytypes.MCPOAuthGrantMetadata
	if len(mcpIDs) > 0 {
		var err error
		if stored, err = s.gateway.MCPOAuthGrantMetadata(ctx, []string{owner}, mcpIDs); err != nil {
			return nil, fmt.Errorf("read the preconfigured OAuth grants of vMCP %q: %w", vmcp.Name, err)
		}
	}

	result := make([]ComponentOAuthGrant, 0, len(components))
	for _, component := range components {
		grant := ComponentOAuthGrant{ComponentID: component.ID, Required: ComponentOAuthCredentialReference(component) != ""}
		if ComponentUsesOAuth(component) {
			mcpID := PreconfiguredOAuthMCPID(vmcp.Name, component.ID)
			grant.Required = requiresOAuth(vmcp, component, stored)
			if i := slices.IndexFunc(stored, func(g gatewaytypes.MCPOAuthGrantMetadata) bool { return g.UserID == owner && g.MCPID == mcpID }); i >= 0 {
				grant.Owner = stored[i].UserID
				grant.MCPID = stored[i].MCPID
				grant.URL = stored[i].URL
				grant.State, grant.InvalidReason = grantState(stored[i], component)
				grant.stored = stored[i]
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
		return types.VMCPPreconfiguredOAuthInvalid, grant.InvalidReason
	}
	if target, ok := StaticComponentTargetURL(component); ok && target != grant.URL {
		return types.VMCPPreconfiguredOAuthInvalid, "the component's upstream URL changed since the grant was authorized"
	}
	return types.VMCPPreconfiguredOAuthAuthorized, ""
}

// checkGrantTarget compares the URL that grant, selected for a component (SelectGrants), was authorized for, if it is
// usable, with the URL the grant is bound to now (ResolveComponentTarget), and marks the grant invalid if the URL
// differs. A URL that cannot be resolved leaves the grant as it is: the component lacks a value the URL needs, which
// configuration readiness reports. Without a resolver, only a URL the component's definition fixes is compared, as
// grantState already did; the MCP gateway's token store still refuses the grant for any other URL than its own.
func (s *PreconfiguredOAuthStore) checkGrantTarget(ctx context.Context, vmcp v1.VMCP, component types.VMCPComponent, grant *ComponentOAuthGrant) error {
	if !grant.Usable() {
		return nil
	}
	target, refused, err := ResolveComponentTarget(ctx, s.targets, vmcp, component)
	switch {
	case err != nil:
		return fmt.Errorf("resolve the upstream URL of component %q: %w", component.ID, err)
	case refused != nil:
		// The configured URL no longer satisfies the component, so no grant can be used for it.
		grant.State, grant.InvalidReason = types.VMCPPreconfiguredOAuthInvalid, refused.Error()
	case target.URL != "" && target.URL != grant.URL:
		grant.State, grant.InvalidReason = types.VMCPPreconfiguredOAuthInvalid, GrantTargetMismatch
	}
	return nil
}

// componentReasons returns why component of vmcp keeps every agent granted it from the vMCP, where grant is the vMCP's
// grant for it (SelectGrants), which it checks against the URL agents reach first (checkGrantTarget): configuration
// that agents cannot be given (AgentMissingConfiguration), and the OAuth the component needs (ReadinessReasons). Agent
// readiness and the connect path both report these, so they never disagree.
func (s *PreconfiguredOAuthStore) componentReasons(ctx context.Context, vmcp v1.VMCP, component types.VMCPComponent, grant *ComponentOAuthGrant) ([]ReadinessReason, error) {
	// The MCP gateway refuses a grant for another URL than the agent's connection reaches; readiness says so first.
	if err := s.checkGrantTarget(ctx, vmcp, component, grant); err != nil {
		return nil, err
	}
	configuration := ConfigurationRequiredReasons(component.ID, AgentMissingConfiguration(component))
	return slices.Concat(configuration, grant.ReadinessReasons(component)), nil
}

// ResolveComponentTarget resolves the upstream URL that the vMCP's preconfigured OAuth grant for a component is bound
// to, which an agent's connection to the component reaches: the URL the component's definition fixes
// (StaticComponentTargetURL), without resolving anything, or else the one that targets resolves from the vMCP's fixed
// configuration. Without targets, a URL that comes from configuration is left empty. refused is the error, an ErrHTTP
// that names no URL, of a configured URL that does not satisfy the component; err is any other failure to resolve it.
func ResolveComponentTarget(ctx context.Context, targets ComponentTargetResolver, vmcp v1.VMCP, component types.VMCPComponent) (target ComponentTarget, refused, err error) {
	if url, ok := StaticComponentTargetURL(component); ok {
		return ComponentTarget{URL: url}, nil, nil
	}
	if targets == nil {
		return ComponentTarget{}, nil, nil
	}
	target, err = targets.ComponentTarget(ctx, vmcp, component)
	if _, ok := errors.AsType[*types.ErrHTTP](err); ok {
		return ComponentTarget{}, err, nil
	}
	return target, nil, err
}

// Held reports whether the vMCP holds a grant for the component, working or not.
func (g ComponentOAuthGrant) Held() bool {
	return g.Owner != ""
}

// Usable reports whether the vMCP holds a grant for the component that works.
func (g ComponentOAuthGrant) Usable() bool {
	return g.Held() && g.State == types.VMCPPreconfiguredOAuthAuthorized
}

// ReadinessReasons returns why the grant, as SelectGrants selected it and checkGrantTarget checked it, keeps every
// agent granted component from the vMCP (AgentReadiness), where an administrator fixes it, in the component's settings
// on the vMCP:
//
//   - types.AgentReadinessOAuthInvalid for a grant that no longer works, including one authorized for another upstream
//     URL than the one agents' connections reach.
//   - types.AgentReadinessOAuthRequired for a component that is known to need OAuth (requiresOAuth) when the vMCP
//     holds no grant for it: one that requires static OAuth, or one that the vMCP controller's probe found asks for
//     OAuth at the URL agents reach.
//
// Readiness never contacts a component: it reads what the vMCP controller recorded. A component whose probe failed,
// or that is not probed yet, is not known to need OAuth, so an agent is never kept from it for that. If it asks for
// OAuth when the agent calls it, the gateway answers the agent with a 409 that names it.
func (g ComponentOAuthGrant) ReadinessReasons(component types.VMCPComponent) []ReadinessReason {
	switch {
	case g.Held() && g.State != types.VMCPPreconfiguredOAuthAuthorized:
		return []ReadinessReason{{ComponentID: component.ID, Code: types.AgentReadinessOAuthInvalid}}
	case g.Held() || !g.Required:
		return nil
	default:
		return []ReadinessReason{{ComponentID: component.ID, Code: types.AgentReadinessOAuthRequired}}
	}
}

// Status reports the grant, as SelectGrants selected it and checkGrantTarget checked it, for a response
// (types.VMCPPreconfiguredOAuth): its state, who authorized it, when, with which scopes, and for which target, never a
// token, and only the hostname of a URL that comes from the vMCP's fixed configuration (GrantTargetForResponse).
// Without a grant, the state says whether the component is known to need OAuth (Required).
func (g ComponentOAuthGrant) Status(vmcpID string, component types.VMCPComponent) types.VMCPPreconfiguredOAuth {
	status := types.VMCPPreconfiguredOAuth{VMCPID: vmcpID, ComponentID: component.ID, State: types.VMCPPreconfiguredOAuthNotRequired}
	if g.Required {
		status.State = types.VMCPPreconfiguredOAuthNeedsAuthorization
	}
	if !g.Held() {
		return status
	}
	status.State, status.InvalidReason = g.State, g.InvalidReason
	status.Target, status.TargetHostname = GrantTargetForResponse(component, g.URL)
	status.Scopes = strings.Fields(g.stored.Scopes)
	if p, err := principal.Parse(g.stored.AuthorizedBy); err == nil {
		status.AuthorizedBy = p.ID
	}
	if g.stored.AuthorizedAt != nil {
		status.AuthorizedAt = types.NewTime(*g.stored.AuthorizedAt)
	}
	return status
}

// Status reports the preconfigured OAuth of the component of vmcp with componentID (ComponentOAuthGrant.Status), with
// the grant checked against the URL it is bound to now (checkGrantTarget), as agent readiness checks it. It refuses a
// component that cannot hold a grant as PreconfigurableComponent does. It never reads or returns a token.
func (s *PreconfiguredOAuthStore) Status(ctx context.Context, vmcp v1.VMCP, componentID string) (types.VMCPPreconfiguredOAuth, error) {
	component, err := PreconfigurableComponent(&vmcp, componentID)
	if err != nil {
		return types.VMCPPreconfiguredOAuth{}, err
	}
	grants, err := s.SelectGrants(ctx, vmcp, []types.VMCPComponent{component})
	if err != nil {
		return types.VMCPPreconfiguredOAuth{}, err
	}
	if err := s.checkGrantTarget(ctx, vmcp, component, &grants[0]); err != nil {
		return types.VMCPPreconfiguredOAuth{}, err
	}
	return grants[0].Status(vmcp.Name, component), nil
}

// GrantTargetForResponse returns what a response may say of the URL a grant was authorized for, from the grant's URL
// (grantURL) and its component. A component whose definition fixes the URL (StaticComponentTargetURL) is catalog data, so
// the full URL is returned as target. Any other URL comes from the vMCP's fixed configuration, which responses never
// carry, so only its hostname is returned, as targetHostname, and target is empty.
func GrantTargetForResponse(component types.VMCPComponent, grantURL string) (target, hostname string) {
	if _, static := StaticComponentTargetURL(component); static {
		return grantURL, ""
	}
	return "", URLHostname(grantURL)
}

// URLHostname returns the hostname of rawURL, without its port, userinfo, path or query, or "" if it has none or cannot
// be parsed.
func URLHostname(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

// RedactURL replaces rawURL, and its parts that an error message may repeat, in text with a placeholder, so that a URL
// that comes from the vMCP's fixed configuration is never put into a response or an error.
func RedactURL(text, rawURL string) string {
	if rawURL == "" {
		return text
	}
	text = strings.ReplaceAll(text, rawURL, "<configured URL>")
	if parsed, err := url.Parse(rawURL); err == nil {
		parts := []string{parsed.String(), parsed.RawQuery, parsed.EscapedPath()}
		if parsed.User != nil {
			parts = append(parts, parsed.User.String())
		}
		for _, part := range parts {
			if len(part) > 1 {
				text = strings.ReplaceAll(text, part, "<redacted>")
			}
		}
	}
	return text
}

// PreconfigurableComponent returns the component of vmcp with componentID that can hold a preconfigured OAuth grant,
// or the error that refuses it: the vMCP must be shared (PreconfigurableComponents), and the component must exist and
// be a remote one, the only kind that authorizes with OAuth. Any remote component of a shared vMCP can, whichever
// profiles grant it.
func PreconfigurableComponent(vmcp *v1.VMCP, componentID string) (types.VMCPComponent, error) {
	if vmcp.Spec.UserID != "" {
		return types.VMCPComponent{}, types.NewErrBadRequest("vMCP %q is personal; only a shared vMCP holds preconfigured OAuth", vmcp.Name)
	}
	components := ComponentsForInstance(*vmcp, v1.VMCPInstance{})
	componentIndex := slices.IndexFunc(components, func(component types.VMCPComponent) bool { return component.ID == componentID })
	if componentID == "" || componentIndex < 0 {
		return types.VMCPComponent{}, types.NewErrNotFound("vMCP %q has no component %q", vmcp.Name, componentID)
	}
	component := components[componentIndex]
	if !ComponentUsesOAuth(component) {
		return types.VMCPComponent{}, types.NewErrBadRequest("component %q is not a remote server, so it does not use OAuth", component.ID)
	}
	return component, nil
}

// PreconfiguredOAuthTarget returns the component of vmcp that an administrator authorizes a grant for, or the error
// that refuses it: one that can hold a grant (PreconfigurableComponent), of a vMCP that is not being deleted.
func PreconfiguredOAuthTarget(vmcp *v1.VMCP, componentID string) (types.VMCPComponent, error) {
	if vmcp.Spec.UserID == "" && !vmcp.DeletionTimestamp.IsZero() {
		return types.VMCPComponent{}, types.NewErrHTTP(http.StatusConflict, fmt.Sprintf("vMCP %q is being deleted", vmcp.Name))
	}
	return PreconfigurableComponent(vmcp, componentID)
}

// Authorize stores the grant an administrator authorized for a component of the vMCP with key, and returns the vMCP
// with it recorded. check is called with the vMCP as stored and its component on every attempt, and refuses the grant
// if they are no longer what the administrator authorized; PreconfiguredOAuthTarget refuses it first if the component
// cannot hold one. write stores the grant's token under owner and mcpID, with grantID. Conflicting writes, by other
// administrators or by controllers, are retried from a fresh read, so no write is lost and none is applied over a
// stale one.
//
// The order of its steps keeps every grant deleted with its component:
//
//  1. Before the grant is stored, its ID is recorded in the vMCP's status (v1.VMCPStatus.PreconfiguredOAuth), with the
//     IDs already recorded for the component, in a status update at a resource version whose spec has the component.
//     The vMCP controller deletes every recorded grant whose component leaves the spec, so a component removed from
//     now on takes the grant with it, even one removed before the controller saw the record.
//  2. The grant is stored, replacing any grant stored for the component before.
//  3. It is published, as the only grant recorded for the component, in a status update at the same resource version.
//     A conflict means the vMCP changed, possibly by losing the component, and possibly after the controller deleted
//     the component's recorded grants, so the unpublished grant is deleted, by its ID only, before the write is
//     retried.
func (s *PreconfiguredOAuthStore) Authorize(ctx context.Context, client kclient.Client, key kclient.ObjectKey, componentID string, check func(*v1.VMCP, types.VMCPComponent) error, write func(ctx context.Context, owner, mcpID, grantID string) error) (*v1.VMCP, error) {
	owner := principal.PreconfiguredOAuthOwner(key.Name)
	mcpID := PreconfiguredOAuthMCPID(key.Name, componentID)
	var result *v1.VMCP
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		vmcp, err := readVMCP(ctx, client, key)
		if err != nil {
			return err
		}
		component, err := PreconfiguredOAuthTarget(vmcp, componentID)
		if err != nil {
			return err
		}
		if err := check(vmcp, component); err != nil {
			return err
		}

		grantID := rand.Text()
		recordGrantIDs(vmcp, componentID, slices.Concat(recordedGrantIDs(vmcp, componentID), []string{grantID}))
		if err := client.Status().Update(ctx, vmcp); err != nil {
			return err
		}

		if err := write(ctx, owner, mcpID, grantID); err != nil {
			return fmt.Errorf("store the preconfigured OAuth grant of vMCP %q for component %q: %w", vmcp.Name, componentID, err)
		}

		recordGrantIDs(vmcp, componentID, []string{grantID})
		if err := client.Status().Update(ctx, vmcp); err != nil {
			// Only a conflict proves the update was not applied; after any other error the grant may be published.
			if apierrors.IsConflict(err) {
				if err := s.gateway.DeleteMCPOAuthGrants(ctx, owner, mcpID, []string{grantID}); err != nil {
					slog.Warn("Failed to delete an unpublished vMCP preconfigured OAuth grant", "vmcp", vmcp.Name, "component", componentID, "error", err)
				}
			}
			return err
		}
		result = vmcp
		return nil
	})
	return result, err
}

// Revoke deletes the grant the vMCP with key holds for a component, and reports whether it held one. Every agent
// granted the component is refused it until an administrator authorizes it again. Unlike Authorize, it is allowed for
// a component the vMCP no longer has, or that no agent is granted: removing a grant can never give an agent the wrong
// one.
//
// It lists the grants that may be stored for the component, those recorded in status and the one stored, then
// publishes the removal of their record in a status update at the resource version it listed them at, and only then
// deletes them, by their IDs. A grant authorized after the listing has another ID and is kept; one recorded before
// the publication makes it conflict, and the revocation is retried from a fresh read that lists it.
func (s *PreconfiguredOAuthStore) Revoke(ctx context.Context, client kclient.Client, key kclient.ObjectKey, componentID string) (*v1.VMCP, bool, error) {
	owner := principal.PreconfiguredOAuthOwner(key.Name)
	mcpID := PreconfiguredOAuthMCPID(key.Name, componentID)
	var (
		result  *v1.VMCP
		revoked bool
	)
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		vmcp, err := readVMCP(ctx, client, key)
		if err != nil {
			return err
		}

		listed := slices.Clone(recordedGrantIDs(vmcp, componentID))
		stored, err := s.gateway.MCPOAuthGrantMetadata(ctx, []string{owner}, []string{mcpID})
		if err != nil {
			return fmt.Errorf("read the preconfigured OAuth grant of vMCP %q for component %q: %w", vmcp.Name, componentID, err)
		}
		for _, grant := range stored {
			if !slices.Contains(listed, grant.GrantID) {
				listed = append(listed, grant.GrantID)
			}
		}
		revoked = len(stored) > 0
		if len(listed) == 0 {
			result = vmcp
			return nil
		}

		if recordGrantIDs(vmcp, componentID, nil) {
			if err := client.Status().Update(ctx, vmcp); err != nil {
				return err
			}
		}
		if err := s.gateway.DeleteMCPOAuthGrants(ctx, owner, mcpID, listed); err != nil {
			return fmt.Errorf("delete the preconfigured OAuth grant of vMCP %q for component %q: %w", vmcp.Name, componentID, err)
		}
		result = vmcp
		return nil
	})
	return result, revoked, err
}

// DeleteRemovedComponentGrants deletes the grants recorded for components that vmcp's manifest no longer has, by their
// recorded IDs, and removes their records from vmcp's status. It reports whether it changed the status, which the
// caller saves.
//
// Grants are deleted before their records, so a failure at any point is retried with the records still in place. A
// grant is deleted only by an ID recorded for it: if vmcp is stale, and a component with the same ID was added back and
// authorized again since, that grant has an ID vmcp does not record, and is kept; saving the stale status then fails
// with a conflict.
func (s *PreconfiguredOAuthStore) DeleteRemovedComponentGrants(ctx context.Context, vmcp *v1.VMCP) (bool, error) {
	var components []string
	for _, component := range ComponentsForInstance(*vmcp, v1.VMCPInstance{}) {
		components = append(components, component.ID)
	}
	owner := principal.PreconfiguredOAuthOwner(vmcp.Name)
	var kept []v1.VMCPPreconfiguredOAuthGrant
	for _, record := range vmcp.Status.PreconfiguredOAuth {
		if slices.Contains(components, record.ComponentID) {
			kept = append(kept, record)
			continue
		}
		mcpID := PreconfiguredOAuthMCPID(vmcp.Name, record.ComponentID)
		// A grant is recorded before it is stored, so one is recorded as deleted only if one was stored.
		var stored int
		if s.removed != nil {
			if grants, err := s.gateway.MCPOAuthGrantMetadata(ctx, []string{owner}, []string{mcpID}); err != nil {
				slog.Warn("Failed to read the preconfigured OAuth grant about to be deleted, so its deletion is not audited", "vmcp", vmcp.Name, "component", record.ComponentID, "error", err)
			} else {
				stored = len(grants)
			}
		}
		if err := s.gateway.DeleteMCPOAuthGrants(ctx, owner, mcpID, record.GrantIDs); err != nil {
			return false, fmt.Errorf("delete the preconfigured OAuth grant of removed component %q of vMCP %q: %w", record.ComponentID, vmcp.Name, err)
		}
		slog.Info("Deleted the preconfigured OAuth grant of a removed vMCP component", "vmcp", vmcp.Name, "component", record.ComponentID)
		if stored > 0 {
			s.removed(ctx, vmcp.Name, record.ComponentID, "component removed")
		}
	}
	if len(kept) == len(vmcp.Status.PreconfiguredOAuth) {
		return false, nil
	}
	vmcp.Status.PreconfiguredOAuth = kept
	return true, nil
}

// DeleteVMCPGrants deletes every grant of vmcp, which is being deleted, with one statement. Deleting what is already
// gone succeeds, so the vMCP's finalizer can call it again after any failure.
func (s *PreconfiguredOAuthStore) DeleteVMCPGrants(ctx context.Context, vmcp v1.VMCP) error {
	owner := principal.PreconfiguredOAuthOwner(vmcp.Name)
	// Only the components that hold a grant are recorded, so a vMCP that never had one adds nothing to the audit. A
	// failure to find them is not a failure to delete.
	var held []string
	if s.removed != nil {
		var mcpIDs []string
		for _, record := range vmcp.Status.PreconfiguredOAuth {
			mcpIDs = append(mcpIDs, PreconfiguredOAuthMCPID(vmcp.Name, record.ComponentID))
		}
		for _, component := range ComponentsForInstance(vmcp, v1.VMCPInstance{}) {
			if mcpID := PreconfiguredOAuthMCPID(vmcp.Name, component.ID); !slices.Contains(mcpIDs, mcpID) {
				mcpIDs = append(mcpIDs, mcpID)
			}
		}
		if len(mcpIDs) > 0 {
			stored, err := s.gateway.MCPOAuthGrantMetadata(ctx, []string{owner}, mcpIDs)
			if err != nil {
				slog.Warn("Failed to find the preconfigured OAuth grants about to be deleted, so their deletion is not audited", "vmcp", vmcp.Name, "error", err)
			}
			for _, grant := range stored {
				held = append(held, strings.TrimPrefix(grant.MCPID, vmcp.Name+"/"))
			}
		}
	}
	if err := s.gateway.DeleteMCPOAuthGrantsForOwner(ctx, owner); err != nil {
		return fmt.Errorf("delete the preconfigured OAuth grants of vMCP %q: %w", vmcp.Name, err)
	}
	for _, componentID := range held {
		s.removed(ctx, vmcp.Name, componentID, "vMCP deleted")
	}
	return nil
}

// recordedGrantIDs returns the grant IDs vmcp's status records for a component.
func recordedGrantIDs(vmcp *v1.VMCP, componentID string) []string {
	for _, record := range vmcp.Status.PreconfiguredOAuth {
		if record.ComponentID == componentID {
			return record.GrantIDs
		}
	}
	return nil
}

// recordGrantIDs records grantIDs as those that may be stored for a component, removing the record when there are
// none, and reports whether that changed the status.
func recordGrantIDs(vmcp *v1.VMCP, componentID string, grantIDs []string) bool {
	index := slices.IndexFunc(vmcp.Status.PreconfiguredOAuth, func(record v1.VMCPPreconfiguredOAuthGrant) bool {
		return record.ComponentID == componentID
	})
	switch {
	case index < 0 && len(grantIDs) == 0:
		return false
	case index < 0:
		vmcp.Status.PreconfiguredOAuth = append(vmcp.Status.PreconfiguredOAuth, v1.VMCPPreconfiguredOAuthGrant{ComponentID: componentID, GrantIDs: grantIDs})
		return true
	case len(grantIDs) == 0:
		vmcp.Status.PreconfiguredOAuth = slices.Delete(vmcp.Status.PreconfiguredOAuth, index, index+1)
		if len(vmcp.Status.PreconfiguredOAuth) == 0 {
			vmcp.Status.PreconfiguredOAuth = nil
		}
		return true
	case slices.Equal(vmcp.Status.PreconfiguredOAuth[index].GrantIDs, grantIDs):
		return false
	default:
		vmcp.Status.PreconfiguredOAuth[index].GrantIDs = grantIDs
		return true
	}
}

// readVMCP reads the vMCP with key, mapping its absence to a 404. A write of a grant must read it from storage, not a
// cache.
func readVMCP(ctx context.Context, client kclient.Client, key kclient.ObjectKey) (*v1.VMCP, error) {
	var vmcp v1.VMCP
	if err := client.Get(ctx, key, &vmcp); apierrors.IsNotFound(err) {
		return nil, types.NewErrNotFound("vMCP %q not found", key.Name)
	} else if err != nil {
		return nil, err
	}
	return &vmcp, nil
}

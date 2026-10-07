package setup

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/groupref"
	"github.com/obot-platform/obot/pkg/scim"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultPageSize is the size of each list in a review, and of a page when none is asked for.
	DefaultPageSize = 50
	// MaxPageSize bounds the page a caller can ask for.
	MaxPageSize = 200

	// GroupListBound selects the groups that SCIM manages.
	GroupListBound GroupList = "bound"
	// GroupListUnboundReferenced selects the referenced groups that the identity provider has not pushed.
	GroupListUnboundReferenced GroupList = "unboundReferenced"
	// GroupListUnreferenced selects the unbound groups that nothing references.
	GroupListUnreferenced GroupList = "unreferenced"

	warningEveryoneGroup = "everyoneGroup"
	warningMissingGroup  = "missingGroup"
	warningDuplicateName = "duplicateName"
	warningNamesake      = "unreferencedNamesake"

	// everyoneGroupName is the normalized name of the identity provider group that every user is in. Okta cannot
	// push it.
	everyoneGroupName = "everyone"
)

var (
	referenceKindLabels = map[groupref.Kind]string{
		groupref.KindAccessControlRule:     "access control rule",
		groupref.KindModelAccessPolicy:     "model access policy",
		groupref.KindSkillAccessRule:       "skill access rule",
		groupref.KindMessagePolicy:         "message policy",
		groupref.KindHostedAgentAccessRule: "hosted agent access rule",
		groupref.KindPublishedArtifact:     "published artifact",
		groupref.KindGroupRoleAssignment:   "group role assignment",
		groupref.KindVMCPProfile:           "virtual MCP server",
	}
)

// GroupList selects one of the lists of groups in a review.
type GroupList string

// AuthProviders reports which auth provider serves sign-ins, and which one is staged to replace it.
type AuthProviders interface {
	GetConfiguredAuthProvider(ctx context.Context) (string, error)
	GetStagedAuthProvider(ctx context.Context) (string, error)
}

// Actor is the user making a request, as far as SCIM administration cares.
type Actor struct {
	UserID                uint
	AuthProviderNamespace string
	AuthProviderName      string
	Owner                 bool
	Bootstrap             bool
}

// Page selects a page of a list. Offset is zero-based.
type Page struct {
	Offset int
	Limit  int
}

// Service runs the administration of SCIM connections: enabling SCIM for an auth provider that synchronizes its
// directory at sign-in, reviewing a connection, deleting its auth provider's unreferenced groups, enforcing it, and
// managing its bearer token.
type Service struct {
	gateway   *gclient.Client
	storage   kclient.Reader
	providers AuthProviders
	finder    *groupref.Finder
	serverURL string
}

// provider is the auth provider of a SCIM connection.
type provider struct {
	namespace     string
	name          string
	displayName   string
	groupIDPrefix string
	adapter       adapter.Adapter
	issuer        string
}

// groupPlan sorts a provider's groups by whether anything references them and whether SCIM has bound them.
type groupPlan struct {
	references groupref.References
	// referenced holds the IDs of the referenced groups, including referenced IDs that no group has.
	referenced          map[string]struct{}
	bound               []types2.SCIMSetupGroup
	unboundReferenced   []types2.SCIMSetupGroup
	unreferencedUnbound []types2.SCIMSetupGroup
	// duplicates are the names that more than one unbound referenced group has. A group pushed under such a name
	// could bind to neither.
	duplicates []types2.SCIMDuplicateGroupName
	// namesakes are the unbound groups that nothing references, and that have the name of an unbound referenced
	// group. A group pushed under that name could bind to neither until they are deleted.
	namesakes []types2.SCIMSetupGroup
	warnings  []types2.SCIMSetupWarning
}

// New returns the Service. storage must read without a cache, and serverURL is Obot's public URL, which SCIM base
// URLs are built from.
func New(gateway *gclient.Client, storage kclient.Reader, providers AuthProviders, serverURL string) *Service {
	return &Service{
		gateway:   gateway,
		storage:   storage,
		providers: providers,
		finder:    groupref.NewFinder(storage, gateway),
		serverURL: strings.TrimSuffix(serverURL, "/"),
	}
}

// NormalizePage bounds a requested page.
func NormalizePage(offset, limit int) Page {
	if limit <= 0 {
		limit = DefaultPageSize
	}
	return Page{
		Offset: max(offset, 0),
		Limit:  min(limit, MaxPageSize),
	}
}

// Connections returns every SCIM connection: zero or one.
func (s *Service) Connections(ctx context.Context) ([]types2.SCIMConnection, error) {
	conns, err := s.gateway.SCIMConnections(ctx)
	if err != nil {
		return nil, err
	}
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}

	result := make([]types2.SCIMConnection, 0, len(conns))
	for i := range conns {
		p, err := s.connectionProvider(ctx, &conns[i])
		if err != nil {
			return nil, err
		}
		result = append(result, s.connectionView(&conns[i], p, configured, ""))
	}
	return result, nil
}

// viewConnection returns the SCIM connection with the given ID, as the API serves it without a token.
func (s *Service) viewConnection(ctx context.Context, id string) (*types2.SCIMConnection, error) {
	conn, p, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	view := s.connectionView(conn, p, configured, "")
	return &view, nil
}

// Review reports the state of a SCIM connection, and what enforcing it would do. Each list holds its first page of
// pageSize items. actor is the requesting user, whose ability to sign in afterwards is one of Enforce's
// preconditions.
func (s *Service) Review(ctx context.Context, id string, actor Actor, pageSize int) (*types2.SCIMConnectionReview, error) {
	conn, p, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	plan, err := s.planGroups(ctx, p)
	if err != nil {
		return nil, err
	}

	page := NormalizePage(0, pageSize)
	provisioned, err := s.users(ctx, conn, true, page)
	if err != nil {
		return nil, err
	}
	unprovisioned, err := s.users(ctx, conn, false, page)
	if err != nil {
		return nil, err
	}
	failures, err := s.failures(ctx, conn.ID, page)
	if err != nil {
		return nil, err
	}

	review := &types2.SCIMConnectionReview{
		Connection:              s.connectionView(conn, p, configured, ""),
		ProvisionedUsers:        *provisioned,
		UnprovisionedUsers:      *unprovisioned,
		BoundGroups:             groupPage(plan.bound, page),
		UnboundReferencedGroups: groupPage(plan.unboundReferenced, page),
		UnreferencedGroups:      groupPage(plan.unreferencedUnbound, page),
		Warnings:                append(plan.warnings, nameWarnings(p, plan)...),
		EnforceBlockers:         []string{},
		Activity: types2.SCIMConnectionActivity{
			LastRequestAt:  optionalTime(conn.LastRequestAt),
			LastSuccessAt:  optionalTime(conn.LastSuccessAt),
			RecentFailures: *failures,
		},
	}

	if conn.State == types.SCIMConnectionStateConnected {
		if review.EnforceBlockers, err = s.enforceBlockers(ctx, conn, p, configured, actor); err != nil {
			return nil, err
		}
	}
	return review, nil
}

// Users returns a page of the connection's provisioned users, or of its auth provider's unprovisioned users.
func (s *Service) Users(ctx context.Context, id string, provisioned bool, page Page) (*types2.SCIMSetupUserPage, error) {
	conn, _, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.users(ctx, conn, provisioned, page)
}

// Groups returns a page of one of the lists of the connection's groups.
func (s *Service) Groups(ctx context.Context, id string, list GroupList, page Page) (*types2.SCIMSetupGroupPage, error) {
	_, p, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	plan, err := s.planGroups(ctx, p)
	if err != nil {
		return nil, err
	}

	var groups []types2.SCIMSetupGroup
	switch list {
	case GroupListBound:
		groups = plan.bound
	case GroupListUnboundReferenced:
		groups = plan.unboundReferenced
	case GroupListUnreferenced:
		groups = plan.unreferencedUnbound
	default:
		return nil, types2.NewErrBadRequest("unknown group list %q; use %q, %q, or %q", list, GroupListBound, GroupListUnboundReferenced, GroupListUnreferenced)
	}
	result := groupPage(groups, page)
	return &result, nil
}

// Failures returns a page of the connection's recent failed requests, newest first.
func (s *Service) Failures(ctx context.Context, id string, page Page) (*types2.SCIMRequestFailurePage, error) {
	conn, _, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.failures(ctx, conn.ID, page)
}

// Enforce requires a SCIM binding for sign-in with the connection's auth provider, permanently. It disables the
// provider's users that SCIM has not provisioned, and deletes its unbound groups that nothing references. It is
// refused unless the provider is the configured auth provider, every referenced group of the provider is bound, and
// actor is an Owner who signed in through the provider and is provisioned and active.
//
// The unreferenced groups are deleted in two phases, as withUnreferencedGroupsMarked describes. Enforcing keeps any
// marked group that gained a reference, and blocks if that group is unbound. It clears every mark for deletion, so it
// is refused with a conflict while another deletion of unreferenced groups holds marks.
func (s *Service) Enforce(ctx context.Context, id string, actor Actor) (*types2.SCIMEnforceResult, error) {
	conn, p, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	if conn.State == types.SCIMConnectionStateEnforced {
		return nil, types2.NewErrHTTP(http.StatusConflict, "SCIM is already enforced, and enforcement cannot be reversed")
	}
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}

	plan, err := s.planGroups(ctx, p)
	if err != nil {
		return nil, err
	}
	blockers, err := s.enforceBlockers(ctx, conn, p, configured, actor)
	if err != nil {
		return nil, err
	}
	for _, group := range plan.unboundReferenced {
		blockers = append(blockers, unboundGroupMessage(p, group))
	}
	if len(blockers) > 0 {
		return nil, types2.NewErrBadRequest("%s", blockedMessage("enforced", blockers))
	}

	// Enforcing finishes no other deletion of unreferenced groups, so it waits for one under way. The gateway checks
	// again once the groups are marked, in case one started meanwhile.
	if inProgress, err := s.gateway.SCIMGroupDeletionInProgress(ctx, conn.ID); err != nil {
		return nil, err
	} else if inProgress {
		return nil, groupDeletionInProgressError()
	}

	var result *gclient.EnforceSCIMResult
	if err := s.withUnreferencedGroupsMarked(ctx, conn, plan, func(run *gclient.SCIMDeletionRun) error {
		var err error
		result, err = s.enforceMarked(ctx, conn, p, run.ID, actor)
		return err
	}); errors.Is(err, gclient.ErrSCIMGroupDeletionInProgress) {
		return nil, groupDeletionInProgressError()
	} else if err != nil {
		return nil, err
	}

	slog.Info("Enforced SCIM", "connection", conn.ID, "authProvider", p.name, "disabledUsers", len(result.DisabledUserIDs), "deletedGroupIDs", result.DeletedGroupIDs)
	return &types2.SCIMEnforceResult{
		Connection:        s.connectionView(result.Connection, p, configured, ""),
		DisabledUserCount: len(result.DisabledUserIDs),
		DeletedGroupCount: len(result.DeletedGroupIDs),
	}, nil
}

// RotateToken issues a new bearer token for the connection, including its first token, and returns the connection
// with it. The token it replaces is still accepted for up to a day, but never past its own expiry, or until it is
// revoked.
func (s *Service) RotateToken(ctx context.Context, id string) (*types2.SCIMConnection, error) {
	return s.replaceToken(ctx, id, s.gateway.RotateSCIMConnectionToken)
}

// RevokeCurrentToken replaces a leaked bearer token: it issues a new token, which it returns with the connection,
// and stops accepting both the current and the previous token.
func (s *Service) RevokeCurrentToken(ctx context.Context, id string) (*types2.SCIMConnection, error) {
	return s.replaceToken(ctx, id, s.gateway.RevokeCurrentSCIMConnectionToken)
}

// replaceToken issues a new bearer token with replace, and returns the connection with it. A new token is shown only
// in this response, and replacing it again retires the token the identity provider still uses, so everything that
// can fail is read before the token is issued.
func (s *Service) replaceToken(ctx context.Context, id string, replace func(context.Context, string) (*types.SCIMConnection, string, error)) (*types2.SCIMConnection, error) {
	current, p, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	// Only the connection of the configured auth provider can serve the identity provider, so only it gets a token. A
	// connection that staging created stays tokenless until the provider is activated.
	if !providerConfigured(current, configured) {
		return nil, types2.NewErrBadRequest("%s is not the configured auth provider; SCIM tokens are issued only for the configured auth provider", p.displayName)
	}

	conn, token, err := replace(ctx, id)
	if err != nil {
		return nil, connectionError(id, err)
	}
	view := s.connectionView(conn, p, configured, token)
	return &view, nil
}

// RevokePreviousToken stops accepting the token that the last rotation replaced.
func (s *Service) RevokePreviousToken(ctx context.Context, id string) (*types2.SCIMConnection, error) {
	if err := s.gateway.RevokePreviousSCIMConnectionToken(ctx, id); err != nil {
		return nil, connectionError(id, err)
	}
	return s.viewConnection(ctx, id)
}

// enforceMarked reads the references again, now that the deletion runID has marked the unreferenced groups, and
// enforces the connection.
func (s *Service) enforceMarked(ctx context.Context, conn *types.SCIMConnection, p *provider, runID string, actor Actor) (*gclient.EnforceSCIMResult, error) {
	plan, err := s.planGroups(ctx, p)
	if err != nil {
		return nil, err
	}

	// The configured auth provider lives in the controller store, so the transaction that enforces cannot check it.
	// Reading it again last leaves the least time for a switch away from the provider to go unnoticed.
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if !providerConfigured(conn, configured) {
		return nil, types2.NewErrBadRequest("%s", blockedMessage("enforced", []string{
			fmt.Sprintf("%s is not the configured auth provider.", p.displayName),
		}))
	}

	result, err := s.gateway.EnforceSCIMConnection(ctx, conn.ID, gclient.EnforceSCIMOptions{
		RunID:              runID,
		ReferencedGroupIDs: plan.referenced,
		Actor:              gatewayActor(actor),
	})
	if blocked, ok := errors.AsType[*gclient.SCIMEnforceBlockedError](err); ok {
		var blockers []string
		for _, group := range blocked.UnboundGroups {
			blockers = append(blockers, unboundGroupMessage(p, p.group(group, plan.references[group.ID])))
		}
		if blocked.ActorProblem != "" {
			blockers = append(blockers, s.actorProblemMessage(ctx, p, actor, blocked.ActorProblem))
		}
		return nil, types2.NewErrBadRequest("%s", blockedMessage("enforced", blockers))
	} else if state, ok := errors.AsType[*gclient.SCIMConnectionStateError](err); ok {
		return nil, types2.NewErrHTTP(http.StatusConflict, state.Error())
	} else if err != nil {
		return nil, connectionError(conn.ID, err)
	}
	return result, nil
}

// enforceBlockers returns the reasons actor cannot enforce the connection now, other than its unbound referenced
// groups, which a review lists apart.
func (s *Service) enforceBlockers(ctx context.Context, conn *types.SCIMConnection, p *provider, configured string, actor Actor) ([]string, error) {
	blockers := []string{}
	switch {
	case actor.Bootstrap:
		blockers = append(blockers, "Only an Owner who signed in through "+p.displayName+" can enforce SCIM. The bootstrap user cannot.")
	case !actor.Owner:
		blockers = append(blockers, "Only an Owner who signed in through "+p.displayName+" can enforce SCIM.")
	}
	if !providerConfigured(conn, configured) {
		blockers = append(blockers, fmt.Sprintf("%s is not the configured auth provider.", p.displayName))
	}
	// Whether the acting user could sign in afterwards matters only to an Owner, who could enforce.
	if actor.Owner && !actor.Bootstrap {
		problem, err := s.gateway.CheckSCIMEnforceActor(ctx, conn, gatewayActor(actor))
		if err != nil {
			return nil, err
		}
		if problem != "" {
			blockers = append(blockers, s.actorProblemMessage(ctx, p, actor, problem))
		}
	}
	return blockers, nil
}

func (s *Service) users(ctx context.Context, conn *types.SCIMConnection, provisioned bool, page Page) (*types2.SCIMSetupUserPage, error) {
	list := s.gateway.SCIMUnprovisionedUsers
	if provisioned {
		list = s.gateway.SCIMProvisionedUsers
	}
	users, total, err := list(ctx, conn, gclient.SCIMPage{
		Offset: page.Offset,
		Limit:  page.Limit,
	})
	if err != nil {
		return nil, err
	}

	result := &types2.SCIMSetupUserPage{
		Items: make([]types2.SCIMSetupUser, 0, len(users)),
		Total: total,
	}
	for _, user := range users {
		status := types2.UserStatusActive
		if user.DisabledAt != nil {
			status = types2.UserStatusDisabled
		}
		result.Items = append(result.Items, types2.SCIMSetupUser{
			ID:             fmt.Sprint(user.UserID),
			Username:       user.Username,
			Email:          user.Email,
			DisplayName:    user.DisplayName,
			Status:         status,
			DisabledReason: string(user.DisabledReason),
			SCIMID:         user.SCIMID,
			Active:         user.Active,
			SignedIn:       user.SignedIn,
		})
	}
	return result, nil
}

func (s *Service) failures(ctx context.Context, connectionID string, page Page) (*types2.SCIMRequestFailurePage, error) {
	failures, total, err := s.gateway.SCIMRequestFailurePage(ctx, connectionID, gclient.SCIMPage{
		Offset: page.Offset,
		Limit:  page.Limit,
	})
	if err != nil {
		return nil, err
	}

	result := &types2.SCIMRequestFailurePage{
		Items: make([]types2.SCIMRequestFailure, 0, len(failures)),
		Total: total,
	}
	for _, failure := range failures {
		result.Items = append(result.Items, types2.SCIMRequestFailure{
			Time:     *types2.NewTime(failure.CreatedAt),
			Method:   failure.Method,
			Resource: failure.Resource,
			Status:   failure.Status,
			SCIMType: failure.SCIMType,
			Detail:   failure.Detail,
		})
	}
	return result, nil
}

func (s *Service) connection(ctx context.Context, id string) (*types.SCIMConnection, *provider, error) {
	conn, err := s.gateway.SCIMConnection(ctx, id)
	if err != nil {
		return nil, nil, connectionError(id, err)
	}
	p, err := s.connectionProvider(ctx, conn)
	if err != nil {
		return nil, nil, err
	}
	return conn, p, nil
}

// connectionProvider returns the provider of a connection, whose persisted adapter type, group ID prefix, and
// issuer never change.
func (s *Service) connectionProvider(ctx context.Context, conn *types.SCIMConnection) (*provider, error) {
	a, ok := adapter.Lookup(conn.AdapterType)
	if !ok {
		return nil, fmt.Errorf("SCIM connection %s has unknown adapter type %q", conn.ID, conn.AdapterType)
	}

	name := conn.AuthProviderName
	var authProvider v1.AuthProvider
	if err := s.storage.Get(ctx, kclient.ObjectKey{Namespace: conn.AuthProviderNamespace, Name: conn.AuthProviderName}, &authProvider); err == nil {
		name = displayName(authProvider)
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get auth provider %q: %w", conn.AuthProviderName, err)
	}

	return &provider{
		namespace:     conn.AuthProviderNamespace,
		name:          conn.AuthProviderName,
		displayName:   name,
		groupIDPrefix: conn.GroupIDPrefix,
		adapter:       a,
		issuer:        conn.Issuer,
	}, nil
}

func (s *Service) connectionView(conn *types.SCIMConnection, p *provider, configured, token string) types2.SCIMConnection {
	view := types2.SCIMConnection{
		ID:                      conn.ID,
		AdapterType:             conn.AdapterType,
		Origin:                  string(conn.Origin),
		AuthProviderNamespace:   conn.AuthProviderNamespace,
		AuthProviderName:        conn.AuthProviderName,
		AuthProviderDisplayName: p.displayName,
		State:                   string(conn.State),
		BaseURL:                 scim.BaseURL(s.serverURL),
		Issuer:                  conn.Issuer,
		EnabledAt:               *types2.NewTime(conn.EnabledAt),
		EnforcedAt:              optionalTime(conn.EnforcedAt),
		HasToken:                conn.HasToken(),
		TokenIssuedAt:           optionalTime(conn.TokenIssuedAt),
		TokenExpiresAt:          optionalTime(conn.TokenExpiresAt()),
		PreviousTokenAccepted:   conn.PreviousTokenAccepted(time.Now()),
		AuthProviderConfigured:  providerConfigured(conn, configured),
		Token:                   token,
	}
	if view.PreviousTokenAccepted {
		view.PreviousTokenExpiresAt = optionalTime(conn.PreviousTokenExpiresAt)
	}
	return view
}

// planGroups reads the provider's groups and every reference to them, and to any other group ID with the
// provider's prefix.
func (s *Service) planGroups(ctx context.Context, p *provider) (*groupPlan, error) {
	return planProviderGroups(ctx, s.gateway, s.finder, p)
}

// planProviderGroups is planGroups for callers without a Service.
func planProviderGroups(ctx context.Context, gateway *gclient.Client, finder *groupref.Finder, p *provider) (*groupPlan, error) {
	groups, err := gateway.SCIMProviderGroups(ctx, p.namespace, p.name)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]struct{}, len(groups))
	for _, group := range groups {
		ids[group.ID] = struct{}{}
	}

	refs, err := finder.Find(ctx, p.namespace, func(groupID string) bool {
		_, ok := ids[groupID]
		return ok || strings.HasPrefix(groupID, p.groupIDPrefix)
	})
	if err != nil {
		return nil, err
	}

	plan := &groupPlan{
		references:          refs,
		referenced:          make(map[string]struct{}, len(refs)),
		bound:               []types2.SCIMSetupGroup{},
		unboundReferenced:   []types2.SCIMSetupGroup{},
		unreferencedUnbound: []types2.SCIMSetupGroup{},
		duplicates:          []types2.SCIMDuplicateGroupName{},
		warnings:            []types2.SCIMSetupWarning{},
	}
	for groupID := range refs {
		plan.referenced[groupID] = struct{}{}
	}

	var (
		names []string
		named = make(map[string][]types2.SCIMSetupGroup, len(groups))
	)
	for _, group := range groups {
		groupRefs, referenced := refs[group.ID]
		setupGroup := p.group(group, groupRefs)

		switch {
		case group.SCIMID != "":
			plan.bound = append(plan.bound, setupGroup)
		case referenced:
			plan.unboundReferenced = append(plan.unboundReferenced, setupGroup)

			normalized := types.NormalizeSCIMGroupName(group.Name)
			if _, ok := named[normalized]; !ok {
				names = append(names, normalized)
			}
			named[normalized] = append(named[normalized], setupGroup)

			if normalized == everyoneGroupName {
				plan.warnings = append(plan.warnings, types2.SCIMSetupWarning{
					Type:       warningEveryoneGroup,
					Message:    fmt.Sprintf("The group %q cannot be pushed from %s. Replace its references with the all-users selector.", group.Name, p.displayName),
					GroupID:    group.ID,
					GroupName:  group.Name,
					References: setupGroup.References,
				})
			}
		default:
			plan.unreferencedUnbound = append(plan.unreferencedUnbound, setupGroup)
		}
	}

	for _, name := range names {
		if duplicates := named[name]; len(duplicates) > 1 {
			plan.duplicates = append(plan.duplicates, types2.SCIMDuplicateGroupName{
				Name:   strings.TrimSpace(duplicates[0].Name),
				Groups: duplicates,
			})
		}
	}
	for _, group := range plan.unreferencedUnbound {
		if _, ok := named[types.NormalizeSCIMGroupName(group.Name)]; ok {
			plan.namesakes = append(plan.namesakes, group)
		}
	}

	for _, groupID := range slices.Sorted(maps.Keys(refs)) {
		if _, ok := ids[groupID]; ok {
			continue
		}
		groupRefs := references(refs[groupID])
		plan.warnings = append(plan.warnings, types2.SCIMSetupWarning{
			Type:       warningMissingGroup,
			Message:    missingGroupMessage(groupID, groupRefs),
			GroupID:    groupID,
			References: groupRefs,
		})
	}

	return plan, nil
}

// nameWarnings warns about the names under which a pushed group can bind to none of the provider's unbound
// referenced groups. Enabling SCIM refuses the first kind, and deletes the groups of the second, so only a review of a
// connection has them.
func nameWarnings(p *provider, plan *groupPlan) []types2.SCIMSetupWarning {
	warnings := make([]types2.SCIMSetupWarning, 0, len(plan.duplicates)+len(plan.namesakes))
	for _, duplicate := range plan.duplicates {
		warnings = append(warnings, types2.SCIMSetupWarning{
			Type: warningDuplicateName,
			Message: fmt.Sprintf("%d referenced groups are named %q, so a group pushed from %s under that name binds to neither. "+
				"Remove the references to all but one of them, and then delete the unreferenced groups.",
				len(duplicate.Groups), duplicate.Name, p.displayName),
			GroupID:   duplicate.Groups[0].ID,
			GroupName: duplicate.Name,
		})
	}
	for _, group := range plan.namesakes {
		warnings = append(warnings, types2.SCIMSetupWarning{
			Type: warningNamesake,
			Message: fmt.Sprintf("The unreferenced group %q has the name of a referenced group, so a group pushed from %s under that name binds to neither. Delete the unreferenced groups.",
				group.Name, p.displayName),
			GroupID:   group.ID,
			GroupName: group.Name,
		})
	}
	return warnings
}

func (p *provider) group(group gclient.SCIMProviderGroup, refs []groupref.Reference) types2.SCIMSetupGroup {
	nativeID := p.adapter.NativeGroupID(p.groupIDPrefix, group.ID)
	return types2.SCIMSetupGroup{
		ID:         group.ID,
		Name:       group.Name,
		NativeID:   nativeID,
		ConsoleURL: p.adapter.GroupConsoleURL(p.issuer, nativeID),
		SCIMID:     group.SCIMID,
		References: references(refs),
	}
}

// providerConfigured reports whether the connection's auth provider is the configured auth provider.
func providerConfigured(conn *types.SCIMConnection, configured string) bool {
	return configured != "" && conn.AuthProviderName == configured && conn.AuthProviderNamespace == system.DefaultNamespace
}

func gatewayActor(actor Actor) gclient.SCIMEnforceActor {
	return gclient.SCIMEnforceActor{
		UserID:                actor.UserID,
		AuthProviderNamespace: actor.AuthProviderNamespace,
		AuthProviderName:      actor.AuthProviderName,
	}
}

func groupPage(groups []types2.SCIMSetupGroup, page Page) types2.SCIMSetupGroupPage {
	start := min(page.Offset, len(groups))
	end := min(start+page.Limit, len(groups))
	return types2.SCIMSetupGroupPage{
		// Never nil, so that an empty page lists no items rather than null.
		Items: append([]types2.SCIMSetupGroup{}, groups[start:end]...),
		Total: int64(len(groups)),
	}
}

func references(refs []groupref.Reference) []types2.GroupReference {
	if len(refs) == 0 {
		return nil
	}
	result := make([]types2.GroupReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, types2.GroupReference{
			Kind:        string(ref.Kind),
			ID:          ref.Name,
			DisplayName: ref.DisplayName,
			Detail:      ref.Detail,
		})
	}
	return result
}

// actorProblemMessage explains what keeps actor from enforcing SCIM. A message about the actor's account names it,
// so that the administrator can find it in the identity provider.
func (s *Service) actorProblemMessage(ctx context.Context, p *provider, actor Actor, problem gclient.SCIMEnforceActorProblem) string {
	if problem == gclient.SCIMEnforceActorOtherAuthProvider {
		return fmt.Sprintf("Sign in through %s to enforce SCIM, so that you are known to be able to sign in once it is enforced.", p.displayName)
	}

	account := "Your account"
	// The user may have been deleted since the check of their account, which still names them. The name only helps
	// the administrator find the account, so a user that cannot be read leaves it out rather than failing.
	if user, err := s.gateway.UserByIDIncludeDeleted(ctx, strconv.FormatUint(uint64(actor.UserID), 10)); err != nil {
		slog.Warn("Failed to get the user that cannot enforce SCIM", "userID", actor.UserID, "error", err)
	} else if name := accountName(user); name != "" {
		account += ", " + name + ","
	}

	switch problem {
	case gclient.SCIMEnforceActorNotSignedIn:
		return fmt.Sprintf("%s has not signed in through %s, so it is not known to be able to sign in once SCIM is enforced.", account, p.displayName)
	case gclient.SCIMEnforceActorUnprovisioned:
		return fmt.Sprintf("%s has not been provisioned through SCIM. Assign yourself to the SCIM application in %s.", account, p.displayName)
	case gclient.SCIMEnforceActorDeactivated:
		return fmt.Sprintf("%s is deactivated in %s, so you could not sign in once SCIM is enforced.", account, p.displayName)
	default:
		return account + " is not active in Obot."
	}
}

// accountName names a user's account by its username and email address, as in "alice (alice@example.com)", or by
// only one of them when they are the same or the other is empty. It is empty when the user has neither.
func accountName(user *types.User) string {
	switch {
	case user.Email == "":
		return user.Username
	case user.Username == "" || strings.EqualFold(user.Username, user.Email):
		return user.Email
	default:
		return fmt.Sprintf("%s (%s)", user.Username, user.Email)
	}
}

func unboundGroupMessage(p *provider, group types2.SCIMSetupGroup) string {
	return fmt.Sprintf("The referenced group %s has not been pushed from %s. Push it, renaming it there first if its name differs, or remove its references.",
		describeGroup(group), p.displayName)
}

// blockedMessage lists everything that blocks the SCIM transition named by its past participle, such as "enforced".
func blockedMessage(transition string, blockers []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "SCIM cannot be %s:", transition)
	for _, blocker := range blockers {
		b.WriteString("\n- ")
		b.WriteString(blocker)
	}
	return b.String()
}

// describeGroup names a group, where to find it in the identity provider, and what references it.
func describeGroup(group types2.SCIMSetupGroup) string {
	var b strings.Builder
	if group.Name != "" {
		fmt.Fprintf(&b, "%q (%s", group.Name, group.ID)
	} else {
		fmt.Fprintf(&b, "(%s", group.ID)
	}
	if group.ConsoleURL != "" {
		fmt.Fprintf(&b, ", %s", group.ConsoleURL)
	}
	b.WriteString(")")
	if len(group.References) > 0 {
		descriptions := make([]string, 0, len(group.References))
		for _, ref := range group.References {
			descriptions = append(descriptions, describeReference(ref))
		}
		fmt.Fprintf(&b, ", referenced by %s", strings.Join(descriptions, "; "))
	}
	return b.String()
}

// missingGroupMessage warns that a group that no longer exists is still referenced. Deleting a group removes it from
// access policies and group role assignments, but not from the profiles of virtual MCP servers, which must keep at
// least one subject, so those are usually what is left.
func missingGroupMessage(groupID string, refs []types2.GroupReference) string {
	if !slices.ContainsFunc(refs, func(ref types2.GroupReference) bool {
		return groupref.Kind(ref.Kind) != groupref.KindVMCPProfile
	}) {
		profiles := make([]string, 0, len(refs))
		for _, ref := range refs {
			// The detail of a profile reference is "profile <name>".
			profiles = append(profiles, fmt.Sprintf("%s (%s)", strings.TrimPrefix(ref.Detail, "profile "), cmp.Or(ref.DisplayName, ref.ID)))
		}
		return fmt.Sprintf("The group with ID %s was removed but is still referenced by the following vMCP profiles: %s", groupID, strings.Join(profiles, ", "))
	}

	descriptions := make([]string, 0, len(refs))
	for _, ref := range refs {
		descriptions = append(descriptions, describeReference(ref))
	}
	return fmt.Sprintf("The group with ID %s was removed but is still referenced by the following: %s", groupID, strings.Join(descriptions, "; "))
}

func describeReference(ref types2.GroupReference) string {
	label := cmp.Or(referenceKindLabels[groupref.Kind(ref.Kind)], ref.Kind)
	if groupref.Kind(ref.Kind) == groupref.KindGroupRoleAssignment {
		if ref.Detail != "" {
			return fmt.Sprintf("%s (%s)", label, ref.Detail)
		}
		return label
	}

	description := label
	if ref.DisplayName != "" {
		description += fmt.Sprintf(" %q (%s)", ref.DisplayName, ref.ID)
	} else {
		description += " " + ref.ID
	}
	if ref.Detail != "" {
		description += ", " + ref.Detail
	}
	return description
}

// groupDeletionInProgressError refuses to enforce SCIM while another deletion of unreferenced groups is under way.
func groupDeletionInProgressError() error {
	return types2.NewErrHTTP(http.StatusConflict, "unreferenced groups are being deleted; try again once that finishes")
}

func connectionError(id string, err error) error {
	if errors.Is(err, gclient.ErrSCIMConnectionNotFound) {
		return types2.NewErrNotFound("SCIM connection %q not found", id)
	}
	return err
}

func optionalTime(t *time.Time) *types2.Time {
	if t == nil {
		return nil
	}
	return types2.NewTime(*t)
}

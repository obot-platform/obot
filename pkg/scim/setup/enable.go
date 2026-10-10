package setup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/groupref"
	"github.com/obot-platform/obot/pkg/i18n"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// EnableBlockedError reports why SCIM cannot be enabled for an auth provider.
type EnableBlockedError struct {
	Blockers   []string
	Duplicates []types2.SCIMDuplicateGroupName
	Locale     string
}

// enablePlan is what blocks enabling SCIM for an auth provider.
type enablePlan struct {
	// provider is nil when the auth provider does not support SCIM.
	provider *provider
	// duplicates are the names that more than one referenced group of the provider has. They are only planned when
	// the provider supports SCIM, and no SCIM connection exists.
	duplicates []types2.SCIMDuplicateGroupName
	blockers   []string
}

func (e *EnableBlockedError) Error() string {
	var b strings.Builder
	b.WriteString(blockedMessageLocale(e.Locale, "enabled", e.Blockers))
	for _, duplicate := range e.Duplicates {
		fmt.Fprintf(&b, "\n\n%s", i18n.Message(e.Locale, "scim_groups_named", map[string]string{"name": fmt.Sprintf("%q", duplicate.Name)}))
		for _, group := range duplicate.Groups {
			b.WriteString("\n- ")
			b.WriteString(describeGroupLocale(e.Locale, group))
		}
	}
	return b.String()
}

// EnablePreview reports the configured auth provider that SCIM can be enabled for, and what blocks enabling it.
func (s *Service) EnablePreview(ctx context.Context) (*types2.SCIMEnablePreview, error) {
	preview := &types2.SCIMEnablePreview{
		Blockers:            []string{},
		DuplicateGroupNames: []types2.SCIMDuplicateGroupName{},
	}

	plan, err := s.configuredEnablePlan(ctx)
	if err != nil {
		return nil, err
	}
	if plan == nil {
		preview.Blockers = append(preview.Blockers, message(ctx, "scim_no_provider"))
		return preview, nil
	}
	if plan.provider == nil {
		preview.Blockers = append(preview.Blockers, plan.blockers...)
		return preview, nil
	}
	preview.AuthProviderNamespace = plan.provider.namespace
	preview.AuthProviderName = plan.provider.name
	preview.AuthProviderDisplayName = plan.provider.displayName

	// Enabling runs as a provider configuration change, which checks these again under the serialization of such
	// changes.
	staged, err := s.providers.GetStagedAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get staged auth provider: %w", err)
	}
	if staged != "" {
		preview.Blockers = append(preview.Blockers, message(ctx, "scim_switch_staged",
			"provider", authProviderDisplayName(ctx, s.storage, system.DefaultNamespace, staged)))
	}
	if err := s.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: system.ProviderChangeAuthName}, new(v1.ProviderConfigurationChange)); err == nil {
		preview.Blockers = append(preview.Blockers, message(ctx, "scim_change_in_progress"))
	} else if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to check for auth provider configuration changes: %w", err)
	}
	preview.Blockers = append(preview.Blockers, plan.blockers...)

	if len(plan.duplicates) > 0 {
		preview.DuplicateGroupNames = plan.duplicates
	}
	return preview, nil
}

// CheckEnable returns the namespace and name of the configured auth provider when SCIM can be enabled for it, and
// otherwise a bad request that lists everything that blocks enabling it. The provider configuration change that
// enables SCIM checks it all again under the serialization of such changes.
func (s *Service) CheckEnable(ctx context.Context) (string, string, error) {
	preview, err := s.EnablePreview(ctx)
	if err != nil {
		return "", "", err
	}
	if len(preview.Blockers) > 0 {
		return "", "", types2.NewErrBadRequest("%s", (&EnableBlockedError{
			Blockers:   preview.Blockers,
			Duplicates: preview.DuplicateGroupNames,
			Locale:     i18n.FromContext(ctx),
		}).Error())
	}
	return preview.AuthProviderNamespace, preview.AuthProviderName, nil
}

// CompleteEnable finishes enabling SCIM for an auth provider, once a provider configuration change created its
// connection with EnableConnection: it deletes the provider's groups that nothing references, and then issues the
// connection's first bearer token.
//
// The token cannot be retrieved again, so it is issued last, and nothing that can fail or take long runs between
// issuing and returning it. A request that ends before then leaves a connection without a token, whose token an Owner
// generates on the SCIM sub-tab. For the same reason, a failed deletion is reported in the result rather than failing it.
// The deletion can be retried.
//
// It returns a conflict when the connection has a token already, because SCIM was enabled before, or a concurrent
// request or an Owner was given the token, and when the connection was not created by enabling SCIM.
func (s *Service) CompleteEnable(ctx context.Context, namespace, name string) (*types2.SCIMEnableResult, error) {
	conn, err := s.gateway.SCIMConnectionForAuthProvider(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, fmt.Errorf("SCIM was not enabled: auth provider %q has no SCIM connection", name)
	}
	p, err := s.connectionProvider(ctx, conn)
	if err != nil {
		return nil, err
	}
	if conn.Origin != types.SCIMConnectionOriginMigrated {
		return nil, types2.NewErrHTTP(http.StatusConflict, message(ctx, "scim_wrong_origin", "provider", p.displayName))
	}

	// Login-time synchronization stopped when the connection was created, so no sign-in can recreate a deleted
	// group. The deletion runs even when an Owner generated the token meanwhile, because deletions of the same
	// groups never interfere.
	result := new(types2.SCIMEnableResult)
	deleted, err := s.deleteUnreferencedGroups(ctx, conn, p)
	if err != nil {
		slog.Error("Failed to delete unreferenced groups after enabling SCIM", "connection", conn.ID, "authProvider", p.name, "error", err)
		result.DeletionError = message(ctx, "scim_deletion_error", "provider", p.displayName)
	}
	result.DeletedGroupCount = len(deleted)

	id := conn.ID
	conn, token, err := s.gateway.IssueFirstSCIMConnectionToken(ctx, id)
	if errors.Is(err, gclient.ErrSCIMConnectionHasToken) {
		return nil, types2.NewErrHTTP(http.StatusConflict, message(ctx, "scim_already_enabled", "provider", p.displayName))
	} else if err != nil {
		return nil, connectionError(ctx, id, err)
	}
	// The connection was created for the configured auth provider, under the serialization of provider
	// configuration changes, so it is not looked up again before the token is returned.
	result.Connection = s.connectionView(conn, p, conn.AuthProviderName, token)

	// The deleted groups are logged, as nothing else records them.
	slog.Info("Completed enabling SCIM", "connection", conn.ID, "authProvider", p.name, "deletedGroupIDs", deleted)
	return result, nil
}

// DeleteUnreferencedGroups deletes the unbound groups of the connection's auth provider that nothing references. Such
// groups grant nothing, so their deletion triggers no cleanup of the resources of their former members.
func (s *Service) DeleteUnreferencedGroups(ctx context.Context, id string) (*types2.SCIMGroupDeletionResult, error) {
	conn, p, err := s.connection(ctx, id)
	if err != nil {
		return nil, err
	}

	deleted, err := s.deleteUnreferencedGroups(ctx, conn, p)
	if err != nil {
		return nil, connectionError(ctx, conn.ID, err)
	}
	slog.Info("Deleted unreferenced groups of a SCIM connection", "connection", conn.ID, "authProvider", p.name, "deletedGroupIDs", deleted)
	return &types2.SCIMGroupDeletionResult{
		DeletedGroupCount: len(deleted),
	}, nil
}

// deleteUnreferencedGroups deletes the unbound groups of the connection's auth provider that nothing references, and
// returns their IDs.
func (s *Service) deleteUnreferencedGroups(ctx context.Context, conn *types.SCIMConnection, p *provider) ([]string, error) {
	plan, err := s.planGroups(ctx, p)
	if err != nil {
		return nil, err
	}

	var deleted []string
	err = s.withUnreferencedGroupsMarked(ctx, conn, plan, func(run *gclient.SCIMDeletionRun) error {
		if len(run.GroupIDs) == 0 {
			return nil
		}
		plan, err := s.planGroups(ctx, p)
		if err != nil {
			return err
		}
		deleted, err = s.gateway.DeleteMarkedSCIMGroups(ctx, conn.ID, run.ID, plan.referenced)
		return err
	})
	return deleted, err
}

// withUnreferencedGroupsMarked runs the first phase of a deletion of the unbound groups of the connection's auth
// provider that plan does not reference, and then finish, which reads the references again and deletes the groups
// that the run marked and nothing references then.
//
// The references live in the controller store, which cannot share a transaction with the gateway database, so the
// groups are deleted in two phases. They are first marked for deletion, which makes writers of group references
// refuse new references to them. Once the marks are committed, and the reference writes that could have missed them
// have finished, the references that finish reads are the last that can reach the marked groups, and a marked group
// that gained a reference in between is kept. If the deletion does not finish, its marks are cleared, so that
// references to its groups are accepted again.
func (s *Service) withUnreferencedGroupsMarked(ctx context.Context, conn *types.SCIMConnection, plan *groupPlan, finish func(run *gclient.SCIMDeletionRun) error) error {
	run, err := s.gateway.MarkUnreferencedSCIMGroups(ctx, conn, plan.referenced)
	if err == nil {
		err = finish(run)
	}
	if err != nil && run != nil && len(run.GroupIDs) > 0 {
		if clearErr := s.gateway.ClearSCIMGroupDeletionMarks(context.WithoutCancel(ctx), conn.ID, run.ID); clearErr != nil {
			slog.Error("Failed to clear the marks of a deletion of unreferenced groups that did not finish", "connection", conn.ID, "error", clearErr)
		}
	}
	return err
}

// EnableConnection creates the SCIM connection that permanently replaces the login-time directory synchronization of
// an auth provider, and returns it. The connection has no bearer token; CompleteEnable issues it. A provider that
// already has a connection created this way has it returned, so that a retried provider configuration change finds
// the connection an earlier attempt created.
//
// It is refused with *EnableBlockedError when the provider does not support SCIM, another connection exists, an auth
// provider cleanup is pending for the provider's name or group ID prefix, or two referenced groups of the provider
// have the same name. The caller must be serialized with provider configuration changes, which are the only creators
// of auth provider cleanups, and must have checked that the provider is the configured auth provider and that no
// replacement is staged. storage must read without a cache.
func EnableConnection(ctx context.Context, storage kclient.Reader, gateway *gclient.Client, authProvider v1.AuthProvider) (*types.SCIMConnection, error) {
	conn, err := gateway.SCIMConnectionForAuthProvider(ctx, authProvider.Namespace, authProvider.Name)
	if err != nil {
		return nil, err
	}
	if conn != nil && conn.Origin == types.SCIMConnectionOriginMigrated {
		return conn, nil
	}

	plan, err := planEnable(ctx, storage, gateway, groupref.NewFinder(storage, gateway), authProvider)
	if err != nil {
		return nil, err
	}
	if len(plan.blockers) > 0 {
		return nil, &EnableBlockedError{
			Blockers:   plan.blockers,
			Duplicates: plan.duplicates,
			Locale:     i18n.FromContext(ctx),
		}
	}

	conn, _, err = gateway.CreateSCIMConnection(ctx, gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: authProvider.Namespace,
		AuthProviderName:      authProvider.Name,
		GroupIDPrefix:         plan.provider.groupIDPrefix,
		Issuer:                plan.provider.issuer,
		Origin:                types.SCIMConnectionOriginMigrated,
	})
	if exists, ok := errors.AsType[*gclient.SCIMConnectionExistsError](err); ok {
		other, lookupErr := gateway.SCIMConnection(ctx, exists.ConnectionID)
		if lookupErr != nil {
			return nil, errors.Join(err, lookupErr)
		}
		return nil, &EnableBlockedError{
			Blockers: []string{
				otherConnectionBlocker(ctx, storage, other),
			},
		}
	} else if err != nil {
		return nil, err
	}
	return conn, nil
}

// configuredEnablePlan returns what blocks enabling SCIM for the configured auth provider, or nil when no auth
// provider is configured.
func (s *Service) configuredEnablePlan(ctx context.Context) (*enablePlan, error) {
	configured, err := s.providers.GetConfiguredAuthProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get configured auth provider: %w", err)
	}
	if configured == "" {
		return nil, nil
	}

	var authProvider v1.AuthProvider
	if err := s.storage.Get(ctx, kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: configured}, &authProvider); err != nil {
		return nil, fmt.Errorf("failed to get auth provider %q: %w", configured, err)
	}
	return planEnable(ctx, s.storage, s.gateway, s.finder, authProvider)
}

// planEnable returns what blocks enabling SCIM for an auth provider, except for what depends on other provider
// configuration changes: whether the provider is the configured one, and whether a replacement is staged.
func planEnable(ctx context.Context, storage kclient.Reader, gateway *gclient.Client, finder *groupref.Finder, authProvider v1.AuthProvider) (*enablePlan, error) {
	name := displayName(authProvider)
	a, ok := adapter.ForAuthProvider(authProvider.Name)
	if !ok || !adapter.SupportsSCIM(authProvider.Name, authProvider.Spec.AuthProviderManifest) {
		return &enablePlan{
			blockers: []string{
				message(ctx, "scim_unsupported", "provider", name),
			},
		}, nil
	}

	issuer, err := storedIssuer(ctx, gateway, authProvider)
	if err != nil {
		return nil, err
	}
	plan := &enablePlan{
		provider: &provider{
			namespace:     authProvider.Namespace,
			name:          authProvider.Name,
			displayName:   name,
			groupIDPrefix: authProvider.Spec.GroupIDPrefix,
			adapter:       a,
			issuer:        issuer,
		},
	}

	conns, err := gateway.SCIMConnections(ctx)
	if err != nil {
		return nil, err
	}
	for i := range conns {
		conn := &conns[i]
		if conn.AuthProviderNamespace == authProvider.Namespace && conn.AuthProviderName == authProvider.Name {
			plan.blockers = append(plan.blockers, message(ctx, "scim_already_provisions", "provider", name))
		} else {
			plan.blockers = append(plan.blockers, otherConnectionBlocker(ctx, storage, conn))
		}
	}
	if len(conns) > 0 {
		return plan, nil
	}

	if err := refusePendingCleanup(ctx, storage, authProvider); err != nil {
		if pending, ok := errors.AsType[*CleanupPendingError](err); ok {
			plan.blockers = append(plan.blockers, message(ctx, "scim_cleanup_pending", "provider", name, "cleanup", pending.CleanupName))
		} else {
			return nil, err
		}
	}

	groups, err := planProviderGroups(ctx, gateway, finder, plan.provider)
	if err != nil {
		return nil, err
	}
	plan.duplicates = groups.duplicates
	for _, duplicate := range plan.duplicates {
		plan.blockers = append(plan.blockers, message(ctx, "scim_duplicate_enable",
			"count", fmt.Sprint(len(duplicate.Groups)),
			"name", fmt.Sprintf("%q", duplicate.Name),
			"provider", name))
	}
	return plan, nil
}

// otherConnectionBlocker reports that another auth provider has the installation's only SCIM connection.
func otherConnectionBlocker(ctx context.Context, storage kclient.Reader, conn *types.SCIMConnection) string {
	return message(ctx, "scim_other_connection",
		"provider", authProviderDisplayName(ctx, storage, conn.AuthProviderNamespace, conn.AuthProviderName),
		"connection", conn.ID)
}

// authProviderDisplayName returns the name to show for an auth provider, or its name when it cannot be read.
func authProviderDisplayName(ctx context.Context, storage kclient.Reader, namespace, name string) string {
	var authProvider v1.AuthProvider
	if err := storage.Get(ctx, kclient.ObjectKey{Namespace: namespace, Name: name}, &authProvider); err != nil {
		return name
	}
	return displayName(authProvider)
}

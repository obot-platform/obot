package setup

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/groupref"
	"github.com/obot-platform/obot/pkg/scim/adapter"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// IneligibleError reports that an auth provider cannot be configured without directory credentials.
type IneligibleError struct {
	Message string
}

// ResidualGroupDataError reports that an auth provider cannot be configured without directory credentials, because
// it still has group data from an earlier configuration. Running the provider's auth provider cleanup removes it.
type ResidualGroupDataError struct {
	AuthProviderName        string
	AuthProviderDisplayName string
	Data                    types2.ResidualGroupData
}

func (e *IneligibleError) Error() string {
	return e.Message
}

func (e *ResidualGroupDataError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s still has group data from an earlier configuration, so it cannot be configured to provision users and groups through SCIM. "+
		"Remove it by deconfiguring %s, which runs its auth provider cleanup, and configure it again once the cleanup finishes, "+
		"or provide the directory credentials to synchronize its directory at sign-in instead.",
		e.AuthProviderDisplayName, e.AuthProviderDisplayName)
	if len(e.Data.Groups) > 0 {
		b.WriteString("\n\nGroups:")
		for _, group := range e.Data.Groups {
			b.WriteString("\n- ")
			b.WriteString(describeGroup(group))
		}
	}
	if e.Data.MembershipCount > 0 {
		fmt.Fprintf(&b, "\n\nGroup memberships: %d", e.Data.MembershipCount)
	}
	return b.String()
}

// ResidualGroupData returns what remains of an auth provider's groups from an earlier configuration: exactly the data
// that its auth provider cleanup removes. That is its groups; the memberships, group role assignments, and access
// policy subjects of group IDs with its group ID prefix. storage must read without a cache.
func ResidualGroupData(ctx context.Context, storage kclient.Reader, gateway *gclient.Client, authProvider v1.AuthProvider) (*types2.ResidualGroupData, error) {
	a, ok := adapter.ForAuthProvider(authProvider.Name)
	if !ok || !adapter.SupportsSCIM(authProvider.Name, authProvider.Spec.AuthProviderManifest) {
		return nil, &IneligibleError{
			Message: fmt.Sprintf("%s does not support SCIM provisioning", displayName(authProvider)),
		}
	}
	prefix := authProvider.Spec.GroupIDPrefix

	data, err := gateway.AuthProviderGroupData(ctx, authProvider.Namespace, authProvider.Name, prefix)
	if err != nil {
		return nil, err
	}

	refs, err := groupref.NewFinder(storage, gateway).Find(ctx, authProvider.Namespace, groupref.HasPrefix(prefix))
	if err != nil {
		return nil, err
	}
	// Cleanup never changes virtual MCP server profiles, which must keep a subject, so they are not residual data.
	for id := range refs {
		refs[id] = slices.DeleteFunc(refs[id], func(ref groupref.Reference) bool {
			return ref.Kind == groupref.KindVMCPProfile
		})
		if len(refs[id]) == 0 {
			delete(refs, id)
		}
	}

	p := &provider{
		namespace:     authProvider.Namespace,
		name:          authProvider.Name,
		displayName:   displayName(authProvider),
		groupIDPrefix: prefix,
		adapter:       a,
	}
	residual := &types2.ResidualGroupData{
		Groups:          make([]types2.SCIMSetupGroup, 0, len(data.Groups)+len(refs)),
		MembershipCount: data.MembershipCount,
	}
	for _, group := range data.Groups {
		residual.Groups = append(residual.Groups, p.group(group, refs[group.ID]))
		delete(refs, group.ID)
	}
	for _, id := range slices.Sorted(maps.Keys(refs)) {
		residual.Groups = append(residual.Groups, p.group(gclient.SCIMProviderGroup{ID: id}, refs[id]))
	}
	return residual, nil
}

// residualEmpty reports whether there is no residual group data.
func residualEmpty(data *types2.ResidualGroupData) bool {
	return len(data.Groups) == 0 && data.MembershipCount == 0
}

// EnsureSCIMFirstConnection makes sure that an auth provider being configured or staged without directory
// credentials has its SCIM connection, and returns it. A provider that already has one keeps it, which is also how
// a retried configuration change reuses the connection an earlier attempt created. Otherwise the connection is
// created in the connected state, with no token, before any sign-in through the provider can ask it for groups.
//
// It is refused with *IneligibleError, *CleanupPendingError, or *ResidualGroupDataError when the provider does not
// support SCIM, another connection exists, an auth provider cleanup is pending for the provider's name or group ID
// prefix, or the provider has residual group data. The caller must be serialized with provider configuration
// changes, which are the only creators of auth provider cleanups, and must not be the configured provider. storage
// must read without a cache, and config is the submitted configuration, which holds the issuer.
func EnsureSCIMFirstConnection(ctx context.Context, storage kclient.Reader, gateway *gclient.Client, authProvider v1.AuthProvider, config map[string]string) (*types.SCIMConnection, error) {
	a, ok := adapter.ForAuthProvider(authProvider.Name)
	if !ok || !adapter.SupportsSCIM(authProvider.Name, authProvider.Spec.AuthProviderManifest) {
		return nil, &IneligibleError{
			Message: fmt.Sprintf("%s does not support SCIM provisioning, so it needs every configuration parameter it requires", displayName(authProvider)),
		}
	}

	conn, err := gateway.SCIMConnectionForAuthProvider(ctx, authProvider.Namespace, authProvider.Name)
	if err != nil || conn != nil {
		return conn, err
	}

	if err := refusePendingCleanup(ctx, storage, authProvider); err != nil {
		return nil, err
	}

	// The access policies live in the controller store, which cannot share the gateway transaction that creates the
	// connection, so they are read first. The gateway data is checked again in that transaction.
	residual, err := ResidualGroupData(ctx, storage, gateway, authProvider)
	if err != nil {
		return nil, err
	}
	if !residualEmpty(residual) {
		return nil, &ResidualGroupDataError{
			AuthProviderName:        authProvider.Name,
			AuthProviderDisplayName: displayName(authProvider),
			Data:                    *residual,
		}
	}

	conn, _, err = gateway.CreateSCIMConnection(ctx, gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: authProvider.Namespace,
		AuthProviderName:      authProvider.Name,
		GroupIDPrefix:         authProvider.Spec.GroupIDPrefix,
		Issuer:                strings.TrimSuffix(strings.TrimSpace(config[a.IssuerConfigurationParameter()]), "/"),
		Origin:                types.SCIMConnectionOriginSCIMFirst,
		RequireNoGroupData:    true,
	})
	if exists, ok := errors.AsType[*gclient.SCIMConnectionExistsError](err); ok {
		return nil, &IneligibleError{
			Message: fmt.Sprintf("%s cannot provision users and groups through SCIM, because another auth provider has the installation's only SCIM connection (%s)", displayName(authProvider), exists.ConnectionID),
		}
	} else if _, ok := errors.AsType[*gclient.SCIMResidualGroupDataError](err); ok {
		// Group data appeared after it was listed.
		residual, listErr := ResidualGroupData(ctx, storage, gateway, authProvider)
		if listErr != nil {
			return nil, errors.Join(err, listErr)
		}
		return nil, &ResidualGroupDataError{
			AuthProviderName:        authProvider.Name,
			AuthProviderDisplayName: displayName(authProvider),
			Data:                    *residual,
		}
	} else if err != nil {
		return nil, err
	}
	return conn, nil
}

// displayName returns the name to show for an auth provider.
func displayName(authProvider v1.AuthProvider) string {
	return cmp.Or(authProvider.Spec.Name, authProvider.Name)
}

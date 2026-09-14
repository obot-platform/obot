package compositemigration

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type filterUpdateFailureClient struct {
	kclient.WithWatch
	fail bool
}

func TestMigrateCompositeFilters(t *testing.T) {
	entry := migrationEntry(t)
	parent := migrationParent(t, "ms1connection")
	entry.Finalizers = []string{"test-cleanup"}
	parent.Finalizers = []string{"test-cleanup"}
	parent.Spec.MCPCatalogID = "connection-catalog"
	target := types.Resource{
		Type: types.ResourceTypeMCPServer,
		ID:   migrationName(system.VMCPPrefix, entry.Namespace, entry.Name),
	}
	entryResource := types.Resource{Type: types.ResourceTypeMCPServerCatalogEntry, ID: entry.Name}
	serverResource := types.Resource{Type: types.ResourceTypeMCPServer, ID: parent.Name}
	for _, tc := range []struct {
		name      string
		resources []types.Resource
		namespace string
		disabled  bool
		wantAdded bool
	}{
		{
			name:      "catalog entry",
			resources: []types.Resource{entryResource},
			wantAdded: true,
		},
		{
			name:      "connection",
			resources: []types.Resource{serverResource},
			wantAdded: true,
		},
		{
			name:      "entry catalog",
			resources: []types.Resource{{Type: types.ResourceTypeMcpCatalog, ID: "default"}},
			wantAdded: true,
		},
		{
			name:      "connection catalog",
			resources: []types.Resource{{Type: types.ResourceTypeMcpCatalog, ID: "connection-catalog"}},
			wantAdded: true,
		},
		{
			name:      "wildcard",
			resources: []types.Resource{{Type: types.ResourceTypeSelector, ID: "*"}},
			wantAdded: true,
		},
		{
			name:      "multiple matches",
			resources: []types.Resource{entryResource, serverResource},
			wantAdded: true,
		},
		{
			name:      "already migrated",
			resources: []types.Resource{entryResource, target},
		},
		{
			name:      "disabled",
			resources: []types.Resource{entryResource},
			disabled:  true,
			wantAdded: true,
		},
		{
			name:      "unrelated",
			resources: []types.Resource{{Type: types.ResourceTypeMCPServer, ID: "ms1other"}},
		},
		{
			name:      "wrong resource type",
			resources: []types.Resource{{Type: types.ResourceTypeMCPServer, ID: entry.Name}},
		},
		{
			name:      "other namespace",
			resources: []types.Resource{entryResource},
			namespace: "other",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			filter := &v1.MCPWebhookValidation{}
			filter.Name = "mwv1filter"
			filter.Namespace = entry.Namespace
			if tc.namespace != "" {
				filter.Namespace = tc.namespace
			}
			filter.Spec.Manifest = types.MCPWebhookValidationManifest{
				Resources:       slices.Clone(tc.resources),
				Disabled:        tc.disabled,
				Selectors:       types.MCPSelectors{{Method: "tools/call", Identifiers: []string{"echo"}}},
				AllowedToMutate: true,
			}
			want := filter.Spec.Manifest
			if tc.wantAdded {
				want.Resources = append(slices.Clone(tc.resources), target)
			}
			client := migrationClient(entry.DeepCopy(), parent.DeepCopy(), filter)
			handler := credentialHandler(t, nil, map[string]map[string]string{})
			for range 2 {
				require.NoError(t, handler.MigrateAll(t.Context(), client))
				require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
				require.Equal(t, want, filter.Spec.Manifest)
			}
		})
	}
}

func (c *filterUpdateFailureClient) Update(ctx context.Context, obj kclient.Object, opts ...kclient.UpdateOption) error {
	if _, ok := obj.(*v1.MCPWebhookValidation); ok && c.fail {
		return errors.New("filter update failed")
	}
	return c.WithWatch.Update(ctx, obj, opts...)
}

func TestMigrateRetainsCompositeOnFilterFailure(t *testing.T) {
	entry := migrationEntry(t)
	parent := migrationParent(t, "ms1connection")
	filter := &v1.MCPWebhookValidation{}
	filter.Name = "mwv1filter"
	filter.Namespace = entry.Namespace
	filter.Spec.Manifest.Resources = []types.Resource{{Type: types.ResourceTypeMCPServer, ID: parent.Name}}
	client := &filterUpdateFailureClient{
		WithWatch: migrationClient(entry, parent, filter),
		fail:      true,
	}
	handler := credentialHandler(t, nil, map[string]map[string]string{})
	require.ErrorContains(t, handler.MigrateAll(t.Context(), client), "filter update failed")
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(entry), entry))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(parent), parent))
	require.True(t, entry.DeletionTimestamp.IsZero())
	require.True(t, parent.DeletionTimestamp.IsZero())
	client.fail = false
	require.NoError(t, handler.MigrateAll(t.Context(), client))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(filter), filter))
	require.Equal(t, []types.Resource{
		{Type: types.ResourceTypeMCPServer, ID: parent.Name},
		{Type: types.ResourceTypeMCPServer, ID: migrationName(system.VMCPPrefix, entry.Namespace, entry.Name)},
	}, filter.Spec.Manifest.Resources)
}

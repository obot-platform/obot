package mcpcatalog

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/nah/pkg/untriggered"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestStoredRulesGrantCatalogEntry checks the confirmation that keeps a server from being
// deleted when its access rule hasn't reached the rule helper's index yet.
func TestStoredRulesGrantCatalogEntry(t *testing.T) {
	rule := &v1.AccessControlRule{
		Name:      "acr1",
		Namespace: system.DefaultNamespace,
		Spec: v1.AccessControlRuleSpec{
			MCPCatalogID: system.DefaultCatalog,
			Manifest: types.AccessControlRuleManifest{
				Subjects:  []types.Subject{{Type: types.SubjectTypeUser, ID: "7"}, {Type: types.SubjectTypeGroup, ID: "eng"}, {Type: types.SubjectTypeObotGroup, ID: "power-user"}},
				Resources: []types.Resource{{Type: types.ResourceTypeMCPServerCatalogEntry, ID: "entry"}},
			},
		},
	}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithIndex(&v1.AccessControlRule{}, "spec.mcpCatalogID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.AccessControlRule).Spec.MCPCatalogID}
		}).
		WithObjects(rule).
		// The controller's client reads untriggered lists from storage; the fake has only one.
		WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, c kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if holder, ok := list.(*untriggered.HolderList); ok {
				list = holder.ObjectList
			}
			return c.List(ctx, list, opts...)
		}}).
		Build()
	req := router.Request{Ctx: t.Context(), Client: client, Object: &v1.MCPCatalog{Name: system.DefaultCatalog, Namespace: system.DefaultNamespace}}

	for _, tc := range []struct {
		name  string
		user  kuser.Info
		entry string
		want  bool
	}{
		{
			name:  "user subject",
			user:  &kuser.DefaultInfo{UID: "7"},
			entry: "entry",
			want:  true,
		},
		{
			name:  "group subject",
			user:  &kuser.DefaultInfo{UID: "8", Extra: map[string][]string{"auth_provider_groups": {"eng"}}},
			entry: "entry",
			want:  true,
		},
		{
			name:  "obot group subject",
			user:  &kuser.DefaultInfo{UID: "10", Extra: map[string][]string{"obot_groups": {"power-user"}}},
			entry: "entry",
			want:  true,
		},
		{
			name:  "a group subject doesn't match an Obot role group of the same name",
			user:  &kuser.DefaultInfo{UID: "11", Groups: []string{"eng"}},
			entry: "entry",
			want:  false,
		},
		{
			name:  "an Obot group subject doesn't match an auth provider group of the same name",
			user:  &kuser.DefaultInfo{UID: "12", Extra: map[string][]string{"auth_provider_groups": {"power-user"}}},
			entry: "entry",
			want:  false,
		},
		{
			name:  "other user",
			user:  &kuser.DefaultInfo{UID: "9"},
			entry: "entry",
			want:  false,
		},
		{
			name:  "other entry",
			user:  &kuser.DefaultInfo{UID: "7"},
			entry: "other",
			want:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := storedRulesGrantCatalogEntry(req, tc.user, tc.entry, system.DefaultCatalog)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

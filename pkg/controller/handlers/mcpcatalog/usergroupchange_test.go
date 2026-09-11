package mcpcatalog

import (
	"testing"

	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/client-go/tools/cache"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDeleteUnauthorizedServersForUserPreservesVMCPComponents(t *testing.T) {
	entry := &v1.MCPServerCatalogEntry{
		Name:      "entry",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerCatalogEntrySpec{
			MCPCatalogName: system.DefaultCatalog,
		},
	}
	standalone := &v1.MCPServer{
		Name:      "standalone",
		Namespace: system.DefaultNamespace,
		Spec: v1.MCPServerSpec{
			UserID:                    "1",
			MCPServerCatalogEntryName: entry.Name,
		},
	}
	component := standalone.DeepCopy()
	component.Name = "component"
	component.Spec.VMCPInstanceID = "vmcpi1dedicated"
	component.Spec.VMCPComponentID = "one"
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).
		WithIndex(&v1.MCPServer{}, "spec.userID", func(obj kclient.Object) []string {
			return []string{obj.(*v1.MCPServer).Spec.UserID}
		}).WithObjects(entry, standalone, component).Build()
	// No remaining ACR grants access to the live catalog entry.
	indexer := cache.NewIndexer(cache.MetaNamespaceKeyFunc, cache.Indexers{
		"selectors":           cache.MetaNamespaceIndexFunc,
		"catalog-entry-names": cache.MetaNamespaceIndexFunc,
	})
	h := &Handler{accessControlRuleHelper: accesscontrolrule.NewAccessControlRuleHelper(indexer, client)}
	user := &userInfo{Info: &kuser.DefaultInfo{UID: "1"}}

	require.NoError(t, h.deleteUnauthorizedServersForUser(t.Context(), client, system.DefaultNamespace, "1", user))
	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(component), &v1.MCPServer{}))
	require.True(t, apierrors.IsNotFound(client.Get(t.Context(), kclient.ObjectKeyFromObject(standalone), &v1.MCPServer{})))
}

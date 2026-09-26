package vmcp

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindInstancePreservesLegacyCreationOrder(t *testing.T) {
	original := metav1.NewTime(time.Unix(10, 0))
	newerOriginal := metav1.NewTime(time.Unix(20, 0))
	for _, test := range []struct {
		name            string
		legacyCreatedAt *metav1.Time
		legacySlug      string
		want            string
	}{
		{
			name:            "original timestamp wins over reversed migration order",
			legacyCreatedAt: &newerOriginal,
			legacySlug:      "ms1a",
			want:            "vmcpi1z",
		},
		{
			name:            "legacy slug breaks original timestamp tie",
			legacyCreatedAt: &original,
			legacySlug:      "ms1a",
			want:            "vmcpi1a",
		},
		{
			name:            "name breaks slug and timestamp tie",
			legacyCreatedAt: &original,
			legacySlug:      "ms1z",
			want:            "vmcpi1a",
		},
		{
			name: "ordinary instance uses its creation timestamp",
			want: "vmcpi1z",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			migrated := &v1.VMCPInstance{
				Name: "vmcpi1z", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Unix(200, 0)),
				Spec: v1.VMCPInstanceSpec{LegacyCreatedAt: &original, LegacySlug: "ms1z", UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1test"}},
			}
			other := &v1.VMCPInstance{
				Name: "vmcpi1a", Namespace: "default", CreationTimestamp: metav1.NewTime(time.Unix(100, 0)),
				Spec: v1.VMCPInstanceSpec{LegacyCreatedAt: test.legacyCreatedAt, LegacySlug: test.legacySlug, UserID: "7", Manifest: types.VMCPInstanceManifest{VMCPID: "vmcp1test"}},
			}
			storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).
				WithIndex(&v1.VMCPInstance{}, "spec.userID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.UserID} }).
				WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
				WithObjects(other, migrated).Build()
			instance, err := FindInstance(t.Context(), storage, "default", "vmcp1test", "7")
			if err != nil || instance == nil || instance.Name != test.want {
				t.Fatalf("instance=%#v error=%v", instance, err)
			}
		})
	}
}

func TestResolveMigratedStandaloneAliases(t *testing.T) {
	target := &v1.VMCP{
		Name:      "vmcp1shared",
		Namespace: "default",
		Spec:      v1.VMCPSpec{LegacySlug: "ms1shared"},
	}
	instance := &v1.VMCPInstance{
		Name:      "vmcpi1connection",
		Namespace: "default",
		Spec: v1.VMCPInstanceSpec{
			LegacySlug: "msi1connection",
			UserID:     "7",
			Manifest:   types.VMCPInstanceManifest{VMCPID: target.Name},
		},
	}
	for _, tc := range []struct {
		name         string
		id           string
		user         string
		source       kclient.Object
		wantVMCP     bool
		wantInstance bool
		wantNotFound bool
	}{
		{
			name:     "shared server alias",
			id:       target.Spec.LegacySlug,
			user:     "7",
			wantVMCP: true,
		},
		{
			name:         "instance alias",
			id:           instance.Spec.LegacySlug,
			user:         "7",
			wantVMCP:     true,
			wantInstance: true,
		},
		{
			name:         "instance alias rejects another user",
			id:           instance.Spec.LegacySlug,
			user:         "8",
			wantNotFound: true,
		},
		{
			name:   "existing shared server remains authoritative",
			id:     target.Spec.LegacySlug,
			source: &v1.MCPServer{Name: target.Spec.LegacySlug, Namespace: "default"},
		},
		{
			name:   "existing instance remains authoritative",
			id:     instance.Spec.LegacySlug,
			source: &v1.MCPServerInstance{Name: instance.Spec.LegacySlug, Namespace: "default"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(target, instance).
				WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCP).Spec.LegacySlug} }).
				WithIndex(&v1.VMCPInstance{}, "spec.legacySlug", func(obj kclient.Object) []string { return []string{obj.(*v1.VMCPInstance).Spec.LegacySlug} })
			if tc.source != nil {
				builder.WithObjects(tc.source)
			}
			resolved, connection, err := ResolveConnectID(t.Context(), builder.Build(), tc.id, tc.user)
			if tc.wantNotFound {
				require.True(t, apierrors.IsNotFound(err))
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.wantVMCP, resolved != nil)
			require.Equal(t, tc.wantInstance, connection != nil)
			if resolved != nil {
				require.Equal(t, target.Name, resolved.Name)
			}
			if connection != nil {
				require.Equal(t, instance.Name, connection.Name)
			}
		})
	}
}

func TestResolveRetainedCatalogEntry(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(map[bool]string{false: "unmigrated", true: "migrated"}[migrated], func(t *testing.T) {
			entry := &v1.MCPServerCatalogEntry{
				Name:      "default-single",
				Namespace: "default",
			}
			builder := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(entry).
				WithIndex(&v1.VMCP{}, "spec.legacySlug", func(obj kclient.Object) []string {
					return []string{obj.(*v1.VMCP).Spec.LegacySlug}
				})
			if migrated {
				builder.WithObjects(&v1.VMCP{
					Name:      "vmcp1single",
					Namespace: "default",
					Spec: v1.VMCPSpec{
						LegacySlug: entry.Name,
					},
				})
			}

			resolved, instance, err := ResolveConnectID(t.Context(), builder.Build(), entry.Name, "7")
			require.NoError(t, err)
			require.Nil(t, instance)
			if migrated {
				require.NotNil(t, resolved)
				require.Equal(t, "vmcp1single", resolved.Name)
			} else {
				require.Nil(t, resolved)
			}
		})
	}
}

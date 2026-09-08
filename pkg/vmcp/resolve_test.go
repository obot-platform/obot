package vmcp

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
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

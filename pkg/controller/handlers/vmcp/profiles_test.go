package vmcp

import (
	"context"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// recordingProfileData records the profiles whose data it is asked to delete.
type recordingProfileData struct {
	deleted [][]string
}

func (*recordingProfileData) ProfilesWithData(context.Context, string, []string) ([]string, error) {
	return nil, nil
}

func (r *recordingProfileData) DeleteProfileData(_ context.Context, _ string, profileIDs []string) error {
	r.deleted = append(r.deleted, profileIDs)
	return nil
}

func TestReconcileProfiles(t *testing.T) {
	everyone := types.Subject{Type: types.SubjectTypeSelector, ID: types.SubjectWildcardID}
	user := types.Subject{Type: types.SubjectTypeUser, ID: "1"}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{Profiles: []types.VMCPProfile{
		{Name: "a", Subjects: []types.Subject{everyone}},
		{Name: "b", Subjects: []types.Subject{everyone}},
	}}}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp).WithObjects(vmcp).Build()
	data := &recordingProfileData{}
	handler := &Handler{profileData: data}
	reconcile := func() *v1.VMCP {
		t.Helper()
		current := &v1.VMCP{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		require.NoError(t, handler.ReconcileProfiles(router.Request{Ctx: t.Context(), Client: client, Object: current}, nil))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		return current
	}

	// A vMCP stored before profiles had IDs gets the IDs catalog sync would derive.
	current := reconcile()
	a, b := vmcpconfig.DerivedProfileID(vmcp.Name, "a"), vmcpconfig.DerivedProfileID(vmcp.Name, "b")
	require.Equal(t, a, current.Spec.Manifest.Profiles[0].ID)
	require.Equal(t, b, current.Spec.Manifest.Profiles[1].ID)
	require.Empty(t, current.Status.ProfileIDs)

	// The next reconcile records them, and reports that both profiles name every agent.
	current = reconcile()
	require.Equal(t, []string{a, b}, current.Status.ProfileIDs)
	condition := meta.FindStatusCondition(current.Status.Conditions, types.VMCPConditionAgentSelectionAmbiguous)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionTrue, condition.Status)
	require.Contains(t, condition.Message, `"a"`)
	require.Empty(t, data.deleted)

	// Removing a profile deletes its data, and leaves selection unambiguous. A rename is not a removal.
	current.Spec.Manifest.Profiles = []types.VMCPProfile{{ID: a, Name: "renamed", Subjects: []types.Subject{everyone}}, {ID: "vp1new", Name: "c", Subjects: []types.Subject{user}}}
	require.NoError(t, client.Update(t.Context(), current))
	current = reconcile()
	require.Equal(t, [][]string{{b}}, data.deleted)
	require.Equal(t, []string{a, "vp1new"}, current.Status.ProfileIDs)
	condition = meta.FindStatusCondition(current.Status.Conditions, types.VMCPConditionAgentSelectionAmbiguous)
	require.NotNil(t, condition)
	require.Equal(t, metav1.ConditionFalse, condition.Status)

	// Nothing changed, so nothing is deleted again.
	reconcile()
	require.Len(t, data.deleted, 1)

	// Deleting the vMCP deletes the data of every profile it has or last recorded.
	current.Status.ProfileIDs = append(current.Status.ProfileIDs, "vp1removedjustnow")
	require.NoError(t, handler.RemoveProfileData(router.Request{Ctx: t.Context(), Client: client, Object: current}, nil))
	require.Equal(t, []string{a, "vp1new", "vp1removedjustnow"}, data.deleted[1])
}

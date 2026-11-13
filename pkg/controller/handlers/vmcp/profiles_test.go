package vmcp

import (
	"context"
	"maps"
	"slices"
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	vmcpconfig "github.com/obot-platform/obot/pkg/vmcp"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// recordingProfileData records the profiles whose data it is asked to delete.
type recordingProfileData struct {
	deleted [][]string
}

// profileCredentials is a credential store in memory for profile configuration.
type profileCredentials struct {
	credentials map[string]map[string]map[string]string
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

// TestReconcileProfilesRemovesProfileConfiguration checks that removing a profile deletes its configuration revisions
// and its record in status, and that deleting the vMCP deletes every profile's, with a real store.
func TestReconcileProfilesRemovesProfileConfiguration(t *testing.T) {
	agents := types.Subject{Type: types.SubjectTypeAgent, ID: types.SubjectWildcardID}
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Components: []types.VMCPComponent{{
			ID:            "api",
			Configuration: []types.VMCPConfigurationPolicy{{Key: "REGION", Policy: types.VMCPConfigurationPolicyUserAllowed}},
			CatalogEntry:  types.MCPServerCatalogEntrySnapshot{Manifest: types.MCPServerCatalogEntryManifest{Config: []types.MCPConfig{{Key: "REGION", Required: true}}}},
		}},
		Profiles: []types.VMCPProfile{
			{ID: "vp1kept", Name: "kept", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}},
			{ID: "vp1removed", Name: "removed", Subjects: []types.Subject{agents}},
		},
	}}}
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp).WithObjects(vmcp).Build()
	credentials := newProfileCredentials()
	store := vmcpconfig.NewProfileConfigurationStore(credentials)
	handler := &Handler{profileData: vmcpconfig.ProfileDataStores{store}, profileConfiguration: store}
	reconcile := func() *v1.VMCP {
		t.Helper()
		current := &v1.VMCP{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		require.NoError(t, handler.ReconcileProfiles(router.Request{Ctx: t.Context(), Client: client, Object: current}, nil))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		return current
	}
	reconcile()
	for _, profileID := range []string{"vp1kept", "vp1removed"} {
		_, err := store.Update(t.Context(), client, kclient.ObjectKeyFromObject(vmcp), profileID, map[string]map[string]*string{"api": {"REGION": new("us-east")}})
		require.NoError(t, err)
	}

	current := reconcile()
	current.Spec.Manifest.Profiles = current.Spec.Manifest.Profiles[:1]
	require.NoError(t, client.Update(t.Context(), current))
	current = reconcile()
	require.Equal(t, []string{"vp1kept"}, current.Status.ProfileIDs)
	require.Equal(t, []string{"vp1kept"}, slices.Sorted(maps.Keys(current.Status.ProfileConfigurationRevisions)))
	withData, err := store.ProfilesWithData(t.Context(), vmcp.Name, []string{"vp1kept", "vp1removed"})
	require.NoError(t, err)
	require.Equal(t, []string{"vp1kept"}, withData)

	require.NoError(t, handler.RemoveProfileData(router.Request{Ctx: t.Context(), Client: client, Object: current}, nil))
	withData, err = store.ProfilesWithData(t.Context(), vmcp.Name, []string{"vp1kept", "vp1removed"})
	require.NoError(t, err)
	require.Empty(t, withData)
}

// TestCollectProfileConfiguration checks that a superseded revision is deleted once no agent's instance records it,
// and that a vMCP watches its instances only while a revision waits to be deleted.
func TestCollectProfileConfiguration(t *testing.T) {
	vmcp := &v1.VMCP{Name: "vmcp1test", Namespace: "default", Spec: v1.VMCPSpec{Manifest: types.VMCPManifest{
		Profiles: []types.VMCPProfile{{ID: "vp1agent", Name: "agent", Subjects: []types.Subject{{Type: types.SubjectTypeAgent, ID: "7"}}}},
	}}}
	vmcp.Status.ProfileConfigurationRevisions = map[string]v1.VMCPProfileConfigurationRevision{
		"vp1agent": {Name: "configuration-new", Keys: []string{"k"}},
	}
	instance := &v1.VMCPInstance{Name: "vmcpi1agent", Namespace: "default", Spec: v1.VMCPInstanceSpec{
		UserID:   "agent:7",
		Manifest: types.VMCPInstanceManifest{VMCPID: vmcp.Name},
	}, Status: v1.VMCPInstanceStatus{AgentConfigurationRevisions: map[string]string{"vp1agent": "configuration-old"}}}
	var listed int
	client := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithStatusSubresource(vmcp, instance).WithObjects(vmcp, instance).
		WithIndex(&v1.VMCPInstance{}, "spec.manifest.vmcpID", func(o kclient.Object) []string { return []string{o.(*v1.VMCPInstance).Spec.Manifest.VMCPID} }).
		WithInterceptorFuncs(interceptor.Funcs{List: func(ctx context.Context, client kclient.WithWatch, list kclient.ObjectList, opts ...kclient.ListOption) error {
			if _, ok := list.(*v1.VMCPInstanceList); ok {
				listed++
				require.Equal(t, "spec.manifest.vmcpID=vmcp1test", (&kclient.ListOptions{}).ApplyOptions(opts).FieldSelector.String())
			}
			return client.List(ctx, list, opts...)
		}}).
		Build()
	credentials := newProfileCredentials()
	for _, name := range []string{"configuration-new", "configuration-old"} {
		require.NoError(t, credentials.UpsertCredential(t.Context(), gatewaytypes.Credential{Context: vmcpconfig.ProfileConfigurationCredentialContext(vmcp.Name, "vp1agent"), Name: name}))
	}
	handler := &Handler{profileConfiguration: vmcpconfig.NewProfileConfigurationStore(credentials)}
	collect := func() *v1.VMCP {
		t.Helper()
		current := &v1.VMCP{}
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		require.NoError(t, handler.CollectProfileConfiguration(router.Request{Ctx: t.Context(), Client: client, Object: current}, nil))
		require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(vmcp), current))
		return current
	}

	// Nothing is superseded, so instances are not listed.
	collect()
	require.Zero(t, listed)

	current := collect()
	current.Status.ProfileConfigurationRevisions["vp1agent"] = v1.VMCPProfileConfigurationRevision{Name: "configuration-new", Keys: []string{"k"}, Superseded: []string{"configuration-old"}}
	require.NoError(t, client.Status().Update(t.Context(), current))

	// The instance still records the old revision.
	current = collect()
	require.Equal(t, 1, listed)
	require.Equal(t, []string{"configuration-old"}, current.Status.ProfileConfigurationRevisions["vp1agent"].Superseded)
	require.ElementsMatch(t, []string{"configuration-new", "configuration-old"}, credentials.names(vmcpconfig.ProfileConfigurationCredentialContext(vmcp.Name, "vp1agent")))

	require.NoError(t, client.Get(t.Context(), kclient.ObjectKeyFromObject(instance), instance))
	instance.Status.AgentConfigurationRevisions["vp1agent"] = "configuration-new"
	require.NoError(t, client.Status().Update(t.Context(), instance))
	current = collect()
	require.Empty(t, current.Status.ProfileConfigurationRevisions["vp1agent"].Superseded)
	require.Equal(t, []string{"configuration-new"}, credentials.names(vmcpconfig.ProfileConfigurationCredentialContext(vmcp.Name, "vp1agent")))
}

func newProfileCredentials() *profileCredentials {
	return &profileCredentials{credentials: map[string]map[string]map[string]string{}}
}

func (p *profileCredentials) names(credentialContext string) []string {
	return slices.Sorted(maps.Keys(p.credentials[credentialContext]))
}

func (p *profileCredentials) RevealCredential(_ context.Context, contexts []string, name string) (gatewaytypes.Credential, error) {
	for _, credentialContext := range contexts {
		if secrets, ok := p.credentials[credentialContext][name]; ok {
			return gatewaytypes.Credential{Context: credentialContext, Name: name, Secrets: maps.Clone(secrets)}, nil
		}
	}
	return gatewaytypes.Credential{}, gateway.CredentialNotFoundError{Contexts: contexts, Name: name}
}

func (p *profileCredentials) UpsertCredential(_ context.Context, credential gatewaytypes.Credential) error {
	if p.credentials[credential.Context] == nil {
		p.credentials[credential.Context] = map[string]map[string]string{}
	}
	p.credentials[credential.Context][credential.Name] = maps.Clone(credential.Secrets)
	return nil
}

func (p *profileCredentials) DeleteCredential(_ context.Context, credentialContext, name string) (bool, error) {
	_, ok := p.credentials[credentialContext][name]
	delete(p.credentials[credentialContext], name)
	return ok, nil
}

func (p *profileCredentials) CredentialContextsWithCredentials(_ context.Context, contexts []string) ([]string, error) {
	var found []string
	for _, credentialContext := range contexts {
		if len(p.credentials[credentialContext]) > 0 {
			found = append(found, credentialContext)
		}
	}
	return found, nil
}

func (p *profileCredentials) DeleteCredentialsInContexts(_ context.Context, contexts []string) error {
	for _, credentialContext := range contexts {
		delete(p.credentials, credentialContext)
	}
	return nil
}

package modelaccesspolicy

import (
	"slices"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kuser "k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func profile(name string, subjects ...types2.Subject) *v1.ModelAccessPolicy {
	return &v1.ModelAccessPolicy{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec:      v1.ModelAccessPolicySpec{Manifest: types2.ModelAccessPolicyManifest{DisplayName: name, Subjects: subjects}},
	}
}

func TestUserProfileIDs(t *testing.T) {
	h := newModelHelper(t, nil,
		profile("everyone", types2.Subject{Type: types2.SubjectTypeSelector, ID: "*"}),
		profile("alice", types2.Subject{Type: types2.SubjectTypeUser, ID: "alice"}),
		profile("sales", types2.Subject{Type: types2.SubjectTypeGroup, ID: "okta/sales"}),
		profile("admins", types2.Subject{Type: types2.SubjectTypeObotGroup, ID: types2.GroupAdmin}),
		// Named twice, through the user and a group: still listed once.
		profile("both",
			types2.Subject{Type: types2.SubjectTypeUser, ID: "alice"},
			types2.Subject{Type: types2.SubjectTypeGroup, ID: "okta/sales"}),
	)

	tests := []struct {
		name string
		user kuser.Info
		want []string
	}{
		{
			name: "a user named directly and through an auth provider group",
			user: &kuser.DefaultInfo{UID: "alice", Extra: map[string][]string{"auth_provider_groups": {"okta/sales"}}},
			want: []string{"alice", "both", "everyone", "sales"},
		},
		{
			name: "an Obot group",
			user: &kuser.DefaultInfo{UID: "bob", Extra: map[string][]string{"obot_groups": {types2.GroupAdmin}}},
			want: []string{"admins", "everyone"},
		},
		{
			name: "an auth provider group with an Obot group's name isn't the Obot group",
			user: &kuser.DefaultInfo{UID: "carol", Extra: map[string][]string{"auth_provider_groups": {types2.GroupAdmin}}},
			want: []string{"everyone"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.UserProfileIDs(tt.user)
			require.NoError(t, err)
			slices.Sort(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEnsureEveryoneProfile(t *testing.T) {
	everyone := types2.Subject{Type: types2.SubjectTypeSelector, ID: "*"}
	tests := []struct {
		name       string
		existing   []kclient.Object
		want       string
		wantModels int
	}{
		{
			name:       "no policies: the Default Policy, as on first start",
			want:       DefaultPolicyName,
			wantModels: 2,
		},
		{
			name: "the Default Policy is preferred",
			existing: []kclient.Object{
				profile("aaa", everyone),
				profile(DefaultPolicyName, everyone),
			},
			want: DefaultPolicyName,
		},
		{
			name:     "another policy for everyone",
			existing: []kclient.Object{profile("all-models", everyone)},
			want:     "all-models",
		},
		{
			name:     "policies, but none for everyone: an Everyone profile without models",
			existing: []kclient.Object{profile("sales", types2.Subject{Type: types2.SubjectTypeGroup, ID: "sales"})},
			want:     everyoneProfileName,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(tt.existing...).Build()
			// Twice, as replicas starting together would.
			for range 2 {
				got, err := EnsureEveryoneProfile(t.Context(), c)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			var p v1.ModelAccessPolicy
			require.NoError(t, c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: tt.want}, &p))
			assert.True(t, AppliesToEveryone(p.Spec.Manifest.Subjects))
			if tt.existing == nil || tt.want == everyoneProfileName {
				assert.Len(t, p.Spec.Manifest.Models, tt.wantModels)
			}
		})
	}
}

func TestEnsureProfiles(t *testing.T) {
	everyone := types2.Subject{Type: types2.SubjectTypeSelector, ID: "*"}
	deleting := profile("deleting", everyone)
	deleting.Finalizers = []string{"test"}
	deleting.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	tests := []struct {
		name        string
		existing    []kclient.Object
		want        []string
		wantDefault bool
	}{
		{
			name:        "no policies: the Default Policy, as on first start",
			want:        []string{DefaultPolicyName},
			wantDefault: true,
		},
		{
			name: "every existing profile, whoever it applies to, and no new one",
			existing: []kclient.Object{
				profile("sales", types2.Subject{Type: types2.SubjectTypeGroup, ID: "sales"}),
				profile("all-models", everyone),
				profile("nobody"),
			},
			want: []string{"all-models", "nobody", "sales"},
		},
		{
			name: "a profile being deleted is left out",
			existing: []kclient.Object{
				profile("sales", types2.Subject{Type: types2.SubjectTypeGroup, ID: "sales"}),
				deleting,
			},
			want: []string{"sales"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(tt.existing...).Build()
			// Twice, as replicas starting together would.
			for range 2 {
				got, err := EnsureProfiles(t.Context(), c)
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
			var list v1.ModelAccessPolicyList
			require.NoError(t, c.List(t.Context(), &list))
			assert.Len(t, list.Items, max(len(tt.existing), 1))
			if tt.wantDefault {
				var p v1.ModelAccessPolicy
				require.NoError(t, c.Get(t.Context(), kclient.ObjectKey{Namespace: system.DefaultNamespace, Name: DefaultPolicyName}, &p))
				assert.True(t, AppliesToEveryone(p.Spec.Manifest.Subjects))
				assert.Len(t, p.Spec.Manifest.Models, 2)
			}
		})
	}
}

func profileGranting(name string, subject types2.Subject, models ...string) *v1.ModelAccessPolicy {
	p := profile(name, subject)
	for _, m := range models {
		p.Spec.Manifest.Models = append(p.Spec.Manifest.Models, types2.ModelResource{ID: m})
	}
	return p
}

func TestUserProfileIDsForModel(t *testing.T) {
	const (
		modelA = "m1-model-a"
		modelB = "m1-model-b"
	)
	models := []*v1.Model{
		newModel(modelA, "openai-model-provider", "gpt-a", true),
		newModel(modelB, "openai-model-provider", "gpt-b", true),
	}
	everyone := types2.Subject{Type: types2.SubjectTypeSelector, ID: "*"}
	alice := types2.Subject{Type: types2.SubjectTypeUser, ID: "alice"}
	h := newModelHelper(t, models,
		profileGranting("everyone", everyone, modelA),
		profileGranting("power", alice, modelA, modelB),
		// A profile that grants no models never brings its budgets.
		profileGranting("empty", alice),
		profileGranting("all", types2.Subject{Type: types2.SubjectTypeGroup, ID: "okta/all"}, "*"),
	)

	tests := []struct {
		name  string
		user  kuser.Info
		model string
		want  []string
	}{
		{
			name:  "a model only one profile grants",
			user:  &kuser.DefaultInfo{UID: "alice"},
			model: modelB,
			want:  []string{"power"},
		},
		{
			name:  "a model several profiles grant",
			user:  &kuser.DefaultInfo{UID: "alice"},
			model: modelA,
			want:  []string{"everyone", "power"},
		},
		{
			name:  "a model none of the user's profiles grants",
			user:  &kuser.DefaultInfo{UID: "bob"},
			model: modelB,
			want:  nil,
		},
		{
			name:  "a wildcard profile grants every model",
			user:  &kuser.DefaultInfo{UID: "carol", Extra: map[string][]string{"auth_provider_groups": {"okta/all"}}},
			model: modelB,
			want:  []string{"all"},
		},
		{
			name:  "a model that doesn't exist",
			user:  &kuser.DefaultInfo{UID: "carol", Extra: map[string][]string{"auth_provider_groups": {"okta/all"}}},
			model: "m1-missing",
			want:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := h.UserProfileIDsForModel(tt.user, tt.model)
			require.NoError(t, err)
			slices.Sort(got)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPolicyAllowedModels(t *testing.T) {
	models := []*v1.Model{
		newModel("m1-model-a", "openai-model-provider", "gpt-a", true),
		newModel("m1-model-b", "openai-model-provider", "gpt-b", true),
	}
	h := newModelHelper(t, models)
	everyone := types2.Subject{Type: types2.SubjectTypeSelector, ID: "*"}

	tests := []struct {
		name   string
		policy *v1.ModelAccessPolicy
		want   map[string]bool
	}{
		{
			name:   "specific models",
			policy: profileGranting("p", everyone, "m1-model-a"),
			want:   map[string]bool{"m1-model-a": true},
		},
		{
			name:   "every model",
			policy: profileGranting("p", everyone, "*"),
			want:   map[string]bool{"m1-model-a": true, "m1-model-b": true},
		},
		{
			name:   "no models",
			policy: profileGranting("p", everyone),
			want:   map[string]bool{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, h.PolicyAllowedModels(tt.policy))
		})
	}
}

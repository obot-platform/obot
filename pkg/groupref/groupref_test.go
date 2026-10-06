package groupref

import (
	"context"
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	namespace = "default"
)

type fakeRoleAssignments []gatewaytypes.GroupRoleAssignment

func (f fakeRoleAssignments) ListGroupRoleAssignments(context.Context) ([]gatewaytypes.GroupRoleAssignment, error) {
	return f, nil
}

func groupSubject(id string) types.Subject {
	return types.Subject{
		Type: types.SubjectTypeGroup,
		ID:   id,
	}
}

// referencingObjects returns one object of every kind that can reference a group, each referencing target and
// an unrelated group.
func referencingObjects(target string) []kclient.Object {
	subjects := []types.Subject{
		{
			Type: types.SubjectTypeUser,
			ID:   "1",
		},
		groupSubject(target),
		groupSubject("entra/other"),
		{
			Type: types.SubjectTypeObotGroup,
			ID:   target,
		},
	}

	return []kclient.Object{
		&v1.AccessControlRule{
			Name:      "acr",
			Namespace: namespace,
			Spec: v1.AccessControlRuleSpec{
				Manifest: types.AccessControlRuleManifest{
					DisplayName: "Engineering servers",
					Subjects:    subjects,
				},
			},
		},
		&v1.ModelAccessPolicy{
			Name:      "map",
			Namespace: namespace,
			Spec: v1.ModelAccessPolicySpec{
				Manifest: types.ModelAccessPolicyManifest{
					DisplayName: "Models",
					Subjects:    subjects,
				},
			},
		},
		&v1.SkillAccessRule{
			Name:      "sar",
			Namespace: namespace,
			Spec: v1.SkillAccessRuleSpec{
				Manifest: types.SkillAccessRuleManifest{
					DisplayName: "Skills",
					Subjects:    subjects,
				},
			},
		},
		&v1.MessagePolicy{
			Name:      "mp",
			Namespace: namespace,
			Spec: v1.MessagePolicySpec{
				Manifest: types.MessagePolicyManifest{
					DisplayName: "Messages",
					Subjects:    subjects,
				},
			},
		},
		&v1.HostedAgentAccessRule{
			Name:      "haar",
			Namespace: namespace,
			Spec: v1.HostedAgentAccessRuleSpec{
				Manifest: types.HostedAgentAccessRuleManifest{
					DisplayName: "Agents",
					Subjects:    subjects,
				},
			},
		},
		&v1.PublishedArtifact{
			Name:      "pa",
			Namespace: namespace,
			Spec: v1.PublishedArtifactSpec{
				PublishedArtifactManifest: types.PublishedArtifactManifest{
					Name: "Workflow",
				},
			},
			Status: v1.PublishedArtifactStatus{
				Versions: []types.PublishedArtifactVersionEntry{
					{
						Version:  1,
						Subjects: subjects,
					},
					{
						Version: 2,
						Subjects: []types.Subject{
							groupSubject(target),
						},
					},
				},
			},
		},
		&v1.VMCP{
			Name:      "vmcp",
			Namespace: namespace,
			Spec: v1.VMCPSpec{
				Manifest: types.VMCPManifest{
					DisplayName: "Toolbox",
					Profiles: []types.VMCPProfile{
						{
							Name:     "engineers",
							Subjects: subjects,
						},
						{
							Name: "everyone",
							Subjects: []types.Subject{
								{
									Type: types.SubjectTypeSelector,
									ID:   "*",
								},
							},
						},
					},
				},
			},
		},
		// Objects in other namespaces are not references.
		&v1.AccessControlRule{
			Name:      "elsewhere",
			Namespace: "other",
			Spec: v1.AccessControlRuleSpec{
				Manifest: types.AccessControlRuleManifest{
					Subjects: subjects,
				},
			},
		},
	}
}

func TestFindCoversEveryReferenceKind(t *testing.T) {
	const target = "okta/00g-engineering"

	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(referencingObjects(target)...).
		Build()
	roles := fakeRoleAssignments{
		{
			GroupName: target,
			Role:      types.RoleAdmin | types.RoleAuditor,
		},
		{
			GroupName: "entra/other",
			Role:      types.RoleBasic,
		},
	}

	refs, err := NewFinder(storage, roles).Find(t.Context(), namespace, HasPrefix("okta/"))
	if err != nil {
		t.Fatal(err)
	}

	want := References{
		target: {
			{
				Kind:        KindAccessControlRule,
				Name:        "acr",
				DisplayName: "Engineering servers",
			},
			{
				Kind:   KindGroupRoleAssignment,
				Name:   target,
				Detail: "role Admin, Auditor",
			},
			{
				Kind:        KindHostedAgentAccessRule,
				Name:        "haar",
				DisplayName: "Agents",
			},
			{
				Kind:        KindMessagePolicy,
				Name:        "mp",
				DisplayName: "Messages",
			},
			{
				Kind:        KindModelAccessPolicy,
				Name:        "map",
				DisplayName: "Models",
			},
			{
				Kind:        KindPublishedArtifact,
				Name:        "pa",
				DisplayName: "Workflow",
				Detail:      "version 1",
			},
			{
				Kind:        KindPublishedArtifact,
				Name:        "pa",
				DisplayName: "Workflow",
				Detail:      "version 2",
			},
			{
				Kind:        KindSkillAccessRule,
				Name:        "sar",
				DisplayName: "Skills",
			},
			{
				Kind:        KindVMCPProfile,
				Name:        "vmcp",
				DisplayName: "Toolbox",
				Detail:      "profile engineers",
			},
		},
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("references = %#v\nwant %#v", refs, want)
	}
}

func TestRemoveGroupSubjectsChangesOnlyAccessPolicies(t *testing.T) {
	const target = "okta/00g-engineering"

	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.PublishedArtifact{}).
		WithObjects(referencingObjects(target)...).
		Build()

	counts, err := RemoveGroupSubjects(t.Context(), storage, namespace, HasPrefix("okta/"))
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := map[Kind]int{
		KindAccessControlRule:     1,
		KindModelAccessPolicy:     1,
		KindSkillAccessRule:       1,
		KindMessagePolicy:         1,
		KindHostedAgentAccessRule: 1,
		KindPublishedArtifact:     1,
	}
	if !reflect.DeepEqual(counts, wantCounts) {
		t.Fatalf("counts = %v, want %v", counts, wantCounts)
	}

	// Nothing that the finder reports among the access policies is left, and virtual MCP profiles, which must
	// keep a subject, are unchanged.
	refs, err := NewFinder(storage, fakeRoleAssignments{}).Find(t.Context(), namespace, HasPrefix("okta/"))
	if err != nil {
		t.Fatal(err)
	}
	want := References{
		target: {
			{
				Kind:        KindVMCPProfile,
				Name:        "vmcp",
				DisplayName: "Toolbox",
				Detail:      "profile engineers",
			},
		},
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("references after removal = %#v, want %#v", refs, want)
	}

	var elsewhere v1.AccessControlRule
	if err := storage.Get(t.Context(), kclient.ObjectKey{Namespace: "other", Name: "elsewhere"}, &elsewhere); err != nil {
		t.Fatal(err)
	}
	if len(elsewhere.Spec.Manifest.Subjects) != 4 {
		t.Fatalf("a policy in another namespace was changed: %v", elsewhere.Spec.Manifest.Subjects)
	}
}

func TestRemoveSubjects(t *testing.T) {
	tests := []struct {
		name          string
		subjects      []types.Subject
		groupIDPrefix string
		want          []types.Subject
		wantChange    bool
	}{
		{
			name: "removes matching groups and preserves order",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeUser,
					ID:   "1",
				},
				{
					Type: types.SubjectTypeGroup,
					ID:   "entra/engineering",
				},
				{
					Type: types.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
			},
			groupIDPrefix: "entra/",
			want: []types.Subject{
				{
					Type: types.SubjectTypeUser,
					ID:   "1",
				},
				{
					Type: types.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
			},
			wantChange: true,
		},
		{
			name: "keeps an empty policy after removing its only subject",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeGroup,
					ID:   "entra/engineering",
				},
			},
			groupIDPrefix: "entra/",
			want:          []types.Subject{},
			wantChange:    true,
		},
		{
			name: "does not change unrelated subjects",
			subjects: []types.Subject{
				{
					Type: types.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
				{
					Type: types.SubjectTypeObotGroup,
					ID:   "entra/engineering",
				},
			},
			groupIDPrefix: "entra/",
			want: []types.Subject{
				{
					Type: types.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
				{
					Type: types.SubjectTypeObotGroup,
					ID:   "entra/engineering",
				},
			},
			wantChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := RemoveSubjects(tt.subjects, HasPrefix(tt.groupIDPrefix))
			if changed != tt.wantChange {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChange)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("subjects = %#v, want %#v", got, tt.want)
			}
		})
	}
}

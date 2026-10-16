package groupref

import (
	"reflect"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestFindCoversGuardrailFilters(t *testing.T) {
	const target = "okta/00g-engineering"

	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(
			&v1.MCPWebhookValidation{
				Name:      "filter",
				Namespace: namespace,
				Spec: v1.MCPWebhookValidationSpec{
					Manifest: types.MCPWebhookValidationManifest{
						Name: "No secrets",
						Guardrail: &types.MCPGuardrailFilterConfig{
							Subjects: []types.MCPGuardrailFilterSubject{
								{
									Type: types.MCPGuardrailSubjectUser,
									ID:   target,
								},
								{
									Type: types.MCPGuardrailSubjectGroup,
									ID:   target,
								},
							},
						},
					},
				},
			},
			// A filter without a guardrail has no subjects.
			&v1.MCPWebhookValidation{
				Name:      "webhook",
				Namespace: namespace,
			},
		).
		Build()
	refs, err := NewFinder(storage, fakeRoleAssignments{}).Find(t.Context(), namespace, HasPrefix("okta/"))
	if err != nil {
		t.Fatal(err)
	}

	want := References{
		target: {
			{
				Kind:        KindMCPGuardrailFilter,
				Name:        "filter",
				DisplayName: "No secrets",
			},
		},
	}
	if !reflect.DeepEqual(refs, want) {
		t.Fatalf("references = %#v\nwant %#v", refs, want)
	}
}

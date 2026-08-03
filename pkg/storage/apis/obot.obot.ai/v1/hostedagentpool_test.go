package v1

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestHostedAgentPoolTypesRegistered(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := AddToScheme(scheme); err != nil {
		t.Fatalf("AddToScheme() error = %v", err)
	}

	for _, obj := range []runtime.Object{
		&HostedAgentPool{},
		&HostedAgentPoolList{},
		&HostedAgentPoolDefaults{},
		&HostedAgentPoolDefaultsList{},
		&HostedAgentPoolAssignment{},
		&HostedAgentPoolAssignmentList{},
	} {
		gvks, _, err := scheme.ObjectKinds(obj)
		if err != nil {
			t.Fatalf("ObjectKinds(%T) error = %v", obj, err)
		}
		if len(gvks) != 1 || gvks[0].GroupVersion() != SchemeGroupVersion {
			t.Fatalf("ObjectKinds(%T) = %v, want group version %s", obj, gvks, SchemeGroupVersion)
		}
	}
}

func TestHostedAgentPoolAssignmentFields(t *testing.T) {
	assignment := &HostedAgentPoolAssignment{
		Spec: HostedAgentPoolAssignmentSpec{
			Manifest: types.HostedAgentPoolAssignmentManifest{
				UserID:  "user-1",
				PoolID:  "pool-1",
				Default: true,
			},
		},
	}

	for field, want := range map[string]string{
		"spec.userID":  "user-1",
		"spec.poolID":  "pool-1",
		"spec.default": "true",
	} {
		if !assignment.Has(field) {
			t.Errorf("Has(%q) = false", field)
		}
		if got := assignment.Get(field); got != want {
			t.Errorf("Get(%q) = %q, want %q", field, got, want)
		}
	}
}

package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPublishedArtifactAuthorization(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(&v1.PublishedArtifact{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pa1test",
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.PublishedArtifactSpec{
			AuthorID:      "owner",
			LatestVersion: 2,
		},
		Status: v1.PublishedArtifactStatus{
			Versions: []types.PublishedArtifactVersionEntry{
				{
					Version:  1,
					Subjects: []types.Subject{{Type: types.SubjectTypeGroup, ID: "group1"}},
				},
				{
					Version: 2,
				},
			},
		},
	}).Build()
	authorizer := NewAuthorizer(storage, storage, false, nil, false)

	tests := []struct {
		name    string
		method  string
		path    string
		user    user.Info
		allowed bool
	}{
		{
			name:   "owner can update",
			method: http.MethodPut,
			path:   "/api/published-artifacts/pa1test",
			user: &user.DefaultInfo{
				UID:    "owner",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
			},
			allowed: true,
		},
		{
			name:   "subject can get shared artifact",
			method: http.MethodGet,
			path:   "/api/published-artifacts/pa1test",
			user: &user.DefaultInfo{
				UID:    "other",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
				Extra:  map[string][]string{"auth_provider_groups": {"group1"}},
			},
			allowed: true,
		},
		{
			name:   "subject cannot update shared artifact",
			method: http.MethodPut,
			path:   "/api/published-artifacts/pa1test",
			user: &user.DefaultInfo{
				UID:    "other",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
				Extra:  map[string][]string{"auth_provider_groups": {"group1"}},
			},
			allowed: false,
		},
		{
			name:   "subject cannot get unshared version",
			method: http.MethodGet,
			path:   "/api/published-artifacts/pa1test/2/skill",
			user: &user.DefaultInfo{
				UID:    "other",
				Groups: []string{types.GroupBasic, types.GroupAuthenticated},
				Extra:  map[string][]string{"auth_provider_groups": {"group1"}},
			},
			allowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if allowed := authorizer.Authorize(req, tt.user); allowed != tt.allowed {
				t.Fatalf("Authorize() = %v, want %v", allowed, tt.allowed)
			}
		})
	}
}

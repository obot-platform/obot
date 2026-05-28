package registry

import (
	"testing"

	"github.com/obot-platform/obot/pkg/api/authz"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestRegistryUserAuthenticated(t *testing.T) {
	tests := []struct {
		name string
		user user.Info
		want bool
	}{
		{
			name: "authenticated user",
			user: &user.DefaultInfo{UID: "user-1", Name: "user@example.com"},
			want: true,
		},
		{
			name: "no auth nobody owner user",
			user: &user.DefaultInfo{UID: "1", Name: "nobody", Groups: []string{"owner"}},
			want: true,
		},
		{
			name: "anonymous user",
			user: &user.DefaultInfo{UID: "anonymous", Name: "anonymous", Groups: []string{authz.UnauthenticatedGroup}},
			want: false,
		},
		{
			name: "nil user",
			user: nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := registryUserAuthenticated(tt.user); got != tt.want {
				t.Fatalf("registryUserAuthenticated() = %v, want %v", got, tt.want)
			}
		})
	}
}

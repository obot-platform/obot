package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestOpenAPIImportAuthorization(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build()
	authorizer := NewAuthorizer(nil, storage, storage, false, nil, nil, false)
	for _, test := range []struct {
		name    string
		role    types.Role
		allowed bool
	}{
		{
			name:    "owner",
			role:    types.RoleOwner,
			allowed: true,
		},
		{
			name:    "admin",
			role:    types.RoleAdmin,
			allowed: true,
		},
		{
			name: "power user",
			role: types.RolePowerUser,
		},
		{
			name: "power user plus",
			role: types.RolePowerUserPlus,
		},
		{
			name: "basic user",
			role: types.RoleBasic,
		},
		{
			name: "auditor",
			role: types.RoleAuditor,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			principal := &user.DefaultInfo{Name: test.name, UID: test.name, Groups: test.role.Groups()}
			request := httptest.NewRequest(http.MethodPost, "/api/openapi-import", nil)
			require.Equal(t, test.allowed, authorizer.Authorize(request, principal))
		})
	}

	anonymous := &user.DefaultInfo{Name: "anonymous", Groups: []string{UnauthenticatedGroup}}
	require.False(t, authorizer.Authorize(httptest.NewRequest(http.MethodPost, "/api/openapi-import", nil), anonymous))
}

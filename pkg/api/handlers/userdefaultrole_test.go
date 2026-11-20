package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestSetDefaultUserRole(t *testing.T) {
	for _, role := range []types.Role{
		types.RoleBasic, types.RolePowerUser, types.RolePowerUserPlus, types.RoleAdmin,
		types.RoleAdmin | types.RoleAuditor, types.RoleOwner, types.RoleUnknown,
		types.RoleAuditor, types.RoleBasic | types.RoleAdmin, types.RoleAdmin | 256, 10,
	} {
		t.Run(fmt.Sprint(role), func(t *testing.T) {
			for _, owner := range []bool{false, true} {
				setting := &v1.UserDefaultRoleSetting{
					Name:      system.DefaultRoleSettingName,
					Namespace: system.DefaultNamespace,
					Spec:      v1.UserDefaultRoleSettingSpec{Role: types.RoleBasic},
				}
				store := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(setting).Build()
				groups := []string{types.GroupAdmin}
				if owner {
					groups = append(groups, types.GroupOwner)
				}
				request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(fmt.Sprintf(`{"role":%d}`, role)))
				err := new(UserDefaultRoleSettingHandler).Set(api.Context{
					Request:        request,
					ResponseWriter: httptest.NewRecorder(),
					User:           &user.DefaultInfo{Groups: groups},
					Storage:        store,
				})

				want := role
				switch {
				case role == types.RoleUnknown || role == types.RoleAuditor || role == types.RoleBasic|types.RoleAdmin || role == types.RoleAdmin|256 || role == 10:
					var httpErr *types.ErrHTTP
					require.ErrorAs(t, err, &httpErr)
					require.Equal(t, http.StatusBadRequest, httpErr.Code)
					want = types.RoleBasic
				case !owner && (role == types.RoleOwner || role.HasAuditorRole()):
					var httpErr *types.ErrHTTP
					require.ErrorAs(t, err, &httpErr)
					require.Equal(t, http.StatusForbidden, httpErr.Code)
					want = types.RoleBasic
				default:
					require.NoError(t, err)
				}
				require.NoError(t, store.Get(t.Context(), kclient.ObjectKeyFromObject(setting), setting))
				require.Equal(t, want, setting.Spec.Role)
			}
		})
	}
}

package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestEgressAuditAuthorization(t *testing.T) {
	a := NewAuthorizer(nil, nil, nil, false, nil, nil, false)
	for _, tc := range []struct {
		group   string
		allowed bool
	}{
		{
			group:   types.GroupAdmin,
			allowed: true,
		},
		{
			group:   types.GroupOwner,
			allowed: true,
		},
		{
			group:   types.GroupAuditor,
			allowed: true,
		},
		{
			group:   types.GroupBasic,
			allowed: false,
		},
	} {
		for _, path := range []string{"/api/egress-audit-logs", "/api/egress-audit-logs/test", "/api/egress-audit-logs/environments"} {
			t.Run(tc.group+path, func(t *testing.T) {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				allowed := a.Authorize(req, &user.DefaultInfo{UID: "7", Groups: []string{tc.group}})
				require.Equal(t, tc.allowed, allowed)
			})
		}
	}
}

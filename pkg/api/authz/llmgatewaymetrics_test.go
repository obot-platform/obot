package authz

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/stretchr/testify/require"
	"k8s.io/apiserver/pkg/authentication/user"
	clientfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGatewayMetricsAuthorization(t *testing.T) {
	storage := clientfake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	a := NewAuthorizer(nil, storage, storage, false, nil, nil, nil)
	for _, tc := range []struct {
		group   string
		allowed bool
	}{
		{
			group:   types.GroupAdmin,
			allowed: true,
		},
		{
			group:   types.GroupAuditor,
			allowed: true,
		},
		{
			group:   MetricsGroup,
			allowed: true,
		},
		{
			group: types.GroupAuthenticated,
		},
	} {
		t.Run(tc.group, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/llm-gateway/metrics", nil)
			require.Equal(t, tc.allowed, a.Authorize(req, &user.DefaultInfo{UID: "7", Groups: []string{tc.group}}))
		})
	}
}

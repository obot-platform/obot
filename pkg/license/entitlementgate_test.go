package license

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	apitypes "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	kfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestProviderEntitlementGateBlocksARestrictedInstallation(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		restricted bool
		wantStatus int
	}{
		{
			name:   "an ungated path is never blocked",
			method: http.MethodGet,
			path:   "/api/version",
		},
		{
			name:   "a gated path is allowed when the installation fits",
			method: http.MethodGet,
			path:   "/api/llm-proxy/v1/chat/completions",
		},
		{
			name:       "MCP traffic is blocked while restricted",
			method:     http.MethodPost,
			path:       "/mcp-connect/ms1abc",
			restricted: true,
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "LLM traffic is blocked while restricted",
			method:     http.MethodPost,
			path:       "/api/llm-proxy/v1/chat/completions",
			restricted: true,
			wantStatus: http.StatusPaymentRequired,
		},
		{
			name:       "an ungated path is not blocked while restricted",
			method:     http.MethodGet,
			path:       "/api/version",
			restricted: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userCount := int64(1)
			if tt.restricted {
				userCount = 2
			}

			provider := &Provider{entitlements: map[keygen.EntitlementCode]struct{}{}}
			restrictor := NewRestrictor(true,
				fakeLimits{userLimit: gatewayclient.UserLimit{Maximum: 1}},
				&fakeCounts{users: userCount},
			)
			gate := NewProviderEntitlementGate(provider, restrictor, kfake.NewClientBuilder().WithScheme(storagescheme.Scheme).Build())

			err := gate.Check(httptest.NewRequestWithContext(t.Context(), tt.method, tt.path, nil))
			if tt.wantStatus == 0 {
				if err != nil {
					t.Fatalf("Check() error = %v, want nil", err)
				}
				return
			}

			var httpErr *apitypes.ErrHTTP
			if !errors.As(err, &httpErr) {
				t.Fatalf("Check() error = %v, want *types.ErrHTTP", err)
			}
			if httpErr.Code != tt.wantStatus {
				t.Fatalf("Check() status = %d, want %d", httpErr.Code, tt.wantStatus)
			}
		})
	}
}

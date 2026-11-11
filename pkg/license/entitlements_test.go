package license

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	keygen "github.com/keygen-sh/keygen-go/v3"
	"github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"github.com/stretchr/testify/require"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// grants is an entitlement provider that grants itself.
type grants types.Entitlements

func (g grants) Entitlements(context.Context) (types.Entitlements, error) {
	return types.Entitlements(g), nil
}

func TestProviderEntitlements(t *testing.T) {
	for _, tt := range []struct {
		name  string
		codes []string
		want  types.Entitlements
	}{
		{
			name: "no license",
			want: types.Entitlements{
				Users:   new(int64(gatewayclient.DefaultUserLimit)),
				Devices: new(int64(gatewayclient.DefaultDeviceLimit)),
			},
		},
		{
			name:  "community",
			codes: []string{CommunityEntitlement},
			want: types.Entitlements{
				Community: true,
				Users:     new(int64(gatewayclient.DefaultUserLimit)),
				Devices:   new(int64(gatewayclient.DefaultDeviceLimit)),
			},
		},
		{
			name:  "community with the enterprise auth providers",
			codes: []string{CommunityEntitlement, EnterpriseAuthProvidersEntitlement},
			want: types.Entitlements{
				Community:               true,
				EnterpriseAuthProviders: true,
				Users:                   new(int64(gatewayclient.DefaultUserLimit)),
				Devices:                 new(int64(gatewayclient.DefaultDeviceLimit)),
			},
		},
		{
			name:  "enterprise is unlimited",
			codes: []string{CommunityEntitlement, EnterpriseEntitlement, EnterpriseAuthProvidersEntitlement, EnterpriseModelProvidersEntitlement},
			want: types.Entitlements{
				Community:                true,
				Enterprise:               true,
				EnterpriseAuthProviders:  true,
				EnterpriseModelProviders: true,
			},
		},
		{
			name:  "enterprise with a user limit",
			codes: []string{EnterpriseEntitlement, "OBOT_ENTERPRISE_250_USERS"},
			want: types.Entitlements{
				Enterprise: true,
				Users:      new(int64(250)),
			},
		},
		{
			name:  "cloud",
			codes: []string{CloudEntitlement, EnterpriseEntitlement},
			want: types.Entitlements{
				Cloud:      true,
				Enterprise: true,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			codes := make(map[keygen.EntitlementCode]struct{}, len(tt.codes))
			for _, code := range tt.codes {
				codes[keygen.EntitlementCode(code)] = struct{}{}
			}

			got, err := (&Provider{entitlements: codes}).Entitlements(t.Context())
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestLimitViolations(t *testing.T) {
	granted := types.Entitlements{
		Users:     new(int64(10)),
		MemoryMiB: new(int64(0)),
	}

	// At a limit isn't over it, a limit of 0 is a limit, and no limit is never over.
	violations := LimitViolations(granted, types.EntitlementsUsage{
		Users:         10,
		Devices:       1 << 20,
		MemoryMiB:     1,
		CPUMillicores: 1 << 20,
	})
	if len(violations) != 1 || violations[0].Type != "memoryLimit" || violations[0].Message == "" {
		t.Fatalf("LimitViolations() = %+v, want memoryLimit", violations)
	}
	if violations := LimitViolations(granted, types.EntitlementsUsage{}); len(violations) != 0 {
		t.Fatalf("LimitViolations() = %+v, want none", violations)
	}
}

func TestUsage(t *testing.T) {
	gatewayClient := newTestLicenseGatewayClient(t)

	for _, tt := range []struct {
		name  string
		usage *v1.EntitlementUsage
		want  types.EntitlementsUsage
	}{
		{
			name: "the compute last counted",
			usage: &v1.EntitlementUsage{
				Namespace: system.DefaultNamespace,
				Name:      system.EntitlementUsageName,
				Status: v1.EntitlementUsageStatus{
					MemoryMiB:     200,
					CPUMillicores: 10,
				},
			},
			want: types.EntitlementsUsage{
				MemoryMiB:     200,
				CPUMillicores: 10,
			},
		},
		{
			name: "no compute where nothing counts it",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().WithScheme(scheme.Scheme)
			if tt.usage != nil {
				builder = builder.WithObjects(tt.usage)
			}
			got, err := Usage(t.Context(), gatewayClient, builder.Build())
			if err != nil {
				t.Fatalf("Usage() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("Usage() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestMissing(t *testing.T) {
	for _, tt := range []struct {
		name    string
		granted types.Entitlements
		codes   []string
		want    bool
	}{
		{
			name:  "nothing required",
			codes: nil,
		},
		{
			name:  "a feature that isn't granted",
			codes: []string{EnterpriseAuthProvidersEntitlement},
			want:  true,
		},
		{
			name: "a feature that's granted",
			granted: types.Entitlements{
				EnterpriseAuthProviders: true,
			},
			codes: []string{EnterpriseAuthProvidersEntitlement},
		},
		{
			name: "a code for no feature is missing whatever is granted",
			granted: types.Entitlements{
				Enterprise:               true,
				EnterpriseAuthProviders:  true,
				EnterpriseModelProviders: true,
			},
			codes: []string{EnterpriseAuthProvidersEntitlement, "SOMETHING_ELSE"},
			want:  true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := missing(tt.granted, tt.codes); got != tt.want {
				t.Fatalf("missing() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestViolations(t *testing.T) {
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects([]kclient.Object{
		configuredAuthProvider("okta", EnterpriseAuthProvidersEntitlement),
		configuredAuthProvider("github"),
	}...).Build()

	for _, tt := range []struct {
		name    string
		granted types.Entitlements
		used    types.EntitlementsUsage
		want    []string
	}{
		{
			name: "within the limits, with the features every configured auth provider needs",
			granted: types.Entitlements{
				EnterpriseAuthProviders: true,
				Users:                   new(int64(1)),
				MemoryMiB:               new(int64(100)),
			},
			used: types.EntitlementsUsage{
				Users:     1,
				MemoryMiB: 100,
			},
		},
		{
			name: "a configured auth provider that needs a missing feature, then each limit usage is over",
			granted: types.Entitlements{
				Users:     new(int64(1)),
				MemoryMiB: new(int64(100)),
			},
			used: types.EntitlementsUsage{
				Users:         2,
				Devices:       50,
				MemoryMiB:     101,
				CPUMillicores: 50,
			},
			want: []string{"authProvider:okta", "userLimit:", "memoryLimit:"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			violations, err := Violations(t.Context(), storage, tt.granted, tt.used)
			if err != nil {
				t.Fatalf("Violations() error = %v", err)
			}
			var got []string
			for _, violation := range violations {
				got = append(got, violation.Type+":"+violation.Name)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("Violations() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProviderEntitlementGate(t *testing.T) {
	authProvider := configuredAuthProvider("okta", EnterpriseAuthProvidersEntitlement)
	storage := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(authProvider).Build()

	for _, tt := range []struct {
		name        string
		path        string
		granted     types.Entitlements
		wantRefused bool
	}{
		{
			name:        "a gated path while a configured auth provider needs a missing feature",
			path:        "/api/llm-proxy/v1/chat",
			wantRefused: true,
		},
		{
			name: "a gated path once the feature is granted",
			path: "/api/llm-proxy/v1/chat",
			granted: types.Entitlements{
				EnterpriseAuthProviders: true,
			},
		},
		{
			name: "other paths aren't gated",
			path: "/api/me",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gate := NewProviderEntitlementGate(grants(tt.granted), storage)

			err := gate.Check(httptest.NewRequest(http.MethodGet, tt.path, nil))
			if httpErr, ok := errors.AsType[*types.ErrHTTP](err); tt.wantRefused != (ok && httpErr.Code == http.StatusPaymentRequired) || (!tt.wantRefused && err != nil) {
				t.Fatalf("Check() = %v, want refused %v", err, tt.wantRefused)
			}

			missing, err := EntitlementsMissing(t.Context(), grants(tt.granted), authProvider.Spec.RequiredEntitlements)
			if err != nil {
				t.Fatal(err)
			}
			if missing != !tt.granted.EnterpriseAuthProviders {
				t.Fatalf("EntitlementsMissing() = %v, want %v", missing, !tt.granted.EnterpriseAuthProviders)
			}
		})
	}

	// A server without a gate gates nothing.
	if err := (*ProviderEntitlementGate)(nil).Check(httptest.NewRequest(http.MethodGet, "/api/llm-proxy/v1/chat", nil)); err != nil {
		t.Fatalf("Check() on a nil gate = %v", err)
	}
}

// configuredAuthProvider returns a configured auth provider whose manifest requires codes.
func configuredAuthProvider(name string, codes ...string) *v1.AuthProvider {
	authProvider := &v1.AuthProvider{}
	authProvider.Namespace, authProvider.Name = system.DefaultNamespace, name
	authProvider.Spec.RequiredEntitlements = codes
	authProvider.Status.Configured = true
	return authProvider
}

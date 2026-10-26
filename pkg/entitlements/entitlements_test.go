package entitlements

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/license"
)

type fakeReader struct {
	entitlements types.Entitlements
}

func (f fakeReader) Entitlements(context.Context) (types.Entitlements, error) {
	return f.entitlements, nil
}

func limit(n int64) *int64 {
	return new(n)
}

func TestMissing(t *testing.T) {
	granted := types.Entitlements{
		Cloud:                   true,
		EnterpriseAuthProviders: true,
	}
	got := Missing(granted, []string{
		license.EnterpriseAuthProvidersEntitlement,
		license.EnterpriseModelProvidersEntitlement,
		"UNKNOWN",
	})
	if !slices.Equal(got, []string{license.EnterpriseModelProvidersEntitlement, "UNKNOWN"}) {
		t.Fatalf("Missing() = %v", got)
	}
}

func TestRequireEntitlements(t *testing.T) {
	reader := fakeReader{
		entitlements: types.Entitlements{
			EnterpriseModelProviders: true,
		},
	}
	if err := RequireEntitlements(t.Context(), reader, []string{license.EnterpriseModelProvidersEntitlement}); err != nil {
		t.Fatalf("RequireEntitlements() error = %v", err)
	}

	err := RequireEntitlements(t.Context(), reader, []string{license.EnterpriseAuthProvidersEntitlement})
	if httpErr, ok := errors.AsType[*types.ErrHTTP](err); !ok || httpErr.Code != http.StatusPaymentRequired {
		t.Fatalf("RequireEntitlements() error = %v, want 402", err)
	}
}

func TestLimitViolations(t *testing.T) {
	for _, tt := range []struct {
		name         string
		entitlements types.Entitlements
		usage        types.EntitlementsUsage
		want         []string
	}{
		{
			name:  "unlimited",
			usage: types.EntitlementsUsage{Users: 1000},
		},
		{
			name:         "at a zero limit",
			entitlements: types.Entitlements{Users: limit(0)},
		},
		{
			name: "over every limit",
			entitlements: types.Entitlements{
				Users:         limit(0),
				Devices:       limit(1),
				MemoryMiB:     limit(1024),
				CPUMillicores: limit(1000),
			},
			usage: types.EntitlementsUsage{
				Users:         1,
				Devices:       2,
				MemoryMiB:     2048,
				CPUMillicores: 2000,
			},
			want: []string{"userLimit", "deviceLimit", "memoryLimit", "cpuLimit"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, violation := range LimitViolations(tt.entitlements, tt.usage) {
				got = append(got, violation.Type)
			}
			if !slices.Equal(got, tt.want) {
				t.Fatalf("LimitViolations() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExceeded(t *testing.T) {
	hosted := []Limit{Memory, CPU}
	for _, tt := range []struct {
		name   string
		plan   types.Entitlements
		before types.EntitlementsUsage
		after  types.EntitlementsUsage
		want   string
	}{
		{
			name: "unlimited",
			after: types.EntitlementsUsage{
				MemoryMiB: 100000,
			},
		},
		{
			name: "within the limits",
			plan: types.Entitlements{
				MemoryMiB:     limit(1024),
				CPUMillicores: limit(1000),
			},
			before: types.EntitlementsUsage{
				MemoryMiB: 512,
			},
			after: types.EntitlementsUsage{
				MemoryMiB:     1024,
				CPUMillicores: 1000,
			},
		},
		{
			name: "past the memory limit",
			plan: types.Entitlements{
				MemoryMiB: limit(1024),
			},
			after: types.EntitlementsUsage{
				MemoryMiB: 1025,
			},
			want: "There isn't room on the plan's hosted MCP server memory for this. Free some up, or ask an owner to change the plan.",
		},
		{
			name: "past the CPU limit",
			plan: types.Entitlements{
				CPUMillicores: limit(1000),
			},
			after: types.EntitlementsUsage{
				CPUMillicores: 1250,
			},
			want: "There isn't room on the plan's hosted MCP server CPU for this. Free some up, or ask an owner to change the plan.",
		},
		{
			name: "over but coming down",
			plan: types.Entitlements{
				MemoryMiB: limit(1024),
			},
			before: types.EntitlementsUsage{
				MemoryMiB: 4096,
			},
			after: types.EntitlementsUsage{
				MemoryMiB: 2048,
			},
		},
		{
			name: "over a limit the change doesn't add to",
			plan: types.Entitlements{
				MemoryMiB:     limit(1024),
				CPUMillicores: limit(1000),
			},
			before: types.EntitlementsUsage{
				MemoryMiB:     4096,
				CPUMillicores: 500,
			},
			after: types.EntitlementsUsage{
				MemoryMiB:     4096,
				CPUMillicores: 750,
			},
		},
		{
			name: "only the given limits are checked",
			plan: types.Entitlements{
				Users: limit(1),
			},
			after: types.EntitlementsUsage{
				Users: 2,
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := Exceeded(hosted, tt.plan, tt.before, tt.after)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Exceeded() = %v, want nil", err)
				}
				return
			}
			var httpErr *types.ErrHTTP
			if !errors.As(err, &httpErr) || httpErr.Code != http.StatusPaymentRequired || httpErr.Message != tt.want {
				t.Fatalf("Exceeded() = %v, want a %d with %q", err, http.StatusPaymentRequired, tt.want)
			}
		})
	}
}

package billing

import (
	"context"
	"testing"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

type fakeEntitlementSource struct {
	entitlements *Entitlements
}

type fakeLicenseLimits struct {
	userLimit        gatewayclient.UserLimit
	deviceLimit      gatewayclient.DeviceLimit
	hostedServers    gatewayclient.HostedMCPServerLimit
	auditLogRetained gatewayclient.AuditLogRetention
}

func (f fakeEntitlementSource) Entitlements() *Entitlements {
	return f.entitlements
}

func (f fakeLicenseLimits) UserLimit(context.Context) (gatewayclient.UserLimit, error) {
	return f.userLimit, nil
}

func (f fakeLicenseLimits) DeviceLimit(context.Context) (gatewayclient.DeviceLimit, error) {
	return f.deviceLimit, nil
}

func (f fakeLicenseLimits) HostedMCPServerLimit(context.Context) (gatewayclient.HostedMCPServerLimit, error) {
	return f.hostedServers, nil
}

func (f fakeLicenseLimits) AuditLogRetention(context.Context) (gatewayclient.AuditLogRetention, error) {
	return f.auditLogRetained, nil
}

func TestLimitsPreferTheSubscription(t *testing.T) {
	licenseLimits := fakeLicenseLimits{
		userLimit:        gatewayclient.UserLimit{Maximum: 500},
		deviceLimit:      gatewayclient.DeviceLimit{Maximum: 250},
		hostedServers:    gatewayclient.HostedMCPServerLimit{Unlimited: true},
		auditLogRetained: gatewayclient.AuditLogRetention{Unlimited: true},
	}

	tests := []struct {
		name              string
		entitlements      *Entitlements
		wantUserLimit     gatewayclient.UserLimit
		wantDeviceLimit   gatewayclient.DeviceLimit
		wantHostedServers gatewayclient.HostedMCPServerLimit
		wantRetention     gatewayclient.AuditLogRetention
	}{
		{
			name:              "no entitlements leaves every limit to the license",
			entitlements:      nil,
			wantUserLimit:     licenseLimits.userLimit,
			wantDeviceLimit:   licenseLimits.deviceLimit,
			wantHostedServers: licenseLimits.hostedServers,
			wantRetention:     licenseLimits.auditLogRetained,
		},
		{
			name: "a subscription wins over the license",
			entitlements: &Entitlements{
				Seats:                 20,
				HostedMCPServers:      15,
				AuditLogRetentionDays: 30,
			},
			wantUserLimit:     gatewayclient.UserLimit{Maximum: 20},
			wantDeviceLimit:   licenseLimits.deviceLimit,
			wantHostedServers: gatewayclient.HostedMCPServerLimit{Maximum: 15},
			wantRetention:     gatewayclient.AuditLogRetention{Days: 30},
		},
		{
			name: "limits the subscription omits come from the license",
			entitlements: &Entitlements{
				Seats: 10,
			},
			wantUserLimit:     gatewayclient.UserLimit{Maximum: 10},
			wantDeviceLimit:   licenseLimits.deviceLimit,
			wantHostedServers: licenseLimits.hostedServers,
			wantRetention:     licenseLimits.auditLogRetained,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := NewLimits(fakeEntitlementSource{entitlements: tt.entitlements}, licenseLimits)

			userLimit, err := limits.UserLimit(t.Context())
			if err != nil {
				t.Fatalf("UserLimit() error = %v, want nil", err)
			}
			if userLimit != tt.wantUserLimit {
				t.Fatalf("UserLimit() = %+v, want %+v", userLimit, tt.wantUserLimit)
			}

			deviceLimit, err := limits.DeviceLimit(t.Context())
			if err != nil {
				t.Fatalf("DeviceLimit() error = %v, want nil", err)
			}
			if deviceLimit != tt.wantDeviceLimit {
				t.Fatalf("DeviceLimit() = %+v, want %+v", deviceLimit, tt.wantDeviceLimit)
			}

			hostedServers, err := limits.HostedMCPServerLimit(t.Context())
			if err != nil {
				t.Fatalf("HostedMCPServerLimit() error = %v, want nil", err)
			}
			if hostedServers != tt.wantHostedServers {
				t.Fatalf("HostedMCPServerLimit() = %+v, want %+v", hostedServers, tt.wantHostedServers)
			}

			retention, err := limits.AuditLogRetention(t.Context())
			if err != nil {
				t.Fatalf("AuditLogRetention() error = %v, want nil", err)
			}
			if retention != tt.wantRetention {
				t.Fatalf("AuditLogRetention() = %+v, want %+v", retention, tt.wantRetention)
			}
		})
	}
}

func TestLimitsAreNotAdditive(t *testing.T) {
	limits := NewLimits(
		fakeEntitlementSource{entitlements: &Entitlements{Seats: 20}},
		fakeLicenseLimits{userLimit: gatewayclient.UserLimit{Maximum: 500}},
	)

	userLimit, err := limits.UserLimit(t.Context())
	if err != nil {
		t.Fatalf("UserLimit() error = %v, want nil", err)
	}
	if userLimit.Maximum != 20 {
		t.Fatalf("UserLimit() = %+v, want the subscription's 20 seats", userLimit)
	}
}

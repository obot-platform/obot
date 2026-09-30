package billing

import (
	"slices"
	"testing"
)

func TestEntitlementNames(t *testing.T) {
	tests := []struct {
		name         string
		entitlements *Entitlements
		want         []string
	}{
		{
			name: "no entitlements",
			want: nil,
		},
		{
			name:         "empty entitlements name nothing",
			entitlements: &Entitlements{},
			want:         nil,
		},
		{
			name: "every limit",
			entitlements: &Entitlements{
				Plan:                  "Business",
				Status:                StatusActive,
				Seats:                 20,
				HostedMCPServers:      15,
				AuditLogRetentionDays: 30,
			},
			want: []string{
				"OBOT_20_USERS",
				"OBOT_15_HOSTED_MCP_SERVERS",
				"OBOT_30_AUDIT_LOG_DAYS",
			},
		},
		{
			name: "zero and negative numbers name nothing",
			entitlements: &Entitlements{
				Seats:                 0,
				HostedMCPServers:      -1,
				AuditLogRetentionDays: 7,
			},
			want: []string{"OBOT_7_AUDIT_LOG_DAYS"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.entitlements.Names(); !slices.Equal(got, tt.want) {
				t.Fatalf("Names() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEntitlementStatus(t *testing.T) {
	tests := []struct {
		name                string
		status              string
		wantEnded           bool
		wantPaymentRequired bool
	}{
		{
			name:   "trialing",
			status: StatusTrialing,
		},
		{
			name:   "active",
			status: StatusActive,
		},
		{
			name:                "past due",
			status:              StatusPastDue,
			wantPaymentRequired: true,
		},
		{
			name:                "incomplete",
			status:              StatusIncomplete,
			wantPaymentRequired: true,
		},
		{
			name:                "unpaid",
			status:              StatusUnpaid,
			wantEnded:           true,
			wantPaymentRequired: true,
		},
		{
			name:      "canceled",
			status:    StatusCanceled,
			wantEnded: true,
		},
		{
			name:      "incomplete expired",
			status:    StatusIncompleteExpired,
			wantEnded: true,
		},
		{
			name:   "an unrecognized status is live",
			status: "something_new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entitlements := &Entitlements{Status: tt.status}
			if got := entitlements.Ended(); got != tt.wantEnded {
				t.Fatalf("Ended() = %t, want %t", got, tt.wantEnded)
			}
			if got := entitlements.PaymentRequired(); got != tt.wantPaymentRequired {
				t.Fatalf("PaymentRequired() = %t, want %t", got, tt.wantPaymentRequired)
			}
		})
	}
}

func TestNilEntitlementsAreLive(t *testing.T) {
	var entitlements *Entitlements
	if entitlements.Ended() {
		t.Fatal("Ended() = true for unknown entitlements")
	}
	if entitlements.PaymentRequired() {
		t.Fatal("PaymentRequired() = true for unknown entitlements")
	}
	if got := entitlements.Names(); got != nil {
		t.Fatalf("Names() = %v, want nil", got)
	}
}

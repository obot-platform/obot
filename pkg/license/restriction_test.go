package license

import (
	"context"
	"errors"
	"testing"
	"time"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
)

type fakeLimits struct {
	userLimit     gatewayclient.UserLimit
	hostedServers gatewayclient.HostedMCPServerLimit
	err           error
}

type fakeCounts struct {
	users         int64
	hostedServers int64
	err           error
}

func (f fakeLimits) UserLimit(context.Context) (gatewayclient.UserLimit, error) {
	return f.userLimit, f.err
}

func (f fakeLimits) DeviceLimit(context.Context) (gatewayclient.DeviceLimit, error) {
	return gatewayclient.DeviceLimit{}, f.err
}

func (f fakeLimits) HostedMCPServerLimit(context.Context) (gatewayclient.HostedMCPServerLimit, error) {
	return f.hostedServers, f.err
}

func (f fakeLimits) AuditLogRetention(context.Context) (gatewayclient.AuditLogRetention, error) {
	return gatewayclient.AuditLogRetention{}, f.err
}

func (f *fakeCounts) UserCount(context.Context) (int64, error) {
	return f.users, f.err
}

func (f *fakeCounts) HostedMCPServerCount(context.Context) (int64, error) {
	return f.hostedServers, f.err
}

func TestRestrictorIsOffByDefault(t *testing.T) {
	restrictor := NewRestrictor(false, fakeLimits{
		userLimit:     gatewayclient.UserLimit{Maximum: 1},
		hostedServers: gatewayclient.HostedMCPServerLimit{Maximum: 1},
	}, &fakeCounts{
		users:         100,
		hostedServers: 100,
	})

	if restrictor.Enabled() {
		t.Fatal("Enabled() = true, want false")
	}
	restricted, err := restrictor.Restricted(t.Context())
	if err != nil {
		t.Fatalf("Restricted() error = %v, want nil", err)
	}
	if restricted {
		t.Fatal("Restricted() = true with enforcement disabled")
	}
}

func TestRestrictorNilIsInert(t *testing.T) {
	var restrictor *Restrictor
	if restrictor.Enabled() {
		t.Fatal("Enabled() = true for a nil restrictor")
	}
	restricted, err := restrictor.Restricted(t.Context())
	if err != nil {
		t.Fatalf("Restricted() error = %v, want nil", err)
	}
	if restricted {
		t.Fatal("Restricted() = true for a nil restrictor")
	}
}

func TestRestrictorEvaluatesEveryLimit(t *testing.T) {
	tests := []struct {
		name           string
		limits         fakeLimits
		counts         fakeCounts
		wantRestricted bool
		wantViolation  string
	}{
		{
			name: "within every limit",
			limits: fakeLimits{
				userLimit:     gatewayclient.UserLimit{Maximum: 20},
				hostedServers: gatewayclient.HostedMCPServerLimit{Maximum: 15},
			},
			counts: fakeCounts{
				users:         20,
				hostedServers: 15,
			},
		},
		{
			name: "over the seat limit",
			limits: fakeLimits{
				userLimit:     gatewayclient.UserLimit{Maximum: 20},
				hostedServers: gatewayclient.HostedMCPServerLimit{Maximum: 15},
			},
			counts: fakeCounts{
				users:         21,
				hostedServers: 15,
			},
			wantRestricted: true,
			wantViolation:  "userLimit",
		},
		{
			name: "over the hosted server limit",
			limits: fakeLimits{
				userLimit:     gatewayclient.UserLimit{Maximum: 20},
				hostedServers: gatewayclient.HostedMCPServerLimit{Maximum: 15},
			},
			counts: fakeCounts{
				users:         20,
				hostedServers: 16,
			},
			wantRestricted: true,
			wantViolation:  "hostedMCPServerLimit",
		},
		{
			name: "unlimited seats are never exceeded",
			limits: fakeLimits{
				userLimit:     gatewayclient.UserLimit{Unlimited: true},
				hostedServers: gatewayclient.HostedMCPServerLimit{Unlimited: true},
			},
			counts: fakeCounts{
				users:         100000,
				hostedServers: 100000,
			},
		},
		{
			name: "an unbounded hosted server limit is never exceeded",
			limits: fakeLimits{
				userLimit: gatewayclient.UserLimit{Maximum: 20},
			},
			counts: fakeCounts{
				users:         20,
				hostedServers: 100000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restrictor := NewRestrictor(true, tt.limits, &tt.counts)

			restriction, err := restrictor.Restriction(t.Context())
			if err != nil {
				t.Fatalf("Restriction() error = %v, want nil", err)
			}
			if restriction.Restricted != tt.wantRestricted {
				t.Fatalf("Restriction() = %+v, want restricted %t", restriction, tt.wantRestricted)
			}
			if tt.wantViolation == "" {
				return
			}
			if len(restriction.Violations) != 1 || restriction.Violations[0].Type != tt.wantViolation {
				t.Fatalf("Restriction() violations = %+v, want %q", restriction.Violations, tt.wantViolation)
			}
		})
	}
}

func TestRestrictorLiftsWhenTheInstallationFits(t *testing.T) {
	counts := &fakeCounts{
		users:         21,
		hostedServers: 0,
	}
	restrictor := NewRestrictor(true, fakeLimits{userLimit: gatewayclient.UserLimit{Maximum: 20}}, counts)

	now := time.Now()
	restrictor.now = func() time.Time { return now }

	restricted, err := restrictor.Restricted(t.Context())
	if err != nil {
		t.Fatalf("Restricted() error = %v, want nil", err)
	}
	if !restricted {
		t.Fatal("Restricted() = false while over the seat limit")
	}

	// A user is soft-deleted, which frees its seat. Nothing else happens.
	counts.users = 20
	now = now.Add(restrictionTTL + time.Second)

	restricted, err = restrictor.Restricted(t.Context())
	if err != nil {
		t.Fatalf("Restricted() error = %v, want nil", err)
	}
	if restricted {
		t.Fatal("Restricted() = true after the installation fits again")
	}
}

func TestRestrictorCachesWithinItsInterval(t *testing.T) {
	counts := &fakeCounts{users: 21}
	restrictor := NewRestrictor(true, fakeLimits{userLimit: gatewayclient.UserLimit{Maximum: 20}}, counts)

	now := time.Now()
	restrictor.now = func() time.Time { return now }

	if restricted, err := restrictor.Restricted(t.Context()); err != nil || !restricted {
		t.Fatalf("Restricted() = (%t, %v), want (true, nil)", restricted, err)
	}

	counts.users = 1
	if restricted, err := restrictor.Restricted(t.Context()); err != nil || !restricted {
		t.Fatalf("Restricted() = (%t, %v), want the cached (true, nil)", restricted, err)
	}
}

func TestRestrictorReportsFailures(t *testing.T) {
	restrictor := NewRestrictor(true, fakeLimits{err: errors.New("license lookup failed")}, &fakeCounts{})
	if _, err := restrictor.Restricted(t.Context()); err == nil {
		t.Fatal("Restricted() error = nil, want a failure")
	}

	restrictor = NewRestrictor(true,
		fakeLimits{userLimit: gatewayclient.UserLimit{Maximum: 20}},
		&fakeCounts{err: errors.New("database is unreachable")},
	)
	if _, err := restrictor.Restricted(t.Context()); err == nil {
		t.Fatal("Restricted() error = nil, want a failure")
	}
}

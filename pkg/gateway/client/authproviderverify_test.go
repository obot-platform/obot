package client

import (
	"testing"
	"time"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

// The verification is the only thing that lets a staged auth provider serve a login, and it is
// scoped to its own ID so that opening one does not open the provider to everyone else.
func TestAuthProviderVerificationActive(t *testing.T) {
	c := newTestClient(t)

	mustCreate := func(id string, purpose string, expiresAt time.Time) {
		t.Helper()
		if err := c.CreateTokenRequest(t.Context(), &types.TokenRequest{
			ID:               id,
			Purpose:          purpose,
			RequestExpiresAt: expiresAt,
		}); err != nil {
			t.Fatalf("creating token request %q: %v", id, err)
		}
	}

	mustCreate("live-verification", types.TokenRequestPurposeAuthProviderVerify, time.Now().Add(15*time.Minute))
	mustCreate("expired-verification", types.TokenRequestPurposeAuthProviderVerify, time.Now().Add(-time.Minute))
	mustCreate("setup-request", types.TokenRequestPurposeSetup, time.Now().Add(time.Hour))

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{
			name: "live verification",
			id:   "live-verification",
			want: true,
		},
		{
			name: "expired verification",
			id:   "expired-verification",
		},
		// A different flow's token must not stand in for a verification.
		{
			name: "setup token request",
			id:   "setup-request",
		},
		{
			name: "unknown id",
			id:   "not-a-real-id",
		},
		// An absent cookie reaches this as an empty ID and must not open the window.
		{
			name: "empty id",
			id:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			active, err := c.AuthProviderVerificationActive(t.Context(), tt.id)
			if err != nil {
				t.Fatalf("checking verification: %v", err)
			}
			if active != tt.want {
				t.Errorf("AuthProviderVerificationActive(%q) = %v, want %v", tt.id, active, tt.want)
			}
		})
	}
}

// A verification is tied to the Owner who started the switch, so holding the ID is not on its own
// enough to open the replacement provider's login.
func TestAuthProviderVerificationStartableBy(t *testing.T) {
	c := newTestClient(t)

	const owner, other = uint(7), uint(8)
	mustCreate := func(id string, purpose string, expiresAt time.Time, ownerID *uint) {
		t.Helper()
		if err := c.CreateTokenRequest(t.Context(), &types.TokenRequest{
			ID:               id,
			Purpose:          purpose,
			RequestExpiresAt: expiresAt,
			OwnerUserID:      ownerID,
		}); err != nil {
			t.Fatalf("creating token request %q: %v", id, err)
		}
	}

	ownerID := owner
	mustCreate("owned", types.TokenRequestPurposeAuthProviderVerify, time.Now().Add(15*time.Minute), &ownerID)
	mustCreate("expired", types.TokenRequestPurposeAuthProviderVerify, time.Now().Add(-time.Minute), &ownerID)
	mustCreate("setup", types.TokenRequestPurposeSetup, time.Now().Add(time.Hour), &ownerID)
	mustCreate("unowned", types.TokenRequestPurposeAuthProviderVerify, time.Now().Add(15*time.Minute), nil)

	tests := []struct {
		name   string
		id     string
		userID uint
		want   bool
	}{
		{
			name:   "the owner who started it",
			id:     "owned",
			userID: owner,
			want:   true,
		},
		// The whole point: another signed-in user holding the ID is still refused.
		{
			name:   "a different signed-in user",
			id:     "owned",
			userID: other,
		},
		{
			name:   "expired",
			id:     "expired",
			userID: owner,
		},
		{
			name:   "another flow's token",
			id:     "setup",
			userID: owner,
		},
		// A verification with no recorded owner predates this binding and must not be startable.
		{
			name:   "no recorded owner",
			id:     "unowned",
			userID: owner,
		},
		{
			name:   "unauthenticated caller",
			id:     "owned",
			userID: 0,
		},
		{
			name:   "empty id",
			id:     "",
			userID: owner,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startable, err := c.AuthProviderVerificationStartableBy(t.Context(), tt.id, tt.userID)
			if err != nil {
				t.Fatalf("checking verification: %v", err)
			}
			if startable != tt.want {
				t.Errorf("AuthProviderVerificationStartableBy(%q, %d) = %v, want %v", tt.id, tt.userID, startable, tt.want)
			}
		})
	}
}

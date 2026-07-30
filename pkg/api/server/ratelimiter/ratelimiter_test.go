package ratelimiter

import (
	"net/http/httptest"
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"k8s.io/apiserver/pkg/authentication/user"
)

func TestTunnelBridgeIsExemptFromRateLimiting(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 1,
		AuthenticatedRateLimit:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	bridge := &user.DefaultInfo{
		Name:   "obot-tunnel-bridge",
		UID:    "obot-tunnel-bridge",
		Groups: []string{types.GroupTunnelBridge},
	}
	request := httptest.NewRequest("POST", "/tunnel/bridge/target", nil)
	response := httptest.NewRecorder()

	for range 2 {
		if err := limiter.ApplyLimit(bridge, response, request); err != nil {
			t.Fatalf("ApplyLimit() error = %v", err)
		}
	}
	if got := response.Header().Get(headerRateLimitLimit); got != "" {
		t.Fatalf("%s = %q, want empty for exempt bridge request", headerRateLimitLimit, got)
	}
}

func TestTunnelPeerIsExemptFromRateLimiting(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 1,
		AuthenticatedRateLimit:   1,
	})
	if err != nil {
		t.Fatal(err)
	}

	peer := &user.DefaultInfo{
		Name:   "obot-tunnel-peer",
		UID:    "obot-tunnel-peer",
		Groups: []string{types.GroupTunnelPeer},
	}
	request := httptest.NewRequest("GET", "/tunnel/peer", nil)
	response := httptest.NewRecorder()

	for range 2 {
		if err := limiter.ApplyLimit(peer, response, request); err != nil {
			t.Fatalf("ApplyLimit() error = %v", err)
		}
	}
	if got := response.Header().Get(headerRateLimitLimit); got != "" {
		t.Fatalf("%s = %q, want empty for exempt peer request", headerRateLimitLimit, got)
	}
}

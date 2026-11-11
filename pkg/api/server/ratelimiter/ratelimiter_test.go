package ratelimiter

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

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
	request := httptest.NewRequest(http.MethodPost, "/tunnel/bridge/target", nil)
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
	request := httptest.NewRequest(http.MethodGet, "/tunnel/peer", nil)
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

func TestCompositeLoopbackDoesNotSpendTheUsersLimit(t *testing.T) {
	// A vMCP's loopback token: the user's identity, scoped to the vMCP and its components.
	loopback := &user.DefaultInfo{
		Name:   "user",
		UID:    "42",
		Groups: []string{types.GroupMCP, types.GroupCompositeMCP, types.GroupAuthenticated},
		Extra:  map[string][]string{"authorized_mcp_ids": {"vmcp1abc", "ms1abc"}},
	}
	direct := &user.DefaultInfo{
		Name:   "user",
		UID:    "42",
		Groups: []string{types.GroupAuthenticated},
	}

	for _, tt := range []struct {
		name    string
		user    user.Info
		path    string
		limited bool
	}{
		{
			name:    "loopback to a component it is scoped to",
			user:    loopback,
			path:    "/mcp-connect/ms1abc",
			limited: false,
		},
		{
			name:    "loopback to a subpath of a component it is scoped to",
			user:    loopback,
			path:    "/mcp-connect/ms1abc/messages",
			limited: false,
		},
		{
			name:    "loopback to the composite it is scoped to",
			user:    loopback,
			path:    "/mcp-connect-composite/vmcp1abc",
			limited: false,
		},
		{
			name:    "loopback token on an API route",
			user:    loopback,
			path:    "/api/mcp-servers",
			limited: true,
		},
		{
			name:    "loopback token for an MCP ID it is not scoped to",
			user:    loopback,
			path:    "/mcp-connect/ms1other",
			limited: true,
		},
		{
			name:    "the user's own request to the vMCP",
			user:    direct,
			path:    "/mcp-connect/vmcp1abc",
			limited: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			limiter, err := New(Options{
				UnauthenticatedRateLimit: 1,
				AuthenticatedRateLimit:   1,
			})
			if err != nil {
				t.Fatal(err)
			}

			// A cold vMCP connect makes several loopback requests per component.
			for i := range 3 {
				response := httptest.NewRecorder()
				err := limiter.ApplyLimit(tt.user, response, httptest.NewRequest(http.MethodPost, tt.path, nil))
				switch {
				case !tt.limited && err != nil:
					t.Fatalf("request %d: ApplyLimit() error = %v, want exempt", i, err)
				case !tt.limited && response.Header().Get(headerRateLimitLimit) != "":
					t.Fatalf("request %d: %s = %q, want empty for an exempt request", i, headerRateLimitLimit, response.Header().Get(headerRateLimitLimit))
				case tt.limited && i == 0 && err != nil:
					t.Fatalf("first request: ApplyLimit() error = %v", err)
				case tt.limited && i > 0 && err != ErrRateLimitExceeded:
					t.Fatalf("request %d: ApplyLimit() error = %v, want %v", i, err, ErrRateLimitExceeded)
				}
			}
		})
	}
}

func TestSCIMConnectionIsLimitedByConnectionAtTheAuthenticatedRate(t *testing.T) {
	limiter, err := New(Options{
		UnauthenticatedRateLimit: 1,
		AuthenticatedRateLimit:   2,
	})
	if err != nil {
		t.Fatal(err)
	}

	connection := &user.DefaultInfo{
		Name:   "scim-connection:conn-1",
		UID:    "conn-1",
		Groups: []string{types.GroupSCIM},
	}
	request := httptest.NewRequest(http.MethodGet, "/scim/v2/Users", nil)

	for i := range 2 {
		response := httptest.NewRecorder()
		if err := limiter.ApplyLimit(connection, response, request); err != nil {
			t.Fatalf("request %d: ApplyLimit() error = %v", i, err)
		}
		if got := response.Header().Get(headerRateLimitLimit); got != "2" {
			t.Fatalf("%s = %q, want the authenticated limit 2", headerRateLimitLimit, got)
		}
	}

	response := httptest.NewRecorder()
	if err := limiter.ApplyLimit(connection, response, request); err != ErrRateLimitExceeded {
		t.Fatalf("ApplyLimit() error = %v, want %v", err, ErrRateLimitExceeded)
	}
	retryAfter := response.Header().Get(headerRetryAfter)
	if seconds, err := strconv.Atoi(retryAfter); err != nil || seconds < 1 {
		t.Fatalf("%s = %q, want a positive integer number of seconds", headerRetryAfter, retryAfter)
	}

	// A new connection, after SCIM is set up again, has its own budget.
	response = httptest.NewRecorder()
	if err := limiter.ApplyLimit(&user.DefaultInfo{
		Name:   "scim-connection:conn-2",
		UID:    "conn-2",
		Groups: []string{types.GroupSCIM},
	}, response, request); err != nil {
		t.Fatalf("ApplyLimit() for another connection error = %v", err)
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name  string
		reset time.Time
		want  int
	}{
		{
			name:  "rounds a partial second up",
			reset: now.Add(1500 * time.Millisecond),
			want:  2,
		},
		{
			name:  "keeps a whole second",
			reset: now.Add(3 * time.Second),
			want:  3,
		},
		{
			name:  "never answers less than a second",
			reset: now.Add(-time.Second),
			want:  1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryAfterSeconds(tt.reset, now); got != tt.want {
				t.Fatalf("retryAfterSeconds() = %d, want %d", got, tt.want)
			}
		})
	}
}

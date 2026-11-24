package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"uuid"

	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/principal"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

const (
	grantTestPostgresDSNEnv = "OBOT_TEST_POSTGRES_DSN"
	grantTestURL            = "https://provider.example.com/mcp"
)

// rotatingTokenEndpoint is an OAuth token endpoint that rotates refresh tokens, as many providers do: each refresh
// token works once, and using it again is invalid_grant. It counts refreshes, and can be told to fail.
type rotatingTokenEndpoint struct {
	server *httptest.Server

	lock      sync.Mutex
	current   string
	refreshes atomic.Int32
	// status, when set, answers every refresh with this status and a server error.
	status int
}

func newRotatingTokenEndpoint(t *testing.T, refreshToken string) *rotatingTokenEndpoint {
	t.Helper()
	endpoint := &rotatingTokenEndpoint{current: refreshToken}
	endpoint.server = httptest.NewServer(http.HandlerFunc(endpoint.serve))
	t.Cleanup(endpoint.server.Close)
	return endpoint
}

func (e *rotatingTokenEndpoint) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || r.Form.Get("grant_type") != "refresh_token" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	e.lock.Lock()
	defer e.lock.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if e.status != 0 {
		w.WriteHeader(e.status)
		_, _ = w.Write([]byte(`{"error":"server_error"}`))
		return
	}
	if r.Form.Get("refresh_token") != e.current {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
		return
	}
	n := e.refreshes.Add(1)
	e.current = "refresh-" + string('a'+n)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"access_token":  "access-" + string('a'+n),
		"refresh_token": e.current,
		"token_type":    "Bearer",
		"expires_in":    3600,
	})
}

func (e *rotatingTokenEndpoint) fail(status int) {
	e.lock.Lock()
	defer e.lock.Unlock()
	e.status = status
}

// endpointContext lets refreshes reach the test's token endpoint, on a loopback address that the default strict
// egress client refuses.
func endpointContext(t *testing.T) context.Context {
	t.Helper()
	return context.WithValue(t.Context(), oauth2.HTTPClient, http.DefaultClient)
}

// newGrantTestGateway returns a gateway client over a new SQLite database.
func newGrantTestGateway(t *testing.T) *gatewayclient.Client {
	t.Helper()
	services, err := sservices.New(sservices.Config{DSN: "sqlite://:memory:"})
	require.NoError(t, err)
	db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate())
	gateway := gatewayclient.New(t.Context(), db, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
	t.Cleanup(func() { require.NoError(t, gateway.Close()) })
	return gateway
}

// storeExpiredGrant stores a vMCP preconfigured OAuth grant whose access token has expired, issued by endpoint.
func storeExpiredGrant(t *testing.T, gateway *gatewayclient.Client, owner, mcpID string, endpoint *rotatingTokenEndpoint) {
	t.Helper()
	require.NoError(t, gateway.ReplaceMCPOAuthGrant(t.Context(), owner, mcpID, grantTestURL, "grant-1", "1", &oauth2.Config{
		ClientID: "client",
		Endpoint: oauth2.Endpoint{TokenURL: endpoint.server.URL, AuthStyle: oauth2.AuthStyleInParams},
		Scopes:   []string{"read"},
	}, &oauth2.Token{
		AccessToken:  "access-expired",
		RefreshToken: endpoint.current,
		TokenType:    "Bearer",
		Expiry:       time.Now().Add(-time.Hour),
	}))
}

// refreshConcurrently gets a token for the grant from callers goroutines on each gateway, each through a token source
// of its own, as every agent's connection has, and returns the access tokens they got.
func refreshConcurrently(t *testing.T, gateways []*gatewayclient.Client, owner, mcpID string, callers int) []string {
	t.Helper()
	var (
		wait    sync.WaitGroup
		start   = make(chan struct{})
		lock    sync.Mutex
		results []string
		errs    []error
	)
	for _, gateway := range gateways {
		store := NewGlobalTokenStore(gateway)
		for range callers {
			wait.Go(func() {
				<-start
				source, err := store.ForUserAndMCP(owner, mcpID, grantTestURL).TokenSource(endpointContext(t))
				var token *oauth2.Token
				if err == nil {
					token, err = source.Token()
				}
				lock.Lock()
				defer lock.Unlock()
				if err != nil {
					errs = append(errs, err)
					return
				}
				results = append(results, token.AccessToken)
			})
		}
	}
	close(start)
	wait.Wait()
	require.Empty(t, errs)
	return results
}

// postgresGrantGateways returns the gateway clients of n replicas, each with a connection pool of its own, on one
// PostgreSQL schema of their own. It skips the test unless OBOT_TEST_POSTGRES_DSN names a PostgreSQL URL whose user
// can create schemas.
func postgresGrantGateways(t *testing.T, n int) []*gatewayclient.Client {
	t.Helper()
	dsn := os.Getenv(grantTestPostgresDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", grantTestPostgresDSNEnv)
	}
	admin, err := sservices.New(sservices.Config{DSN: dsn})
	require.NoError(t, err)
	schema := "obot_grant_refresh_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	require.NoError(t, admin.DB.DB.Exec("CREATE SCHEMA "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, admin.DB.DB.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		require.NoError(t, admin.DB.SQLDB.Close())
	})
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()

	replica := func(migrate bool) *gatewayclient.Client {
		services, err := sservices.New(sservices.Config{DSN: u.String()})
		require.NoError(t, err)
		db, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, migrate)
		require.NoError(t, err)
		require.NoError(t, db.AutoMigrate())
		gateway := gatewayclient.New(t.Context(), db, nil, nil, nil, time.Hour, 10, 90, 90, 90, true)
		t.Cleanup(func() {
			require.NoError(t, gateway.Close())
			require.NoError(t, services.DB.SQLDB.Close())
		})
		return gateway
	}
	gateways := make([]*gatewayclient.Client, 0, n)
	for i := range n {
		gateways = append(gateways, replica(i == 0))
	}
	return gateways
}

// TestSharedGrantRefreshesOnce checks that many agents whose shared vMCP grant expired at once refresh it once:
// every caller but the first finds the token the first stored, rather than refreshing a refresh token the provider
// has already rotated, which would fail and invalidate the grant for everyone.
func TestSharedGrantRefreshesOnce(t *testing.T) {
	gateway := newGrantTestGateway(t)
	endpoint := newRotatingTokenEndpoint(t, "refresh-initial")
	owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
	storeExpiredGrant(t, gateway, owner, mcpID, endpoint)

	results := refreshConcurrently(t, []*gatewayclient.Client{gateway}, owner, mcpID, 20)
	require.Len(t, results, 20)
	for _, token := range results {
		require.Equal(t, "access-b", token)
	}
	require.EqualValues(t, 1, endpoint.refreshes.Load())

	// The refresh was saved with the rotated refresh token, and kept who authorized the grant.
	stored, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
	require.NoError(t, err)
	require.Equal(t, "access-b", stored.AccessToken)
	require.Equal(t, "refresh-b", stored.RefreshToken)
	require.Equal(t, "grant-1", stored.GrantID)
	require.Equal(t, "1", stored.AuthorizedBy)
	require.NotNil(t, stored.AuthorizedAt)
}

// TestSharedGrantRefreshesOnceAcrossPostgresReplicas runs TestSharedGrantRefreshesOnce with 20 agents spread over two
// replicas, each with its own connection pool, which only the advisory lock serializes. Set OBOT_TEST_POSTGRES_DSN to
// a PostgreSQL URL whose user can create schemas to run it.
func TestSharedGrantRefreshesOnceAcrossPostgresReplicas(t *testing.T) {
	replicas := postgresGrantGateways(t, 2)
	first, second := replicas[0], replicas[1]

	endpoint := newRotatingTokenEndpoint(t, "refresh-initial")
	owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
	storeExpiredGrant(t, first, owner, mcpID, endpoint)

	results := refreshConcurrently(t, []*gatewayclient.Client{first, second}, owner, mcpID, 10)
	require.Len(t, results, 20)
	for _, token := range results {
		require.Equal(t, "access-b", token)
	}
	require.EqualValues(t, 1, endpoint.refreshes.Load())

	stored, err := second.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
	require.NoError(t, err)
	require.Equal(t, "refresh-b", stored.RefreshToken)
	require.Nil(t, stored.InvalidatedAt)
}

// TestSharedGrantRefreshFailures checks that a refresh the provider refuses with invalid_grant marks the grant invalid
// for every agent, and that one that fails for a reason that may pass leaves it valid.
func TestSharedGrantRefreshFailures(t *testing.T) {
	for _, tt := range []struct {
		name        string
		status      int
		refresh     string
		wantErr     error
		wantInvalid bool
	}{
		{
			name:        "refused refresh token",
			refresh:     "refresh-stale",
			wantErr:     ErrOAuthGrantInvalid,
			wantInvalid: true,
		},
		{
			name:    "provider outage",
			status:  http.StatusServiceUnavailable,
			wantErr: ErrOAuthGrantUnavailable,
		},
		{
			name:    "provider error",
			status:  http.StatusInternalServerError,
			wantErr: ErrOAuthGrantUnavailable,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			gateway := newGrantTestGateway(t)
			if os.Getenv(grantTestPostgresDSNEnv) != "" {
				// The refresh runs in the lock's transaction there, which must not roll back the grant's invalidation.
				gateway = postgresGrantGateways(t, 1)[0]
			}
			endpoint := newRotatingTokenEndpoint(t, "refresh-initial")
			owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
			storeExpiredGrant(t, gateway, owner, mcpID, endpoint)
			if tt.refresh != "" {
				endpoint.lock.Lock()
				endpoint.current = tt.refresh
				endpoint.lock.Unlock()
			}
			if tt.status != 0 {
				endpoint.fail(tt.status)
			}

			store := NewGlobalTokenStore(gateway).ForUserAndMCP(owner, mcpID, grantTestURL)
			source, err := store.TokenSource(endpointContext(t))
			require.NoError(t, err)
			_, err = source.Token()
			require.ErrorIs(t, err, tt.wantErr)
			// The failure never carries the provider's body or a token.
			require.NotContains(t, err.Error(), "refresh-")
			require.NotContains(t, err.Error(), "access-")

			stored, err := gateway.GetMCPOAuthToken(t.Context(), owner, mcpID, grantTestURL)
			require.NoError(t, err)
			require.Equal(t, tt.wantInvalid, stored.InvalidatedAt != nil)

			// Every later agent is refused outright while the grant is invalid, and the others may try again.
			_, err = store.TokenSource(endpointContext(t))
			if tt.wantInvalid {
				require.ErrorIs(t, err, ErrOAuthGrantInvalid)
				require.NotEmpty(t, stored.InvalidReason)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSharedGrantTargetBinding checks that a grant is never used for an upstream other than the one it was authorized
// for, and that a revoked grant is refused rather than read as having no token.
func TestSharedGrantTargetBinding(t *testing.T) {
	gateway := newGrantTestGateway(t)
	endpoint := newRotatingTokenEndpoint(t, "refresh-initial")
	owner, mcpID := principal.PreconfiguredOAuthOwner("vmcp1test"), "vmcp1test/component"
	storeExpiredGrant(t, gateway, owner, mcpID, endpoint)
	store := NewGlobalTokenStore(gateway)

	_, err := store.ForUserAndMCP(owner, mcpID, "https://elsewhere.example.com/mcp").TokenSource(t.Context())
	require.ErrorIs(t, err, ErrOAuthGrantInvalid)

	require.NoError(t, gateway.DeleteMCPOAuthGrants(t.Context(), owner, mcpID, []string{"grant-1"}))
	_, err = store.ForUserAndMCP(owner, mcpID, grantTestURL).TokenSource(t.Context())
	require.ErrorIs(t, err, ErrOAuthGrantInvalid)
	require.False(t, errors.Is(err, ErrOAuthGrantUnavailable))
}

// TestSharedGrantRefreshesOfManyGrantsKeepThePool refreshes more distinct grants at once than the connection pool has
// connections: each refresh holds one, and only a few refresh at a time, so none waits for a connection another holds.
func TestSharedGrantRefreshesOfManyGrantsKeepThePool(t *testing.T) {
	gateway := postgresGrantGateways(t, 1)[0]
	endpoints := make([]*rotatingTokenEndpoint, 8)
	owner := principal.PreconfiguredOAuthOwner("vmcp1test")
	mcpIDs := make([]string, len(endpoints))
	for i := range endpoints {
		endpoints[i] = newRotatingTokenEndpoint(t, "refresh-initial")
		mcpIDs[i] = "vmcp1test/component-" + string('a'+rune(i))
		storeExpiredGrant(t, gateway, owner, mcpIDs[i], endpoints[i])
	}
	store := NewGlobalTokenStore(gateway)
	ctx, cancel := context.WithTimeout(endpointContext(t), 30*time.Second)
	defer cancel()
	var (
		wait sync.WaitGroup
		lock sync.Mutex
		errs []error
	)
	for i := range endpoints {
		for range 3 {
			wait.Go(func() {
				source, err := store.ForUserAndMCP(owner, mcpIDs[i], grantTestURL).TokenSource(ctx)
				if err == nil {
					_, err = source.Token()
				}
				if err != nil {
					lock.Lock()
					errs = append(errs, err)
					lock.Unlock()
				}
			})
		}
	}
	wait.Wait()
	require.Empty(t, errs)
	for _, endpoint := range endpoints {
		require.EqualValues(t, 1, endpoint.refreshes.Load())
	}
}

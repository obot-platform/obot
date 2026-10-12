package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authn"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/api/server/audit"
	"github.com/obot-platform/obot/pkg/api/server/ratelimiter"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/principal"
	"github.com/obot-platform/obot/pkg/scim"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"k8s.io/apiserver/pkg/authentication/authenticator"
	"k8s.io/apiserver/pkg/authentication/request/union"
	"k8s.io/apiserver/pkg/authentication/user"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	scimTestUserHeader   = "X-Test-User"
	scimTestProviderName = "okta-auth-provider"
)

// ownerAuthenticator stands in for every user authenticator: it authenticates an Owner for a test header, and counts
// the requests it sees, so a test can show that no user authenticator runs on SCIM routes.
type ownerAuthenticator struct {
	lock  sync.Mutex
	calls int
}

// recordingAuditLogger keeps the audit entries it is given.
type recordingAuditLogger struct {
	lock    sync.Mutex
	entries []audit.LogEntry
}

// scimTestEnvironment always reports the SCIM test provider as the configured one.
type scimTestEnvironment struct{}

type scimServerTest struct {
	t       *testing.T
	server  *Server
	gateway *gclient.Client
	db      *gorm.DB
	users   *ownerAuthenticator
	audit   *recordingAuditLogger
}

func (a *ownerAuthenticator) AuthenticateRequest(req *http.Request) (*authenticator.Response, bool, error) {
	a.lock.Lock()
	a.calls++
	a.lock.Unlock()

	if req.Header.Get(scimTestUserHeader) != "owner" {
		return nil, false, nil
	}
	extra := map[string][]string{}
	principal.RecordUserStatus(extra, types2.UserStatusActive)
	return &authenticator.Response{
		User: &user.DefaultInfo{
			Name:   "owner",
			UID:    "1",
			Groups: types2.RoleOwner.Groups(),
			Extra:  extra,
		},
	}, true, nil
}

func (a *ownerAuthenticator) callCount() int {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.calls
}

func (l *recordingAuditLogger) LogEntry(entry audit.LogEntry) error {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.entries = append(l.entries, entry)
	return nil
}

func (*recordingAuditLogger) Close() error {
	return nil
}

func (l *recordingAuditLogger) all() []audit.LogEntry {
	l.lock.Lock()
	defer l.lock.Unlock()
	return append([]audit.LogEntry(nil), l.entries...)
}

func (scimTestEnvironment) ConfiguredAuthProvider(context.Context) (string, string, error) {
	return system.DefaultNamespace, scimTestProviderName, nil
}

func (scimTestEnvironment) UserLimit(context.Context) (gclient.UserLimit, error) {
	return gclient.UserLimit{Unlimited: true}, nil
}

func (scimTestEnvironment) DefaultRole(context.Context) (types2.Role, error) {
	return types2.RoleBasic, nil
}

// newSCIMServerTest returns an API server that serves the SCIM endpoint as the real one does: the SCIM authenticator
// ahead of the user authenticators, the admission check around them all, and the real authorizer and rate limiter.
func newSCIMServerTest(t *testing.T, authenticatedRateLimit int) *scimServerTest {
	t.Helper()

	services, err := sservices.New(sservices.Config{
		DSN: "sqlite://:memory:",
	})
	if err != nil {
		t.Fatalf("failed to create storage services: %v", err)
	}
	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatalf("failed to migrate gateway db: %v", err)
	}

	storage := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			Namespace: system.DefaultNamespace,
			Name:      system.DefaultRoleSettingName,
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: types2.RoleBasic,
			},
		}).
		Build()
	gateway := gclient.New(t.Context(), database, storage, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		_ = gateway.Close()
	})

	limiter, err := ratelimiter.New(ratelimiter.Options{
		UnauthenticatedRateLimit: 1000,
		AuthenticatedRateLimit:   authenticatedRateLimit,
	})
	if err != nil {
		t.Fatalf("failed to create rate limiter: %v", err)
	}

	users := &ownerAuthenticator{}
	chain := authn.NewAdmissionCheck(union.NewFailOnError(
		scim.NewAuthenticator(gateway),
		union.NewFailOnError(users, authn.Anonymous{}),
	))

	logger := &recordingAuditLogger{}
	s := &Server{
		gatewayClient: gateway,
		authenticator: authn.NewAuthenticator(chain),
		authorizer:    authz.NewAuthorizer(gateway, nil, nil, false, nil, nil, nil, false),
		auditLogger:   logger,
		rateLimiter:   limiter,
		mux:           http.NewServeMux(),
	}
	s.otelHandler = traced(s.mux)
	s.HandleSCIM(scim.NewHandler(gateway, scimTestEnvironment{}, "https://obot.example.com").Serve)
	s.HandleFunc("GET /api/me", func(req api.Context) error {
		return req.Write(map[string]string{
			"uid": req.User.GetUID(),
		})
	})

	return &scimServerTest{
		t:       t,
		server:  s,
		gateway: gateway,
		db:      services.DB.DB,
		users:   users,
		audit:   logger,
	}
}

// enable creates the SCIM connection and returns it with its token.
func (s *scimServerTest) enable() (*gatewaytypes.SCIMConnection, string) {
	s.t.Helper()

	conn, token, err := s.gateway.CreateSCIMConnection(s.t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      scimTestProviderName,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginMigrated,
		IssueToken:            true,
	})
	if err != nil {
		s.t.Fatalf("failed to create SCIM connection: %v", err)
	}
	return conn, token
}

// do sends a request through the server's mux, and returns the response and its decoded SCIM body.
func (s *scimServerTest) do(req *http.Request) (*httptest.ResponseRecorder, map[string]any) {
	s.t.Helper()

	rec := httptest.NewRecorder()
	s.server.ServeHTTP(rec, req)

	var body map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			s.t.Fatalf("%s %s returned a body that is not a SCIM response: %v: %s", req.Method, req.URL.Path, err, rec.Body.String())
		}
	}
	return rec, body
}

func scimRequest(path, token string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	return req
}

// assertSCIMError fails the test unless the response is a SCIM error with the given status.
func assertSCIMError(t *testing.T, rec *httptest.ResponseRecorder, body map[string]any, status int) {
	t.Helper()

	if rec.Code != status {
		t.Fatalf("status = %d, want %d: %s", rec.Code, status, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/scim+json") {
		t.Fatalf("Content-Type = %q, want application/scim+json", ct)
	}
	if schemas, _ := body["schemas"].([]any); len(schemas) != 1 || schemas[0] != "urn:ietf:params:scim:api:messages:2.0:Error" {
		t.Fatalf("schemas = %v, want the SCIM error schema", body["schemas"])
	}
	if got := body["status"]; got != strconv.Itoa(status) {
		t.Fatalf("status = %#v, want the string %q", got, strconv.Itoa(status))
	}
}

func TestSCIMRoutesWithoutAConnectionAreUnavailable(t *testing.T) {
	s := newSCIMServerTest(t, 1000)

	// Without a connection, the endpoint answers 503 before it looks at the token.
	for _, token := range []string{"", "obot_scim_anything"} {
		rec, body := s.do(scimRequest(scim.PathPrefix+"Users", token))
		assertSCIMError(t, rec, body, http.StatusServiceUnavailable)
		if rec.Header().Get("Retry-After") == "" {
			t.Fatal("503 without Retry-After")
		}
	}
	if calls := s.users.callCount(); calls != 0 {
		t.Fatalf("user authenticators ran %d times on SCIM routes", calls)
	}
}

func TestSCIMRoutesAuthenticateOnlyTheConnectionToken(t *testing.T) {
	s := newSCIMServerTest(t, 1000)
	_, token := s.enable()

	tests := []struct {
		name   string
		req    func() *http.Request
		status int
	}{
		{
			name: "the connection's token",
			req: func() *http.Request {
				return scimRequest(scim.PathPrefix+"Users", token)
			},
			status: http.StatusOK,
		},
		{
			name: "no token",
			req: func() *http.Request {
				return scimRequest(scim.PathPrefix+"Users", "")
			},
			status: http.StatusUnauthorized,
		},
		{
			name: "a wrong token",
			req: func() *http.Request {
				return scimRequest(scim.PathPrefix+"Users", "obot_scim_wrong")
			},
			status: http.StatusUnauthorized,
		},
		{
			name: "a path that names a connection, which the base URL does not",
			req: func() *http.Request {
				return scimRequest(scim.PathPrefix+"00000000-0000-0000-0000-000000000000/Users", token)
			},
			status: http.StatusNotFound,
		},
		{
			name: "an Owner's credential",
			req: func() *http.Request {
				req := scimRequest(scim.PathPrefix+"Users", "")
				req.Header.Set(scimTestUserHeader, "owner")
				return req
			},
			status: http.StatusUnauthorized,
		},
		{
			name: "an Owner's credential alongside the token",
			req: func() *http.Request {
				req := scimRequest(scim.PathPrefix+"Users", token)
				req.Header.Set(scimTestUserHeader, "owner")
				return req
			},
			status: http.StatusOK,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := s.do(tt.req())
			if tt.status == http.StatusOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
				}
				return
			}
			assertSCIMError(t, rec, body, tt.status)
			if tt.status == http.StatusUnauthorized && rec.Header().Get("WWW-Authenticate") == "" {
				t.Fatal("401 without WWW-Authenticate")
			}
		})
	}

	if calls := s.users.callCount(); calls != 0 {
		t.Fatalf("user authenticators ran %d times on SCIM routes", calls)
	}
}

func TestSCIMConnectionPrincipalReachesOnlySCIMRoutes(t *testing.T) {
	s := newSCIMServerTest(t, 1000)
	_, token := s.enable()

	// The connection's token authenticates nothing outside SCIM routes: the SCIM authenticator declines, the user
	// authenticators run as usual, and the request is anonymous.
	before := s.users.callCount()
	rec := httptest.NewRecorder()
	s.server.ServeHTTP(rec, scimRequest("/api/me", token))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/me with the SCIM token = %d: %s", rec.Code, rec.Body.String())
	}
	if s.users.callCount() == before {
		t.Fatal("the user authenticators did not run off SCIM routes")
	}
}

func TestSCIMPathsThatAreNotClean(t *testing.T) {
	s := newSCIMServerTest(t, 1000)
	_, token := s.enable()

	tests := []struct {
		name   string
		path   string
		token  string
		status int
	}{
		{
			name:   "the base URL, which names no resource",
			path:   strings.TrimSuffix(scim.PathPrefix, "/"),
			token:  token,
			status: http.StatusNotFound,
		},
		{
			name:   "the base URL without a token",
			path:   strings.TrimSuffix(scim.PathPrefix, "/"),
			status: http.StatusUnauthorized,
		},
		{
			name:   "the SCIM root",
			path:   scim.PathPrefix,
			token:  token,
			status: http.StatusNotFound,
		},
		{
			name:   "the SCIM root without a token",
			path:   scim.PathPrefix,
			status: http.StatusUnauthorized,
		},
		{
			name:   "a doubled slash, as a base URL entered with a trailing slash makes",
			path:   scim.PathPrefix + "/Users",
			token:  token,
			status: http.StatusOK,
		},
		{
			name:   "a trailing slash",
			path:   scim.PathPrefix + "Users/",
			token:  token,
			status: http.StatusOK,
		},
		{
			name:   "a doubled slash before the base URL, as a server URL with a trailing slash makes",
			path:   "/" + scim.PathPrefix + "Users",
			token:  token,
			status: http.StatusOK,
		},
		{
			name:   "a doubled slash before the base URL without a token",
			path:   "/" + scim.PathPrefix + "Users",
			status: http.StatusUnauthorized,
		},
		{
			name:   "a path that climbs into the SCIM endpoint",
			path:   "/api/.." + scim.PathPrefix + "Users",
			token:  token,
			status: http.StatusOK,
		},
		{
			name:   "a path that climbs out of the SCIM endpoint",
			path:   scim.PathPrefix + "../api/me",
			token:  token,
			status: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, body := s.do(scimRequest(tt.path, tt.token))
			if tt.status == http.StatusOK {
				if rec.Code != http.StatusOK {
					t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
				}
				return
			}
			assertSCIMError(t, rec, body, tt.status)
		})
	}
}

func TestSCIMRoutesRefuseEveryRequestWhileTwoConnectionsExist(t *testing.T) {
	s := newSCIMServerTest(t, 1000)
	conn, token := s.enable()

	// Creating a connection refuses a second one, so only a database changed by other means can hold two.
	second := *conn
	second.ID = "second"
	second.AuthProviderNamespace = "other"
	if err := s.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{token, ""} {
		rec, body := s.do(scimRequest(scim.PathPrefix+"Users", token))
		assertSCIMError(t, rec, body, http.StatusInternalServerError)
	}
}

func TestSCIMRoutesAreAuditedWithoutQueryOrBody(t *testing.T) {
	s := newSCIMServerTest(t, 1000)
	conn, token := s.enable()

	req := scimRequest(scim.PathPrefix+"Users?filter=userName%20eq%20%22secret%40example.com%22", token)
	if rec, _ := s.do(req); rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	s.do(scimRequest(scim.PathPrefix+"Users", ""))

	entries := s.audit.all()
	if len(entries) != 2 {
		t.Fatalf("got %d audit entries, want 2: %+v", len(entries), entries)
	}
	if entries[0].UserID != conn.ID || entries[0].Path != scim.PathPrefix+"Users" || entries[0].ResponseCode != http.StatusOK {
		t.Errorf("audit entry = %+v", entries[0])
	}
	if entries[1].ResponseCode != http.StatusUnauthorized {
		t.Errorf("audit entry = %+v", entries[1])
	}
	for _, entry := range entries {
		data, _ := json.Marshal(entry)
		if strings.Contains(string(data), "secret") || strings.Contains(string(data), token) {
			t.Errorf("audit entry %s contains the query string or the token", data)
		}
	}
}

func TestSCIMRateLimitAnswersWithIntegerRetryAfter(t *testing.T) {
	s := newSCIMServerTest(t, 1)
	_, token := s.enable()

	var (
		limited *httptest.ResponseRecorder
		body    map[string]any
	)
	for range 5 {
		rec, b := s.do(scimRequest(scim.PathPrefix+"Users", token))
		if rec.Code == http.StatusTooManyRequests {
			limited, body = rec, b
			break
		}
	}
	if limited == nil {
		t.Fatal("the connection was never rate limited")
	}
	assertSCIMError(t, limited, body, http.StatusTooManyRequests)
	retryAfter := limited.Header().Get("Retry-After")
	var seconds int
	if err := json.Unmarshal([]byte(retryAfter), &seconds); err != nil || seconds < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer number of seconds", retryAfter)
	}
}

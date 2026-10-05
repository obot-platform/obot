package scim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
	"uuid"

	types2 "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	"github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	sservices "github.com/obot-platform/obot/pkg/storage/services"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testServerURL        = "https://obot.example.com"
	testOktaProviderName = "okta-auth-provider"
	postgresTestDSNEnv   = "OBOT_TEST_POSTGRES_DSN"
)

// testEnvironment is an Environment whose answers the test controls.
type testEnvironment struct {
	namespace   string
	name        string
	userLimit   gclient.UserLimit
	defaultRole types2.Role
}

// scimTest is a SCIM handler over a real gateway database, behind the SCIM authenticator.
type scimTest struct {
	t             *testing.T
	gateway       *gclient.Client
	db            *gatewaydb.DB
	env           *testEnvironment
	authenticator *Authenticator
	handler       *Handler
	conn          *types.SCIMConnection
	token         string
}

type testResponse struct {
	status int
	header http.Header
	body   map[string]any
}

func (e *testEnvironment) ConfiguredAuthProvider(context.Context) (string, string, error) {
	return e.namespace, e.name, nil
}

func (e *testEnvironment) UserLimit(context.Context) (gclient.UserLimit, error) {
	return e.userLimit, nil
}

func (e *testEnvironment) DefaultRole(context.Context) (types2.Role, error) {
	return e.defaultRole, nil
}

// newSCIMTest returns a SCIM test over SQLite.
func newSCIMTest(t *testing.T) *scimTest {
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
	return newSCIMTestWithDB(t, database)
}

// newPostgresSCIMTest returns a SCIM test over an isolated PostgreSQL schema, or skips the test when no
// PostgreSQL server is configured.
func newPostgresSCIMTest(t *testing.T) *scimTest {
	t.Helper()

	dsn := os.Getenv(postgresTestDSNEnv)
	if dsn == "" {
		t.Skipf("set %s to a PostgreSQL URL whose user can create schemas", postgresTestDSNEnv)
	}

	admin, err := sservices.New(sservices.Config{
		DSN: dsn,
	})
	if err != nil {
		t.Fatalf("failed to open PostgreSQL admin connection: %v", err)
	}
	schema := "obot_scim_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	if err := admin.DB.DB.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		_ = admin.DB.SQLDB.Close()
		t.Fatalf("failed to create PostgreSQL schema: %v", err)
	}
	t.Cleanup(func() {
		if err := admin.DB.DB.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Errorf("failed to drop PostgreSQL schema: %v", err)
		}
		_ = admin.DB.SQLDB.Close()
	})

	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", postgresTestDSNEnv, err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()

	services, err := sservices.New(sservices.Config{
		DSN: u.String(),
	})
	if err != nil {
		t.Fatalf("failed to open PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() {
		_ = services.DB.SQLDB.Close()
	})

	database, err := gatewaydb.New(services.DB.DB, services.DB.SQLDB, true)
	if err != nil {
		t.Fatalf("failed to create gateway db: %v", err)
	}
	return newSCIMTestWithDB(t, database)
}

func newSCIMTestWithDB(t *testing.T, database *gatewaydb.DB) *scimTest {
	t.Helper()

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

	env := &testEnvironment{
		namespace: system.DefaultNamespace,
		name:      testOktaProviderName,
		userLimit: gclient.UserLimit{
			Unlimited: true,
		},
		defaultRole: types2.RoleBasic,
	}

	return &scimTest{
		t:             t,
		gateway:       gateway,
		db:            database,
		env:           env,
		authenticator: NewAuthenticator(gateway),
		handler:       NewHandler(gateway, env, testServerURL),
	}
}

// enable creates the Okta SCIM connection.
func (s *scimTest) enable() {
	s.t.Helper()

	conn, token, err := s.gateway.CreateSCIMConnection(s.t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      testOktaProviderName,
		GroupIDPrefix:         "okta/",
		Issuer:                "https://example.okta.com",
		Origin:                types.SCIMConnectionOriginMigrated,
		IssueToken:            true,
	})
	if err != nil {
		s.t.Fatalf("failed to create SCIM connection: %v", err)
	}
	s.conn = conn
	s.token = token
}

// path returns the URL path of a SCIM resource of the test's connection.
func (s *scimTest) path(resource string) string {
	return PathPrefix + strings.TrimPrefix(resource, "/")
}

// do sends an authenticated request to a resource of the test's connection.
func (s *scimTest) do(method, resource string, body any) testResponse {
	s.t.Helper()
	return s.request(method, s.path(resource), s.token, body)
}

// request sends a request with the given bearer token, if any.
func (s *scimTest) request(method, path, token string, body any) testResponse {
	s.t.Helper()

	var reader *strings.Reader
	switch b := body.(type) {
	case nil:
		reader = strings.NewReader("")
	case string:
		reader = strings.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			s.t.Fatalf("failed to encode request body: %v", err)
		}
		reader = strings.NewReader(string(data))
	}

	req := httptest.NewRequestWithContext(s.t.Context(), method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/scim+json; charset=utf-8")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	s.serve(rec, req)

	resp := testResponse{
		status: rec.Code,
		header: rec.Header(),
	}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &resp.body); err != nil {
			s.t.Fatalf("%s %s returned a body that is not a JSON object: %v: %s", method, path, err, rec.Body.String())
		}
		if ct := rec.Header().Get("Content-Type"); ct != contentType {
			s.t.Fatalf("%s %s returned Content-Type %q", method, path, ct)
		}
	}
	return resp
}

// serve handles a request as the API server does: the SCIM authenticator authenticates it, anonymous principals are
// refused, and the handler serves the connection's principal. The API server's own tests cover the server's part.
func (s *scimTest) serve(w http.ResponseWriter, req *http.Request) {
	resp, ok, err := s.authenticator.AuthenticateRequest(req)
	if _, unavailable := errors.AsType[*UnavailableError](err); unavailable {
		WriteUnavailable(w, "SCIM is not enabled")
		return
	} else if err != nil {
		s.t.Errorf("failed to authenticate SCIM request: %v", err)
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	} else if !ok {
		s.t.Errorf("the SCIM authenticator declined %s", req.URL.Path)
		WriteError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if !IsConnectionPrincipal(resp.User) {
		WriteUnauthorized(w)
		return
	}
	s.handler.ServeHTTP(w, req, resp.User)
}

// expect fails the test unless the response has the given status.
func (r testResponse) expect(t *testing.T, status int) testResponse {
	t.Helper()
	if r.status != status {
		t.Fatalf("got status %d, want %d: %v", r.status, status, r.body)
	}
	return r
}

func (r testResponse) id() string {
	id, _ := r.body["id"].(string)
	return id
}

func (r testResponse) resources() []map[string]any {
	items, _ := r.body["Resources"].([]any)
	resources := make([]map[string]any, 0, len(items))
	for _, item := range items {
		m, _ := item.(map[string]any)
		resources = append(resources, m)
	}
	return resources
}

func (r testResponse) memberIDs() []string {
	members, _ := r.body["members"].([]any)
	ids := make([]string, 0, len(members))
	for _, member := range members {
		m, _ := member.(map[string]any)
		id, _ := m["value"].(string)
		ids = append(ids, id)
	}
	return ids
}

// seedUser creates an Okta user through sign-in, as existing installations have them.
func (s *scimTest) seedUser(nativeID, email string, role types2.Role) *types.User {
	s.t.Helper()
	return s.seedProviderUser(testOktaProviderName, nativeID, email, role)
}

func (s *scimTest) seedProviderUser(providerName, nativeID, email string, role types2.Role) *types.User {
	s.t.Helper()

	user, err := s.gateway.EnsureIdentityWithRole(s.t.Context(), &types.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      providerName,
		ProviderUsername:      nativeID,
		ProviderUserID:        nativeID,
		Email:                 email,
	}, "", role, gclient.UserLimit{
		Unlimited: true,
	})
	if err != nil {
		s.t.Fatalf("failed to seed user %s: %v", nativeID, err)
	}
	return user
}

// seedGroup creates a cached group of the Okta provider with the given members.
func (s *scimTest) seedGroup(id, name string, memberIDs ...uint) {
	s.t.Helper()

	if err := s.gorm().Create(&types.Group{
		ID:                    id,
		AuthProviderName:      testOktaProviderName,
		AuthProviderNamespace: system.DefaultNamespace,
		Name:                  name,
	}).Error; err != nil {
		s.t.Fatalf("failed to seed group %s: %v", id, err)
	}
	for _, userID := range memberIDs {
		if err := s.gorm().Create(&types.GroupMemberships{
			UserID:  userID,
			GroupID: id,
		}).Error; err != nil {
			s.t.Fatalf("failed to seed membership of %d in %s: %v", userID, id, err)
		}
	}
}

func (s *scimTest) gorm() *gorm.DB {
	return s.db.WithContext(s.t.Context())
}

// memberships returns the IDs of the group's members.
func (s *scimTest) memberships(groupID string) []uint {
	s.t.Helper()

	var userIDs []uint
	if err := s.gorm().Model(new(types.GroupMemberships)).Where("group_id = ?", groupID).Order("user_id").Pluck("user_id", &userIDs).Error; err != nil {
		s.t.Fatalf("failed to list memberships: %v", err)
	}
	return userIDs
}

// user returns the user row, including deleted users.
func (s *scimTest) user(id uint) *types.User {
	s.t.Helper()

	u, err := s.gateway.UserByIDIncludeDeleted(s.t.Context(), fmt.Sprint(id))
	if err != nil {
		s.t.Fatalf("failed to get user %d: %v", id, err)
	}
	return u
}

// count returns the number of rows of a model that match a condition.
func (s *scimTest) count(model any, query string, args ...any) int64 {
	s.t.Helper()

	var n int64
	db := s.gorm().Model(model)
	if query != "" {
		db = db.Where(query, args...)
	}
	if err := db.Count(&n).Error; err != nil {
		s.t.Fatalf("failed to count: %v", err)
	}
	return n
}

// events returns the lifecycle outbox events recorded for a user.
func (s *scimTest) events(userID uint) []types.UserLifecycleEvent {
	s.t.Helper()

	var events []types.UserLifecycleEvent
	if err := s.gorm().Where("user_id = ?", userID).Order("id").Find(&events).Error; err != nil {
		s.t.Fatalf("failed to list events: %v", err)
	}
	return events
}

// scimUser returns a user create body.
func scimUser(userName, externalID string) map[string]any {
	return map[string]any{
		"schemas":    []string{userSchema},
		"userName":   userName,
		"externalId": externalID,
		"active":     true,
		"name": map[string]any{
			"givenName":  "Given",
			"familyName": "Family",
		},
		"displayName": "Given Family",
		"emails": []any{
			map[string]any{
				"primary": true,
				"type":    "work",
				"value":   userName,
			},
		},
		"locale":   "en-US",
		"password": "placeholder",
		"groups":   []any{},
	}
}

// scimGroup returns a group create body.
func scimGroup(displayName string, memberIDs ...string) map[string]any {
	members := make([]any, 0, len(memberIDs))
	for _, id := range memberIDs {
		members = append(members, map[string]any{
			"value": id,
		})
	}
	return map[string]any{
		"schemas":     []string{groupSchema},
		"displayName": displayName,
		"members":     members,
	}
}

func patchOp(ops ...map[string]any) map[string]any {
	operations := make([]any, 0, len(ops))
	for _, op := range ops {
		operations = append(operations, op)
	}
	return map[string]any{
		"schemas":    []string{patchOpSchema},
		"Operations": operations,
	}
}

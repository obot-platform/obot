package client

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesstoken"
	"github.com/obot-platform/obot/pkg/auth"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	"github.com/obot-platform/obot/pkg/system"
)

var (
	// directoryEndpoints are the auth provider endpoints that answer from the identity provider's directory.
	directoryEndpoints = []string{
		"/obot-list-user-auth-groups",
		"/obot-list-auth-groups",
		"/obot-get-auth-groups",
		"/obot-get-group-migration-mapping",
	}
)

// authProviderStub stands in for an auth provider daemon. It counts the requests to each endpoint, and answers the
// directory endpoints with one group, so a test can show whether Obot asked the directory.
type authProviderStub struct {
	lock     sync.Mutex
	requests map[string]int
	// during maps an endpoint to a function that runs while a request to it is being answered.
	during map[string]func()
}

func newAuthProviderStub(t *testing.T) (*authProviderStub, *httptest.Server) {
	t.Helper()

	stub := &authProviderStub{
		requests: make(map[string]int, len(directoryEndpoints)+1),
		during:   make(map[string]func()),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stub.lock.Lock()
		stub.requests[r.URL.Path]++
		during := stub.during[r.URL.Path]
		stub.lock.Unlock()
		if during != nil {
			during()
		}

		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/obot-list-user-auth-groups":
			_ = json.NewEncoder(w).Encode([]auth.GroupInfo{
				{
					ID:   "okta/00g-directory",
					Name: "directory",
				},
			})
		case "/obot-list-auth-groups", "/obot-get-auth-groups":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items": []auth.GroupInfo{
					{
						ID:   "okta/00g-directory",
						Name: "directory",
					},
				},
			})
		case "/obot-get-user-info":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":   "00u-lookup",
				"name": "Login Name",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)

	return stub, srv
}

// directoryRequests returns the number of requests that reached a directory endpoint.
func (s *authProviderStub) directoryRequests() int {
	s.lock.Lock()
	defer s.lock.Unlock()

	var n int
	for _, endpoint := range directoryEndpoints {
		n += s.requests[endpoint]
	}
	return n
}

func (s *authProviderStub) count(path string) int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return s.requests[path]
}

// runDuring makes f run while the stub answers each later request to path.
func (s *authProviderStub) runDuring(path string, f func()) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.during[path] = f
}

// testSCIMConnectionOptions describe the SCIM connection of the lifecycle test provider.
func testSCIMConnectionOptions(issueToken bool) CreateSCIMConnectionOptions {
	return CreateSCIMConnectionOptions{
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		AuthProviderName:      lifecycleTestProvider.Name,
		GroupIDPrefix:         "okta/",
		Issuer:                "https://example.okta.com",
		Origin:                types.SCIMConnectionOriginMigrated,
		IssueToken:            issueToken,
	}
}

// createTestSCIMConnection creates the SCIM connection of the lifecycle test provider.
func createTestSCIMConnection(t *testing.T, c *Client, issueToken bool) (*types.SCIMConnection, string) {
	t.Helper()

	conn, token, err := c.CreateSCIMConnection(t.Context(), testSCIMConnectionOptions(issueToken))
	if err != nil {
		t.Fatalf("failed to create SCIM connection: %v", err)
	}
	return conn, token
}

// signInIdentity returns the identity that a sign-in through the lifecycle test provider presents.
func signInIdentity(nativeID, email string) *types.Identity {
	return &types.Identity{
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		AuthProviderName:      lifecycleTestProvider.Name,
		ProviderUsername:      nativeID,
		ProviderUserID:        nativeID,
		Email:                 email,
	}
}

func provisionTestSCIMUser(t *testing.T, c *Client, conn *types.SCIMConnection, nativeID, userName string) *SCIMUser {
	t.Helper()

	user, err := c.CreateSCIMUser(t.Context(), conn, SCIMUserInput{
		UserName:   userName,
		ExternalID: nativeID,
		Profile: types.SCIMUserProfile{
			DisplayName: "SCIM Name",
			Emails: []types.SCIMMultiValue{
				{
					Value:   userName,
					Primary: true,
				},
			},
		},
	}, SCIMUserCreateOptions{
		UserLimit: UserLimit{
			Unlimited: true,
		},
		DefaultRole: apitypes.RoleBasic,
	})
	if err != nil {
		t.Fatalf("failed to provision %s: %v", nativeID, err)
	}
	return user
}

func setSCIMConnectionState(t *testing.T, c *Client, conn *types.SCIMConnection, state types.SCIMConnectionState) {
	t.Helper()

	if err := c.db.WithContext(t.Context()).Model(conn).UpdateColumn("state", state).Error; err != nil {
		t.Fatalf("failed to set the SCIM connection state: %v", err)
	}
}

func TestCreateSCIMConnection(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()

	if _, _, err := c.CreateSCIMConnection(ctx, CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "github-auth-provider",
		GroupIDPrefix:         "github/",
		Origin:                types.SCIMConnectionOriginMigrated,
	}); err == nil {
		t.Fatal("a SCIM connection was created for an auth provider without a SCIM adapter")
	}

	conn, token := createTestSCIMConnection(t, c, false)
	if token != "" || conn.HasToken() || conn.TokenIssuedAt != nil {
		t.Fatalf("a connection created without a token has one: %+v", conn)
	}
	if conn.AdapterType != "okta" || conn.State != types.SCIMConnectionStateConnected || conn.Origin != types.SCIMConnectionOriginMigrated {
		t.Fatalf("connection = %+v", conn)
	}

	found, err := c.SCIMConnectionForAuthProvider(ctx, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name)
	if err != nil || found == nil || found.ID != conn.ID {
		t.Fatalf("SCIMConnectionForAuthProvider() = %v, %v", found, err)
	}
	if other, err := c.SCIMConnectionForAuthProvider(ctx, system.DefaultNamespace, system.LocalAuthProvider); err != nil || other != nil {
		t.Fatalf("SCIMConnectionForAuthProvider() of another provider = %v, %v", other, err)
	}

	// The installation has at most one connection.
	if _, _, err := c.CreateSCIMConnection(ctx, CreateSCIMConnectionOptions{
		AuthProviderNamespace: lifecycleTestProvider.Namespace,
		AuthProviderName:      lifecycleTestProvider.Name,
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	}); err == nil {
		t.Fatal("a second SCIM connection was created")
	} else if exists, ok := errors.AsType[*SCIMConnectionExistsError](err); !ok || exists.ConnectionID != conn.ID {
		t.Fatalf("creating a second connection failed with %v", err)
	}
}

func TestAuthenticatingSCIMRequiresExactlyOneConnection(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()

	if _, err := c.AuthenticateSCIMConnection(ctx, "obot_scim_anything"); !errors.Is(err, ErrSCIMConnectionNotFound) {
		t.Fatalf("AuthenticateSCIMConnection() without a connection = %v, want ErrSCIMConnectionNotFound", err)
	}

	conn, token := createTestSCIMConnection(t, c, true)
	if got, err := c.AuthenticateSCIMConnection(ctx, token); err != nil || got.ID != conn.ID {
		t.Fatalf("AuthenticateSCIMConnection() = %+v, %v, want connection %s", got, err, conn.ID)
	}

	// Creating a connection refuses a second one, so only a database changed by other means can hold two. No request
	// is served then, not even with the first connection's token.
	second := *conn
	second.ID = "second"
	second.AuthProviderNamespace = "other"
	if err := c.db.WithContext(ctx).Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := c.AuthenticateSCIMConnection(ctx, token); !errors.Is(err, ErrMultipleSCIMConnections) {
		t.Fatalf("AuthenticateSCIMConnection() with two connections = %v, want ErrMultipleSCIMConnections", err)
	}
}

func TestSCIMConnectionTokens(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	conn, _ := createTestSCIMConnection(t, c, false)

	authenticates := func(token string) bool {
		t.Helper()
		_, err := c.AuthenticateSCIMConnection(ctx, token)
		if err == nil {
			return true
		}
		if _, ok := errors.AsType[*SCIMAuthenticationError](err); !ok {
			t.Fatalf("AuthenticateSCIMConnection() failed with %v, want a SCIMAuthenticationError", err)
		}
		return false
	}

	// A connection without a token refuses every token, including an empty one and the hash of an empty one.
	for _, token := range []string{"", "obot_scim_anything", hash.String("")} {
		if authenticates(token) {
			t.Fatalf("a connection without a token accepted %q", token)
		}
	}

	// The first token of a tokenless connection has no predecessor.
	_, first, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatalf("failed to issue the first token: %v", err)
	}
	if !strings.HasPrefix(first, "obot_scim_") || !authenticates(first) {
		t.Fatalf("the first token %q does not authenticate", first)
	}
	stored, err := c.SCIMConnection(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.TokenVerifier != hash.String(first) || stored.PreviousTokenVerifier != "" || stored.TokenIssuedAt == nil {
		t.Fatalf("stored connection after the first token = %+v", stored)
	}

	// Rotation keeps the previous token until it expires.
	_, second, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatalf("failed to rotate: %v", err)
	}
	if !authenticates(first) || !authenticates(second) {
		t.Fatal("rotation did not keep both tokens")
	}
	if err := c.db.WithContext(ctx).Model(new(types.SCIMConnection)).Where("id = ?", conn.ID).
		UpdateColumn("previous_token_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
	if authenticates(first) || !authenticates(second) {
		t.Fatal("an expired previous token still authenticates")
	}

	// Rotating twice in a row cuts off the token that the first rotation replaced, though it was still accepted.
	_, third, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatalf("failed to rotate: %v", err)
	}
	if !authenticates(second) || !authenticates(third) {
		t.Fatal("rotation did not keep both tokens")
	}
	_, fourth, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatalf("failed to rotate: %v", err)
	}
	if authenticates(second) || !authenticates(third) || !authenticates(fourth) {
		t.Fatal("a second rotation did not retire the token that the first one replaced")
	}

	// Revoking the previous token leaves the current one.
	if err := c.RevokePreviousSCIMConnectionToken(ctx, conn.ID); err != nil {
		t.Fatalf("failed to revoke the previous token: %v", err)
	}
	if authenticates(third) || !authenticates(fourth) {
		t.Fatal("revoking the previous token did not keep only the current one")
	}

	// Revoking the current token replaces it in one step, and nothing earlier is accepted.
	if _, _, err := c.RotateSCIMConnectionToken(ctx, conn.ID); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := c.RevokeCurrentSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatalf("failed to revoke the current token: %v", err)
	}
	if authenticates(fourth) || !authenticates(fresh) {
		t.Fatal("revoking the current token did not leave only the new one")
	}
	stored, err = c.SCIMConnection(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.PreviousTokenVerifier != "" || stored.PreviousTokenExpiresAt != nil {
		t.Fatalf("a previous token survived revoking the current one: %+v", stored)
	}

	for _, fn := range []func() error{
		func() error {
			_, _, err := c.RotateSCIMConnectionToken(ctx, "unknown")
			return err
		},
		func() error {
			_, _, err := c.RevokeCurrentSCIMConnectionToken(ctx, "unknown")
			return err
		},
		func() error {
			return c.RevokePreviousSCIMConnectionToken(ctx, "unknown")
		},
	} {
		if err := fn(); !errors.Is(err, ErrSCIMConnectionNotFound) {
			t.Fatalf("a token change of an unknown connection failed with %v", err)
		}
	}
}

func TestSCIMProvidersNeverReachTheDirectory(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub, srv := newAuthProviderStub(t)
	ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), srv.URL), "okta/"), "access-token")
	unlimited := UserLimit{
		Unlimited: true,
	}

	// Without a connection, sign-in synchronizes the directory.
	existing, err := c.EnsureIdentity(ctx, signInIdentity("00u-existing", "existing@example.com"), "", unlimited)
	if err != nil {
		t.Fatalf("failed to sign in: %v", err)
	}
	if stub.count("/obot-list-user-auth-groups") != 1 {
		t.Fatalf("sign-in without a connection made %d group requests, want 1", stub.count("/obot-list-user-auth-groups"))
	}
	before, userInfoBefore := stub.directoryRequests(), stub.count("/obot-get-user-info")

	createTestSCIMConnection(t, c, true)

	// Sign-in, even once the group check is due, just-in-time creation, browsing, and resolution all answer from the
	// groups table.
	if err := c.db.WithContext(ctx).Model(new(types.Identity)).Where("user_id = ?", existing.ID).
		UpdateColumn(groupsLastCheckedColumn, time.Time{}).Error; err != nil {
		t.Fatal(err)
	}
	signedIn := signInIdentity("00u-existing", "existing@example.com")
	if _, err := c.EnsureIdentity(ctx, signedIn, "", unlimited); err != nil {
		t.Fatalf("failed to sign in with a connection: %v", err)
	}
	if ids := signedIn.GetAuthProviderGroupIDs(); len(ids) != 1 || ids[0] != "okta/00g-directory" {
		t.Fatalf("sign-in with a connection reported groups %v, want the cached group", ids)
	}
	if _, err := c.EnsureIdentity(ctx, signInIdentity("00u-new", "new@example.com"), "", unlimited); err != nil {
		t.Fatalf("failed to create a user just in time with a connection: %v", err)
	}
	if err := c.ensureGroups(ctx, signedIn); err != nil {
		t.Fatalf("failed to ensure groups: %v", err)
	}

	listed, err := c.ListAuthGroups(ctx, srv.URL, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name, ListAuthGroupsOptions{})
	if err != nil {
		t.Fatalf("failed to list groups: %v", err)
	}
	if listed.Source != types.GroupSourceCache || listed.Degraded || len(listed.Groups) != 1 {
		t.Fatalf("group listing with a connection = %+v", listed)
	}

	resolved, err := c.ResolveAuthGroups(ctx, srv.URL, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name, []string{"okta/00g-directory", "okta/00g-missing"})
	if err != nil {
		t.Fatalf("failed to resolve groups: %v", err)
	}
	if len(resolved) != 2 || resolved[1].Name != "okta/00g-missing" {
		t.Fatalf("resolved groups = %+v", resolved)
	}

	if after := stub.directoryRequests(); after != before {
		t.Fatalf("%d directory requests were made while a SCIM connection exists", after-before)
	}
	if n := stub.count("/obot-get-user-info") - userInfoBefore; n != 0 {
		t.Fatalf("%d user info requests were made while a SCIM connection exists", n)
	}
}

func TestSCIMModeLookupFailureNeverFallsBackToTheDirectory(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub, srv := newAuthProviderStub(t)
	ctx := auth.ContextWithProviderURL(t.Context(), srv.URL)

	if err := c.db.WithContext(ctx).Migrator().DropTable(&types.SCIMConnection{}); err != nil {
		t.Fatal(err)
	}

	if _, err := c.EnsureIdentity(ctx, signInIdentity("00u-alice", "alice@example.com"), "", UserLimit{Unlimited: true}); err == nil {
		t.Fatal("sign-in succeeded although the SCIM mode could not be read")
	}
	if _, err := c.ListAuthGroups(ctx, srv.URL, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name, ListAuthGroupsOptions{}); err == nil {
		t.Fatal("group listing succeeded although the SCIM mode could not be read")
	}
	if _, err := c.ResolveAuthGroups(ctx, srv.URL, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name, []string{"okta/00g-missing"}); err == nil {
		t.Fatal("group resolution succeeded although the SCIM mode could not be read")
	}
	if n := stub.directoryRequests(); n != 0 {
		t.Fatalf("%d directory requests were made although the SCIM mode could not be read", n)
	}
}

func TestSignInRequiresABindingOnceSCIMIsEnforced(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	unlimited := UserLimit{
		Unlimited: true,
	}

	unbound, err := c.EnsureIdentity(ctx, signInIdentity("00u-unbound", "unbound@example.com"), "", unlimited)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := createTestSCIMConnection(t, c, true)
	bound := provisionTestSCIMUser(t, c, conn, "00u-bound", "bound@example.com")
	departed := provisionTestSCIMUser(t, c, conn, "00u-departed", "departed@example.com")
	setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)

	// A provisioned user signs in.
	user, err := c.EnsureIdentity(ctx, signInIdentity("00u-bound", "bound@example.com"), "", unlimited)
	if err != nil || user.ID != bound.UserID {
		t.Fatalf("a provisioned user's sign-in = %v, %v", user, err)
	}

	// An unprovisioned user, a new person, and a deleted user are refused, and nothing is created for them.
	if _, err := c.UpdateSCIMUser(ctx, conn, departed.ID, func(current SCIMUser) (SCIMUserInput, error) {
		inactive := false
		return SCIMUserInput{
			UserName:   current.UserName,
			ExternalID: current.ExternalID,
			Active:     &inactive,
			Profile:    current.Profile,
		}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteUser(ctx, fmtUserID(departed.UserID)); err != nil {
		t.Fatalf("failed to delete the departed user: %v", err)
	}

	var users, identities int64
	c.db.WithContext(ctx).Model(new(types.User)).Count(&users)
	c.db.WithContext(ctx).Model(new(types.Identity)).Count(&identities)
	for _, identity := range []*types.Identity{
		signInIdentity("00u-unbound", "unbound@example.com"),
		signInIdentity("00u-stranger", "stranger@example.com"),
		signInIdentity("00u-departed", "departed@example.com"),
	} {
		_, err := c.EnsureIdentity(ctx, identity, "", unlimited)
		if denied, ok := errors.AsType[*UserAccessDeniedError](err); !ok || denied.Status != apitypes.UserStatusDisabled {
			t.Fatalf("sign-in of %s failed with %v, want a UserAccessDeniedError", identity.ProviderUserID, err)
		}
	}
	var usersAfter, identitiesAfter int64
	c.db.WithContext(ctx).Model(new(types.User)).Count(&usersAfter)
	c.db.WithContext(ctx).Model(new(types.Identity)).Count(&identitiesAfter)
	if usersAfter != users || identitiesAfter != identities {
		t.Fatalf("refused sign-ins created %d users and %d identities", usersAfter-users, identitiesAfter-identities)
	}
	if got := storedLifecycleUser(t, c, unbound.ID); got.DisabledAt != nil {
		t.Fatalf("a refused sign-in changed the unbound user: %+v", got)
	}
}

func TestSignInLeavesTheSCIMProfileAlone(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	unlimited := UserLimit{
		Unlimited: true,
	}

	unbound, err := c.EnsureIdentity(ctx, signInIdentity("00u-unbound", "unbound@example.com"), "", unlimited)
	if err != nil {
		t.Fatal(err)
	}
	conn, _ := createTestSCIMConnection(t, c, true)
	provisioned := provisionTestSCIMUser(t, c, conn, "00u-provisioned", "scim@example.com")

	// Explicit roles of a provisioned user follow the email SCIM stored, as every other check of explicit roles does,
	// not the one the identity provider asserts at sign-in.
	c.emailsWithExplicitRoles = map[string]apitypes.Role{
		"login@example.com": apitypes.RoleOwner,
	}
	user, err := c.EnsureIdentity(ctx, signInIdentity("00u-provisioned", "login@example.com"), "", unlimited)
	if err != nil {
		t.Fatalf("failed to sign in: %v", err)
	}
	stored := storedLifecycleUser(t, c, provisioned.UserID)
	if user.ID != provisioned.UserID || stored.Email != "scim@example.com" || stored.Username != "00u-provisioned" || stored.DisplayName != "SCIM Name" {
		t.Fatalf("sign-in changed the SCIM profile: %+v", stored)
	}
	if stored.Role.HasRole(apitypes.RoleOwner) {
		t.Fatalf("sign-in applied the explicit role of the asserted email rather than the SCIM email: role %d", stored.Role)
	}

	c.emailsWithExplicitRoles = map[string]apitypes.Role{
		"scim@example.com": apitypes.RoleOwner,
	}
	if _, err := c.EnsureIdentity(ctx, signInIdentity("00u-provisioned", "login@example.com"), "", unlimited); err != nil {
		t.Fatalf("failed to sign in: %v", err)
	}
	stored = storedLifecycleUser(t, c, provisioned.UserID)
	if !stored.Role.HasRole(apitypes.RoleOwner) {
		t.Fatalf("sign-in did not apply the explicit role of the SCIM email: role %d", stored.Role)
	}
	// Demoting the user is refused for the same email.
	if _, err := c.UpdateUser(ctx, true, &types.User{Role: apitypes.RoleBasic}, fmtUserID(provisioned.UserID)); err == nil {
		t.Fatal("a user with an explicit role was demoted")
	} else if _, ok := errors.AsType[*ExplicitRoleError](err); !ok {
		t.Fatalf("UpdateUser() failed with %v, want an ExplicitRoleError", err)
	}
	c.emailsWithExplicitRoles = nil

	// A user SCIM has not provisioned still gets the profile the sign-in asserts.
	if _, err := c.EnsureIdentity(ctx, signInIdentity("00u-unbound", "renamed@example.com"), "", unlimited); err != nil {
		t.Fatal(err)
	}
	if got := storedLifecycleUser(t, c, unbound.ID); got.Email != "renamed@example.com" {
		t.Fatalf("sign-in of an unprovisioned user did not update the email: %+v", got)
	}
}

func TestSCIMProvisionedUsersAreManagedByTheIdentityProvider(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub, srv := newAuthProviderStub(t)
	ctx := accesstoken.ContextWithAccessToken(t.Context(), "access-token")
	conn, _ := createTestSCIMConnection(t, c, true)
	provisioned := provisionTestSCIMUser(t, c, conn, "00u-provisioned", "scim@example.com")

	// Identity unlinking is refused.
	if err := c.RemoveIdentity(ctx, &types.Identity{UserID: provisioned.UserID}); err == nil {
		t.Fatal("the identity of a provisioned user was removed")
	} else if _, ok := errors.AsType[*SCIMManagedUserError](err); !ok {
		t.Fatalf("RemoveIdentity() failed with %v, want a SCIMManagedUserError", err)
	}

	// A username change is refused, while settings that SCIM does not manage can still change.
	if _, err := c.UpdateUser(ctx, true, &types.User{Username: "renamed"}, fmtUserID(provisioned.UserID)); err == nil {
		t.Fatal("the username of a provisioned user was changed")
	} else if _, ok := errors.AsType[*SCIMManagedUserError](err); !ok {
		t.Fatalf("UpdateUser() failed with %v, want a SCIMManagedUserError", err)
	}
	if _, err := c.UpdateUser(ctx, true, &types.User{Timezone: "Europe/Paris"}, fmtUserID(provisioned.UserID)); err != nil {
		t.Fatalf("failed to change the timezone: %v", err)
	}

	// The login profile refresh neither asks the provider nor overwrites the SCIM profile.
	user := storedLifecycleUser(t, c, provisioned.UserID)
	if err := c.decryptUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateProfileIfNeeded(ctx, &user, lifecycleTestProvider.Name, lifecycleTestProvider.Namespace, srv.URL); err != nil {
		t.Fatalf("failed to refresh the profile: %v", err)
	}
	if n := stub.count("/obot-get-user-info"); n != 0 {
		t.Fatalf("the profile refresh of a provisioned user made %d requests", n)
	}
	if got := storedLifecycleUser(t, c, provisioned.UserID); got.DisplayName != "SCIM Name" || got.Timezone != "Europe/Paris" {
		t.Fatalf("provisioned user = %+v", got)
	}
}

func TestLoginProfileRefreshIsModeAware(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub, srv := newAuthProviderStub(t)
	ctx := accesstoken.ContextWithAccessToken(t.Context(), "access-token")

	refresh := func(userID uint) {
		t.Helper()
		user := storedLifecycleUser(t, c, userID)
		if err := c.decryptUser(ctx, &user); err != nil {
			t.Fatal(err)
		}
		if err := c.UpdateProfileIfNeeded(ctx, &user, lifecycleTestProvider.Name, lifecycleTestProvider.Namespace, srv.URL); err != nil {
			t.Fatalf("failed to refresh the profile: %v", err)
		}
	}

	// Without a connection, the refresh asks the provider and writes its answer.
	before := createLifecycleTestUser(t, c, "before", lifecycleTestProvider)
	refresh(before.ID)
	if n := stub.count("/obot-get-user-info"); n != 1 {
		t.Fatalf("the refresh without a connection made %d requests, want 1", n)
	}
	if got := storedLifecycleUser(t, c, before.ID); got.DisplayName != "Login Name" {
		t.Fatalf("the refresh without a connection did not write the profile: %+v", got)
	}

	// With a connection, even a user SCIM has not provisioned is left alone.
	createTestSCIMConnection(t, c, true)
	unprovisioned := createLifecycleTestUser(t, c, "unprovisioned", lifecycleTestProvider)
	refresh(unprovisioned.ID)
	if n := stub.count("/obot-get-user-info"); n != 1 {
		t.Fatalf("the refresh with a connection made %d more requests", n-1)
	}
	if got := storedLifecycleUser(t, c, unprovisioned.ID); got.DisplayName != "" {
		t.Fatalf("the refresh with a connection wrote the profile: %+v", got)
	}
}

func TestLoginProfileRefreshInFlightWhenSCIMStartsWritesNothing(t *testing.T) {
	c := newLifecycleTestClient(t)
	stub, srv := newAuthProviderStub(t)
	ctx := accesstoken.ContextWithAccessToken(t.Context(), "access-token")
	user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)

	// The connection is created while the provider is answering the refresh.
	started := make(chan error, 1)
	stub.runDuring("/obot-get-user-info", func() {
		_, _, err := c.CreateSCIMConnection(ctx, testSCIMConnectionOptions(true))
		started <- err
	})

	stored := storedLifecycleUser(t, c, user.ID)
	if err := c.decryptUser(ctx, &stored); err != nil {
		t.Fatal(err)
	}
	if err := c.UpdateProfileIfNeeded(ctx, &stored, lifecycleTestProvider.Name, lifecycleTestProvider.Namespace, srv.URL); err != nil {
		t.Fatalf("failed to refresh the profile: %v", err)
	}
	requireStarted(t, started)
	if n := stub.count("/obot-get-user-info"); n != 1 {
		t.Fatalf("the refresh made %d requests, want 1", n)
	}
	if got := storedLifecycleUser(t, c, user.ID); got.DisplayName != "" {
		t.Fatalf("a refresh in flight when SCIM started wrote the profile: %+v", got)
	}
}

// requireStarted fails the test unless a function that runDuring registered ran and reported no error.
func requireStarted(t *testing.T, started <-chan error) {
	t.Helper()

	select {
	case err := <-started:
		if err != nil {
			t.Fatalf("failed to start SCIM while the provider was answering: %v", err)
		}
	default:
		t.Fatal("SCIM did not start while the provider was answering")
	}
}

func TestDirectoryResponsesInFlightWhenSCIMStartsAreDiscarded(t *testing.T) {
	t.Run("sign-in", func(t *testing.T) {
		c := newLifecycleTestClient(t)
		stub, srv := newAuthProviderStub(t)
		ctx := accesstoken.ContextWithAccessToken(auth.ContextWithProviderGroupIDPrefix(auth.ContextWithProviderURL(t.Context(), srv.URL), "okta/"), "access-token")
		unlimited := UserLimit{
			Unlimited: true,
		}

		// Without a connection, sign-in adds alice to the directory's group.
		alice, err := c.EnsureIdentity(ctx, signInIdentity("00u-alice", "alice@example.com"), "", unlimited)
		if err != nil {
			t.Fatalf("failed to sign in: %v", err)
		}
		if err := c.db.WithContext(ctx).Model(new(types.Identity)).Where("user_id = ?", alice.ID).
			UpdateColumn(groupsLastCheckedColumn, time.Time{}).Error; err != nil {
			t.Fatal(err)
		}

		// While the provider answers her next group check, SCIM starts, binds the group under a new name, and removes
		// her from it. The directory's response still holds the old name and her membership.
		started := make(chan error, 1)
		stub.runDuring("/obot-list-user-auth-groups", func() {
			conn, _, err := c.CreateSCIMConnection(ctx, testSCIMConnectionOptions(true))
			if err == nil {
				_, err = c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
					DisplayName: "Directory",
				})
			}
			started <- err
		})

		signedIn := signInIdentity("00u-alice", "alice@example.com")
		if _, err := c.EnsureIdentity(ctx, signedIn, "", unlimited); err != nil {
			t.Fatalf("failed to sign in while SCIM starts: %v", err)
		}
		requireStarted(t, started)
		if n := stub.count("/obot-list-user-auth-groups"); n != 2 {
			t.Fatalf("sign-in made %d group requests, want 2", n)
		}

		var group types.Group
		if err := c.db.WithContext(ctx).Where("id = ?", "okta/00g-directory").First(&group).Error; err != nil {
			t.Fatal(err)
		}
		if group.Name != "Directory" {
			t.Fatalf("the directory's response renamed the bound group to %q", group.Name)
		}
		groupIDs, err := c.ListGroupIDsForUser(ctx, alice.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(groupIDs) != 0 {
			t.Fatalf("the directory's response restored memberships %v", groupIDs)
		}
		if ids := signedIn.GetAuthProviderGroupIDs(); len(ids) != 0 {
			t.Fatalf("sign-in reported groups %v, want the stored ones", ids)
		}
	})

	t.Run("resolution", func(t *testing.T) {
		c := newLifecycleTestClient(t)
		stub, srv := newAuthProviderStub(t)
		ctx := auth.ContextWithProviderGroupIDPrefix(t.Context(), "okta/")

		started := make(chan error, 1)
		stub.runDuring("/obot-get-auth-groups", func() {
			_, _, err := c.CreateSCIMConnection(ctx, testSCIMConnectionOptions(true))
			started <- err
		})

		// The caller still learns the name it asked for, but the group is not stored.
		resolved, err := c.ResolveAuthGroups(ctx, srv.URL, lifecycleTestProvider.Namespace, lifecycleTestProvider.Name, []string{"okta/00g-directory"})
		if err != nil {
			t.Fatalf("failed to resolve groups: %v", err)
		}
		requireStarted(t, started)
		if len(resolved) != 1 || resolved[0].Name != "directory" {
			t.Fatalf("resolved groups = %+v", resolved)
		}

		var stored int64
		if err := c.db.WithContext(ctx).Model(new(types.Group)).Where("id = ?", "okta/00g-directory").Count(&stored).Error; err != nil {
			t.Fatal(err)
		}
		if stored != 0 {
			t.Fatal("a group resolved while SCIM started was stored")
		}
	})
}

func TestDeleteAuthProviderGroupDataWhileASCIMConnectionOwnsIt(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	user := createLifecycleTestUser(t, c, "alice", lifecycleTestProvider)

	seed := func(provider AuthProviderRef, groupID string) {
		t.Helper()
		if err := c.db.WithContext(ctx).Create(&types.Group{
			ID:                    groupID,
			AuthProviderName:      provider.Name,
			AuthProviderNamespace: provider.Namespace,
			Name:                  groupID,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := c.db.WithContext(ctx).Create(&types.GroupMemberships{
			UserID:  user.ID,
			GroupID: groupID,
		}).Error; err != nil {
			t.Fatal(err)
		}
		if err := c.db.WithContext(ctx).Create(&types.GroupRoleAssignment{
			GroupName: groupID,
			Role:      apitypes.RoleAdmin,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	entra := AuthProviderRef{
		Namespace: system.DefaultNamespace,
		Name:      "entra-auth-provider",
	}
	seed(lifecycleTestProvider, "okta/00g-team")
	seed(entra, "entra/team")

	createTestSCIMConnection(t, c, true)

	// The connection owns its provider's data, and every group ID with its prefix, even under another provider name.
	// Deconfiguring its own provider deletes the connection first, so a cleanup of that provider is retried, while one
	// of another provider with the same prefix keeps the data for good.
	for _, tt := range []struct {
		name        string
		wantManaged bool
	}{
		{
			name: lifecycleTestProvider.Name,
		},
		{
			name:        "other-okta-auth-provider",
			wantManaged: true,
		},
	} {
		err := c.DeleteAuthProviderGroupData(ctx, system.DefaultNamespace, tt.name, "okta/")
		if err == nil || errors.Is(err, ErrSCIMManagedGroupData) != tt.wantManaged {
			t.Fatalf("DeleteAuthProviderGroupData(%s) = %v, want ErrSCIMManagedGroupData %v", tt.name, err, tt.wantManaged)
		}
	}
	for _, model := range []any{new(types.Group), new(types.GroupMemberships), new(types.GroupRoleAssignment)} {
		var n int64
		if err := c.db.WithContext(ctx).Model(model).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Fatalf("got %d rows of %T, want the 2 seeded", n, model)
		}
	}

	// Another provider's data is still cleaned up.
	if err := c.DeleteAuthProviderGroupData(ctx, entra.Namespace, entra.Name, "entra/"); err != nil {
		t.Fatalf("failed to delete the group data of another provider: %v", err)
	}
	var groups []types.Group
	if err := c.db.WithContext(ctx).Find(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].ID != "okta/00g-team" {
		t.Fatalf("groups after cleaning up another provider = %+v", groups)
	}
}

func fmtUserID(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}

// TestEnforcedSignInNeverCreatesAnAccount covers sign-ins, once SCIM is enforced, whose user was deleted or whose
// identity does not exist: sign-in refuses rather than create an account.
func TestEnforcedSignInNeverCreatesAnAccount(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	conn, _ := createTestSCIMConnection(t, c, false)
	setSCIMConnectionState(t, c, conn, types.SCIMConnectionStateEnforced)
	deleted := createLifecycleTestUser(t, c, "deleted", lifecycleTestProvider)
	if err := c.db.WithContext(ctx).Model(deleted).UpdateColumn("deleted_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}

	var users, identities int64
	c.db.WithContext(ctx).Model(new(types.User)).Count(&users)
	c.db.WithContext(ctx).Model(new(types.Identity)).Count(&identities)

	for _, id := range []*types.Identity{
		signInIdentity("00u-deleted", "deleted@example.com"),
		signInIdentity("00u-unknown", "unknown@example.com"),
	} {
		_, err := c.EnsureIdentity(ctx, id, "", UserLimit{
			Unlimited: true,
		})
		if _, ok := errors.AsType[*UserAccessDeniedError](err); !ok {
			t.Fatalf("enforced sign-in of %s failed with %v, want a UserAccessDeniedError", id.ProviderUserID, err)
		}
	}

	var usersAfter, identitiesAfter int64
	c.db.WithContext(ctx).Model(new(types.User)).Count(&usersAfter)
	c.db.WithContext(ctx).Model(new(types.Identity)).Count(&identitiesAfter)
	if usersAfter != users || identitiesAfter != identities {
		t.Fatalf("enforced sign-ins created %d users and %d identities", usersAfter-users, identitiesAfter-identities)
	}
}

func TestSignInThroughAnotherProviderLeavesTheSCIMProfileAlone(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	conn, _ := createTestSCIMConnection(t, c, true)
	provisioned := provisionTestSCIMUser(t, c, conn, "00u-provisioned", "person@example.com")

	// A user whose email was never marked unverified matches a verified provider's sign-in by email.
	if err := c.db.WithContext(ctx).Model(new(types.User)).Where("id = ?", provisioned.UserID).UpdateColumn("verified_email", nil).Error; err != nil {
		t.Fatal(err)
	}

	user, err := c.EnsureIdentity(ctx, &types.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "google-auth-provider",
		ProviderUsername:      "person-google",
		ProviderUserID:        "google-1",
		Email:                 "person@example.com",
	}, "", UserLimit{
		Unlimited: true,
	})
	if err != nil {
		t.Fatalf("failed to sign in through another provider: %v", err)
	}
	if user.ID != provisioned.UserID {
		t.Fatalf("the sign-in matched user %d, want %d", user.ID, provisioned.UserID)
	}
	if got := storedLifecycleUser(t, c, provisioned.UserID); got.Username != "00u-provisioned" || got.Email != "person@example.com" || got.DisplayName != "SCIM Name" {
		t.Fatalf("a sign-in through another provider changed the SCIM profile: %+v", got)
	}
}

func TestSCIMConnectionTokensExpire(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	conn, _ := createTestSCIMConnection(t, c, false)

	authenticates := func(token string) bool {
		t.Helper()
		_, err := c.AuthenticateSCIMConnection(ctx, token)
		if err == nil {
			return true
		}
		if _, ok := errors.AsType[*SCIMAuthenticationError](err); !ok {
			t.Fatalf("AuthenticateSCIMConnection() failed with %v, want a SCIMAuthenticationError", err)
		}
		return false
	}
	issuedAgo := func(age time.Duration) {
		t.Helper()
		if err := c.db.WithContext(ctx).Model(new(types.SCIMConnection)).Where("id = ?", conn.ID).
			UpdateColumn("token_issued_at", time.Now().Add(-age)).Error; err != nil {
			t.Fatal(err)
		}
	}

	issued, first, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if expiresAt := issued.TokenExpiresAt(); expiresAt == nil || !expiresAt.Equal(issued.TokenIssuedAt.Add(types.SCIMTokenLifetime)) {
		t.Fatalf("TokenExpiresAt() = %v, want a year after %v", expiresAt, issued.TokenIssuedAt)
	}

	// A token is accepted until it is a year old.
	issuedAgo(types.SCIMTokenLifetime - time.Minute)
	if !authenticates(first) {
		t.Fatal("a token was refused before it expired")
	}
	issuedAgo(types.SCIMTokenLifetime + time.Minute)
	if authenticates(first) {
		t.Fatal("an expired token was accepted")
	}

	// Rotating an expired token does not accept it again for a day.
	rotated, second, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if authenticates(first) || rotated.PreviousTokenAccepted(time.Now()) || !authenticates(second) {
		t.Fatal("rotating an expired token accepted it again")
	}

	// The token that a rotation replaces is accepted for a day, but never past its own expiry.
	issuedAgo(types.SCIMTokenLifetime - time.Hour)
	expiring, err := c.SCIMConnection(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	replaced, _, err := c.RotateSCIMConnectionToken(ctx, conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.PreviousTokenExpiresAt == nil || !replaced.PreviousTokenExpiresAt.Equal(*expiring.TokenExpiresAt()) {
		t.Fatalf("PreviousTokenExpiresAt = %v, want the replaced token's expiry %v", replaced.PreviousTokenExpiresAt, expiring.TokenExpiresAt())
	}
	if !authenticates(second) {
		t.Fatal("the replaced token was refused before it expired")
	}
}

func TestDeletingASCIMGroup(t *testing.T) {
	for _, tt := range []struct {
		name        string
		state       types.SCIMConnectionState
		wantDeleted bool
	}{
		{
			name:        "before SCIM is enforced, the group stays and can be bound again by its name",
			state:       types.SCIMConnectionStateConnected,
			wantDeleted: false,
		},
		{
			name:        "once SCIM is enforced, the group is deleted with what it was granted",
			state:       types.SCIMConnectionStateEnforced,
			wantDeleted: true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newLifecycleTestClient(t)
			ctx := t.Context()
			conn, _ := createTestSCIMConnection(t, c, false)
			scimUser := provisionTestSCIMUser(t, c, conn, "00u-user", "user@example.com")
			group, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
				DisplayName: "Admins",
				MemberIDs:   []string{scimUser.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := c.CreateGroupRoleAssignment(ctx, group.GroupID, apitypes.RoleAdmin, ""); err != nil {
				t.Fatal(err)
			}
			setSCIMConnectionState(t, c, conn, tt.state)

			if err := c.DeleteSCIMGroup(ctx, conn, group.ID); err != nil {
				t.Fatal(err)
			}
			if err := c.DeleteSCIMGroup(ctx, conn, group.ID); !errors.As(err, new(*SCIMNotFoundError)) {
				t.Fatalf("a second DeleteSCIMGroup() error = %v, want not found", err)
			}

			count := func(model any, query string, args ...any) int64 {
				t.Helper()
				var n int64
				if err := c.db.WithContext(ctx).Model(model).Where(query, args...).Count(&n).Error; err != nil {
					t.Fatal(err)
				}
				return n
			}
			if n := count(new(types.GroupMemberships), "group_id = ?", group.GroupID); n != 0 {
				t.Fatalf("%d memberships of the deleted group remain", n)
			}
			wantKept := int64(1)
			if tt.wantDeleted {
				wantKept = 0
			}
			if n := count(new(types.Group), "id = ?", group.GroupID); n != wantKept {
				t.Fatalf("%d groups remain, want %d", n, wantKept)
			}
			if n := count(new(types.GroupRoleAssignment), "group_name = ?", group.GroupID); n != wantKept {
				t.Fatalf("%d group role assignments remain, want %d", n, wantKept)
			}
			if n := count(new(types.SCIMGroupSubjectCleanup), "group_id = ?", group.GroupID); n != 1-wantKept {
				t.Fatalf("%d cleanups of the group's subjects were recorded, want %d", n, 1-wantKept)
			}

			// A group pushed later under the same name binds to the kept group, or gets a new one with no grants.
			pushed, err := c.CreateSCIMGroup(ctx, conn, SCIMGroupInput{
				DisplayName: "Admins",
				MemberIDs:   []string{scimUser.ID},
			})
			if err != nil {
				t.Fatal(err)
			}
			if (pushed.GroupID == group.GroupID) == tt.wantDeleted {
				t.Fatalf("the pushed group's ID is %q, and the deleted group's %q", pushed.GroupID, group.GroupID)
			}
			user, _, err := c.UserByIDWithEffectiveRole(ctx, scimUser.UserID)
			if err != nil {
				t.Fatal(err)
			}
			if user.Role.HasRole(apitypes.RoleAdmin) == tt.wantDeleted {
				t.Fatalf("the member of the pushed group has role %v", user.Role)
			}

			// A new reference to the deleted group is refused, as it names no group.
			err = c.WithNewSCIMGroupReferences(ctx, []string{group.GroupID}, func() error { return nil })
			if _, refused := errors.AsType[*SCIMGroupReferenceError](err); refused != tt.wantDeleted {
				t.Fatalf("WithNewSCIMGroupReferences() for the deleted group's ID error = %v", err)
			}
		})
	}
}

func TestSCIMGroupSubjectCleanupClaims(t *testing.T) {
	c := newLifecycleTestClient(t)
	ctx := t.Context()
	for _, groupID := range []string{"okta/first", "okta/second"} {
		if err := c.db.WithContext(ctx).Create(&types.SCIMGroupSubjectCleanup{
			GroupID:   groupID,
			Namespace: system.DefaultNamespace,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	claim := func() []string {
		t.Helper()
		cleanups, err := c.ClaimSCIMGroupSubjectCleanups(ctx, 10)
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(cleanups))
		for _, cleanup := range cleanups {
			ids = append(ids, cleanup.GroupID)
		}
		slices.Sort(ids)
		return ids
	}

	if got := claim(); !slices.Equal(got, []string{"okta/first", "okta/second"}) {
		t.Fatalf("claimed %v", got)
	}
	// A claimed cleanup is not handed out again until its claim expires.
	if got := claim(); len(got) != 0 {
		t.Fatalf("claimed cleanups were claimed again: %v", got)
	}

	if err := c.CompleteSCIMGroupSubjectCleanup(ctx, "okta/first"); err != nil {
		t.Fatal(err)
	}
	if err := c.FailSCIMGroupSubjectCleanup(ctx, "okta/second", errors.New("conflict")); err != nil {
		t.Fatal(err)
	}
	if err := c.db.WithContext(ctx).Model(new(types.SCIMGroupSubjectCleanup)).Where("group_id = ?", "okta/second").
		UpdateColumn("claimed_until", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatal(err)
	}
	if got := claim(); !slices.Equal(got, []string{"okta/second"}) {
		t.Fatalf("after the claim expired, claimed %v", got)
	}
	var failed types.SCIMGroupSubjectCleanup
	if err := c.db.WithContext(ctx).Where("group_id = ?", "okta/second").Take(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if failed.Attempts != 1 || failed.LastError != "conflict" {
		t.Fatalf("the failed cleanup = %+v", failed)
	}
}

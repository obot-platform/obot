package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newIdentityUserLimitTestClient(t *testing.T) *Client {
	t.Helper()

	c := newTestClient(t)
	c.storageClient = fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithObjects(&v1.UserDefaultRoleSetting{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: system.DefaultNamespace,
				Name:      system.DefaultRoleSettingName,
			},
			Spec: v1.UserDefaultRoleSettingSpec{
				Role: apitypes.RoleBasic,
			},
		}).
		Build()
	return c
}

func ensureUserLimitTestIdentity(ctx context.Context, c *Client, username, email string, userLimit UserLimit) (*gatewaytypes.User, error) {
	return c.EnsureIdentityWithRole(ctx, &gatewaytypes.Identity{
		ProviderUsername: username,
		ProviderUserID:   username,
		Email:            email,
	}, "", apitypes.RoleBasic, userLimit)
}

func countIdentityUserLimitTestUsers(t *testing.T, c *Client, countableOnly bool) int64 {
	t.Helper()

	if countableOnly {
		count, err := countUsersTowardLimit(c.db.WithContext(t.Context()))
		if err != nil {
			t.Fatalf("failed to count users toward limit: %v", err)
		}
		return count
	}

	var count int64
	if err := c.db.WithContext(t.Context()).Model(new(gatewaytypes.User)).Count(&count).Error; err != nil {
		t.Fatalf("failed to count users: %v", err)
	}
	return count
}

func countIdentityUserLimitTestIdentities(t *testing.T, c *Client) int64 {
	t.Helper()

	var count int64
	if err := c.db.WithContext(t.Context()).Model(new(gatewaytypes.Identity)).Count(&count).Error; err != nil {
		t.Fatalf("failed to count identities: %v", err)
	}
	return count
}

func countIdentityUserLimitTestLocalAuthUsers(t *testing.T, c *Client) int64 {
	t.Helper()

	var count int64
	if err := c.db.WithContext(t.Context()).Model(new(gatewaytypes.LocalAuthUser)).Count(&count).Error; err != nil {
		t.Fatalf("failed to count local auth users: %v", err)
	}
	return count
}

func requireIdentityUserLimitForbiddenError(t *testing.T, err error) {
	t.Helper()

	var httpErr *apitypes.ErrHTTP
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %v, want *types.ErrHTTP", err, err)
	}
	if httpErr.Code != http.StatusForbidden {
		t.Fatalf("HTTP status = %d, want %d", httpErr.Code, http.StatusForbidden)
	}
}

func TestEnsureIdentityWithRoleEnforcesUserLimit(t *testing.T) {
	const maximum = 2
	c := newIdentityUserLimitTestClient(t)
	userLimit := UserLimit{Maximum: maximum}

	for i := 1; i <= maximum; i++ {
		username := fmt.Sprintf("user-%d", i)
		if _, err := ensureUserLimitTestIdentity(t.Context(), c, username, username+"@example.com", userLimit); err != nil {
			t.Fatalf("creating user %d: %v", i, err)
		}
	}

	_, err := ensureUserLimitTestIdentity(t.Context(), c, "user-3", "user-3@example.com", userLimit)
	requireIdentityUserLimitForbiddenError(t, err)

	if got := countIdentityUserLimitTestUsers(t, c, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}
	if got := countIdentityUserLimitTestIdentities(t, c); got != maximum {
		t.Fatalf("identities = %d, want %d; rejected identity was not rolled back", got, maximum)
	}
}

func TestEnsureIdentityWithRoleAllowsExistingUserWhenOverLimit(t *testing.T) {
	limit := UserLimit{Unlimited: true}
	c := newIdentityUserLimitTestClient(t)

	var thirdUser *gatewaytypes.User
	for i := 1; i <= 3; i++ {
		username := fmt.Sprintf("user-%d", i)
		user, err := ensureUserLimitTestIdentity(t.Context(), c, username, username+"@example.com", limit)
		if err != nil {
			t.Fatalf("creating user %d: %v", i, err)
		}
		if i == 3 {
			thirdUser = user
		}
	}

	limit = UserLimit{Maximum: 2}

	existing, err := ensureUserLimitTestIdentity(t.Context(), c, "user-3", "user-3@example.com", limit)
	if err != nil {
		t.Fatalf("ensuring an existing user while over limit: %v", err)
	}
	if existing.ID != thirdUser.ID {
		t.Fatalf("existing user ID = %d, want %d", existing.ID, thirdUser.ID)
	}

	_, err = ensureUserLimitTestIdentity(t.Context(), c, "user-4", "user-4@example.com", limit)
	requireIdentityUserLimitForbiddenError(t, err)
	if got := countIdentityUserLimitTestUsers(t, c, true); got != 3 {
		t.Fatalf("users counted toward limit = %d, want 3", got)
	}
}

func TestEnsureIdentityWithRoleAllowsVerifiedIdentityForExistingUserAtLimit(t *testing.T) {
	limit := UserLimit{Unlimited: true}
	c := newIdentityUserLimitTestClient(t)

	googleIdentity := &gatewaytypes.Identity{
		AuthProviderNamespace: "default",
		AuthProviderName:      "google-auth-provider",
		ProviderUsername:      "google-user",
		ProviderUserID:        "google-user",
		Email:                 "same-user@example.com",
	}
	first, err := c.EnsureIdentityWithRole(t.Context(), googleIdentity, "", apitypes.RoleBasic, limit)
	if err != nil {
		t.Fatalf("creating user through first verified provider: %v", err)
	}

	limit = UserLimit{Maximum: 1}
	githubIdentity := &gatewaytypes.Identity{
		AuthProviderNamespace: "default",
		AuthProviderName:      "github-auth-provider",
		ProviderUsername:      "github-user",
		ProviderUserID:        "github-user",
		Email:                 "same-user@example.com",
	}
	existing, err := c.EnsureIdentityWithRole(t.Context(), githubIdentity, "", apitypes.RoleBasic, limit)
	if err != nil {
		t.Fatalf("linking a verified identity at the user limit: %v", err)
	}
	if existing.ID != first.ID {
		t.Fatalf("linked user ID = %d, want existing user ID %d", existing.ID, first.ID)
	}
	if got := countIdentityUserLimitTestUsers(t, c, true); got != 1 {
		t.Fatalf("users counted toward limit = %d, want 1", got)
	}
	if got := countIdentityUserLimitTestIdentities(t, c); got != 2 {
		t.Fatalf("identities = %d, want 2", got)
	}
}

func TestEnsureIdentityWithRoleRecoversAfterUserDeletion(t *testing.T) {
	const maximum = 1
	c := newIdentityUserLimitTestClient(t)
	userLimit := UserLimit{Maximum: maximum}

	first, err := ensureUserLimitTestIdentity(t.Context(), c, "user-1", "user-1@example.com", userLimit)
	if err != nil {
		t.Fatalf("creating first user: %v", err)
	}

	_, err = ensureUserLimitTestIdentity(t.Context(), c, "user-2", "user-2@example.com", userLimit)
	requireIdentityUserLimitForbiddenError(t, err)

	if _, err := c.DeleteUser(t.Context(), strconv.FormatUint(uint64(first.ID), 10)); err != nil {
		t.Fatalf("deleting first user: %v", err)
	}
	if _, err := ensureUserLimitTestIdentity(t.Context(), c, "user-2", "user-2@example.com", userLimit); err != nil {
		t.Fatalf("creating user after returning below the limit: %v", err)
	}
	if got := countIdentityUserLimitTestUsers(t, c, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}
}

func TestEnsureIdentityWithRoleDoesNotCountBootstrapUser(t *testing.T) {
	const maximum = 2
	c := newIdentityUserLimitTestClient(t)
	userLimit := UserLimit{Maximum: maximum}

	for _, systemUser := range []string{system.BootstrapName, "nobody", "somebody"} {
		if _, err := ensureUserLimitTestIdentity(t.Context(), c, systemUser, "", userLimit); err != nil {
			t.Fatalf("creating user row for %q: %v", systemUser, err)
		}
	}

	if got := countIdentityUserLimitTestUsers(t, c, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}

	_, err := ensureUserLimitTestIdentity(t.Context(), c, "user-3", "user-3@example.com", userLimit)
	requireIdentityUserLimitForbiddenError(t, err)
}

func TestCreateLocalAuthUserDoesNotConsumeUserLimit(t *testing.T) {
	const maximum = 1
	c := newIdentityUserLimitTestClient(t)
	userLimit := UserLimit{Maximum: maximum}

	const localAuthUsers = 3
	for i := 1; i <= localAuthUsers; i++ {
		email := fmt.Sprintf("local-user-%d@example.com", i)
		if _, err := c.CreateLocalAuthUser(t.Context(), email, "password-hash"); err != nil {
			t.Fatalf("creating local auth user %d: %v", i, err)
		}
	}

	activeUser, err := ensureUserLimitTestIdentity(t.Context(), c, "active-user", "active-user@example.com", userLimit)
	if err != nil {
		t.Fatalf("creating active user: %v", err)
	}

	if got := countIdentityUserLimitTestLocalAuthUsers(t, c); got != localAuthUsers {
		t.Fatalf("local auth users = %d, want %d", got, localAuthUsers)
	}
	if got := countIdentityUserLimitTestUsers(t, c, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}

	firstLocalEmail := "local-user-1@example.com"
	_, err = c.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      system.LocalAuthProvider,
		ProviderUsername:      firstLocalEmail,
		ProviderUserID:        firstLocalEmail,
		Email:                 firstLocalEmail,
	}, "", apitypes.RoleBasic, userLimit)
	requireIdentityUserLimitForbiddenError(t, err)

	if _, err := c.DeleteUser(t.Context(), strconv.FormatUint(uint64(activeUser.ID), 10)); err != nil {
		t.Fatalf("deleting active user: %v", err)
	}
	if _, err := c.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      system.LocalAuthProvider,
		ProviderUsername:      firstLocalEmail,
		ProviderUserID:        firstLocalEmail,
		Email:                 firstLocalEmail,
	}, "", apitypes.RoleBasic, userLimit); err != nil {
		t.Fatalf("creating user row for local auth account after capacity was freed: %v", err)
	}

	if got := countIdentityUserLimitTestUsers(t, c, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}

	secondLocalEmail := "local-user-2@example.com"
	_, err = c.EnsureIdentityWithRole(t.Context(), &gatewaytypes.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      system.LocalAuthProvider,
		ProviderUsername:      secondLocalEmail,
		ProviderUserID:        secondLocalEmail,
		Email:                 secondLocalEmail,
	}, "", apitypes.RoleBasic, userLimit)
	requireIdentityUserLimitForbiddenError(t, err)
}

func TestEnsureIdentityWithRoleAllowsUnlimitedUsers(t *testing.T) {
	c := newIdentityUserLimitTestClient(t)
	userLimit := UserLimit{Maximum: 1, Unlimited: true}

	for i := 1; i <= 3; i++ {
		username := fmt.Sprintf("user-%d", i)
		if _, err := ensureUserLimitTestIdentity(t.Context(), c, username, username+"@example.com", userLimit); err != nil {
			t.Fatalf("creating unlimited user %d: %v", i, err)
		}
	}
	if got := countIdentityUserLimitTestUsers(t, c, true); got != 3 {
		t.Fatalf("users counted toward limit = %d, want 3", got)
	}
}

func TestEnsureIdentityWithRoleEnforcesUserLimitConcurrently(t *testing.T) {
	const (
		maximum  = 2
		attempts = 8
	)
	c := newIdentityUserLimitTestClient(t)
	userLimit := UserLimit{Maximum: maximum}

	start := make(chan struct{})
	errorsByAttempt := make(chan error, attempts)
	var wg sync.WaitGroup
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			username := fmt.Sprintf("concurrent-user-%d", i)
			_, err := ensureUserLimitTestIdentity(t.Context(), c, username, username+"@example.com", userLimit)
			errorsByAttempt <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errorsByAttempt)

	var succeeded, rejected int
	for err := range errorsByAttempt {
		if err == nil {
			succeeded++
			continue
		}

		var httpErr *apitypes.ErrHTTP
		if errors.As(err, &httpErr) && httpErr.Code == http.StatusForbidden {
			rejected++
			continue
		}
		t.Fatalf("concurrent creation returned unexpected error: %v", err)
	}

	if succeeded != maximum {
		t.Fatalf("successful concurrent creations = %d, want %d", succeeded, maximum)
	}
	if rejected != attempts-maximum {
		t.Fatalf("rejected concurrent creations = %d, want %d", rejected, attempts-maximum)
	}
	if got := countIdentityUserLimitTestUsers(t, c, true); got != maximum {
		t.Fatalf("users counted toward limit = %d, want %d", got, maximum)
	}
	if got := countIdentityUserLimitTestIdentities(t, c); got != maximum {
		t.Fatalf("identities = %d, want %d", got, maximum)
	}
}

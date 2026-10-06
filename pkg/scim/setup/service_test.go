package setup

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"github.com/obot-platform/obot/pkg/hash"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	"github.com/obot-platform/obot/pkg/system"
	"gorm.io/gorm"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeAuthProviders struct {
	configured    string
	staged        string
	configuredErr error
}

// racingStorage runs beforeList once, before the list of model access policies that follows the first one, as a
// writer racing with Enforce would. When listErr is set, that list fails with it.
type racingStorage struct {
	kclient.Client
	lists      int
	beforeList func(ctx context.Context)
	listErr    error
}

// serviceTest is a SCIM-first connection of the Okta provider with a provisioned Owner who signed in, a user who
// signed in without being provisioned, a pushed group, an unbound group that a policy references, and an unbound
// group that nothing references.
type serviceTest struct {
	t         *testing.T
	gateway   *gclient.Client
	db        *gorm.DB
	storage   *racingStorage
	providers *fakeAuthProviders
	service   *Service
	conn      *types.SCIMConnection
	owner     Actor
	stranger  uint
	pushed    *gclient.SCIMGroup
}

func (f *fakeAuthProviders) GetConfiguredAuthProvider(context.Context) (string, error) {
	return f.configured, f.configuredErr
}

func (f *fakeAuthProviders) GetStagedAuthProvider(context.Context) (string, error) {
	return f.staged, nil
}

func (r *racingStorage) List(ctx context.Context, list kclient.ObjectList, opts ...kclient.ListOption) error {
	if _, ok := list.(*v1.ModelAccessPolicyList); ok {
		r.lists++
		if r.lists == 2 {
			if r.beforeList != nil {
				r.beforeList(ctx)
			}
			if r.listErr != nil {
				return r.listErr
			}
		}
	}
	return r.Client.List(ctx, list, opts...)
}

func newServiceTest(t *testing.T) *serviceTest {
	t.Helper()
	ctx := t.Context()

	storage := &racingStorage{
		Client: fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(
				&v1.AuthProvider{
					Name:      "okta-auth-provider",
					Namespace: system.DefaultNamespace,
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: clienttypes.AuthProviderManifest{
							CommonProviderMetadata: clienttypes.CommonProviderMetadata{
								Name: "Okta",
							},
							GroupIDPrefix: "okta/",
						},
					},
				},
				policy("legacy-policy", "okta/00g00000000000legacy"),
				&v1.UserDefaultRoleSetting{
					Name:      system.DefaultRoleSettingName,
					Namespace: system.DefaultNamespace,
					Spec: v1.UserDefaultRoleSettingSpec{
						Role: clienttypes.RoleBasic,
					},
				},
			).
			Build(),
	}
	gateway, db := newTestGatewayWithDB(t, storage.Client)
	s := &serviceTest{
		t:       t,
		gateway: gateway,
		db:      db,
		storage: storage,
		providers: &fakeAuthProviders{
			configured: "okta-auth-provider",
		},
	}
	s.service = New(s.gateway, s.storage, s.providers, "https://obot.example.com/")

	var err error
	if s.conn, _, err = s.gateway.CreateSCIMConnection(ctx, gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "okta-auth-provider",
		GroupIDPrefix:         "okta/",
		Issuer:                "https://example.okta.com",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	}); err != nil {
		t.Fatal(err)
	}

	ownerID := s.signIn("00u-owner")
	owner := s.provision("00u-owner")
	s.owner = Actor{
		UserID:                ownerID,
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "okta-auth-provider",
		Owner:                 true,
	}
	s.stranger = s.signIn("00u-stranger")

	if s.pushed, err = s.gateway.CreateSCIMGroup(ctx, s.conn, gclient.SCIMGroupInput{
		DisplayName: "Engineering",
		MemberIDs: []string{
			owner.ID,
		},
	}); err != nil {
		t.Fatal(err)
	}
	return s
}

func policy(name, groupID string) *v1.ModelAccessPolicy {
	return &v1.ModelAccessPolicy{
		Name:      name,
		Namespace: system.DefaultNamespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: clienttypes.ModelAccessPolicyManifest{
				DisplayName: name,
				Subjects: []clienttypes.Subject{
					{
						Type: clienttypes.SubjectTypeGroup,
						ID:   groupID,
					},
				},
			},
		},
	}
}

// signIn signs the native Okta user in, as a just-in-time sign-in before Enforce does.
func (s *serviceTest) signIn(nativeID string) uint {
	s.t.Helper()

	user, err := s.gateway.EnsureIdentityWithRole(s.t.Context(), &types.Identity{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "okta-auth-provider",
		ProviderUsername:      nativeID,
		ProviderUserID:        nativeID,
		HashedProviderUserID:  hash.String(nativeID),
		Email:                 nativeID + "@example.com",
	}, "", clienttypes.RoleOwner, gclient.UserLimit{
		Unlimited: true,
	})
	if err != nil {
		s.t.Fatalf("failed to sign in %s: %v", nativeID, err)
	}
	return user.ID
}

func (s *serviceTest) provision(nativeID string) *gclient.SCIMUser {
	s.t.Helper()

	user, err := s.gateway.CreateSCIMUser(s.t.Context(), s.conn, gclient.SCIMUserInput{
		UserName:   nativeID + "@example.com",
		ExternalID: nativeID,
	}, gclient.SCIMUserCreateOptions{
		UserLimit: gclient.UserLimit{
			Unlimited: true,
		},
		DefaultRole: clienttypes.RoleBasic,
	})
	if err != nil {
		s.t.Fatalf("failed to provision %s: %v", nativeID, err)
	}
	return user
}

// addGroups adds the unbound groups: one that the legacy policy references, and one that nothing references.
func (s *serviceTest) addGroups() {
	s.t.Helper()

	groups := []types.Group{
		{
			ID:                    "okta/00g00000000000legacy",
			AuthProviderName:      "okta-auth-provider",
			AuthProviderNamespace: system.DefaultNamespace,
			Name:                  "Legacy",
		},
		{
			ID:                    "okta/00g000000000000stale",
			AuthProviderName:      "okta-auth-provider",
			AuthProviderNamespace: system.DefaultNamespace,
			Name:                  "Stale",
		},
	}
	// The SCIM-first connection refused residual data when it was created, so these arrive later, as a directory
	// response that was already in flight would.
	if err := s.db.WithContext(s.t.Context()).Create(&groups).Error; err != nil {
		s.t.Fatal(err)
	}
}

func (s *serviceTest) userStatus(userID uint) clienttypes.UserStatus {
	s.t.Helper()

	status, err := s.gateway.UserStatus(s.t.Context(), userID)
	if err != nil {
		s.t.Fatal(err)
	}
	return status
}

func TestReview(t *testing.T) {
	s := newServiceTest(t)
	s.addGroups()
	if err := s.storage.Create(t.Context(), policy("missing-policy", "okta/00g0000000000missing")); err != nil {
		t.Fatal(err)
	}

	review, err := s.service.Review(t.Context(), s.conn.ID, s.owner, 0)
	if err != nil {
		t.Fatal(err)
	}

	conn := review.Connection
	if conn.BaseURL != "https://obot.example.com/scim/v2" || conn.Origin != "scim_first" || conn.AuthProviderDisplayName != "Okta" ||
		!conn.AuthProviderConfigured || conn.HasToken {
		t.Fatalf("connection = %+v", conn)
	}
	if review.ProvisionedUsers.Total != 1 || review.UnprovisionedUsers.Total != 1 || !review.UnprovisionedUsers.Items[0].SignedIn {
		t.Fatalf("users = %+v and %+v", review.ProvisionedUsers, review.UnprovisionedUsers)
	}
	if review.BoundGroups.Total != 1 || review.BoundGroups.Items[0].ID != s.pushed.GroupID {
		t.Fatalf("bound groups = %+v", review.BoundGroups)
	}
	if review.UnboundReferencedGroups.Total != 1 || review.UnboundReferencedGroups.Items[0].NativeID != "00g00000000000legacy" ||
		review.UnboundReferencedGroups.Items[0].ConsoleURL != "https://example-admin.okta.com/admin/group/00g00000000000legacy" {
		t.Fatalf("unbound referenced groups = %+v", review.UnboundReferencedGroups)
	}
	if review.UnreferencedGroups.Total != 1 || review.UnreferencedGroups.Items[0].Name != "Stale" {
		t.Fatalf("unreferenced groups = %+v", review.UnreferencedGroups)
	}
	if len(review.Warnings) != 1 || review.Warnings[0].Type != warningMissingGroup || review.Warnings[0].GroupID != "okta/00g0000000000missing" {
		t.Fatalf("warnings = %+v", review.Warnings)
	}
	// The unbound referenced group blocks enforcing, but the review lists it only among the groups.
	if len(review.EnforceBlockers) != 0 {
		t.Fatalf("enforce blockers = %v", review.EnforceBlockers)
	}

	// An administrator who is not an Owner, and the bootstrap user, are told they cannot enforce, and not what an
	// Owner would have to do about their own account.
	tests := []struct {
		name     string
		actor    Actor
		wantRole string
	}{
		{
			name: "an administrator who has not been provisioned",
			actor: Actor{
				UserID:                s.stranger,
				AuthProviderNamespace: s.owner.AuthProviderNamespace,
				AuthProviderName:      s.owner.AuthProviderName,
			},
			wantRole: "Only an Owner who signed in through Okta can enforce SCIM.",
		},
		{
			name: "the bootstrap user",
			actor: Actor{
				UserID:           s.owner.UserID,
				AuthProviderName: system.BootstrapName,
				Owner:            true,
				Bootstrap:        true,
			},
			wantRole: "Only an Owner who signed in through Okta can enforce SCIM. The bootstrap user cannot.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			review, err := s.service.Review(t.Context(), s.conn.ID, tt.actor, 0)
			if err != nil {
				t.Fatal(err)
			}
			// Only the role, and not what an Owner would have to do about their own account.
			if !slices.Equal(review.EnforceBlockers, []string{tt.wantRole}) {
				t.Fatalf("enforce blockers = %q, want %q", review.EnforceBlockers, tt.wantRole)
			}
		})
	}

	// An Owner whose account could not sign in once SCIM is enforced is told which account it is, so they can find it
	// in the identity provider.
	accountTests := []struct {
		name   string
		userID uint
		want   string
	}{
		{
			name:   "an Owner whom the provider has not provisioned",
			userID: s.stranger,
			want:   "Your account, 00u-stranger (00u-stranger@example.com), has not been provisioned through SCIM. Assign yourself to the SCIM application in Okta.",
		},
		{
			name:   "an Owner who has not signed in through the provider",
			userID: s.provision("00u-new").UserID,
			want:   "Your account, 00u-new, has not signed in through Okta, so it is not known to be able to sign in once SCIM is enforced.",
		},
		{
			// The name only helps find the account, so a user that cannot be read leaves it out.
			name:   "an Owner whose user cannot be read",
			userID: 1_000_000,
			want:   "Your account has not signed in through Okta, so it is not known to be able to sign in once SCIM is enforced.",
		},
	}
	for _, tt := range accountTests {
		t.Run(tt.name, func(t *testing.T) {
			review, err := s.service.Review(t.Context(), s.conn.ID, Actor{
				UserID:                tt.userID,
				AuthProviderNamespace: s.owner.AuthProviderNamespace,
				AuthProviderName:      s.owner.AuthProviderName,
				Owner:                 true,
			}, 0)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(review.EnforceBlockers, []string{tt.want}) {
				t.Fatalf("enforce blockers = %q, want %q", review.EnforceBlockers, tt.want)
			}
		})
	}

	// Lists are paginated.
	page, err := s.service.Groups(t.Context(), s.conn.ID, GroupListUnreferenced, Page{
		Offset: 1,
		Limit:  10,
	})
	if err != nil || page.Total != 1 || len(page.Items) != 0 {
		t.Fatalf("second page of unreferenced groups = %+v, %v", page, err)
	}
	if _, err := s.service.Groups(t.Context(), s.conn.ID, "everything", Page{Limit: 10}); err == nil {
		t.Fatal("an unknown group list was served")
	}
}

func TestEnforce(t *testing.T) {
	s := newServiceTest(t)
	s.addGroups()

	// A referenced group that the identity provider has not pushed blocks Enforce, which changes nothing.
	_, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner)
	var httpErr *clienttypes.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest || !strings.Contains(httpErr.Message, `"Legacy"`) {
		t.Fatalf("Enforce() with an unbound referenced group = %v", err)
	}
	if s.userStatus(s.stranger) != clienttypes.UserStatusActive {
		t.Fatal("a blocked Enforce disabled a user")
	}

	// Once nothing references the group, only an Owner who signed in through the provider can enforce.
	if err := s.storage.Delete(t.Context(), &v1.ModelAccessPolicy{Name: "legacy-policy", Namespace: system.DefaultNamespace}); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []Actor{
		{
			UserID:    s.owner.UserID,
			Owner:     true,
			Bootstrap: true,
		},
		{
			UserID:                s.stranger,
			AuthProviderNamespace: system.DefaultNamespace,
			AuthProviderName:      "okta-auth-provider",
			Owner:                 true,
		},
	} {
		if _, err := s.service.Enforce(t.Context(), s.conn.ID, actor); err == nil {
			t.Fatalf("Enforce() by %+v succeeded", actor)
		}
	}
	// So can only the configured provider's connection.
	s.providers.configured = "github-auth-provider"
	if _, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner); err == nil || !strings.Contains(err.Error(), "not the configured auth provider") {
		t.Fatalf("Enforce() of a provider that is not configured = %v", err)
	}
	// The review says so, rather than offering to enforce.
	unconfiguredReview, err := s.service.Review(t.Context(), s.conn.ID, s.owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(unconfiguredReview.EnforceBlockers, func(blocker string) bool {
		return strings.Contains(blocker, "not the configured auth provider")
	}) {
		t.Fatalf("enforce blockers of a provider that is not configured = %q", unconfiguredReview.EnforceBlockers)
	}
	s.providers.configured = "okta-auth-provider"

	// Nor while another deletion of unreferenced groups is under way, whose marks enforcing would clear.
	run, err := s.gateway.MarkUnreferencedSCIMGroups(t.Context(), s.conn, map[string]struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner); !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("Enforce() during another deletion = %v, want a conflict", err)
	}
	if s.userStatus(s.stranger) != clienttypes.UserStatusActive {
		t.Fatal("an Enforce refused during another deletion disabled a user")
	}
	if err := s.gateway.ClearSCIMGroupDeletionMarks(t.Context(), s.conn.ID, run.ID); err != nil {
		t.Fatal(err)
	}

	result, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner)
	if err != nil {
		t.Fatalf("Enforce() = %v", err)
	}
	if result.Connection.State != "enforced" || result.Connection.EnforcedAt == nil || result.DisabledUserCount != 1 || result.DeletedGroupCount != 2 {
		t.Fatalf("Enforce() = %+v", result)
	}
	if s.userStatus(s.stranger) != clienttypes.UserStatusDisabled || s.userStatus(s.owner.UserID) != clienttypes.UserStatusActive {
		t.Fatal("Enforce did not disable exactly the unprovisioned user")
	}

	review, err := s.service.Review(t.Context(), s.conn.ID, s.owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.EnforceBlockers) != 0 || review.BoundGroups.Total != 1 || review.UnreferencedGroups.Total != 0 {
		t.Fatalf("review after Enforce = %+v", review)
	}

	// Enforcement cannot be repeated.
	if _, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner); !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("Enforce() again = %v, want a conflict", err)
	}
}

func TestEnforceKeepsAGroupThatGainsAReferenceWhileItIsMarked(t *testing.T) {
	s := newServiceTest(t)
	s.addGroups()
	if err := s.storage.Delete(t.Context(), &v1.ModelAccessPolicy{Name: "legacy-policy", Namespace: system.DefaultNamespace}); err != nil {
		t.Fatal(err)
	}

	// A writer checked the group before it was marked, and saves its reference after the marks committed.
	s.storage.beforeList = func(ctx context.Context) {
		if err := s.storage.Create(ctx, policy("racing-policy", "okta/00g000000000000stale")); err != nil {
			t.Error(err)
		}
	}

	_, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner)
	if err == nil || !strings.Contains(err.Error(), `"Stale"`) {
		t.Fatalf("Enforce() = %v, want it blocked by the group that gained a reference", err)
	}

	groups, err := s.gateway.SCIMProviderGroups(t.Context(), system.DefaultNamespace, "okta-auth-provider")
	if err != nil {
		t.Fatal(err)
	}
	var kept bool
	for _, group := range groups {
		if group.PendingDeletion {
			t.Fatalf("group %s is still marked for deletion", group.ID)
		}
		kept = kept || group.ID == "okta/00g000000000000stale"
	}
	if !kept {
		t.Fatal("the group that gained a reference was deleted")
	}
	if s.userStatus(s.stranger) != clienttypes.UserStatusActive {
		t.Fatal("a blocked Enforce disabled a user")
	}
}

func TestTokensAreIssuedOnlyForTheConfiguredProvider(t *testing.T) {
	s := newServiceTest(t)

	// A connection that staging created, whose provider is not configured yet, stays tokenless until the switch.
	refused := func(when string) {
		t.Helper()
		for name, issue := range map[string]func(context.Context, string) (*clienttypes.SCIMConnection, error){
			"RotateToken":        s.service.RotateToken,
			"RevokeCurrentToken": s.service.RevokeCurrentToken,
		} {
			var httpErr *clienttypes.ErrHTTP
			if _, err := issue(t.Context(), s.conn.ID); !errors.As(err, &httpErr) || httpErr.Code != http.StatusBadRequest ||
				!strings.Contains(httpErr.Message, "SCIM tokens are issued only for the configured auth provider") {
				t.Fatalf("%s() %s = %v, want a refusal", name, when, err)
			}
		}
		if conn, err := s.gateway.SCIMConnection(t.Context(), s.conn.ID); err != nil || conn.HasToken() {
			t.Fatalf("connection after a refused token %s = %+v, %v", when, conn, err)
		}
	}

	s.providers.configured = "github-auth-provider"
	refused("while another provider is configured")
	s.providers.configured = ""
	refused("while no provider is configured")

	s.providers.configured = "okta-auth-provider"
	if _, err := s.service.RotateToken(t.Context(), s.conn.ID); err != nil {
		t.Fatalf("RotateToken() for the configured provider = %v", err)
	}
}

func TestTokenManagement(t *testing.T) {
	s := newServiceTest(t)

	issued, err := s.service.RotateToken(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !issued.HasToken || !strings.HasPrefix(issued.Token, "obot_scim_") || issued.PreviousTokenAccepted {
		t.Fatalf("first token = %+v", issued)
	}

	rotated, err := s.service.RotateToken(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Token == issued.Token || !rotated.PreviousTokenAccepted || rotated.PreviousTokenExpiresAt == nil ||
		!rotated.PreviousTokenExpiresAt.Time.After(time.Now().Add(23*time.Hour)) {
		t.Fatalf("rotated token = %+v", rotated)
	}
	if _, err := s.gateway.AuthenticateSCIMConnection(t.Context(), issued.Token); err != nil {
		t.Fatalf("the previous token is not accepted after a rotation: %v", err)
	}

	revoked, err := s.service.RevokePreviousToken(t.Context(), s.conn.ID)
	if err != nil || revoked.PreviousTokenAccepted || revoked.Token != "" {
		t.Fatalf("RevokePreviousToken() = %+v, %v", revoked, err)
	}

	replaced, err := s.service.RevokeCurrentToken(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{issued.Token, rotated.Token} {
		if _, err := s.gateway.AuthenticateSCIMConnection(t.Context(), token); err == nil {
			t.Fatal("a revoked token is still accepted")
		}
	}
	if _, err := s.gateway.AuthenticateSCIMConnection(t.Context(), replaced.Token); err != nil {
		t.Fatalf("the replacement token is not accepted: %v", err)
	}

	if _, err := s.service.RotateToken(t.Context(), "unknown"); err == nil {
		t.Fatal("a token was issued for an unknown connection")
	}
}

func TestFailedTokenReplacementKeepsTheCurrentToken(t *testing.T) {
	s := newServiceTest(t)

	current, err := s.service.RotateToken(t.Context(), s.conn.ID)
	if err != nil {
		t.Fatal(err)
	}

	// A token that is issued but never returned is lost, and replacing it again would retire the token the
	// identity provider still uses. So a failure to build the response must leave the current token in place.
	s.providers.configuredErr = errors.New("storage unavailable")
	for name, replace := range map[string]func(context.Context, string) (*clienttypes.SCIMConnection, error){
		"RotateToken":        s.service.RotateToken,
		"RevokeCurrentToken": s.service.RevokeCurrentToken,
	} {
		if _, err := replace(t.Context(), s.conn.ID); err == nil {
			t.Fatalf("%s succeeded without the configured auth provider", name)
		}
		stored, err := s.gateway.SCIMConnection(t.Context(), s.conn.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.TokenVerifier != hash.String(current.Token) || stored.PreviousTokenVerifier != "" {
			t.Fatalf("a failed %s replaced the current token", name)
		}
	}
}

func TestEnforceStopsWhenTheProviderIsSwitchedAwayMeanwhile(t *testing.T) {
	s := newServiceTest(t)
	s.addGroups()
	if err := s.storage.Delete(t.Context(), &v1.ModelAccessPolicy{Name: "legacy-policy", Namespace: system.DefaultNamespace}); err != nil {
		t.Fatal(err)
	}

	// A switch away from the provider completes after the preconditions were checked and the groups were marked.
	s.storage.beforeList = func(context.Context) {
		s.providers.configured = "github-auth-provider"
	}

	_, err := s.service.Enforce(t.Context(), s.conn.ID, s.owner)
	if err == nil || !strings.Contains(err.Error(), "not the configured auth provider") {
		t.Fatalf("Enforce() = %v, want it stopped by the switch", err)
	}
	if s.userStatus(s.stranger) != clienttypes.UserStatusActive {
		t.Fatal("Enforce disabled a user after its provider was switched away")
	}
	groups, err := s.gateway.SCIMProviderGroups(t.Context(), system.DefaultNamespace, "okta-auth-provider")
	if err != nil {
		t.Fatal(err)
	}
	for _, group := range groups {
		if group.PendingDeletion {
			t.Fatalf("group %s is still marked for deletion", group.ID)
		}
	}
	if conn, err := s.gateway.SCIMConnection(t.Context(), s.conn.ID); err != nil || conn.State != types.SCIMConnectionStateConnected {
		t.Fatalf("connection = %+v, %v", conn, err)
	}
}

func TestReviewWarnsAboutEveryone(t *testing.T) {
	s := newServiceTest(t)
	if err := s.db.WithContext(t.Context()).Create(&types.Group{
		ID:                    "okta/00g0000000everyone",
		AuthProviderName:      "okta-auth-provider",
		AuthProviderNamespace: system.DefaultNamespace,
		Name:                  " Everyone ",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.storage.Create(t.Context(), policy("everyone-policy", "okta/00g0000000everyone")); err != nil {
		t.Fatal(err)
	}

	review, err := s.service.Review(t.Context(), s.conn.ID, s.owner, 0)
	if err != nil {
		t.Fatal(err)
	}
	var warned bool
	for _, warning := range review.Warnings {
		if warning.Type == warningEveryoneGroup && warning.GroupID == "okta/00g0000000everyone" && strings.Contains(warning.Message, "all-users selector") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("warnings = %+v, want one about the Everyone group", review.Warnings)
	}
}

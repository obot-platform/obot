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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	oktaIssuerParam            = "OBOT_OKTA_AUTH_PROVIDER_ISSUER_URL"
	oktaServiceClientIDParam   = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_CLIENT_ID"
	oktaServicePrivateKeyParam = "OBOT_OKTA_AUTH_PROVIDER_SERVICE_PRIVATE_KEY"

	engineeringGroupID = "okta/00g00000000000000eng"
	everyoneGroupID    = "okta/00g000000000everyone"
	staleGroupID       = "okta/00g000000000000stale"
	missingGroupID     = "okta/00g0000000000missing"
)

// enableTest is the Okta provider, configured and synchronizing its directory at sign-in, with two users, a group that
// a policy references, the Everyone group that another policy references, a group that nothing references, and a
// policy that references a group ID that no group has.
type enableTest struct {
	t         *testing.T
	gateway   *gclient.Client
	db        *gorm.DB
	storage   *racingStorage
	providers *fakeAuthProviders
	service   *Service
	okta      v1.AuthProvider
	users     []uint
}

func newEnableTest(t *testing.T) *enableTest {
	t.Helper()

	okta := v1.AuthProvider{
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
	}
	storage := &racingStorage{
		Client: fake.NewClientBuilder().
			WithScheme(storagescheme.Scheme).
			WithObjects(
				okta.DeepCopy(),
				&v1.AuthProvider{
					Name:      "github-auth-provider",
					Namespace: system.DefaultNamespace,
					Spec: v1.AuthProviderSpec{
						AuthProviderManifest: clienttypes.AuthProviderManifest{
							CommonProviderMetadata: clienttypes.CommonProviderMetadata{
								Name: "GitHub",
							},
							GroupIDPrefix: "github/",
						},
					},
				},
				policy("engineering-policy", engineeringGroupID),
				policy("everyone-policy", everyoneGroupID),
				policy("missing-policy", missingGroupID),
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
	s := &enableTest{
		t:       t,
		gateway: gateway,
		db:      db,
		storage: storage,
		providers: &fakeAuthProviders{
			configured: okta.Name,
		},
		okta: okta,
	}
	s.service = New(s.gateway, s.storage, s.providers, "https://obot.example.com/")

	s.storeCredential()
	for _, nativeID := range []string{"00u-owner", "00u-member"} {
		user, err := s.gateway.EnsureIdentityWithRole(t.Context(), &types.Identity{
			AuthProviderNamespace: okta.Namespace,
			AuthProviderName:      okta.Name,
			ProviderUsername:      nativeID,
			ProviderUserID:        nativeID,
			HashedProviderUserID:  hash.String(nativeID),
			Email:                 nativeID + "@example.com",
		}, "", clienttypes.RoleOwner, gclient.UserLimit{
			Unlimited: true,
		})
		if err != nil {
			t.Fatalf("failed to sign in %s: %v", nativeID, err)
		}
		s.users = append(s.users, user.ID)
	}

	s.addGroup(engineeringGroupID, "Engineering")
	s.addGroup(everyoneGroupID, "Everyone")
	s.addGroup(staleGroupID, "Stale")
	return s
}

// storeCredential stores the provider's active configuration, which holds the directory parameters, since the provider
// synchronizes its directory at sign-in.
func (s *enableTest) storeCredential() {
	s.t.Helper()

	if err := s.gateway.UpsertCredential(s.t.Context(), types.Credential{
		Context: s.okta.Name,
		Name:    s.okta.Name,
		Secrets: map[string]string{
			oktaIssuerParam:            "https://example.okta.com/",
			oktaServiceClientIDParam:   "service-client",
			oktaServicePrivateKeyParam: "service-key",
		},
	}); err != nil {
		s.t.Fatal(err)
	}
}

// addGroup adds a group of the provider, as login-time synchronization caches it, with both users as members.
func (s *enableTest) addGroup(id, name string) {
	s.t.Helper()

	if err := s.db.WithContext(s.t.Context()).Create(&types.Group{
		ID:                    id,
		AuthProviderName:      s.okta.Name,
		AuthProviderNamespace: s.okta.Namespace,
		Name:                  name,
	}).Error; err != nil {
		s.t.Fatal(err)
	}
	for _, userID := range s.users {
		if err := s.db.WithContext(s.t.Context()).Create(&types.GroupMemberships{
			UserID:  userID,
			GroupID: id,
		}).Error; err != nil {
			s.t.Fatal(err)
		}
	}
}

// enable enables SCIM as the provider configuration change and the API do, and returns the result.
func (s *enableTest) enable() (*clienttypes.SCIMEnableResult, error) {
	s.t.Helper()

	if _, err := EnableConnection(s.t.Context(), s.storage, s.gateway, s.okta); err != nil {
		s.t.Fatalf("EnableConnection() = %v", err)
	}
	// Races and failures are set up for the lists that CompleteEnable makes.
	s.storage.lists = 0
	return s.service.CompleteEnable(s.t.Context(), s.okta.Namespace, s.okta.Name)
}

// groups returns the IDs of the provider's groups, and of those marked for deletion.
func (s *enableTest) groups() ([]string, []string) {
	s.t.Helper()

	groups, err := s.gateway.SCIMProviderGroups(s.t.Context(), s.okta.Namespace, s.okta.Name)
	if err != nil {
		s.t.Fatal(err)
	}
	ids := make([]string, 0, len(groups))
	var pending []string
	for _, group := range groups {
		ids = append(ids, group.ID)
		if group.PendingDeletion {
			pending = append(pending, group.ID)
		}
	}
	slices.Sort(ids)
	return ids, pending
}

// membershipCount returns how many members the group has.
func (s *enableTest) membershipCount(groupID string) int64 {
	s.t.Helper()

	var count int64
	if err := s.db.WithContext(s.t.Context()).Model(new(types.GroupMemberships)).Where("group_id = ?", groupID).Count(&count).Error; err != nil {
		s.t.Fatal(err)
	}
	return count
}

// otherConnection is the SCIM connection of another auth provider, which is the installation's only one.
func (s *enableTest) otherConnection() *types.SCIMConnection {
	s.t.Helper()

	conn := &types.SCIMConnection{
		ID:                    "0b6bd0a4-7e44-4c3c-9d0b-000000000000",
		AdapterType:           "okta",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "github-auth-provider",
		GroupIDPrefix:         "github/",
		State:                 types.SCIMConnectionStateConnected,
		EnabledAt:             time.Now(),
	}
	if err := s.db.WithContext(s.t.Context()).Create(conn).Error; err != nil {
		s.t.Fatal(err)
	}
	return conn
}

func TestEnablePreview(t *testing.T) {
	s := newEnableTest(t)

	preview, err := s.service.EnablePreview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) != 0 || len(preview.DuplicateGroupNames) != 0 {
		t.Fatalf("blockers = %v, duplicates = %+v", preview.Blockers, preview.DuplicateGroupNames)
	}
	if preview.AuthProviderName != s.okta.Name || preview.AuthProviderNamespace != s.okta.Namespace || preview.AuthProviderDisplayName != "Okta" {
		t.Fatalf("auth provider = %q/%q (%q)", preview.AuthProviderNamespace, preview.AuthProviderName, preview.AuthProviderDisplayName)
	}
}

func TestEnablePreviewBlockers(t *testing.T) {
	tests := []struct {
		name string
		// setup changes the installation before the preview.
		setup        func(s *enableTest)
		wantBlocker  string
		wantProvider bool
		// wantDuplicates is whether the preview lists the referenced groups that share a name.
		wantDuplicates bool
	}{
		{
			name: "no auth provider is configured",
			setup: func(s *enableTest) {
				s.providers.configured = ""
			},
			wantBlocker: "No auth provider is configured.",
		},
		{
			name: "the configured auth provider does not support SCIM",
			setup: func(s *enableTest) {
				s.providers.configured = "github-auth-provider"
			},
			wantBlocker: "GitHub does not support SCIM provisioning.",
		},
		{
			name: "a switch is staged",
			setup: func(s *enableTest) {
				s.providers.staged = "github-auth-provider"
			},
			wantBlocker:  "A switch to GitHub is staged.",
			wantProvider: true,
		},
		{
			name: "a provider configuration change is in progress",
			setup: func(s *enableTest) {
				if err := s.storage.Create(s.t.Context(), &v1.ProviderConfigurationChange{
					Name:      system.ProviderChangeAuthName,
					Namespace: system.DefaultNamespace,
				}); err != nil {
					s.t.Fatal(err)
				}
			},
			wantBlocker:  "A change to the auth provider configuration is in progress.",
			wantProvider: true,
		},
		{
			name: "a cleanup of the provider's group ID prefix is pending",
			setup: func(s *enableTest) {
				if err := s.storage.Create(s.t.Context(), &v1.AuthProviderCleanup{
					Name:      "cleanup",
					Namespace: system.DefaultNamespace,
					Spec: v1.AuthProviderCleanupSpec{
						AuthProviderName: "custom-auth-provider",
						GroupIDPrefix:    "okta/",
					},
				}); err != nil {
					s.t.Fatal(err)
				}
			},
			wantBlocker:  "are still being cleaned up (cleanup)",
			wantProvider: true,
		},
		{
			name: "two referenced groups have the same name",
			setup: func(s *enableTest) {
				s.addGroup("okta/00g00000engineering", " engineering ")
				if err := s.storage.Create(s.t.Context(), policy("duplicate-policy", "okta/00g00000engineering")); err != nil {
					s.t.Fatal(err)
				}
			},
			// The groups are listed by name, and the first one names the duplicate.
			wantBlocker:    `2 referenced groups are named "engineering"`,
			wantProvider:   true,
			wantDuplicates: true,
		},
		{
			name: "the provider already has a SCIM connection",
			setup: func(s *enableTest) {
				if _, err := EnableConnection(s.t.Context(), s.storage, s.gateway, s.okta); err != nil {
					s.t.Fatal(err)
				}
			},
			wantBlocker:  "Okta already provisions users and groups through SCIM.",
			wantProvider: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newEnableTest(t)
			tt.setup(s)

			preview, err := s.service.EnablePreview(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Blockers) != 1 || !strings.Contains(preview.Blockers[0], tt.wantBlocker) {
				t.Fatalf("blockers = %q, want one containing %q", preview.Blockers, tt.wantBlocker)
			}
			if got := preview.AuthProviderName != ""; got != tt.wantProvider {
				t.Fatalf("auth provider = %q, want one: %v", preview.AuthProviderName, tt.wantProvider)
			}
			if got := len(preview.DuplicateGroupNames) > 0; got != tt.wantDuplicates {
				t.Fatalf("duplicate group names = %+v, want some: %v", preview.DuplicateGroupNames, tt.wantDuplicates)
			}
		})
	}
}

func TestEnableRefusesDuplicateNamesWithAnActionableError(t *testing.T) {
	s := newEnableTest(t)
	s.addGroup("okta/00g00000engineering", "ENGINEERING")
	if err := s.storage.Create(t.Context(), policy("duplicate-policy", "okta/00g00000engineering")); err != nil {
		t.Fatal(err)
	}

	_, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta)
	blocked, ok := errors.AsType[*EnableBlockedError](err)
	if !ok {
		t.Fatalf("EnableConnection() = %v, want it blocked", err)
	}
	// The error names each group, where to find it in Okta, and what references it, so the admin can resolve it.
	message := blocked.Error()
	for _, want := range []string{
		`Groups named "ENGINEERING":`,
		`"Engineering" (` + engineeringGroupID + `, https://example-admin.okta.com/admin/group/00g00000000000000eng), referenced by model access policy "engineering-policy"`,
		`"ENGINEERING" (okta/00g00000engineering), referenced by model access policy "duplicate-policy"`,
		"Remove the references to all but one of them",
		"rename the group in Okta",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error = %q, want it to contain %q", message, want)
		}
	}
	if conn, err := s.gateway.SCIMConnectionForAuthProvider(t.Context(), s.okta.Namespace, s.okta.Name); err != nil || conn != nil {
		t.Fatalf("a blocked Enable created connection %+v, %v", conn, err)
	}
}

func TestEnable(t *testing.T) {
	s := newEnableTest(t)

	conn, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta)
	if err != nil {
		t.Fatal(err)
	}
	if conn.Origin != types.SCIMConnectionOriginMigrated || conn.State != types.SCIMConnectionStateConnected || conn.HasToken() ||
		conn.Issuer != "https://example.okta.com" || conn.GroupIDPrefix != "okta/" {
		t.Fatalf("connection = %+v", conn)
	}
	// A retried provider configuration change finds the connection that an earlier attempt created.
	if again, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta); err != nil || again.ID != conn.ID {
		t.Fatalf("EnableConnection() again = %+v, %v", again, err)
	}

	result, err := s.service.CompleteEnable(t.Context(), s.okta.Namespace, s.okta.Name)
	if err != nil {
		t.Fatal(err)
	}
	if result.Connection.ID != conn.ID || !result.Connection.HasToken || !strings.HasPrefix(result.Connection.Token, "obot_scim_") ||
		result.Connection.BaseURL != "https://obot.example.com/scim/v2" || !result.Connection.AuthProviderConfigured || result.Connection.Origin != string(types.SCIMConnectionOriginMigrated) {
		t.Fatalf("connection = %+v", result.Connection)
	}
	if _, err := s.gateway.AuthenticateSCIMConnection(t.Context(), result.Connection.Token); err != nil {
		t.Fatalf("the token does not authenticate: %v", err)
	}

	// Only the group that nothing references is deleted, with its memberships. The referenced groups keep theirs.
	if result.DeletedGroupCount != 1 || result.DeletionError != "" {
		t.Fatalf("result = %+v", result)
	}
	ids, pending := s.groups()
	if !slices.Equal(ids, []string{engineeringGroupID, everyoneGroupID}) || len(pending) != 0 {
		t.Fatalf("groups after Enable = %v, pending deletion %v", ids, pending)
	}
	if s.membershipCount(staleGroupID) != 0 || s.membershipCount(engineeringGroupID) != 2 {
		t.Fatal("Enable changed the memberships of groups it kept, or kept those of the group it deleted")
	}

	// The token is shown once: completing Enable again is a conflict, and issues no token.
	_, err = s.service.CompleteEnable(t.Context(), s.okta.Namespace, s.okta.Name)
	var httpErr *clienttypes.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("CompleteEnable() again = %v, want a conflict", err)
	}
	if _, err := s.gateway.AuthenticateSCIMConnection(t.Context(), result.Connection.Token); err != nil {
		t.Fatalf("completing Enable again replaced the token: %v", err)
	}

	// The review lists the groups Okta must push.
	review, err := s.service.Review(t.Context(), conn.ID, Actor{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if review.UnboundReferencedGroups.Total != 2 || review.UnreferencedGroups.Total != 0 {
		t.Fatalf("review = %+v", review)
	}

	// SCIM is enabled for good: enabling it again is blocked.
	preview, err := s.service.EnablePreview(t.Context())
	if err != nil || len(preview.Blockers) != 1 || !strings.Contains(preview.Blockers[0], "already provisions") {
		t.Fatalf("preview after Enable = %+v, %v", preview, err)
	}
}

func TestEnableKeepsAGroupThatGainsAReferenceWhileItIsMarked(t *testing.T) {
	s := newEnableTest(t)

	// A writer checked the group before it was marked, and saves its reference after the marks committed.
	s.storage.beforeList = func(ctx context.Context) {
		if err := s.storage.Create(ctx, policy("racing-policy", staleGroupID)); err != nil {
			t.Error(err)
		}
	}

	result, err := s.enable()
	if err != nil {
		t.Fatal(err)
	}
	if result.DeletedGroupCount != 0 || result.DeletionError != "" {
		t.Fatalf("result = %+v, want the group that gained a reference kept", result)
	}
	ids, pending := s.groups()
	if !slices.Contains(ids, staleGroupID) || len(pending) != 0 {
		t.Fatalf("groups = %v, pending deletion %v", ids, pending)
	}
	if s.membershipCount(staleGroupID) != 2 {
		t.Fatal("the group that gained a reference lost its memberships")
	}
}

func TestEnableSucceedsWhenTheDeletionFails(t *testing.T) {
	s := newEnableTest(t)
	s.storage.listErr = errors.New("the store is unavailable")

	// The token cannot be retrieved again, so Enable succeeds, and reports the failed deletion.
	result, err := s.enable()
	if err != nil {
		t.Fatal(err)
	}
	if result.Connection.Token == "" || result.DeletedGroupCount != 0 || !strings.Contains(result.DeletionError, "Retry the deletion") {
		t.Fatalf("result = %+v", result)
	}
	ids, pending := s.groups()
	if !slices.Contains(ids, staleGroupID) || len(pending) != 0 {
		t.Fatalf("groups = %v, pending deletion %v, want the group kept and unmarked", ids, pending)
	}

	// The deletion can be retried.
	s.storage.listErr = nil
	deleted, err := s.service.DeleteUnreferencedGroups(t.Context(), result.Connection.ID)
	if err != nil || deleted.DeletedGroupCount != 1 {
		t.Fatalf("DeleteUnreferencedGroups() = %+v, %v", deleted, err)
	}
	if ids, _ := s.groups(); slices.Contains(ids, staleGroupID) {
		t.Fatal("the retried deletion kept the unreferenced group")
	}
	if deleted, err := s.service.DeleteUnreferencedGroups(t.Context(), result.Connection.ID); err != nil || deleted.DeletedGroupCount != 0 {
		t.Fatalf("DeleteUnreferencedGroups() with nothing left = %+v, %v", deleted, err)
	}

	var httpErr *clienttypes.ErrHTTP
	if _, err := s.service.DeleteUnreferencedGroups(t.Context(), "unknown"); !errors.As(err, &httpErr) || httpErr.Code != http.StatusNotFound {
		t.Fatalf("DeleteUnreferencedGroups() of an unknown connection = %v", err)
	}
}

func TestEnableConnectionDoesNotAdoptASCIMFirstConnection(t *testing.T) {
	s := newEnableTest(t)
	other, _, err := s.gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: system.DefaultNamespace,
		AuthProviderName:      "okta-auth-provider",
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	})
	if err != nil {
		t.Fatal(err)
	}

	// A SCIM-first connection of the provider is not one that Enable created, so enabling does not adopt it.
	_, err = EnableConnection(t.Context(), s.storage, s.gateway, s.okta)
	if blocked, ok := errors.AsType[*EnableBlockedError](err); !ok || !strings.Contains(blocked.Error(), "already provisions") {
		t.Fatalf("EnableConnection() = %v", err)
	}
	if conn, err := s.gateway.SCIMConnectionForAuthProvider(t.Context(), s.okta.Namespace, s.okta.Name); err != nil || conn.ID != other.ID || conn.Origin != types.SCIMConnectionOriginSCIMFirst {
		t.Fatalf("connection = %+v, %v", conn, err)
	}
}

func TestCompleteEnableRefusesAConnectionThatEnablingDidNotCreate(t *testing.T) {
	s := newEnableTest(t)
	if _, _, err := s.gateway.CreateSCIMConnection(t.Context(), gclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: s.okta.Namespace,
		AuthProviderName:      s.okta.Name,
		GroupIDPrefix:         "okta/",
		Origin:                types.SCIMConnectionOriginSCIMFirst,
	}); err != nil {
		t.Fatal(err)
	}

	_, err := s.service.CompleteEnable(t.Context(), s.okta.Namespace, s.okta.Name)
	var httpErr *clienttypes.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict {
		t.Fatalf("CompleteEnable() of a SCIM-first connection = %v, want a conflict", err)
	}
	conn, err := s.gateway.SCIMConnectionForAuthProvider(t.Context(), s.okta.Namespace, s.okta.Name)
	if err != nil || conn.HasToken() {
		t.Fatalf("connection = %+v, %v, want no token issued", conn, err)
	}
	if ids, _ := s.groups(); !slices.Contains(ids, staleGroupID) {
		t.Fatal("CompleteEnable deleted a group of a connection it refused")
	}
}

func TestCompleteEnableDeletesGroupsEvenWhenAnOwnerGeneratedTheToken(t *testing.T) {
	s := newEnableTest(t)
	conn, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta)
	if err != nil {
		t.Fatal(err)
	}
	// An Owner generates the token on the SCIM tab before Enable issues it.
	generated, err := s.service.RotateToken(t.Context(), conn.ID)
	if err != nil {
		t.Fatal(err)
	}

	_, err = s.service.CompleteEnable(t.Context(), s.okta.Namespace, s.okta.Name)
	var httpErr *clienttypes.ErrHTTP
	if !errors.As(err, &httpErr) || httpErr.Code != http.StatusConflict || !strings.Contains(httpErr.Message, "already enabled") {
		t.Fatalf("CompleteEnable() = %v, want a conflict", err)
	}
	if ids, pending := s.groups(); slices.Contains(ids, staleGroupID) || len(pending) != 0 {
		t.Fatalf("groups = %v, pending deletion %v, want the unreferenced group deleted", ids, pending)
	}
	if _, err := s.gateway.AuthenticateSCIMConnection(t.Context(), generated.Token); err != nil {
		t.Fatalf("the Owner's token no longer authenticates: %v", err)
	}
}

func TestReviewWarnsAboutNamesThatAPushCannotBindBy(t *testing.T) {
	s := newEnableTest(t)
	conn, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta)
	if err != nil {
		t.Fatal(err)
	}

	// Enabling could not delete an unreferenced group with the name of a referenced one, and two referenced groups
	// came to share a name afterwards.
	s.addGroup("okta/00g0000000namesake", "engineering")
	s.addGroup("okta/00g000000everyone2", "EVERYONE")
	if err := s.storage.Create(t.Context(), policy("second-everyone-policy", "okta/00g000000everyone2")); err != nil {
		t.Fatal(err)
	}

	review, err := s.service.Review(t.Context(), conn.ID, Actor{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var warnings []string
	for _, warning := range review.Warnings {
		if warning.Type == warningDuplicateName || warning.Type == warningNamesake {
			warnings = append(warnings, warning.Type+" "+warning.GroupName)
		}
	}
	if !slices.Equal(warnings, []string{warningDuplicateName + " EVERYONE", warningNamesake + " engineering"}) {
		t.Fatalf("name warnings = %v", warnings)
	}
}

func TestEmptyPagesListNoItems(t *testing.T) {
	page := groupPage(nil, NormalizePage(0, 10))
	if page.Items == nil || page.Total != 0 {
		t.Fatalf("page = %+v, want an empty list of items", page)
	}
}

func TestEnableIsBlockedByAnotherProvidersConnection(t *testing.T) {
	s := newEnableTest(t)
	s.otherConnection()

	preview, err := s.service.EnablePreview(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blockers) != 1 || preview.Blockers[0] != "Another auth provider, GitHub, has the installation's only SCIM connection (0b6bd0a4-7e44-4c3c-9d0b-000000000000)." {
		t.Fatalf("blockers = %q", preview.Blockers)
	}
	if _, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta); err == nil {
		t.Fatal("EnableConnection() created a second connection")
	}
}

func TestEnableConnectionRefusesAConnectionCreatedMeanwhile(t *testing.T) {
	s := newEnableTest(t)
	// Another connection appears after the plan found none, and before the connection is created.
	s.storage.lists = 1
	s.storage.beforeList = func(context.Context) {
		s.otherConnection()
	}

	_, err := EnableConnection(t.Context(), s.storage, s.gateway, s.okta)
	blocked, ok := errors.AsType[*EnableBlockedError](err)
	if !ok || !strings.Contains(blocked.Error(), "Another auth provider, GitHub, has the installation's only SCIM connection (0b6bd0a4-7e44-4c3c-9d0b-000000000000).") {
		t.Fatalf("EnableConnection() = %v, want it blocked by the other connection", err)
	}
	if conn, err := s.gateway.SCIMConnectionForAuthProvider(t.Context(), s.okta.Namespace, s.okta.Name); err != nil || conn != nil {
		t.Fatalf("connection = %+v, %v", conn, err)
	}
}

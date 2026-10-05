package cleanup

import (
	"testing"

	"github.com/obot-platform/nah/pkg/router"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaytypes "github.com/obot-platform/obot/pkg/gateway/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestAuthProviderCleanupOfAProviderWithSCIMGroupIDs checks the cleanup of an auth provider whose group ID prefix a
// SCIM connection may manage. Deconfiguring a provider deletes its own connection before its cleanup is ready, so the
// cleanup then runs as for any provider. Until then it is retried. A connection of another provider with the same
// prefix keeps everything, and the cleanup ends.
func TestAuthProviderCleanupOfAProviderWithSCIMGroupIDs(t *testing.T) {
	const (
		namespace = "default"
		provider  = "okta-auth-provider"
		groupID   = "okta/00g-team"
	)

	subjects := []clienttypes.Subject{
		{
			Type: clienttypes.SubjectTypeGroup,
			ID:   groupID,
		},
	}
	tests := []struct {
		name string
		// connectionNamespace is the namespace of the auth provider of the SCIM connection with the group ID prefix,
		// or empty for none.
		connectionNamespace string
		wantErr             bool
		wantDone            bool
		// wantSubjects are the subjects of the policies after the cleanup.
		wantSubjects []clienttypes.Subject
		// wantRows is the number of groups, memberships, and group role assignments each after the cleanup.
		wantRows int64
		// wantChanges is the number of user role changes and user group changes each that the cleanup created.
		wantChanges int
	}{
		{
			name:         "no connection",
			wantDone:     true,
			wantSubjects: nil,
			wantRows:     0,
			wantChanges:  1,
		},
		{
			name:                "the provider's own connection, which deconfiguring deletes first",
			connectionNamespace: namespace,
			wantErr:             true,
			wantSubjects:        subjects,
			wantRows:            1,
			wantChanges:         0,
		},
		{
			name:                "another provider's connection with the same prefix",
			connectionNamespace: "other",
			wantDone:            true,
			wantSubjects:        subjects,
			wantRows:            1,
			wantChanges:         0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanupTask := &v1.AuthProviderCleanup{
				Name:      "cleanup",
				Namespace: namespace,
				Spec: v1.AuthProviderCleanupSpec{
					AuthProviderName: provider,
					GroupIDPrefix:    "okta/",
					Ready:            true,
				},
			}
			accessRule := &v1.AccessControlRule{
				Name:      "access-rule",
				Namespace: namespace,
				Spec: v1.AccessControlRuleSpec{
					Manifest: clienttypes.AccessControlRuleManifest{
						Subjects: subjects,
					},
				},
			}

			modelPolicy := &v1.ModelAccessPolicy{
				Name:      "model-policy",
				Namespace: namespace,
				Spec: v1.ModelAccessPolicySpec{
					Manifest: clienttypes.ModelAccessPolicyManifest{
						Subjects: subjects,
					},
				},
			}

			baseClient := fake.NewClientBuilder().
				WithScheme(storagescheme.Scheme).
				WithObjects(cleanupTask, accessRule, modelPolicy).
				Build()
			storageClient := &generatedNameClient{WithWatch: baseClient}
			gatewayClient, gatewayDB := newAuthProviderCleanupGatewayClient(t)

			if err := gatewayDB.Create(&gatewaytypes.Identity{
				AuthProviderName:      provider,
				AuthProviderNamespace: namespace,
				HashedProviderUserID:  "user-42",
				UserID:                42,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := gatewayDB.Create(&gatewaytypes.Group{
				ID:                    groupID,
				AuthProviderName:      provider,
				AuthProviderNamespace: namespace,
				Name:                  "team",
			}).Error; err != nil {
				t.Fatal(err)
			}
			if err := gatewayDB.Create(&gatewaytypes.GroupMemberships{
				UserID:  42,
				GroupID: groupID,
			}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := gatewayClient.CreateGroupRoleAssignment(t.Context(), groupID, clienttypes.RoleAdmin, "team"); err != nil {
				t.Fatal(err)
			}
			if tt.connectionNamespace != "" {
				if _, _, err := gatewayClient.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
					AuthProviderNamespace: tt.connectionNamespace,
					AuthProviderName:      provider,
					GroupIDPrefix:         "okta/",
					Origin:                gatewaytypes.SCIMConnectionOriginMigrated,
				}); err != nil {
					t.Fatal(err)
				}
			}

			// The cleanup runs until it ends or fails.
			handler := NewAuthProviderCleanup(gatewayClient)
			var err error
			for range 5 {
				pending := &v1.AuthProviderCleanup{}
				if getErr := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(cleanupTask), pending); apierrors.IsNotFound(getErr) {
					break
				} else if getErr != nil {
					t.Fatal(getErr)
				}
				if err = handler.Cleanup(router.Request{
					Client:    storageClient,
					Object:    pending,
					Ctx:       t.Context(),
					Namespace: namespace,
					Name:      pending.Name,
				}, &router.ResponseWrapper{}); err != nil {
					break
				}
			}
			if (err != nil) != tt.wantErr {
				t.Fatalf("Cleanup() error = %v, want error %v", err, tt.wantErr)
			}
			getErr := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(cleanupTask), &v1.AuthProviderCleanup{})
			if done := apierrors.IsNotFound(getErr); done != tt.wantDone {
				t.Fatalf("cleanup task lookup after the cleanup = %v, want it done %v", getErr, tt.wantDone)
			}

			gotAccessRule := &v1.AccessControlRule{}
			mustGet(t, storageClient, accessRule, gotAccessRule)
			assertSubjects(t, gotAccessRule.Spec.Manifest.Subjects, tt.wantSubjects)
			gotModelPolicy := &v1.ModelAccessPolicy{}
			mustGet(t, storageClient, modelPolicy, gotModelPolicy)
			assertSubjects(t, gotModelPolicy.Spec.Manifest.Subjects, tt.wantSubjects)
			for _, model := range []any{new(gatewaytypes.Group), new(gatewaytypes.GroupMemberships), new(gatewaytypes.GroupRoleAssignment)} {
				var n int64
				if err := gatewayDB.Model(model).Count(&n).Error; err != nil {
					t.Fatal(err)
				}
				if n != tt.wantRows {
					t.Fatalf("got %d rows of %T after the cleanup, want %d", n, model, tt.wantRows)
				}
			}

			var roleChanges v1.UserRoleChangeList
			if err := storageClient.List(t.Context(), &roleChanges); err != nil {
				t.Fatal(err)
			}
			var groupChanges v1.UserGroupChangeList
			if err := storageClient.List(t.Context(), &groupChanges); err != nil {
				t.Fatal(err)
			}
			if len(roleChanges.Items) != tt.wantChanges || len(groupChanges.Items) != tt.wantChanges {
				t.Fatalf("cleanup recomputed %d users' roles and %d users' groups, want %d", len(roleChanges.Items), len(groupChanges.Items), tt.wantChanges)
			}
		})
	}
}

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

// TestAuthProviderCleanupKeepsSCIMData checks that deconfiguring an auth provider that a SCIM connection manages
// removes nothing: no policy subject, group, membership, or group role assignment, and recomputes no user's roles or
// groups, so that configuring the provider again resumes SCIM with current data.
func TestAuthProviderCleanupKeepsSCIMData(t *testing.T) {
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
	if _, _, err := gatewayClient.CreateSCIMConnection(t.Context(), gatewayclient.CreateSCIMConnectionOptions{
		AuthProviderNamespace: namespace,
		AuthProviderName:      provider,
		GroupIDPrefix:         "okta/",
		Origin:                gatewaytypes.SCIMConnectionOriginMigrated,
	}); err != nil {
		t.Fatal(err)
	}

	handler := NewAuthProviderCleanup(gatewayClient)
	if err := handler.Cleanup(router.Request{
		Client:    storageClient,
		Object:    cleanupTask,
		Ctx:       t.Context(),
		Namespace: namespace,
		Name:      cleanupTask.Name,
	}, &router.ResponseWrapper{}); err != nil {
		t.Fatal(err)
	}

	// The cleanup completes at once.
	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(cleanupTask), &v1.AuthProviderCleanup{}); !apierrors.IsNotFound(err) {
		t.Fatalf("cleanup task still exists: %v", err)
	}

	gotAccessRule := &v1.AccessControlRule{}
	mustGet(t, storageClient, accessRule, gotAccessRule)
	assertSubjects(t, gotAccessRule.Spec.Manifest.Subjects, subjects)
	gotModelPolicy := &v1.ModelAccessPolicy{}
	mustGet(t, storageClient, modelPolicy, gotModelPolicy)
	assertSubjects(t, gotModelPolicy.Spec.Manifest.Subjects, subjects)

	for _, model := range []any{new(gatewaytypes.Group), new(gatewaytypes.GroupMemberships), new(gatewaytypes.GroupRoleAssignment)} {
		var n int64
		if err := gatewayDB.Model(model).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("got %d rows of %T after cleanup, want the 1 seeded", n, model)
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
	if len(roleChanges.Items) != 0 || len(groupChanges.Items) != 0 {
		t.Fatalf("cleanup recomputed %d users' roles and %d users' groups, want none", len(roleChanges.Items), len(groupChanges.Items))
	}
}

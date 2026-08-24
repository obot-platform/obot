package cleanup

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	clienttypes "github.com/obot-platform/obot/apiclient/types"
	gatewayclient "github.com/obot-platform/obot/pkg/gateway/client"
	gatewaydb "github.com/obot-platform/obot/pkg/gateway/db"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	storagescheme "github.com/obot-platform/obot/pkg/storage/scheme"
	storageservices "github.com/obot-platform/obot/pkg/storage/services"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type generatedNameClient struct {
	kclient.WithWatch
	next int
}

func TestRemoveGroupSubjects(t *testing.T) {
	tests := []struct {
		name          string
		subjects      []clienttypes.Subject
		groupIDPrefix string
		want          []clienttypes.Subject
		wantChange    bool
	}{
		{
			name: "removes matching groups and preserves order",
			subjects: []clienttypes.Subject{
				{
					Type: clienttypes.SubjectTypeUser,
					ID:   "1",
				},
				{
					Type: clienttypes.SubjectTypeGroup,
					ID:   "entra/engineering",
				},
				{
					Type: clienttypes.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
			},
			groupIDPrefix: "entra/",
			want: []clienttypes.Subject{
				{
					Type: clienttypes.SubjectTypeUser,
					ID:   "1",
				},
				{
					Type: clienttypes.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
			},
			wantChange: true,
		},
		{
			name: "keeps an empty policy after removing its only subject",
			subjects: []clienttypes.Subject{
				{
					Type: clienttypes.SubjectTypeGroup,
					ID:   "entra/engineering",
				},
			},
			groupIDPrefix: "entra/",
			want:          []clienttypes.Subject{},
			wantChange:    true,
		},
		{
			name: "does not change unrelated subjects",
			subjects: []clienttypes.Subject{
				{
					Type: clienttypes.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
			},
			groupIDPrefix: "entra/",
			want: []clienttypes.Subject{
				{
					Type: clienttypes.SubjectTypeGroup,
					ID:   "okta/engineering",
				},
			},
			wantChange: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := removeGroupSubjects(tt.subjects, tt.groupIDPrefix)
			if changed != tt.wantChange {
				t.Fatalf("changed = %v, want %v", changed, tt.wantChange)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("subjects = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestAuthProviderCleanupCleansAllGroupReferences(t *testing.T) {
	const (
		targetGroup    = "entra/engineering"
		otherGroup     = "okta/engineering"
		lookalikeGroup = "entra-other/engineering"
		namespace      = "default"
	)

	checkpoint := `{"userIDs":[42]}`
	cleanupTask := &v1.AuthProviderCleanup{
		Name:      "cleanup",
		Namespace: namespace,
		Annotations: map[string]string{
			authProviderCleanupCheckpointAnnotation: checkpoint,
		},
		Spec: v1.AuthProviderCleanupSpec{
			AuthProviderName: "entra-auth-provider",
		},
	}

	mixedSubjects := []clienttypes.Subject{
		{
			Type: clienttypes.SubjectTypeGroup,
			ID:   targetGroup,
		},
		{
			Type: clienttypes.SubjectTypeUser,
			ID:   "7",
		},
		{
			Type: clienttypes.SubjectTypeGroup,
			ID:   otherGroup,
		},
		{
			Type: clienttypes.SubjectTypeGroup,
			ID:   lookalikeGroup,
		},
	}
	wantSubjects := mixedSubjects[1:]

	accessRule := &v1.AccessControlRule{
		Name:      "access-rule",
		Namespace: namespace,
		Spec: v1.AccessControlRuleSpec{
			Manifest: clienttypes.AccessControlRuleManifest{
				Subjects: mixedSubjects,
			},
		},
	}
	modelPolicy := &v1.ModelAccessPolicy{
		Name:      "model-policy",
		Namespace: namespace,
		Spec: v1.ModelAccessPolicySpec{
			Manifest: clienttypes.ModelAccessPolicyManifest{
				Subjects: []clienttypes.Subject{
					{
						Type: clienttypes.SubjectTypeGroup,
						ID:   targetGroup,
					},
				},
			},
		},
	}
	skillRule := &v1.SkillAccessRule{
		Name:      "skill-rule",
		Namespace: namespace,
		Spec: v1.SkillAccessRuleSpec{
			Manifest: clienttypes.SkillAccessRuleManifest{
				Subjects: mixedSubjects,
			},
		},
	}
	messagePolicy := &v1.MessagePolicy{
		Name:      "message-policy",
		Namespace: namespace,
		Spec: v1.MessagePolicySpec{
			Manifest: clienttypes.MessagePolicyManifest{
				Subjects: mixedSubjects,
			},
		},
	}
	hostedAgentRule := &v1.HostedAgentAccessRule{
		Name:      "hosted-agent-rule",
		Namespace: namespace,
		Spec: v1.HostedAgentAccessRuleSpec{
			Manifest: clienttypes.HostedAgentAccessRuleManifest{
				Subjects: mixedSubjects,
			},
		},
	}
	publishedArtifact := &v1.PublishedArtifact{
		Name:      "artifact",
		Namespace: namespace,
		Status: v1.PublishedArtifactStatus{
			Versions: []clienttypes.PublishedArtifactVersionEntry{
				{
					Version:  1,
					Subjects: mixedSubjects,
				},
				{
					Version: 2,
					Subjects: []clienttypes.Subject{
						{
							Type: clienttypes.SubjectTypeGroup,
							ID:   targetGroup,
						},
					},
				},
			},
		},
	}

	baseClient := fake.NewClientBuilder().
		WithScheme(storagescheme.Scheme).
		WithStatusSubresource(&v1.PublishedArtifact{}).
		WithObjects(cleanupTask, accessRule, modelPolicy, skillRule, messagePolicy, hostedAgentRule, publishedArtifact).
		Build()
	storageClient := &generatedNameClient{WithWatch: baseClient}
	gatewayClient := newAuthProviderCleanupGatewayClient(t)
	if _, err := gatewayClient.CreateGroupRoleAssignment(t.Context(), targetGroup, clienttypes.RoleAdmin, "target"); err != nil {
		t.Fatal(err)
	}
	if _, err := gatewayClient.CreateGroupRoleAssignment(t.Context(), otherGroup, clienttypes.RolePowerUser, "other"); err != nil {
		t.Fatal(err)
	}

	handler := NewAuthProviderCleanup(gatewayClient)
	req := router.Request{
		Client:    storageClient,
		Object:    cleanupTask,
		Ctx:       t.Context(),
		Namespace: namespace,
		Name:      cleanupTask.Name,
	}
	if err := handler.Cleanup(req, &router.ResponseWrapper{}); err != nil {
		t.Fatal(err)
	}

	if err := storageClient.Get(t.Context(), kclient.ObjectKeyFromObject(cleanupTask), &v1.AuthProviderCleanup{}); !apierrors.IsNotFound(err) {
		t.Fatalf("cleanup task still exists: %v", err)
	}
	if _, err := gatewayClient.GetGroupRoleAssignment(t.Context(), targetGroup); !errors.Is(err, gatewayclient.ErrGroupRoleAssignmentNotFound) {
		t.Fatalf("target group role assignment lookup error = %v", err)
	}
	if _, err := gatewayClient.GetGroupRoleAssignment(t.Context(), otherGroup); err != nil {
		t.Fatalf("other provider assignment was removed: %v", err)
	}

	gotAccessRule := &v1.AccessControlRule{}
	mustGet(t, storageClient, accessRule, gotAccessRule)
	assertSubjects(t, gotAccessRule.Spec.Manifest.Subjects, wantSubjects)
	gotModelPolicy := &v1.ModelAccessPolicy{}
	mustGet(t, storageClient, modelPolicy, gotModelPolicy)
	assertSubjects(t, gotModelPolicy.Spec.Manifest.Subjects, []clienttypes.Subject{})
	gotSkillRule := &v1.SkillAccessRule{}
	mustGet(t, storageClient, skillRule, gotSkillRule)
	assertSubjects(t, gotSkillRule.Spec.Manifest.Subjects, wantSubjects)
	gotMessagePolicy := &v1.MessagePolicy{}
	mustGet(t, storageClient, messagePolicy, gotMessagePolicy)
	assertSubjects(t, gotMessagePolicy.Spec.Manifest.Subjects, wantSubjects)
	gotHostedAgentRule := &v1.HostedAgentAccessRule{}
	mustGet(t, storageClient, hostedAgentRule, gotHostedAgentRule)
	assertSubjects(t, gotHostedAgentRule.Spec.Manifest.Subjects, wantSubjects)
	gotArtifact := &v1.PublishedArtifact{}
	mustGet(t, storageClient, publishedArtifact, gotArtifact)
	assertSubjects(t, gotArtifact.Status.Versions[0].Subjects, wantSubjects)
	assertSubjects(t, gotArtifact.Status.Versions[1].Subjects, []clienttypes.Subject{})

	var roleChanges v1.UserRoleChangeList
	if err := storageClient.List(t.Context(), &roleChanges); err != nil {
		t.Fatal(err)
	}
	if len(roleChanges.Items) != 1 || roleChanges.Items[0].Spec.UserID != 42 {
		t.Fatalf("user role changes = %#v, want one for user 42", roleChanges.Items)
	}
	var groupChanges v1.UserGroupChangeList
	if err := storageClient.List(t.Context(), &groupChanges); err != nil {
		t.Fatal(err)
	}
	if len(groupChanges.Items) != 1 || groupChanges.Items[0].Spec.UserID != 42 {
		t.Fatalf("user group changes = %#v, want one for user 42", groupChanges.Items)
	}
}

func TestAuthProviderCleanupCheckpointsBeforeDeleting(t *testing.T) {
	task := &v1.AuthProviderCleanup{
		Name:      "cleanup",
		Namespace: "default",
		Spec: v1.AuthProviderCleanupSpec{
			AuthProviderName: "entra-auth-provider",
		},
	}
	storageClient := fake.NewClientBuilder().WithScheme(storagescheme.Scheme).WithObjects(task).Build()
	handler := NewAuthProviderCleanup(newAuthProviderCleanupGatewayClient(t))
	req := router.Request{
		Client:    storageClient,
		Object:    task,
		Ctx:       t.Context(),
		Namespace: task.Namespace,
		Name:      task.Name,
	}

	if err := handler.Cleanup(req, &router.ResponseWrapper{}); err != nil {
		t.Fatal(err)
	}
	got := &v1.AuthProviderCleanup{}
	mustGet(t, storageClient, task, got)
	checkpoint, found, err := authProviderCheckpoint(got)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("cleanup checkpoint was not saved")
	}
	if len(checkpoint.UserIDs) != 0 {
		t.Fatalf("checkpoint = %#v, want empty user IDs", checkpoint)
	}
}

func (c *generatedNameClient) Create(ctx context.Context, obj kclient.Object, opts ...kclient.CreateOption) error {
	if obj.GetName() == "" && obj.GetGenerateName() != "" {
		c.next++
		obj.SetName(fmt.Sprintf("%s%d", obj.GetGenerateName(), c.next))
	}
	return c.WithWatch.Create(ctx, obj, opts...)
}

func newAuthProviderCleanupGatewayClient(t *testing.T) *gatewayclient.Client {
	t.Helper()
	storageServices, err := storageservices.New(storageservices.Config{DSN: "sqlite://:memory:"})
	if err != nil {
		t.Fatal(err)
	}
	database, err := gatewaydb.New(storageServices.DB.DB, storageServices.DB.SQLDB, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	client := gatewayclient.New(t.Context(), database, nil, nil, nil, nil, nil, time.Hour, 10, 0, 0, 0, false)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("close gateway client: %v", err)
		}
	})
	return client
}

func mustGet(t *testing.T, client kclient.Client, keyObject kclient.Object, result kclient.Object) {
	t.Helper()
	if err := client.Get(t.Context(), kclient.ObjectKeyFromObject(keyObject), result); err != nil {
		t.Fatal(err)
	}
}

func assertSubjects(t *testing.T, got, want []clienttypes.Subject) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("subjects = %#v, want %#v", got, want)
	}
}

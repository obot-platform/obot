package cleanup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authProviderCleanupCheckpointAnnotation = "obot.obot.ai/auth-provider-cleanup-checkpoint"
	authProviderCleanupBatchSize            = 500
)

type authProviderCleanupCheckpoint struct {
	DataDeleted bool `json:"dataDeleted,omitempty"`
	LastUserID  uint `json:"lastUserID,omitempty"`
}

type AuthProviderCleanup struct {
	gatewayClient *gclient.Client
}

func NewAuthProviderCleanup(gatewayClient *gclient.Client) *AuthProviderCleanup {
	return &AuthProviderCleanup{gatewayClient: gatewayClient}
}

func (a *AuthProviderCleanup) Cleanup(req router.Request, resp router.Response) error {
	cleanup := req.Object.(*v1.AuthProviderCleanup)
	providerName := cleanup.Spec.AuthProviderName
	providerNamespace := cleanup.Namespace
	if providerName == "" {
		return fmt.Errorf("auth provider cleanup %s has no auth provider name", cleanup.Name)
	}
	if !cleanup.Spec.Ready {
		resp.RetryAfter(time.Second)
		return nil
	}
	groupIDPrefix := cleanup.Spec.GroupIDPrefix
	if groupIDPrefix == "" {
		return fmt.Errorf("auth provider cleanup %s has no group ID prefix", cleanup.Name)
	}
	if err := auth.ValidateGroupIDPrefix(groupIDPrefix); err != nil {
		return fmt.Errorf("auth provider cleanup %s has invalid group ID prefix: %w", cleanup.Name, err)
	}

	checkpoint, err := authProviderCheckpoint(cleanup)
	if err != nil {
		return err
	}
	if !checkpoint.DataDeleted {
		counts := make(map[string]int, 6)
		if counts["accessControlRules"], err = cleanupAccessControlRuleGroups(req, groupIDPrefix); err != nil {
			return err
		}
		if counts["modelAccessPolicies"], err = cleanupModelAccessPolicyGroups(req, groupIDPrefix); err != nil {
			return err
		}
		if counts["skillAccessRules"], err = cleanupSkillAccessRuleGroups(req, groupIDPrefix); err != nil {
			return err
		}
		if counts["messagePolicies"], err = cleanupMessagePolicyGroups(req, groupIDPrefix); err != nil {
			return err
		}
		if counts["hostedAgentAccessRules"], err = cleanupHostedAgentAccessRuleGroups(req, groupIDPrefix); err != nil {
			return err
		}
		if counts["publishedArtifacts"], err = cleanupPublishedArtifactGroups(req, groupIDPrefix); err != nil {
			return err
		}

		if err := a.gatewayClient.DeleteAuthProviderGroupData(req.Ctx, providerNamespace, providerName, groupIDPrefix); err != nil {
			return err
		}

		checkpoint.DataDeleted = true
		if err := saveAuthProviderCheckpoint(req, cleanup, checkpoint); err != nil {
			return err
		}
		slog.Info("Deleted auth provider group data", "authProvider", providerName, "namespace", providerNamespace, "groupIDPrefix", groupIDPrefix, "updatedResources", counts)
		return nil
	}

	userIDs, err := a.gatewayClient.GetAuthProviderGroupCleanupUserIDs(req.Ctx, providerNamespace, providerName, checkpoint.LastUserID, authProviderCleanupBatchSize)
	if err != nil {
		return err
	}
	for _, userID := range userIDs {
		if err := req.Client.Create(req.Ctx, &v1.UserRoleChange{
			GenerateName: system.UserRoleChangePrefix,
			Namespace:    req.Namespace,
			Spec: v1.UserRoleChangeSpec{
				UserID: userID,
			},
		}); err != nil {
			return fmt.Errorf("create user role change for user %d: %w", userID, err)
		}
		if err := req.Client.Create(req.Ctx, &v1.UserGroupChange{
			GenerateName: system.UserGroupChangePrefix,
			Namespace:    req.Namespace,
			Spec: v1.UserGroupChangeSpec{
				UserID: userID,
			},
		}); err != nil {
			return fmt.Errorf("create user group change for user %d: %w", userID, err)
		}
	}

	if len(userIDs) > 0 {
		checkpoint.LastUserID = userIDs[len(userIDs)-1]
		if err := saveAuthProviderCheckpoint(req, cleanup, checkpoint); err != nil {
			return err
		}
		slog.Info("Processed auth provider cleanup user batch", "authProvider", providerName, "namespace", providerNamespace, "groupIDPrefix", groupIDPrefix, "users", len(userIDs), "lastUserID", checkpoint.LastUserID)
		return nil
	}

	slog.Info("Completed auth provider group cleanup", "authProvider", providerName, "namespace", providerNamespace, "groupIDPrefix", groupIDPrefix, "lastUserID", checkpoint.LastUserID)
	return req.Delete(cleanup)
}

func saveAuthProviderCheckpoint(req router.Request, cleanup *v1.AuthProviderCleanup, checkpoint authProviderCleanupCheckpoint) error {
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("marshal auth provider cleanup checkpoint: %w", err)
	}
	if cleanup.Annotations == nil {
		cleanup.Annotations = make(map[string]string, 1)
	}
	cleanup.Annotations[authProviderCleanupCheckpointAnnotation] = string(data)
	if err := req.Client.Update(req.Ctx, cleanup); err != nil {
		return fmt.Errorf("save auth provider cleanup checkpoint: %w", err)
	}
	return nil
}

func authProviderCheckpoint(cleanup *v1.AuthProviderCleanup) (authProviderCleanupCheckpoint, error) {
	value := cleanup.Annotations[authProviderCleanupCheckpointAnnotation]
	if value == "" {
		return authProviderCleanupCheckpoint{}, nil
	}

	var checkpoint authProviderCleanupCheckpoint
	if err := json.Unmarshal([]byte(value), &checkpoint); err != nil {
		return authProviderCleanupCheckpoint{}, fmt.Errorf("parse auth provider cleanup checkpoint: %w", err)
	}
	return checkpoint, nil
}

func removeGroupSubjects(subjects []types.Subject, groupIDPrefix string) ([]types.Subject, bool) {
	result := make([]types.Subject, 0, len(subjects))
	changed := false
	for _, subject := range subjects {
		if subject.Type == types.SubjectTypeGroup && strings.HasPrefix(subject.ID, groupIDPrefix) {
			changed = true
			continue
		}
		result = append(result, subject)
	}
	if !changed {
		return subjects, false
	}
	return result, true
}

func cleanupAccessControlRuleGroups(req router.Request, groupIDPrefix string) (int, error) {
	var list v1.AccessControlRuleList
	if err := req.Client.List(req.Ctx, &list, &kclient.ListOptions{Namespace: req.Namespace}); err != nil {
		return 0, fmt.Errorf("list access control rules for auth provider cleanup: %w", err)
	}
	updated := 0
	for i := range list.Items {
		subjects, changed := removeGroupSubjects(list.Items[i].Spec.Manifest.Subjects, groupIDPrefix)
		if !changed {
			continue
		}
		list.Items[i].Spec.Manifest.Subjects = subjects
		if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
			return updated, fmt.Errorf("update access control rule %s for auth provider cleanup: %w", list.Items[i].Name, err)
		}
		updated++
	}
	return updated, nil
}

func cleanupModelAccessPolicyGroups(req router.Request, groupIDPrefix string) (int, error) {
	var list v1.ModelAccessPolicyList
	if err := req.Client.List(req.Ctx, &list, &kclient.ListOptions{Namespace: req.Namespace}); err != nil {
		return 0, fmt.Errorf("list model access policies for auth provider cleanup: %w", err)
	}
	updated := 0
	for i := range list.Items {
		subjects, changed := removeGroupSubjects(list.Items[i].Spec.Manifest.Subjects, groupIDPrefix)
		if !changed {
			continue
		}
		list.Items[i].Spec.Manifest.Subjects = subjects
		if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
			return updated, fmt.Errorf("update model access policy %s for auth provider cleanup: %w", list.Items[i].Name, err)
		}
		updated++
	}
	return updated, nil
}

func cleanupSkillAccessRuleGroups(req router.Request, groupIDPrefix string) (int, error) {
	var list v1.SkillAccessRuleList
	if err := req.Client.List(req.Ctx, &list, &kclient.ListOptions{Namespace: req.Namespace}); err != nil {
		return 0, fmt.Errorf("list skill access rules for auth provider cleanup: %w", err)
	}
	updated := 0
	for i := range list.Items {
		subjects, changed := removeGroupSubjects(list.Items[i].Spec.Manifest.Subjects, groupIDPrefix)
		if !changed {
			continue
		}
		list.Items[i].Spec.Manifest.Subjects = subjects
		if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
			return updated, fmt.Errorf("update skill access rule %s for auth provider cleanup: %w", list.Items[i].Name, err)
		}
		updated++
	}
	return updated, nil
}

func cleanupMessagePolicyGroups(req router.Request, groupIDPrefix string) (int, error) {
	var list v1.MessagePolicyList
	if err := req.Client.List(req.Ctx, &list, &kclient.ListOptions{Namespace: req.Namespace}); err != nil {
		return 0, fmt.Errorf("list message policies for auth provider cleanup: %w", err)
	}
	updated := 0
	for i := range list.Items {
		subjects, changed := removeGroupSubjects(list.Items[i].Spec.Manifest.Subjects, groupIDPrefix)
		if !changed {
			continue
		}
		list.Items[i].Spec.Manifest.Subjects = subjects
		if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
			return updated, fmt.Errorf("update message policy %s for auth provider cleanup: %w", list.Items[i].Name, err)
		}
		updated++
	}
	return updated, nil
}

func cleanupHostedAgentAccessRuleGroups(req router.Request, groupIDPrefix string) (int, error) {
	var list v1.HostedAgentAccessRuleList
	if err := req.Client.List(req.Ctx, &list, &kclient.ListOptions{Namespace: req.Namespace}); err != nil {
		return 0, fmt.Errorf("list hosted agent access rules for auth provider cleanup: %w", err)
	}
	updated := 0
	for i := range list.Items {
		subjects, changed := removeGroupSubjects(list.Items[i].Spec.Manifest.Subjects, groupIDPrefix)
		if !changed {
			continue
		}
		list.Items[i].Spec.Manifest.Subjects = subjects
		if err := req.Client.Update(req.Ctx, &list.Items[i]); err != nil {
			return updated, fmt.Errorf("update hosted agent access rule %s for auth provider cleanup: %w", list.Items[i].Name, err)
		}
		updated++
	}
	return updated, nil
}

func cleanupPublishedArtifactGroups(req router.Request, groupIDPrefix string) (int, error) {
	var list v1.PublishedArtifactList
	if err := req.Client.List(req.Ctx, &list, &kclient.ListOptions{Namespace: req.Namespace}); err != nil {
		return 0, fmt.Errorf("list published artifacts for auth provider cleanup: %w", err)
	}
	updated := 0
	for i := range list.Items {
		changed := false
		for j := range list.Items[i].Status.Versions {
			subjects, versionChanged := removeGroupSubjects(list.Items[i].Status.Versions[j].Subjects, groupIDPrefix)
			if versionChanged {
				list.Items[i].Status.Versions[j].Subjects = subjects
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := req.Client.Status().Update(req.Ctx, &list.Items[i]); err != nil {
			return updated, fmt.Errorf("update published artifact %s for auth provider cleanup: %w", list.Items[i].Name, err)
		}
		updated++
	}
	return updated, nil
}

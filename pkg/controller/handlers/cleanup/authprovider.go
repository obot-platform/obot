package cleanup

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/auth"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	authProviderCleanupCheckpointAnnotation = "obot.obot.ai/auth-provider-cleanup-checkpoint"
)

type authProviderCleanupCheckpoint struct {
	UserIDs []uint `json:"userIDs"`
}

type AuthProviderCleanup struct {
	gatewayClient *gclient.Client
}

func NewAuthProviderCleanup(gatewayClient *gclient.Client) *AuthProviderCleanup {
	return &AuthProviderCleanup{gatewayClient: gatewayClient}
}

func (a *AuthProviderCleanup) Cleanup(req router.Request, _ router.Response) error {
	cleanup := req.Object.(*v1.AuthProviderCleanup)
	providerName := cleanup.Spec.AuthProviderName
	providerNamespace := cleanup.Namespace
	if providerName == "" {
		return fmt.Errorf("auth provider cleanup %s has no auth provider name", cleanup.Name)
	}
	var provider v1.AuthProvider
	if err := req.Client.Get(req.Ctx, kclient.ObjectKey{Namespace: providerNamespace, Name: providerName}, &provider); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("get auth provider for cleanup generation check: %w", err)
		}
		slog.Info("Discarding auth provider cleanup because the provider no longer exists", "authProvider", providerName, "namespace", providerNamespace, "deconfigurationGeneration", cleanup.Spec.DeconfigurationGeneration)
		return req.Delete(cleanup)
	}
	if provider.Generation != cleanup.Spec.DeconfigurationGeneration {
		slog.Info("Discarding stale auth provider cleanup", "authProvider", providerName, "namespace", providerNamespace, "deconfigurationGeneration", cleanup.Spec.DeconfigurationGeneration, "currentGeneration", provider.Generation)
		return req.Delete(cleanup)
	}
	groupIDPrefix, err := auth.GroupIDPrefixForAuthProvider(providerName)
	if err != nil {
		return err
	}

	checkpoint, found, err := authProviderCheckpoint(cleanup)
	if err != nil {
		return err
	}
	if !found {
		userIDs, err := a.gatewayClient.GetAuthProviderGroupCleanupUserIDs(req.Ctx, groupIDPrefix)
		if err != nil {
			return err
		}
		checkpoint = authProviderCleanupCheckpoint{
			UserIDs: userIDs,
		}
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
		slog.Info("Checkpointed auth provider group cleanup", "authProvider", providerName, "namespace", providerNamespace, "groupIDPrefix", groupIDPrefix, "users", len(userIDs))
		return nil
	}

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

	for _, userID := range checkpoint.UserIDs {
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

	slog.Info("Completed auth provider group cleanup", "authProvider", providerName, "namespace", providerNamespace, "groupIDPrefix", groupIDPrefix, "users", len(checkpoint.UserIDs), "updatedResources", counts)
	return req.Delete(cleanup)
}

func authProviderCheckpoint(cleanup *v1.AuthProviderCleanup) (authProviderCleanupCheckpoint, bool, error) {
	value := cleanup.Annotations[authProviderCleanupCheckpointAnnotation]
	if value == "" {
		return authProviderCleanupCheckpoint{}, false, nil
	}

	var checkpoint authProviderCleanupCheckpoint
	if err := json.Unmarshal([]byte(value), &checkpoint); err != nil {
		return authProviderCleanupCheckpoint{}, false, fmt.Errorf("parse auth provider cleanup checkpoint: %w", err)
	}
	return checkpoint, true, nil
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

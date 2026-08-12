package cleanup

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	hostedAgentPoolCleanupAnnotation = "obot.obot.ai/user-delete-hosted-agent-pools"
)

type UserCleanup struct {
	gatewayClient *gclient.Client
	acrHelper     *accesscontrolrule.Helper
}

func NewUserCleanup(gatewayClient *gclient.Client, acrHelper *accesscontrolrule.Helper) *UserCleanup {
	return &UserCleanup{
		gatewayClient: gatewayClient,
		acrHelper:     acrHelper,
	}
}

func (u *UserCleanup) Cleanup(req router.Request, _ router.Response) error {
	userDelete := req.Object.(*v1.UserDelete)
	userID := strconv.FormatUint(uint64(userDelete.Spec.UserID), 10)
	log.Infof("Starting user cleanup: userID=%s", userID)

	complete, err := cleanupHostedAgents(req, userDelete, userID)
	if err != nil {
		return err
	}
	if !complete {
		return nil
	}

	// Delete identities first so that the user can login again.
	identities, err := u.gatewayClient.FindIdentitiesForUser(req.Ctx, userDelete.Spec.UserID)
	if err != nil {
		return err
	}

	if err = u.gatewayClient.DeleteSessionsForUser(req.Ctx, req.Client, identities, "", ""); err != nil {
		if !errors.Is(err, gclient.LogoutAllErr{}) {
			return err
		}
	}

	for _, identity := range identities {
		if err := u.gatewayClient.RemoveIdentity(req.Ctx, &identity); err != nil {
			return err
		}
	}
	log.Infof("Removed user identities during cleanup: userID=%s identities=%d", userID, len(identities))

	var projects v1.ProjectList
	if err := req.List(&projects, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	for _, project := range projects.Items {
		if err := req.Delete(&project); err != nil {
			return err
		}
	}
	log.Infof("Deleted projects during user cleanup: userID=%s projects=%d", userID, len(projects.Items))

	// Revoke any API keys the user created. Nanobot-agent keys are handled by the
	// NanobotAgent delete flow above; this sweeps user-created keys plus anything
	// the nanobot path missed.
	apiKeys, err := u.gatewayClient.ListAPIKeys(req.Ctx, userDelete.Spec.UserID)
	if err != nil {
		return fmt.Errorf("failed to list API keys for user %d: %w", userDelete.Spec.UserID, err)
	}
	for _, key := range apiKeys {
		if err := u.gatewayClient.RevokeAPIKey(req.Ctx, userDelete.Spec.UserID, key.ID); err != nil {
			return fmt.Errorf("failed to revoke API key %d for user %d: %w", key.ID, userDelete.Spec.UserID, err)
		}
	}
	log.Infof("Revoked API keys during user cleanup: userID=%s keys=%d", userID, len(apiKeys))

	var servers v1.MCPServerList
	if err := req.List(&servers, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	var deletedServers int
	for _, server := range servers.Items {
		// Skip multi-user servers in the default MCPCatalog — they should persist after user deletion.
		// Also skip servers that are associated with an agent because we need the credential to stick
		// around so we can revoke the API key.
		if server.Spec.MCPCatalogID == system.DefaultCatalog || server.Spec.NanobotAgentID != "" {
			continue
		}
		if err := req.Delete(&server); err != nil {
			return err
		}
		deletedServers++
	}
	log.Infof("Deleted MCP servers during user cleanup: userID=%s servers=%d (skipped=%d)", userID, deletedServers, len(servers.Items)-deletedServers)

	// DeleteRefs should handle cleaning up most of the user's MCPServerInstances.
	// But there still might be MCPServerInstances pointing to multi-user servers that we need to delete.
	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	for _, instance := range instances.Items {
		if err := req.Delete(&instance); err != nil {
			return err
		}
	}
	log.Infof("Deleted MCP server instances during user cleanup: userID=%s instances=%d", userID, len(instances.Items))

	// Find the AccessControlRules that the user is on, and update them to remove the user.
	acrs, err := u.acrHelper.GetAccessControlRulesForUser(req.Namespace, userID)
	if err != nil {
		return err
	}

	var updatedACRs int
	for _, acr := range acrs {
		newSubjects := slices.Collect(func(yield func(types.Subject) bool) {
			for _, subject := range acr.Spec.Manifest.Subjects {
				if subject.ID != userID {
					if !yield(subject) {
						return
					}
				}
			}
		})
		acr.Spec.Manifest.Subjects = newSubjects
		if err := req.Client.Update(req.Ctx, &acr); err != nil {
			return err
		}
		updatedACRs++
	}
	log.Infof("Updated access control rules during user cleanup: userID=%s rules=%d", userID, updatedACRs)

	// Delete the user's PowerUserWorkspace if it exists
	var workspaces v1.PowerUserWorkspaceList
	if err := req.List(&workspaces, &kclient.ListOptions{
		Namespace: system.DefaultNamespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return err
	}

	for _, workspace := range workspaces.Items {
		if err := req.Delete(&workspace); err != nil {
			return err
		}
	}
	log.Infof("Deleted power user workspaces during user cleanup: userID=%s workspaces=%d", userID, len(workspaces.Items))

	// If everything is cleaned up successfully, then delete this object because we don't need it.
	log.Infof("Completed user cleanup: userID=%s", userID)
	return req.Delete(userDelete)
}

// cleanupHostedAgents orders deletion so backend finalizers have all of the
// references they need: instances, assignments, then exclusively owned
// pools. Pool IDs are checkpointed on UserDelete metadata before
// assignments are removed, allowing cleanup to wait across reconciliations
// until asynchronous pool deletion has actually completed.
func cleanupHostedAgents(req router.Request, userDelete *v1.UserDelete, userID string) (bool, error) {
	var instances v1.HostedAgentInstanceList
	if err := req.List(&instances, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return false, err
	}
	if len(instances.Items) > 0 {
		for i := range instances.Items {
			if err := req.Delete(&instances.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
		}
		log.Infof("Waiting for hosted agent instance cleanup: userID=%s instances=%d", userID, len(instances.Items))
		return false, nil
	}

	poolIDs, err := savedPoolIDs(userDelete)
	if err != nil {
		return false, err
	}
	var assignments v1.HostedAgentPoolAssignmentList
	if err := req.List(&assignments, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": userID,
		}),
	}); err != nil {
		return false, err
	}

	changed := false
	for _, assignment := range assignments.Items {
		if !slices.Contains(poolIDs, assignment.Spec.Manifest.PoolID) {
			poolIDs = append(poolIDs, assignment.Spec.Manifest.PoolID)
			changed = true
		}
	}
	if changed {
		sort.Strings(poolIDs)
		data, err := json.Marshal(poolIDs)
		if err != nil {
			return false, fmt.Errorf("marshal hosted agent pool cleanup checkpoint: %w", err)
		}
		if userDelete.Annotations == nil {
			userDelete.Annotations = map[string]string{}
		}
		userDelete.Annotations[hostedAgentPoolCleanupAnnotation] = string(data)
		if err := req.Client.Update(req.Ctx, userDelete); err != nil {
			return false, err
		}
		return false, nil
	}

	if len(assignments.Items) > 0 {
		for i := range assignments.Items {
			if err := req.Delete(&assignments.Items[i]); err != nil && !apierrors.IsNotFound(err) {
				return false, err
			}
		}
		log.Infof("Waiting for hosted agent pool assignment cleanup: userID=%s assignments=%d", userID, len(assignments.Items))
		return false, nil
	}

	waiting := false
	for _, poolID := range poolIDs {
		var remainingAssignments v1.HostedAgentPoolAssignmentList
		if err := req.List(&remainingAssignments, &kclient.ListOptions{
			Namespace: req.Namespace,
			FieldSelector: fields.SelectorFromSet(map[string]string{
				"spec.poolID": poolID,
			}),
		}); err != nil {
			return false, err
		}
		if len(remainingAssignments.Items) > 0 {
			// Another user still references this pool. Ownership is
			// shared, so this user's deletion must leave it intact.
			continue
		}

		var pool v1.HostedAgentPool
		if err := req.Get(&pool, req.Namespace, poolID); err != nil {
			if apierrors.IsNotFound(err) {
				continue
			}
			return false, err
		}
		waiting = true
		if err := req.Delete(&pool); err != nil && !apierrors.IsNotFound(err) {
			return false, err
		}
	}
	if waiting {
		log.Infof("Waiting for hosted agent pool cleanup: userID=%s pools=%d", userID, len(poolIDs))
		return false, nil
	}
	return true, nil
}

func savedPoolIDs(userDelete *v1.UserDelete) ([]string, error) {
	value := userDelete.Annotations[hostedAgentPoolCleanupAnnotation]
	if value == "" {
		return nil, nil
	}
	var result []string
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return nil, fmt.Errorf("parse hosted agent pool cleanup checkpoint: %w", err)
	}
	return result, nil
}

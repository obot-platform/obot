package cleanup

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
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

	// Delete identities first so that the user can login again.
	identities, err := u.gatewayClient.FindIdentitiesForUser(req.Ctx, userDelete.Spec.UserID)
	if err != nil {
		return err
	}

	if err = u.gatewayClient.DeleteSessionsForUser(req.Ctx, req.Client, identities, ""); err != nil {
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

	// Delete any API keys the user created. Nanobot-agent keys are handled by the
	// NanobotAgent delete flow above; this sweeps user-created keys plus anything
	// the nanobot path missed.
	apiKeys, err := u.gatewayClient.ListAPIKeys(req.Ctx, userDelete.Spec.UserID)
	if err != nil {
		return fmt.Errorf("failed to list API keys for user %d: %w", userDelete.Spec.UserID, err)
	}
	for _, key := range apiKeys {
		if err := u.gatewayClient.DeleteAPIKey(req.Ctx, userDelete.Spec.UserID, key.ID); err != nil {
			return fmt.Errorf("failed to delete API key %d for user %d: %w", key.ID, userDelete.Spec.UserID, err)
		}
	}
	log.Infof("Deleted API keys during user cleanup: userID=%s keys=%d", userID, len(apiKeys))

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
		// around so we can delete the API key.
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

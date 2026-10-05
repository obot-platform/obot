package cleanup

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/pkg/auth"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/groupref"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
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
		// Deconfiguring a provider that SCIM manages deleted its SCIM connection, with its group data, before the
		// cleanup became ready, so the cleanup goes on as for any other provider. The gateway data goes first, because
		// its deletion is refused in the same transaction that would delete it while a SCIM connection owns it: the
		// provider's own connection, which is retried, or that of another provider with the same group ID prefix,
		// whose groups and policy subjects are kept. The policy subjects follow only once the gateway data is gone. No
		// connection can be created until the cleanup finishes, because connection setup refuses while a cleanup is
		// pending for the provider or its group ID prefix.
		if err := a.gatewayClient.DeleteAuthProviderGroupData(req.Ctx, providerNamespace, providerName, groupIDPrefix); errors.Is(err, gclient.ErrSCIMManagedGroupData) {
			slog.Warn("Kept the group data of an auth provider whose group ID prefix another provider's SCIM connection manages",
				"authProvider", providerName, "namespace", providerNamespace, "groupIDPrefix", groupIDPrefix)
			return req.Delete(cleanup)
		} else if err != nil {
			return err
		}

		counts, err := groupref.RemoveGroupSubjects(req.Ctx, req.Client, req.Namespace, groupref.HasPrefix(groupIDPrefix))
		if err != nil {
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

package cleanup

import (
	"errors"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type UserCleanup struct {
	gatewayClient *gclient.Client
}

func NewUserCleanup(gatewayClient *gclient.Client) *UserCleanup {
	return &UserCleanup{
		gatewayClient: gatewayClient,
	}
}

func (u *UserCleanup) Cleanup(req router.Request, _ router.Response) error {
	userDelete := req.Object.(*v1.UserDelete)
	var threads v1.ThreadList
	if err := req.List(&threads, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userUID": strconv.FormatUint(uint64(userDelete.Spec.UserID), 10),
		}),
	}); err != nil {
		return err
	}

	for _, thread := range threads.Items {
		if thread.Spec.Project {
			if err := req.Delete(&thread); err != nil {
				return err
			}
		}
	}

	var servers v1.MCPServerList
	if err := req.List(&servers, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userUID": strconv.FormatUint(uint64(userDelete.Spec.UserID), 10),
		}),
	}); err != nil {
		return err
	}

	for _, server := range servers.Items {
		if err := req.Delete(&server); err != nil {
			return err
		}
	}

	identities, err := u.gatewayClient.FindIdentitiesForUser(req.Ctx, userDelete.Spec.UserID)
	if err != nil {
		return err
	}

	if err = u.gatewayClient.DeleteSessionsForUser(req.Ctx, req.Client, identities, ""); err != nil {
		if !errors.Is(err, gclient.LogoutAllErr{}) {
			return err
		}
	}

	// If everything is cleaned up successfully, then delete this object because we don't need it.
	return req.Delete(userDelete)
}

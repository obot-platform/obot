package cleanup

import (
	"errors"
	"slices"
	"strconv"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/accesscontrolrule"
	gclient "github.com/obot-platform/obot/pkg/gateway/client"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
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
			"spec.userID": strconv.FormatUint(uint64(userDelete.Spec.UserID), 10),
		}),
	}); err != nil {
		return err
	}

	for _, server := range servers.Items {
		if err := req.Delete(&server); err != nil {
			return err
		}
	}

	// DeleteRefs should handle cleaning up most of the user's MCPServerInstances.
	// But there still might be MCPServerInstances pointing to multi-user servers that we need to delete.
	var instances v1.MCPServerInstanceList
	if err := req.List(&instances, &kclient.ListOptions{
		Namespace: req.Namespace,
		FieldSelector: fields.SelectorFromSet(map[string]string{
			"spec.userID": strconv.FormatUint(uint64(userDelete.Spec.UserID), 10),
		}),
	}); err != nil {
		return err
	}

	for _, instance := range instances.Items {
		if err := req.Delete(&instance); err != nil {
			return err
		}
	}

	// Find the AccessControlRules that the user is on, and update them to remove the user.
	acrs, err := u.acrHelper.GetAccessControlRulesForUser(req.Namespace, strconv.FormatUint(uint64(userDelete.Spec.UserID), 10))
	if err != nil {
		return err
	}
	for _, acr := range acrs {
		newSubjects := slices.Collect(func(yield func(types.Subject) bool) {
			for _, subject := range acr.Spec.Manifest.Subjects {
				if subject.ID != strconv.FormatUint(uint64(userDelete.Spec.UserID), 10) {
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

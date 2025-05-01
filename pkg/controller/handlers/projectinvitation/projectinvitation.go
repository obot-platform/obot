package projectinvitation

import (
	"time"

	"github.com/obot-platform/nah/pkg/router"
	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func SetRespondedTime(req router.Request, _ router.Response) error {
	invitation := req.Object.(*v1.ProjectInvitation)

	switch invitation.Spec.Status {
	case string(types.ProjectInvitationStatusAccepted), string(types.ProjectInvitationStatusRejected):
		if invitation.Status.RespondedTime.IsZero() {
			invitation.Status.RespondedTime = &metav1.Time{Time: time.Now()}
			return req.Client.Status().Update(req.Ctx, invitation)
		}
	}

	return nil
}

// Expiration sets the status of the project invitation to expired if it is pending and has not been accepted or rejected after 7 days.
func Expiration(req router.Request, resp router.Response) error {
	invitation := req.Object.(*v1.ProjectInvitation)

	if invitation.Spec.Status != string(types.ProjectInvitationStatusPending) {
		return nil
	}

	if time.Since(invitation.CreationTimestamp.Time) > 7*24*time.Hour {
		invitation.Spec.Status = string(types.ProjectInvitationStatusExpired)
		invitation.Status.RespondedTime = &metav1.Time{Time: time.Now()}
		return req.Client.Status().Update(req.Ctx, invitation)
	}
	resp.RetryAfter(7*24*time.Hour - time.Since(invitation.CreationTimestamp.Time))

	return nil
}

// Cleanup deletes the project invitation if it was accepted, rejected, or marked as expired more than 7 days ago.
func Cleanup(req router.Request, resp router.Response) error {
	invitation := req.Object.(*v1.ProjectInvitation)

	if !invitation.Status.RespondedTime.IsZero() {
		if time.Since(invitation.Status.RespondedTime.Time) > 7*24*time.Hour {
			return req.Client.Delete(req.Ctx, invitation)
		}
		resp.RetryAfter(7*24*time.Hour - time.Since(invitation.Status.RespondedTime.Time))
	}

	return nil
}

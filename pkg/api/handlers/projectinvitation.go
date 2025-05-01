package handlers

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type ProjectInvitationHandler struct{}

func NewProjectInvitationHandler() *ProjectInvitationHandler {
	return &ProjectInvitationHandler{}
}

func (h *ProjectInvitationHandler) CreateInvitationForProject(req api.Context) error {
	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if !thread.Spec.Project {
		return types.NewErrBadRequest("only projects can have invitations")
	}

	if thread.Spec.UserID != req.User.GetUID() {
		return types.NewErrForbidden("only the project creator can create invitations")
	}

	var project types.Project
	if thread.Spec.ParentThreadName != "" {
		var parentThread *v1.Thread
		if err := req.Get(parentThread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
		project = convertProject(thread, parentThread)
	} else {
		project = convertProject(thread, nil)
	}

	// Generate a random code for the invitation
	code := strings.ReplaceAll(uuid.New().String(), "-", "")

	invitation := v1.ProjectInvitation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      code,
			Namespace: req.Namespace(),
		},
		Spec: v1.ProjectInvitationSpec{
			Status:   string(types.ProjectInvitationStatusPending),
			ThreadID: thread.Name,
		},
	}

	if err := req.Create(&invitation); err != nil {
		return err
	}

	return req.WriteCreated(types.ProjectInvitationManifest{
		Code:    code,
		Project: &project,
		Status:  types.ProjectInvitationStatusPending,
		Created: invitation.CreationTimestamp.Format(time.RFC3339),
	})
}

func (h *ProjectInvitationHandler) ListInvitationsForProject(req api.Context) error {
	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if !thread.Spec.Project {
		return types.NewErrBadRequest("only projects can have invitations")
	}

	if thread.Spec.UserID != req.User.GetUID() {
		return types.NewErrForbidden("only the project creator can list invitations")
	}

	var project types.Project
	if thread.Spec.ParentThreadName != "" {
		var parentThread *v1.Thread
		if err := req.Get(parentThread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
		project = convertProject(thread, parentThread)
	} else {
		project = convertProject(thread, nil)
	}

	var invitations v1.ProjectInvitationList
	if err := req.List(&invitations, kclient.MatchingFields{
		"spec.threadID": thread.Name,
	}); err != nil {
		return err
	}

	// Convert to list of manifests
	manifests := make([]types.ProjectInvitationManifest, len(invitations.Items))
	for i, invitation := range invitations.Items {
		manifests[i] = types.ProjectInvitationManifest{
			Code:    invitation.Name,
			Project: &project,
			Status:  types.ProjectInvitationStatus(invitation.Spec.Status),
			Created: invitation.CreationTimestamp.Format(time.RFC3339),
		}
	}

	return req.Write(manifests)
}

func (h *ProjectInvitationHandler) DeleteInvitationForProject(req api.Context) error {
	var (
		code = req.PathValue("code")
	)

	thread, err := getProjectThread(req)
	if err != nil {
		return err
	}

	if !thread.Spec.Project {
		return types.NewErrBadRequest("only projects can have invitations")
	}

	if thread.Spec.UserID != req.User.GetUID() {
		return types.NewErrForbidden("only the project creator can delete invitations")
	}

	var invitation v1.ProjectInvitation
	if err := req.Get(&invitation, code); err != nil {
		if apierrors.IsNotFound(err) {
			return types.NewErrNotFound("invitation not found")
		}
		return err
	}

	// Verify the invitation belongs to this project
	if invitation.Spec.ThreadID != thread.Name {
		return types.NewErrBadRequest("invitation does not belong to this project")
	}

	if err := req.Delete(&invitation); err != nil {
		return err
	}

	return req.Write(nil)
}

func (h *ProjectInvitationHandler) GetInvitation(req api.Context) error {
	var (
		code       = req.PathValue("code")
		invitation v1.ProjectInvitation
	)

	if err := req.Get(&invitation, code); err != nil {
		if apierrors.IsNotFound(err) {
			return types.NewErrNotFound("invitation not found")
		}
		return err
	}

	var thread v1.Thread
	if err := req.Get(&thread, invitation.Spec.ThreadID); err != nil {
		return err
	}

	var project types.Project
	if thread.Spec.ParentThreadName != "" {
		var parentThread *v1.Thread
		if err := req.Get(parentThread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
		project = convertProject(&thread, parentThread)
	} else {
		project = convertProject(&thread, nil)
	}

	return req.Write(types.ProjectInvitationManifest{
		Code:    invitation.Name,
		Project: &project,
		Status:  types.ProjectInvitationStatus(invitation.Spec.Status),
		Created: invitation.CreationTimestamp.Format(time.RFC3339),
	})
}

func (h *ProjectInvitationHandler) AcceptInvitation(req api.Context) error {
	var (
		code       = req.PathValue("code")
		invitation v1.ProjectInvitation
		thread     v1.Thread
	)

	if err := req.Get(&invitation, code); err != nil {
		if apierrors.IsNotFound(err) {
			return types.NewErrNotFound("invitation not found")
		}
		return err
	}

	if invitation.Spec.Status != string(types.ProjectInvitationStatusPending) {
		return types.NewErrBadRequest("invitation is no longer valid")
	}

	if err := req.Get(&thread, invitation.Spec.ThreadID); err != nil {
		return err
	}

	if thread.Spec.UserID == req.User.GetUID() {
		return types.NewErrBadRequest("you cannot accept an invitation to your own project")
	}

	// Check if the user is already a member of the project
	var memberships v1.ThreadAuthorizationList
	if err := req.List(&memberships, kclient.MatchingFields{
		"spec.threadID": invitation.Spec.ThreadID,
		"spec.userID":   req.User.GetUID(),
	}); err != nil {
		return err
	}

	if len(memberships.Items) > 0 {
		return types.NewErrBadRequest("you are already a member of this project")
	}

	// Update invitation status to accepted
	invitation.Spec.Status = string(types.ProjectInvitationStatusAccepted)
	if err := req.Update(&invitation); err != nil {
		return err
	}

	// Create a new ThreadAuthorization for the user
	threadAuth := v1.ThreadAuthorization{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: system.ThreadAuthorizationPrefix,
			Namespace:    req.Namespace(),
		},
		Spec: v1.ThreadAuthorizationSpec{
			ThreadAuthorizationManifest: types.ThreadAuthorizationManifest{
				ThreadID: invitation.Spec.ThreadID,
				UserID:   req.User.GetUID(),
			},
		},
	}

	if err := req.Create(&threadAuth); err != nil {
		return err
	}

	var project types.Project
	if thread.Spec.ParentThreadName != "" {
		var parentThread *v1.Thread
		if err := req.Get(parentThread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
		project = convertProject(&thread, parentThread)
	} else {
		project = convertProject(&thread, nil)
	}

	return req.Write(types.ProjectInvitationManifest{
		Code:    invitation.Name,
		Project: &project,
		Status:  types.ProjectInvitationStatusAccepted,
		Created: invitation.CreationTimestamp.Format(time.RFC3339),
	})
}

func (h *ProjectInvitationHandler) RejectInvitation(req api.Context) error {
	var (
		code       = req.PathValue("code")
		invitation v1.ProjectInvitation
	)

	if err := req.Get(&invitation, code); err != nil {
		if apierrors.IsNotFound(err) {
			return types.NewErrNotFound("invitation not found")
		}
		return err
	}

	if invitation.Spec.Status != string(types.ProjectInvitationStatusPending) {
		return types.NewErrBadRequest("invitation is no longer valid")
	}

	var thread v1.Thread
	if err := req.Get(&thread, invitation.Spec.ThreadID); err != nil {
		return err
	}

	// Update invitation status to rejected
	invitation.Spec.Status = string(types.ProjectInvitationStatusRejected)
	if err := req.Update(&invitation); err != nil {
		return err
	}

	var project types.Project
	if thread.Spec.ParentThreadName != "" {
		var parentThread *v1.Thread
		if err := req.Get(parentThread, thread.Spec.ParentThreadName); err != nil {
			return err
		}
		project = convertProject(&thread, parentThread)
	} else {
		project = convertProject(&thread, nil)
	}

	return req.Write(types.ProjectInvitationManifest{
		Code:    invitation.Name,
		Project: &project,
		Status:  types.ProjectInvitationStatusRejected,
		Created: invitation.CreationTimestamp.Format(time.RFC3339),
	})
}

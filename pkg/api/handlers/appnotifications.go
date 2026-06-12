package handlers

import (
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type AppNotificationsHandler struct{}

func NewAppNotificationsHandler() *AppNotificationsHandler {
	return &AppNotificationsHandler{}
}

func (h *AppNotificationsHandler) Get(req api.Context) error {
	var notifications v1.AppNotifications
	err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.AppNotificationsName,
	}, &notifications)

	if apierrors.IsNotFound(err) {
		// Return empty notifications if not yet configured
		return req.Write(types.AppNotifications{})
	}
	if err != nil {
		return err
	}

	converted := convertAppNotifications(notifications)
	return req.Write(converted)
}

func (h *AppNotificationsHandler) Update(req api.Context) error {
	var input types.AppNotifications
	if err := req.Read(&input); err != nil {
		return err
	}

	var notifications v1.AppNotifications
	err := req.Get(&notifications, system.AppNotificationsName)

	if apierrors.IsNotFound(err) {
		// Create new notifications
		notifications = v1.AppNotifications{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.AppNotificationsName,
				Namespace: req.Namespace(),
			},
			Spec: v1.AppNotificationsSpec{
				Banner: input.Banner,
			},
		}

		if err := req.Create(&notifications); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update existing notifications
		notifications.Spec.Banner = input.Banner
		notifications.Spec.Updated = metav1.Now()

		if err := req.Update(&notifications); err != nil {
			return err
		}
	}

	converted := convertAppNotifications(notifications)
	return req.Write(converted)
}

func convertAppNotifications(notifications v1.AppNotifications) types.AppNotifications {
	// On first creation, no explicit updated time is stored, so it matches the creation time.
	updated := notifications.Spec.Updated.Time
	if updated.IsZero() {
		updated = notifications.GetCreationTimestamp().Time
	}

	return types.AppNotifications{
		Banner:   notifications.Spec.Banner,
		Updated:  *types.NewTime(updated),
		Metadata: MetadataFrom(&notifications),
	}
}

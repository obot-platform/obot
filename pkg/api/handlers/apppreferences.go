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

type AppPreferencesHandler struct{}

func NewAppPreferencesHandler() *AppPreferencesHandler {
	return &AppPreferencesHandler{}
}

func (h *AppPreferencesHandler) Get(req api.Context) error {
	var prefs v1.AppPreferences
	err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.AppPreferencesName,
	}, &prefs)

	if apierrors.IsNotFound(err) {
		// Return empty preferences if not yet configured
		return req.Write(types.AppPreferences{})
	}
	if err != nil {
		return err
	}

	converted := convertAppPreferences(prefs)
	return req.Write(converted)
}

func (h *AppPreferencesHandler) Update(req api.Context) error {
	var input types.AppPreferences
	if err := req.Read(&input); err != nil {
		return err
	}

	var prefs v1.AppPreferences
	err := req.Storage.Get(req.Context(), client.ObjectKey{
		Namespace: req.Namespace(),
		Name:      system.AppPreferencesName,
	}, &prefs)

	if apierrors.IsNotFound(err) {
		// Create new preferences
		prefs = v1.AppPreferences{
			ObjectMeta: metav1.ObjectMeta{
				Name:      system.AppPreferencesName,
				Namespace: req.Namespace(),
			},
		}
		prefs.Spec.Logos = v1.LogoPreferences(input.Logos)
		prefs.Spec.Theme = v1.ThemePreferences(input.Theme)

		if err := req.Storage.Create(req.Context(), &prefs); err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Update existing preferences
		prefs.Spec.Logos = v1.LogoPreferences(input.Logos)
		prefs.Spec.Theme = v1.ThemePreferences(input.Theme)

		if err := req.Storage.Update(req.Context(), &prefs); err != nil {
			return err
		}
	}

	converted := convertAppPreferences(prefs)
	return req.Write(converted)
}

func convertAppPreferences(prefs v1.AppPreferences) types.AppPreferences {
	return types.AppPreferences{
		Logos:    types.LogoPreferences(prefs.Spec.Logos),
		Theme:    types.ThemePreferences(prefs.Spec.Theme),
		Metadata: MetadataFrom(&prefs),
	}
}


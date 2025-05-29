package oauth

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/handlers"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/system"
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func (h *handler) register(req api.Context) error {
	var oauthClientManifest types.OAuthClientManifest
	if err := req.Read(&oauthClientManifest); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	clientID := system.OAuthClientPrefix + strings.ToLower(rand.Text())

	oauthClient := v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      clientID,
			Namespace: system.DefaultNamespace,
		},
		Spec: v1.OAuthClientSpec{
			Manifest: oauthClientManifest,
		},
	}

	clientSecret, registrationToken, err := updateClientIfNecessary(req.Context(), req.Storage, &oauthClient, false)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	if err = req.Create(&oauthClient); err != nil {
		return err
	}

	return req.Write(convertClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) readClient(req api.Context) error {
	var oauthClient v1.OAuthClient
	if err := req.Get(&oauthClient, req.PathValue("name")); err != nil {
		return err
	}

	clientSecret, registrationToken, err := updateClientIfNecessary(req.Context(), req.Storage, &oauthClient, true)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	return req.Write(convertClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) updateClient(req api.Context) error {
	var oauthClientManifest types.OAuthClientManifest
	if err := req.Read(&oauthClientManifest); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	var oauthClient v1.OAuthClient
	if err := req.Get(&oauthClient, req.PathValue("name")); err != nil {
		return err
	}

	oauthClient.Spec.Manifest = oauthClientManifest
	clientSecret, registrationToken, err := updateClientIfNecessary(req.Context(), req.Storage, &oauthClient, true)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	if err = req.Update(&oauthClient); err != nil {
		return err
	}

	return req.Write(convertClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) deleteClient(req api.Context) error {
	return req.Delete(&v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.PathValue("name"),
			Namespace: system.DefaultNamespace,
		},
	})
}

func updateClientIfNecessary(ctx context.Context, c kclient.Client, oauthClient *v1.OAuthClient, persistent bool) (string, string, error) {
	var (
		clientSecret, registrationToken string
		update                          bool
		err                             error
	)

	if oauthClient.Spec.ClientSecretIssuedAt.IsZero() || oauthClient.Spec.ClientSecretExpiresAt.Sub(oauthClient.Spec.ClientSecretIssuedAt.Time)/2 > oauthClient.Spec.ClientSecretExpiresAt.Sub(time.Now()) {
		// If the client secret is half-way through its lifetime, then update it.
		clientSecret = rand.Text() + rand.Text()
		oauthClient.Spec.ClientSecretHash, err = bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
		if err != nil {
			return "", "", err
		}

		oauthClient.Spec.ClientSecretIssuedAt = metav1.NewTime(time.Now())
		oauthClient.Spec.ClientSecretExpiresAt = metav1.NewTime(time.Now().Add(time.Hour + 15*time.Minute))

		update = true
	}
	if oauthClient.Spec.RegistrationTokenExpiresAt.IsZero() || oauthClient.Spec.RegistrationTokenExpiresAt.Sub(oauthClient.Spec.RegistrationTokenIssuedAt.Time)/2 > oauthClient.Spec.RegistrationTokenExpiresAt.Sub(time.Now()) {
		// If the registration token is half-way through its lifetime, then update it.
		registrationToken = rand.Text() + rand.Text()
		oauthClient.Spec.RegistrationTokenHash, err = bcrypt.GenerateFromPassword([]byte(registrationToken), bcrypt.DefaultCost)
		if err != nil {
			return "", "", err
		}

		oauthClient.Spec.RegistrationTokenIssuedAt = metav1.NewTime(time.Now())
		oauthClient.Spec.RegistrationTokenExpiresAt = metav1.NewTime(time.Now().Add(7 * 24 * time.Hour))

		update = true
	}

	if update && persistent {
		if err := c.Update(ctx, oauthClient); err != nil {
			return "", "", err
		}
	}

	return clientSecret, registrationToken, nil
}

func convertClient(oauthClient v1.OAuthClient, baseURL, clientSecret, registrationToken string) types.OAuthClient {
	return types.OAuthClient{
		Metadata:                handlers.MetadataFrom(&oauthClient),
		OAuthClientManifest:     oauthClient.Spec.Manifest,
		RegistrationAccessToken: registrationToken,
		RegistrationClientURI:   fmt.Sprintf("%s/oauth/register/%s/%s", baseURL, oauthClient.Namespace, oauthClient.Name),
		ClientID:                oauthClient.Name,
		ClientSecret:            clientSecret,
		ClientSecretIssuedAt:    types.NewTime(oauthClient.Spec.ClientSecretIssuedAt.Time),
		ClientSecretExpiresAt:   types.NewTime(oauthClient.Spec.ClientSecretExpiresAt.Time),
	}
}

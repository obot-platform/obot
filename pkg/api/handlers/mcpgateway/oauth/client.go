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

	if err := handlers.ValidateClientConfig(&oauthClient, h.oauthConfig); err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidClientMetadata,
			Description: err.Error(),
		})
	}

	clientSecret, registrationToken, err := ensureTokenAndSecret(&oauthClient)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	if err = req.Create(&oauthClient); err != nil {
		return err
	}

	return req.WriteCreated(handlers.ConvertDynamicClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) readClient(req api.Context) error {
	var oauthClient v1.OAuthClient
	namespace, name, ok := strings.Cut(req.PathValue("client"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client name: %s", name)
	}

	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &oauthClient); err != nil {
		return err
	}

	clientSecret, registrationToken, err := updateClientIfNecessary(req.Context(), req.Storage, &oauthClient)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	return req.Write(handlers.ConvertDynamicClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) updateClient(req api.Context) error {
	var oauthClientManifest types.OAuthClientManifest
	if err := req.Read(&oauthClientManifest); err != nil {
		return types.NewErrBadRequest("invalid request body: %v", err)
	}

	namespace, name, ok := strings.Cut(req.PathValue("client"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client name: %s", name)
	}

	var oauthClient v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: namespace, Name: name}, &oauthClient); err != nil {
		return err
	}

	oauthClient.Spec.Manifest = oauthClientManifest

	if err := handlers.ValidateClientConfig(&oauthClient, h.oauthConfig); err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidClientMetadata,
			Description: err.Error(),
		})
	}

	clientSecret, registrationToken, err := updateClientIfNecessary(req.Context(), req.Storage, &oauthClient)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	if err = req.Update(&oauthClient); err != nil {
		return err
	}

	return req.Write(handlers.ConvertDynamicClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) deleteClient(req api.Context) error {
	namespace, name, ok := strings.Cut(req.PathValue("client"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client name: %s", name)
	}

	return req.Delete(&v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Finalizers: []string{v1.OAuthClientFinalizer},
		},
	})
}

func updateClientIfNecessary(ctx context.Context, c kclient.Client, oauthClient *v1.OAuthClient) (string, string, error) {
	clientSecret, registrationToken, err := ensureTokenAndSecret(oauthClient)
	if err != nil {
		return "", "", err
	}

	if clientSecret != "" || registrationToken != "" {
		if err = c.Update(ctx, oauthClient); err != nil {
			return "", "", err
		}
	}

	return clientSecret, registrationToken, nil
}

func ensureTokenAndSecret(oauthClient *v1.OAuthClient) (string, string, error) {
	var (
		clientSecret, registrationToken string
		err                             error
		now                             = time.Now()
	)
	if oauthClient.Spec.ClientSecretIssuedAt.IsZero() || oauthClient.Spec.ClientSecretExpiresAt.Sub(oauthClient.Spec.ClientSecretIssuedAt.Time)/2 > time.Until(oauthClient.Spec.ClientSecretExpiresAt.Time) {
		// If the client secret is half-way through its lifetime, then update it.
		clientSecret = rand.Text() + rand.Text()
		oauthClient.Spec.ClientSecretHash, err = bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
		if err != nil {
			return "", "", err
		}

		oauthClient.Spec.ClientSecretIssuedAt = metav1.NewTime(now)
		oauthClient.Spec.ClientSecretExpiresAt = metav1.NewTime(now.Add(7 * 24 * time.Hour))
	}
	if oauthClient.Spec.RegistrationTokenExpiresAt.IsZero() || oauthClient.Spec.RegistrationTokenExpiresAt.Sub(oauthClient.Spec.RegistrationTokenIssuedAt.Time)/2 > time.Until(oauthClient.Spec.RegistrationTokenExpiresAt.Time) {
		// If the registration token is half-way through its lifetime, then update it.
		registrationToken = rand.Text() + rand.Text()
		oauthClient.Spec.RegistrationTokenHash, err = bcrypt.GenerateFromPassword([]byte(registrationToken), bcrypt.DefaultCost)
		if err != nil {
			return "", "", err
		}

		oauthClient.Spec.RegistrationTokenIssuedAt = metav1.NewTime(now)
		oauthClient.Spec.RegistrationTokenExpiresAt = metav1.NewTime(now.Add(7 * 24 * time.Hour))
	}

	return clientSecret, registrationToken, nil
}

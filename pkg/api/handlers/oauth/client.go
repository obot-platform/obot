package oauth

import (
	"context"
	"crypto/rand"
	"fmt"
	"slices"
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

	if err := h.validateClientConfig(&oauthClient); err != nil {
		return err
	}

	clientSecret, registrationToken, err := ensureTokenAndSecret(&oauthClient)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	if err = req.Create(&oauthClient); err != nil {
		return err
	}

	return req.WriteCreated(convertClient(oauthClient, h.baseURL, clientSecret, registrationToken))
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

	return req.Write(convertClient(oauthClient, h.baseURL, clientSecret, registrationToken))
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

	if err := h.validateClientConfig(&oauthClient); err != nil {
		return err
	}

	clientSecret, registrationToken, err := updateClientIfNecessary(req.Context(), req.Storage, &oauthClient)
	if err != nil {
		return fmt.Errorf("failed to update client secret: %w", err)
	}

	if err = req.Update(&oauthClient); err != nil {
		return err
	}

	return req.Write(convertClient(oauthClient, h.baseURL, clientSecret, registrationToken))
}

func (h *handler) deleteClient(req api.Context) error {
	namespace, name, ok := strings.Cut(req.PathValue("client"), ":")
	if !ok {
		return types.NewErrBadRequest("invalid client name: %s", name)
	}

	return req.Delete(&v1.OAuthClient{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
	)
	if oauthClient.Spec.ClientSecretIssuedAt.IsZero() || oauthClient.Spec.ClientSecretExpiresAt.Sub(oauthClient.Spec.ClientSecretIssuedAt.Time)/2 > time.Until(oauthClient.Spec.ClientSecretExpiresAt.Time) {
		// If the client secret is half-way through its lifetime, then update it.
		clientSecret = rand.Text() + rand.Text()
		oauthClient.Spec.ClientSecretHash, err = bcrypt.GenerateFromPassword([]byte(clientSecret), bcrypt.DefaultCost)
		if err != nil {
			return "", "", err
		}

		oauthClient.Spec.ClientSecretIssuedAt = metav1.NewTime(time.Now())
		oauthClient.Spec.ClientSecretExpiresAt = metav1.NewTime(time.Now().Add(time.Hour + 15*time.Minute))
	}
	if oauthClient.Spec.RegistrationTokenExpiresAt.IsZero() || oauthClient.Spec.RegistrationTokenExpiresAt.Sub(oauthClient.Spec.RegistrationTokenIssuedAt.Time)/2 > time.Until(oauthClient.Spec.RegistrationTokenExpiresAt.Time) {
		// If the registration token is half-way through its lifetime, then update it.
		registrationToken = rand.Text() + rand.Text()
		oauthClient.Spec.RegistrationTokenHash, err = bcrypt.GenerateFromPassword([]byte(registrationToken), bcrypt.DefaultCost)
		if err != nil {
			return "", "", err
		}

		oauthClient.Spec.RegistrationTokenIssuedAt = metav1.NewTime(time.Now())
		oauthClient.Spec.RegistrationTokenExpiresAt = metav1.NewTime(time.Now().Add(7 * 24 * time.Hour))
	}

	return clientSecret, registrationToken, nil
}

func (h *handler) validateClientConfig(oauthClient *v1.OAuthClient) error {
	//nolint: staticcheck
	if oauthClient.Spec.Manifest.RedirectURI != "" {
		oauthClient.Spec.Manifest.RedirectURIs = append(oauthClient.Spec.Manifest.RedirectURIs, oauthClient.Spec.Manifest.RedirectURI)
	}
	if len(oauthClient.Spec.Manifest.RedirectURIs) == 0 {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidClientMetadata,
			Description: "redirect_uris is required",
		})
	}
	if oauthClient.Spec.Manifest.TokenEndpointAuthMethod != "" && !slices.Contains(h.oauthConfig.TokenEndpointAuthMethodsSupported, oauthClient.Spec.Manifest.TokenEndpointAuthMethod) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidClientMetadata,
			Description: fmt.Sprintf("token_endpoint_auth_method must be %s, not %s", strings.Join(h.oauthConfig.TokenEndpointAuthMethodsSupported, ", "), oauthClient.Spec.Manifest.TokenEndpointAuthMethod),
		})
	}

	return nil
}

func convertClient(oauthClient v1.OAuthClient, baseURL, clientSecret, registrationToken string) types.OAuthClient {
	oauthClient.Name = fmt.Sprintf("%s:%s", oauthClient.Namespace, oauthClient.Name)
	return types.OAuthClient{
		Metadata:                   handlers.MetadataFrom(&oauthClient),
		OAuthClientManifest:        oauthClient.Spec.Manifest,
		RegistrationAccessToken:    registrationToken,
		RegistrationClientURI:      fmt.Sprintf("%s/oauth/register/%s", baseURL, oauthClient.Name),
		RegistrationTokenIssuedAt:  oauthClient.Spec.RegistrationTokenIssuedAt.Unix(),
		RegistrationTokenExpiresAt: oauthClient.Spec.RegistrationTokenExpiresAt.Unix(),
		ClientID:                   oauthClient.Name,
		ClientSecret:               clientSecret,
		ClientSecretIssuedAt:       oauthClient.Spec.ClientSecretIssuedAt.Unix(),
		ClientSecretExpiresAt:      oauthClient.Spec.ClientSecretExpiresAt.Unix(),
	}
}

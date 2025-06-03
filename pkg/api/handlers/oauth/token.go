package oauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"golang.org/x/crypto/bcrypt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var log = logger.Package()

func (h *handler) token(req api.Context) error {
	if err := req.ParseForm(); err != nil {
		return err
	}

	clientName := req.FormValue("client_id")
	if clientName == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client_id is required",
		})
	}

	clientNamespace, clientName, ok := strings.Cut(clientName, ":")
	if !ok {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client_id is invalid",
		})
	}

	var client v1.OAuthClient
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: clientNamespace, Name: clientName}, &client); err != nil {
		return err
	}

	if bcrypt.CompareHashAndPassword(client.Spec.ClientSecretHash, []byte(req.FormValue("client_secret"))) != nil {
		return types.NewErrHTTP(http.StatusUnauthorized, "Invalid client credentials")
	}

	grantType := req.FormValue("grant_type")
	if !slices.Contains(h.oauthConfig.GrantTypesSupported, grantType) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "grant_type must be authorization_code",
		})
	}

	if !slices.Contains(client.Spec.Manifest.GrantTypes, grantType) {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "client is not allowed to use authorization_code grant type",
		})
	}

	if grantType == "authorization_code" {
		return h.doAuthorizationCode(req, client, req.FormValue("code"), req.FormValue("code_verifier"))
	}

	return h.doRefreshToken(req, client, req.FormValue("refresh_token"))
}

func (h *handler) doAuthorizationCode(req api.Context, oauthClient v1.OAuthClient, code, codeVerifier string) error {
	if code == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "code is required",
		})
	}

	var oauthAuthRequestList v1.OAuthAuthRequestList
	if err := req.Storage.List(req.Context(), &oauthAuthRequestList, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"hashedAuthCode": fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
		})),
	}); err != nil {
		return err
	}
	if len(oauthAuthRequestList.Items) != 1 {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "code is invalid",
		})
	}

	oauthAuthRequest := oauthAuthRequestList.Items[0]

	// Authorization codes are one-time use
	if err := req.Storage.Delete(req.Context(), &oauthAuthRequest); err != nil {
		// Don't return an error if we can't delete the auth request
		log.Warnf("failed to delete auth request: %v", err)
	}

	if oauthAuthRequest.Spec.CodeChallenge != "" {
		switch oauthAuthRequest.Spec.CodeChallengeMethod {
		case "S256":
			hashedCodeVerifier := sha256.Sum256([]byte(codeVerifier))
			if oauthAuthRequest.Spec.CodeChallenge != base64.RawURLEncoding.EncodeToString(hashedCodeVerifier[:]) {
				return types.NewErrBadRequest("%v", Error{
					Code:        ErrInvalidRequest,
					Description: "code_verifier is invalid",
				})
			}
		case "plain":
			if oauthAuthRequest.Spec.CodeChallenge != codeVerifier {
				return types.NewErrBadRequest("%v", Error{
					Code:        ErrInvalidRequest,
					Description: "code_verifier is invalid",
				})
			}
		default:
			return types.NewErrBadRequest("%v", Error{
				Code:        ErrInvalidRequest,
				Description: "code_challenge_method must be S256 or plain. ",
			})
		}
	}

	accessToken, err := h.newAccessToken(oauthAuthRequest.Status.ProviderAccessToken)
	if err != nil {
		return err
	}

	refreshToken := strings.ToLower(rand.Text() + rand.Text())

	oauthToken := v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oauthClient.Namespace,
			Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
		},
		Spec: v1.OAuthTokenSpec{
			ClientID:             oauthClient.Name,
			Scope:                oauthAuthRequest.Spec.Scope,
			ProviderRefreshToken: oauthAuthRequest.Status.ProviderRefreshToken,
			ProviderAccessToken:  oauthAuthRequest.Status.ProviderAccessToken,
			ExpiresAt:            oauthAuthRequest.Status.ExpiresAt,
		},
	}

	if err = req.Create(&oauthToken); err != nil {
		return fmt.Errorf("failed to create oauth token: %w", err)
	}

	return req.Write(types.OAuthToken{
		AccessToken:  accessToken,
		TokenType:    "bearer",
		ExpiresIn:    int(time.Until(oauthToken.Spec.ExpiresAt.Time).Milliseconds() / 1000),
		Scope:        oauthAuthRequest.Spec.Scope,
		RefreshToken: refreshToken,
	})
}

func (h *handler) doRefreshToken(req api.Context, oauthClient v1.OAuthClient, refreshToken string) error {
	if refreshToken == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "refresh_token is required",
		})
	}

	var oauthToken v1.OAuthToken
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthClient.Namespace, Name: fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))}, &oauthToken); err != nil {
		return err
	}

	// TODO: Need to refresh the auth provider's token

	if err := req.Storage.Delete(req.Context(), &oauthToken); err != nil {
		return fmt.Errorf("failed to refresh oauth token: %w", err)
	}

	accessToken, err := h.newAccessToken(oauthToken.Spec.ProviderAccessToken)
	if err != nil {
		return err
	}

	refreshToken = strings.ToLower(rand.Text() + rand.Text())

	oauthToken = v1.OAuthToken{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: oauthClient.Namespace,
			Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
		},
		Spec: v1.OAuthTokenSpec{
			ClientID:             oauthClient.Name,
			Scope:                oauthToken.Spec.Scope,
			ProviderRefreshToken: oauthToken.Spec.ProviderRefreshToken,
			ProviderAccessToken:  oauthToken.Spec.ProviderAccessToken,
			ExpiresAt:            oauthToken.Spec.ExpiresAt,
		},
	}

	if err = req.Create(&oauthToken); err != nil {
		return fmt.Errorf("failed to create new oauth token: %w", err)
	}

	return req.Write(types.OAuthToken{
		AccessToken:  accessToken,
		TokenType:    "bearer",
		ExpiresIn:    int(time.Until(oauthToken.Spec.ExpiresAt.Time).Milliseconds() / 1000),
		Scope:        oauthClient.Spec.Manifest.Scope,
		RefreshToken: refreshToken,
	})
}

func (h *handler) newAccessToken(providerAccessToken string) (string, error) {
	now := time.Now()
	claims := tokenClaims{
		ProviderAccessToken: providerAccessToken,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    h.oauthConfig.Issuer,
			Subject:   "",
			Audience:  jwt.ClaimStrings{"mcp"},
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "obot-key"
	token.Header["jku"] = fmt.Sprintf("%s/.well-known/jwks.json", h.baseURL)

	return token.SignedString(h.key)
}

type tokenClaims struct {
	ProviderAccessToken string `json:"provider_access_token"`
	jwt.RegisteredClaims
}

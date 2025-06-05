package oauth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
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
			Description: fmt.Sprintf("grant_type must be one of %s, not %s", strings.Join(h.oauthConfig.GrantTypesSupported, ", "), grantType),
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

	return h.doRefreshToken(req, client, req.FormValue("refresh_token"), req.FormValue("scope"))
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

	accessToken, err := h.newAccessToken(oauthAuthRequest.Status.ProviderAccessToken, oauthAuthRequest.Status.ExpiresAt.Time)
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
			OAuthAppNamespace:    oauthAuthRequest.Status.OAuthAppNamespace,
			OAuthAppName:         oauthAuthRequest.Status.OAuthAppName,
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

func (h *handler) doRefreshToken(req api.Context, oauthClient v1.OAuthClient, refreshToken, scope string) error {
	if refreshToken == "" {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "refresh_token is required",
		})
	}

	var oauthToken v1.OAuthToken
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthClient.Namespace, Name: fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))}, &oauthToken); err != nil {
		return types.NewErrBadRequest("%v", Error{
			Code:        ErrInvalidRequest,
			Description: "refresh_token is invalid",
		})
	}

	var oauthApp v1.OAuthApp
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthToken.Spec.OAuthAppNamespace, Name: oauthToken.Spec.OAuthAppName}, &oauthApp); err != nil {
		return err
	}

	var status v1.OAuthAuthRequestStatus
	if oauthToken.Spec.ProviderRefreshToken != "" {
		var clientSecret string
		// Reveal the credential to get the client secret.
		cred, err := h.gptClient.RevealCredential(req.Context(), []string{oauthApp.Name}, oauthApp.Spec.Manifest.Alias)
		if err != nil {
			return fmt.Errorf("failed to reveal credential: %w", err)
		}

		clientSecret = cred.Env["CLIENT_SECRET"]

		data := url.Values{}
		data.Set("client_id", oauthApp.Spec.Manifest.ClientID)
		data.Set("client_secret", clientSecret)
		if oauthApp.Spec.Manifest.Type != types.OAuthAppTypeSalesforce && oauthApp.Spec.Manifest.Type != types.OAuthAppTypeSmartThings {
			data.Set("scope", scope)
		}
		if oauthApp.Spec.Manifest.Type != types.OAuthAppTypeSmartThings {
			data.Set("redirect_uri", fmt.Sprintf("%s/oauth/callback", h.baseURL))
		}
		data.Set("refresh_token", oauthToken.Spec.ProviderRefreshToken)
		data.Set("grant_type", "refresh_token")

		r, err := http.NewRequest("POST", oauthApp.Spec.Manifest.TokenURL, bytes.NewBufferString(data.Encode()))
		if err != nil {
			return fmt.Errorf("failed to make token request: %w", err)
		}
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if oauthApp.Spec.Manifest.Type == types.OAuthAppTypeSmartThings {
			encodedAuth := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", oauthApp.Spec.Manifest.ClientID, clientSecret)))
			r.Header.Set("Authorization", fmt.Sprintf("Basic %s", encodedAuth))
		}
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			return fmt.Errorf("failed to make token request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBuf := new(bytes.Buffer)
			_, _ = bodyBuf.ReadFrom(resp.Body)
			return fmt.Errorf("failed to get tokens: %d %s", resp.StatusCode, bodyBuf.String())
		}

		switch oauthApp.Spec.Manifest.Type {
		case types.OAuthAppTypeSalesforce:
			salesforceTokenResp := new(salesforceOAuthTokenResponse)
			if err = json.NewDecoder(resp.Body).Decode(salesforceTokenResp); err != nil {
				return fmt.Errorf("failed to parse token response: %w", err)
			}
			issuedAt, err := strconv.ParseInt(salesforceTokenResp.IssuedAt, 10, 64)
			if err != nil {
				return fmt.Errorf("failed to parse token response: %w", err)
			}
			createdAt := time.Unix(issuedAt/1000, (issuedAt%1000)*1000000)

			status = v1.OAuthAuthRequestStatus{
				ProviderAccessToken:    salesforceTokenResp.AccessToken,
				ExpiresAt:              metav1.NewTime(createdAt.Add(time.Second * 7200)), // Relies on Salesforce admin not overriding the default 2 hours
				Ok:                     true,                                              // Assuming true if no error is present
				ProviderTokenCreatedAt: metav1.NewTime(createdAt),
				ProviderRefreshToken:   salesforceTokenResp.RefreshToken,
				Data: map[string]string{
					"salesforce_instance_url": salesforceTokenResp.InstanceURL,
				},
				Scope: salesforceTokenResp.Scope,
			}
		case types.OAuthAppTypeGoogle:
			var tokenResp tokenResponse
			if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
				return fmt.Errorf("failed to parse token response: %w", err)
			}

			status = v1.OAuthAuthRequestStatus{
				ProviderAccessToken:    tokenResp.AccessToken,
				ExpiresAt:              metav1.NewTime(time.Unix(tokenResp.ExpiresIn, 0)),
				Ok:                     true, // Assuming true if no error is present
				ProviderTokenCreatedAt: metav1.NewTime(tokenResp.CreatedAt),
				ProviderRefreshToken:   tokenResp.RefreshToken,
				Scope:                  tokenResp.Scope,
			}
		case types.OAuthAppTypeGitLab:
			var tokenResp tokenResponse
			// For GitLab, decode the standard token response and then add the base URL to extras
			if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
				return fmt.Errorf("failed to parse token response: %w", err)
			}

			status = v1.OAuthAuthRequestStatus{
				ProviderAccessToken:    tokenResp.AccessToken,
				ExpiresAt:              metav1.NewTime(time.Unix(tokenResp.ExpiresIn, 0)),
				Ok:                     true, // Assuming true if no error is present
				ProviderTokenCreatedAt: metav1.NewTime(tokenResp.CreatedAt),
				ProviderRefreshToken:   tokenResp.RefreshToken,
				Scope:                  tokenResp.Scope,
			}

			// Add GitLab base URL to extras if it's a custom instance
			if oauthApp.Spec.Manifest.GitLabBaseURL != "" {
				if status.Data == nil {
					status.Data = make(map[string]string, 1)
				}
				status.Data["gitlab_base_url"] = oauthApp.Spec.Manifest.GitLabBaseURL
			}
		default:
			var tokenResp tokenResponse
			if err = json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
				return fmt.Errorf("failed to parse token response: %w", err)
			}

			status = v1.OAuthAuthRequestStatus{
				ProviderAccessToken:    tokenResp.AccessToken,
				ProviderRefreshToken:   tokenResp.RefreshToken,
				ExpiresAt:              metav1.NewTime(time.Unix(tokenResp.ExpiresIn, 0)),
				Ok:                     true, // Assuming true if no error is present
				ProviderTokenCreatedAt: metav1.NewTime(tokenResp.CreatedAt),
				Scope:                  tokenResp.Scope,
			}
		}

		if status.ProviderRefreshToken == "" {
			status.ProviderRefreshToken = oauthToken.Spec.ProviderRefreshToken
		}
	}

	expiresAt := status.ExpiresAt.Time
	if expiresAt.IsZero() {
		expiresAt = oauthToken.Spec.ExpiresAt.Time
	}
	accessToken, err := h.newAccessToken(oauthToken.Spec.ProviderAccessToken, expiresAt)
	if err != nil {
		return err
	}

	if err = req.Delete(&oauthToken); err != nil {
		return fmt.Errorf("failed to refresh oauth token: %w", err)
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
			ExpiresAt:            metav1.NewTime(expiresAt),
			OAuthAppNamespace:    oauthApp.Namespace,
			OAuthAppName:         oauthApp.Name,
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

func (h *handler) newAccessToken(providerAccessToken string, expiresAt time.Time) (string, error) {
	now := time.Now()
	claims := tokenClaims{
		ProviderAccessToken: providerAccessToken,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    h.oauthConfig.Issuer,
			Subject:   "",
			Audience:  jwt.ClaimStrings{"mcp"},
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	if !expiresAt.IsZero() {
		claims.ExpiresAt = jwt.NewNumericDate(expiresAt)
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

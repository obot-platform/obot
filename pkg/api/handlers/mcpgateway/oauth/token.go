package oauth

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/api/authz"
	"github.com/obot-platform/obot/pkg/jwt/persistent"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/obot-platform/obot/pkg/storage/selectors"
	"github.com/obot-platform/obot/pkg/utils"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/fields"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	tokenExpiration = 10 * time.Minute
	obotErrorSource = "Obot"
	obotErrorPrefix = obotErrorSource + ": "
)

func (h *handler) token(req api.Context) (err error) {
	if err := req.ParseForm(); err != nil {
		return types.NewErrBadRequest("failed to parse request body: %v", err)
	}

	var clientSecret string
	clientID := req.FormValue("client_id")
	if clientID == "" {
		creds := strings.TrimPrefix(req.Request.Header.Get("Authorization"), "Basic ")
		if creds != "" {
			c, err := base64.StdEncoding.DecodeString(creds)
			if err != nil {
				slog.Info("Denied OAuth token request due to invalid basic auth encoding")
				return newInvalidClientErr("invalid client credentials")
			}

			idx := bytes.LastIndex(c, []byte{':'})
			if idx == -1 {
				slog.Info("Denied OAuth token request due to malformed basic auth credentials")
				return newInvalidClientErr("invalid client credentials")
			}

			clientID, clientSecret = string(c[:idx]), string(c[idx+1:])
			if clientID == "" {
				return newInvalidClientErr("client_id is required")
			}

			clientID, err = url.QueryUnescape(clientID)
			if err != nil {
				return newOAuthErrHTTP(http.StatusBadRequest, newOAuthError(ErrInvalidClient, "client_id is invalid", ""))
			}
		}
	} else {
		clientSecret = req.FormValue("client_secret")
	}

	if clientID == "" && req.FormValue("client_assertion_type") == clientAssertionTypeJWTBearer {
		var err error
		clientID, err = clientIDFromClientAssertion(req.Form)
		if err != nil {
			return newInvalidClientErr(err.Error())
		}
	}

	if clientID == "" {
		slog.Info("Denied OAuth token request due to missing client credentials")
		return newInvalidClientErr("invalid client credentials")
	}

	client, err := h.resolveOAuthClient(req.Context(), req.Storage, clientID)
	if err != nil {
		if oauthErr, ok := errors.AsType[oauthError](err); ok {
			if oauthErr.Code == ErrInvalidClient {
				return newOAuthErrHTTP(http.StatusUnauthorized, oauthErr)
			}
			return newOAuthErrHTTP(http.StatusBadRequest, oauthErr)
		}
		return err
	}

	switch client.Spec.Manifest.TokenEndpointAuthMethod {
	case "client_secret_basic", "client_secret_post":
		if bcrypt.CompareHashAndPassword(client.Spec.ClientSecretHash, []byte(clientSecret)) != nil {
			slog.Info("Denied OAuth token request due to invalid client secret", "clientNamespace", client.Namespace, "clientName", client.Name)
			return newInvalidClientErr("invalid client credentials")
		}
	case "private_key_jwt":
		if err := h.validatePrivateKeyJWT(req.Context(), req.Form, client, clientID); err != nil {
			slog.Info("Denied OAuth token request due to invalid private_key_jwt client assertion", "clientNamespace", client.Namespace, "clientName", client.Name, "error", err)
			return newInvalidClientErr(err.Error())
		}
	}

	grantType := req.FormValue("grant_type")
	if !slices.Contains(h.oauthConfig.GrantTypesSupported, grantType) {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, fmt.Sprintf("grant_type must be one of %s, not %s", strings.Join(h.oauthConfig.GrantTypesSupported, ", "), grantType), ""))
	}

	if len(client.Spec.Manifest.GrantTypes) > 0 && !slices.Contains(client.Spec.Manifest.GrantTypes, grantType) || len(client.Spec.Manifest.GrantTypes) == 0 && grantType != "authorization_code" {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidClient, "client is not allowed to use authorization_code grant type", ""))
	}
	slog.Debug("Processing OAuth token request", "clientNamespace", client.Namespace, "clientName", client.Name, "grantType", grantType)

	switch grantType {
	case "authorization_code":
		return h.doAuthorizationCode(req, client, req.FormValue("code"), req.FormValue("code_verifier"))
	case "refresh_token":
		return h.doRefreshToken(req, client, req.FormValue("refresh_token"))
	default:
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, fmt.Sprintf("grant_type must be one of %s, not %s", strings.Join(h.oauthConfig.GrantTypesSupported, ", "), grantType), ""))
	}
}

func (h *handler) doAuthorizationCode(req api.Context, oauthClient v1.OAuthClient, code, codeVerifier string) error {
	if code == "" {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "code is required", ""))
	}

	var oauthAuthRequestList v1.OAuthAuthRequestList
	if err := req.Storage.List(req.Context(), &oauthAuthRequestList, &kclient.ListOptions{
		FieldSelector: fields.SelectorFromSet(selectors.RemoveEmpty(map[string]string{
			"spec.hashedAuthCode": fmt.Sprintf("%x", sha256.Sum256([]byte(code))),
		})),
	}); err != nil {
		return err
	}
	if len(oauthAuthRequestList.Items) != 1 {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "code is invalid", ""))
	}

	oauthAuthRequest := oauthAuthRequestList.Items[0]
	if oauthAuthRequest.Spec.ClientID != oauthClient.Name {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "code is invalid", ""))
	}

	// Authorization codes are one-time use
	if err := req.Storage.Delete(req.Context(), &oauthAuthRequest); err != nil {
		// Don't return an error if we can't delete the auth request
		slog.Warn("failed to delete auth request", "error", err)
	}

	if oauthAuthRequest.Spec.CodeChallenge != "" {
		switch oauthAuthRequest.Spec.CodeChallengeMethod {
		case "S256":
			hashedCodeVerifier := sha256.Sum256([]byte(codeVerifier))
			if oauthAuthRequest.Spec.CodeChallenge != base64.RawURLEncoding.EncodeToString(hashedCodeVerifier[:]) {
				return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "code_verifier is invalid", ""))
			}
		case "plain":
			if oauthAuthRequest.Spec.CodeChallenge != codeVerifier {
				return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "code_verifier is invalid", ""))
			}
		default:
			return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "code_challenge_method must be S256 or plain.", ""))
		}
	}

	userID := fmt.Sprintf("%d", oauthAuthRequest.Spec.UserID)
	user, err := req.GatewayClient.UserByID(req.Context(), userID)
	if err != nil {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "invalid user", ""))
	}

	now := time.Now()
	tknCtx := persistent.TokenContext{
		Audience:              oauthAuthRequest.Spec.Resource,
		OAuthScope:            oauthAuthRequest.Spec.Scope,
		IssuedAt:              persistent.NewTime(now),
		ExpiresAt:             persistent.NewTime(now.Add(tokenExpiration)),
		UserID:                userID,
		UserName:              user.Username,
		UserEmail:             user.Email,
		Picture:               user.IconURL,
		UserGroups:            []string{types.GroupMCP, types.GroupAuthenticated},
		AuthProviderName:      oauthAuthRequest.Spec.AuthProviderName,
		AuthProviderNamespace: oauthAuthRequest.Spec.AuthProviderNamespace,
		AuthProviderUserID:    oauthAuthRequest.Spec.AuthProviderUserID,
		MCPID:                 oauthAuthRequest.Spec.MCPID,
	}
	_, tkn, err := h.tokenService.NewToken(req.Context(), tknCtx)
	if err != nil {
		return fmt.Errorf("failed to create auth token: %w", err)
	}

	refreshToken := strings.ToLower(rand.Text() + rand.Text())

	oauthToken := v1.OAuthToken{
		Namespace: oauthClient.Namespace,
		Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
		Spec: v1.OAuthTokenSpec{
			ClientID:              oauthClient.Name,
			Resource:              oauthAuthRequest.Spec.Resource,
			UserID:                oauthAuthRequest.Spec.UserID,
			Scope:                 oauthAuthRequest.Spec.Scope,
			AuthProviderNamespace: oauthAuthRequest.Spec.AuthProviderNamespace,
			AuthProviderName:      oauthAuthRequest.Spec.AuthProviderName,
			AuthProviderUserID:    oauthAuthRequest.Spec.AuthProviderUserID,
			MCPID:                 oauthAuthRequest.Spec.MCPID,
		},
	}

	if err = req.Create(&oauthToken); err != nil {
		return fmt.Errorf("failed to create oauth token: %w", err)
	}
	slog.Info("Issued OAuth access and refresh token via authorization_code", "client", oauthClient.Name, "userID", oauthAuthRequest.Spec.UserID, "mcpID", oauthAuthRequest.Spec.MCPID)

	return req.Write(types.OAuthToken{
		AccessToken:  tkn,
		TokenType:    "bearer",
		ExpiresIn:    int(time.Until(tknCtx.ExpiresAt.Time).Milliseconds() / 1000),
		RefreshToken: refreshToken,
	})
}

func (h *handler) doRefreshToken(req api.Context, oauthClient v1.OAuthClient, refreshToken string) error {
	if refreshToken == "" {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "refresh_token is required", ""))
	}

	var oauthToken v1.OAuthToken
	if err := req.Storage.Get(req.Context(), kclient.ObjectKey{Namespace: oauthClient.Namespace, Name: fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken)))}, &oauthToken); err != nil {
		if apierrors.IsNotFound(err) {
			return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidGrant, "refresh_token is invalid", ""))
		}
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidRequest, "refresh_token is invalid", ""))
	}
	if oauthToken.Spec.ClientID != oauthClient.Name {
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidGrant, "refresh_token is invalid", ""))
	}

	// Consume terminally invalid grants so they cannot become usable again if the referenced resource is recreated.
	invalidGrant := func(description string) error {
		if err := req.Storage.Delete(req.Context(), &oauthToken); apierrors.IsNotFound(err) {
			return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidGrant, "refresh_token is invalid", ""))
		} else if err != nil {
			return fmt.Errorf("failed to invalidate oauth token: %w", err)
		}
		return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidGrant, description, ""))
	}

	user, err := req.GatewayClient.UserInfoByID(req.Context(), oauthToken.Spec.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return invalidGrant("invalid user")
		}
		return newOAuthError(ErrServerError, fmt.Sprintf("failed to retrieve user: %v", err), "")
	}

	allowed, err := authz.CheckMCPIDAccess(req.Context(), req.Storage, h.acrHelper, user, oauthToken.Spec.MCPID)
	if apierrors.IsNotFound(err) {
		return invalidGrant("invalid MCP server")
	} else if err != nil {
		return newOAuthError(ErrServerError, fmt.Sprintf("failed to check access to MCP server: %v", err), "")
	}
	if !allowed {
		return invalidGrant("invalid MCP server")
	}

	if err := req.Storage.Delete(req.Context(), &oauthToken); err != nil {
		if apierrors.IsNotFound(err) {
			return types.NewErrBadRequest("%v", newOAuthError(ErrInvalidGrant, "refresh_token is invalid", ""))
		}
		return fmt.Errorf("failed to refresh oauth token: %w", err)
	}

	now := time.Now()
	tknCtx := persistent.TokenContext{
		OAuthScope:            oauthToken.Spec.Scope,
		Audience:              oauthToken.Spec.Resource,
		IssuedAt:              persistent.NewTime(now),
		ExpiresAt:             persistent.NewTime(now.Add(tokenExpiration)),
		UserID:                user.GetUID(),
		UserName:              user.GetName(),
		UserEmail:             utils.FirstSet(user.GetExtra()["email"]...),
		UserGroups:            []string{types.GroupMCP, types.GroupAuthenticated},
		AuthProviderName:      oauthToken.Spec.AuthProviderName,
		AuthProviderNamespace: oauthToken.Spec.AuthProviderNamespace,
		AuthProviderUserID:    oauthToken.Spec.AuthProviderUserID,
		MCPID:                 oauthToken.Spec.MCPID,
	}
	_, tkn, err := h.tokenService.NewToken(req.Context(), tknCtx)
	if err != nil {
		return fmt.Errorf("failed to create auth token: %w", err)
	}

	refreshToken = strings.ToLower(rand.Text() + rand.Text())

	oauthToken = v1.OAuthToken{
		Namespace: oauthClient.Namespace,
		Name:      fmt.Sprintf("%x", sha256.Sum256([]byte(refreshToken))),
		Spec: v1.OAuthTokenSpec{
			Scope:                 oauthToken.Spec.Scope,
			Resource:              oauthToken.Spec.Resource,
			ClientID:              oauthClient.Name,
			UserID:                oauthToken.Spec.UserID,
			AuthProviderNamespace: oauthToken.Spec.AuthProviderNamespace,
			AuthProviderName:      oauthToken.Spec.AuthProviderName,
			AuthProviderUserID:    oauthToken.Spec.AuthProviderUserID,
			MCPID:                 oauthToken.Spec.MCPID,
		},
	}

	if err = req.Create(&oauthToken); err != nil {
		return fmt.Errorf("failed to create new oauth token: %w", err)
	}
	slog.Info("Issued OAuth access and refresh token via refresh_token", "client", oauthClient.Name, "userID", oauthToken.Spec.UserID, "mcpID", oauthToken.Spec.MCPID)

	return req.Write(types.OAuthToken{
		AccessToken:  tkn,
		TokenType:    "bearer",
		ExpiresIn:    int(time.Until(tknCtx.ExpiresAt.Time).Milliseconds() / 1000),
		RefreshToken: refreshToken,
	})
}

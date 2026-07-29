package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

type tokenRequestRequest struct {
	Name              string             `json:"name"`
	Description       string             `json:"description"`
	ProviderName      string             `json:"providerName"`
	ProviderNamespace string             `json:"providerNamespace"`
	NoExpiration      bool               `json:"noExpiration"`
	Scopes            types.APIKeyScopes `json:"scopes"`
}

type tokenRequestResponse struct {
	ID         string `json:"id"`
	TokenPath  string `json:"token-path"`
	DeviceCode string `json:"device-code"`
}

type verifyDeviceCodeRequest struct {
	Code string `json:"code"`
}

type refreshTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}

func (s *Server) getTokens(apiContext api.Context) error {
	tokens, err := apiContext.GatewayClient.ListAuthTokens(apiContext.Context(), apiContext.UserID())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("error getting tokens: %v", err))
	}
	pkgLog.Infof("Listed auth tokens for user: userID=%d tokens=%d", apiContext.UserID(), len(tokens))

	return apiContext.Write(tokens)
}

func (s *Server) deleteToken(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	if id == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "id path parameter is required")
	}

	if err := apiContext.GatewayClient.DeleteAuthToken(apiContext.Context(), apiContext.UserID(), id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
			err = fmt.Errorf("not found")
		}
		return types2.NewErrHTTP(status, fmt.Sprintf("error deleting token: %v", err))
	}
	pkgLog.Infof("Deleted auth token for user: userID=%d tokenID=%s", apiContext.UserID(), id)

	return apiContext.Write(map[string]any{"deleted": true})
}

func (s *Server) tokenRequest(apiContext api.Context) error {
	reqObj := new(tokenRequestRequest)
	if err := json.NewDecoder(apiContext.Request.Body).Decode(reqObj); err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid token request body: %v", err))
	}

	if reqObj.ProviderName == "" || reqObj.ProviderNamespace == "" {
		return types2.NewErrHTTP(http.StatusBadRequest, "provider name and namespace are required")
	}
	configuredProvider, err := s.dispatcher.GetConfiguredAuthProvider(apiContext.Context())
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get configured auth provider: %v", err))
	}
	if configuredProvider != reqObj.ProviderName {
		pkgLog.Infof("Rejected token request due to unconfigured auth provider: requestedProvider=%s configuredProvider=%s", reqObj.ProviderName, configuredProvider)
		return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("auth provider %q not found", reqObj.ProviderName))
	}

	tokenReq := &types.TokenRequest{
		Name:         reqObj.Name,
		Description:  reqObj.Description,
		NoExpiration: reqObj.NoExpiration,
		Scopes:       reqObj.Scopes,
	}

	deviceCode, err := apiContext.GatewayClient.CreateDeviceTokenRequest(apiContext.Context(), tokenReq)
	if err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, "failed to create token request")
	}
	pkgLog.Infof("Created token request for auth flow: tokenRequestID=%s provider=%s/%s noExpiration=%v", tokenReq.ID, reqObj.ProviderNamespace, reqObj.ProviderName, reqObj.NoExpiration)

	return apiContext.Write(tokenRequestResponse{
		ID: tokenReq.ID,
		TokenPath: fmt.Sprintf("%s/oauth2/start?rd=%s&obot-auth-provider=%s",
			s.baseURL,
			url.QueryEscape("/auth/device-code"),
			url.QueryEscape(fmt.Sprintf("%s/%s", reqObj.ProviderNamespace, reqObj.ProviderName)),
		),
		DeviceCode: deviceCode,
	})
}

func (s *Server) verifyDeviceCode(apiContext api.Context) error {
	input := new(verifyDeviceCodeRequest)
	if err := apiContext.Read(input); err != nil {
		return types2.NewErrBadRequest("invalid request body: %v", err)
	}

	if err := apiContext.GatewayClient.AuthorizeTokenRequestByDeviceCode(apiContext.Context(), apiContext.UserID(), input.Code); err != nil {
		if errors.Is(err, client.ErrInvalidOrExpiredDeviceCode) {
			return types2.NewErrBadRequest("%s", client.ErrInvalidOrExpiredDeviceCode.Error())
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, "failed to authorize device code")
	}

	return apiContext.Write(map[string]bool{"authorized": true})
}

func (s *Server) redirectForTokenRequest(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	namespace := apiContext.PathValue("namespace")
	name := apiContext.PathValue("name")

	if namespace != "" && name != "" {
		configuredProvider, err := s.dispatcher.GetConfiguredAuthProvider(apiContext.Context())
		if err != nil {
			return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("failed to get configured auth provider: %v", err))
		}
		if configuredProvider != name {
			pkgLog.Infof("Rejected redirect-for-token request due to unconfigured auth provider: requestedProvider=%s configuredProvider=%s tokenRequestID=%s", name, configuredProvider, id)
			return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("auth provider %q not found", name))
		}
	}

	tokenReq, err := apiContext.GatewayClient.GetSetupTokenRequest(apiContext.Context(), id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("token not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, err.Error())
	}
	if tokenReq.RequestExpiresAt.IsZero() || !time.Now().Before(tokenReq.RequestExpiresAt) {
		return types2.NewErrNotFound("token not found")
	}
	pkgLog.Infof("Resolved token request redirect path: tokenRequestID=%s provider=%s/%s", tokenReq.ID, namespace, name)

	return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/api/oauth/start/%s/%s/%s", s.baseURL, tokenReq.ID, namespace, name)})
}

func (s *Server) checkForToken(apiContext api.Context) error {
	tr, err := apiContext.GatewayClient.PollTokenRequest(apiContext.Context(), apiContext.PathValue("id"))
	if err != nil || tr.ID == "" {
		return types2.NewErrNotFound("not found")
	}

	if tr.Error != "" {
		pkgLog.Infof("Token request completed with error: tokenRequestID=%s", tr.ID)
		return apiContext.Write(map[string]any{"error": tr.Error})
	}
	if tr.Token == "" && !tr.RequestExpiresAt.IsZero() && !time.Now().Before(tr.RequestExpiresAt) {
		pkgLog.Infof("Token request expired: tokenRequestID=%s", tr.ID)
		return apiContext.Write(map[string]any{"error": "token request expired"})
	}

	if tr.Token == "" {
		pkgLog.Debugf("Token request polled: tokenRequestID=%s tokenAvailable=%v tokenRetrieved=%v", tr.ID, false, tr.TokenRetrieved)
	} else {
		pkgLog.Infof("Token request polled: tokenRequestID=%s tokenAvailable=%v tokenRetrieved=%v", tr.ID, true, tr.TokenRetrieved)
	}
	return apiContext.Write(refreshTokenResponse{
		Token:     tr.Token,
		ExpiresAt: tr.ExpiresAt,
	})
}

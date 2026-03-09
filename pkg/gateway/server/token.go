package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	types2 "github.com/obot-platform/obot/apiclient/types"
	loggerpkg "github.com/obot-platform/obot/logger"
	"github.com/obot-platform/obot/pkg/api"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tokenRequestRequest struct {
	ID                    string `json:"id"`
	ProviderName          string `json:"providerName"`
	ProviderNamespace     string `json:"providerNamespace"`
	CompletionRedirectURL string `json:"completionRedirectURL"`
	NoExpiration          bool   `json:"noExpiration"`
}

type refreshTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt,omitzero"`
}

var tokenLog = loggerpkg.Package()

func (s *Server) getTokens(apiContext api.Context) error {
	var tokens []types.AuthToken
	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ?", apiContext.UserID()).Find(&tokens).Error; err != nil {
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

	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ? AND id = ?", apiContext.UserID(), id).Delete(new(types.AuthToken)).Error; err != nil {
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
		ID:                    reqObj.ID,
		CompletionRedirectURL: reqObj.CompletionRedirectURL,
		NoExpiration:          reqObj.NoExpiration,
	}

	if err := s.db.WithContext(apiContext.Context()).Create(tokenReq).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return types2.NewErrHTTP(http.StatusConflict, "token request already exists")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, err.Error())
	}
	pkgLog.Infof("Created token request for auth flow: tokenRequestID=%s provider=%s/%s noExpiration=%v", tokenReq.ID, reqObj.ProviderNamespace, reqObj.ProviderName, reqObj.NoExpiration)

	return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/api/oauth/start/%s/%s/%s", s.baseURL, reqObj.ID, reqObj.ProviderNamespace, reqObj.ProviderName)})
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

	tokenReq := new(types.TokenRequest)
	if err := s.db.WithContext(apiContext.Context()).Where("id = ?", id).First(tokenReq).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("token not found")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, err.Error())
	}
	pkgLog.Infof("Resolved token request redirect path: tokenRequestID=%s provider=%s/%s", tokenReq.ID, namespace, name)

	return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/api/oauth/start/%s/%s/%s", s.baseURL, tokenReq.ID, namespace, name)})
}

func (s *Server) checkForToken(apiContext api.Context) error {
	tr := new(types.TokenRequest)
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", apiContext.PathValue("id")).First(tr).Error; err != nil {
			return err
		}

		if tr.Token != "" && !tr.TokenRetrieved {
			return tx.Model(tr).Where("id = ?", tr.ID).Update("token_retrieved", true).Error
		}
		return nil
	}); err != nil || tr.ID == "" {
		return types2.NewErrNotFound("not found")
	}

	if tr.Error != "" {
		pkgLog.Infof("Token request completed with error: tokenRequestID=%s", tr.ID)
		return apiContext.Write(map[string]any{"error": tr.Error})
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

func (s *Server) createState(ctx context.Context, id string) (string, error) {
	state := strings.ReplaceAll(uuid.NewString(), "-", "")

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tr := new(types.TokenRequest)
		if err := tx.Where("id = ?", id).First(tr).Error; err != nil {
			return err
		}

		return tx.Model(tr).Updates(map[string]any{"state": state, "error": ""}).Error
	}); err != nil {
		return "", fmt.Errorf("failed to create state: %w", err)
	}
	pkgLog.Infof("Created OAuth state for token request: tokenRequestID=%s", id)

	return state, nil
}

func (s *Server) verifyState(ctx context.Context, state string) (*types.TokenRequest, error) {
	tr := new(types.TokenRequest)
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("state = ?", state).First(tr).Error; err != nil {
			if tr.ID == "" {
				return err
			}
			tr.Error = err.Error()
		}

		return tx.Model(tr).Clauses(clause.Returning{}).Updates(map[string]any{"state": "", "error": tr.Error}).Error
	})
	pkgLog.Infof("Verified OAuth state for token request: tokenRequestID=%s success=%v", tr.ID, err == nil)
	return tr, err
}

// autoCleanupTokens will delete token requests that have been retrieved and are older than the cleanupTick.
// It will also delete tokens that are older than 2 minutes that have not been retrieved.
// Finally, tokens that are older than the expiration duration and deleted.
func (s *Server) autoCleanupTokens(ctx context.Context) {
	cleanupTick := 30 * time.Second
	timer := time.NewTimer(cleanupTick)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		var (
			errs []error
			now  = time.Now()
		)
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			errs = append(errs, tx.Where("created_at < ?", now.Add(-2*time.Minute)).Where("token_retrieved = ?", false).Delete(new(types.TokenRequest)).Error)
			errs = append(errs, tx.Where("token_retrieved = ?", true).Where("updated_at < ?", time.Now().Add(-cleanupTick)).Delete(new(types.TokenRequest)).Error)
			errs = append(errs, tx.Where("no_expiration = ?", false).Where("expires_at < ?", now).Delete(new(types.AuthToken)).Error)
			return errors.Join(errs...)
		}); err != nil {
			tokenLog.Errorf("error cleaning up state: error=%v", err)
		}

		timer.Reset(cleanupTick)
	}
}

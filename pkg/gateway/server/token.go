package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	types2 "github.com/acorn-io/acorn/apiclient/types"
	"github.com/acorn-io/acorn/pkg/api"
	kcontext "github.com/acorn-io/acorn/pkg/gateway/context"
	ktime "github.com/acorn-io/acorn/pkg/gateway/time"
	"github.com/acorn-io/acorn/pkg/gateway/types"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	randomTokenLength = 32
	tokenIDLength     = 8
)

type tokenRequestRequest struct {
	ID                    string `json:"id"`
	ServiceName           string `json:"serviceName"`
	CompletionRedirectURL string `json:"completionRedirectURL"`
}

type refreshTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func (s *Server) getTokens(apiContext api.Context) error {
	var tokens []types.AuthToken
	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ?", apiContext.UserID()).Find(&tokens).Error; err != nil {
		return types2.NewErrHttp(http.StatusInternalServerError, fmt.Sprintf("error getting tokens: %v", err))
	}

	return apiContext.Write(tokens)
}

func (s *Server) deleteToken(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	if id == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "id path parameter is required")
	}

	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ? AND id = ?", apiContext.UserID(), id).Delete(new(types.AuthToken)).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
			err = fmt.Errorf("not found")
		}
		return types2.NewErrHttp(status, fmt.Sprintf("error deleting token: %v", err))
	}

	return apiContext.Write(map[string]any{"deleted": true})
}

type createTokenRequest struct {
	ExpiresIn string `json:"expiresIn"`
}

func (s *Server) newToken(apiContext api.Context) error {
	authProviderID := apiContext.AuthProviderID()
	userID := apiContext.UserID()
	if authProviderID <= 0 || userID <= 0 {
		return types2.NewErrHttp(http.StatusForbidden, "forbidden")
	}

	var customExpiration time.Duration
	if apiContext.ContentLength != 0 {
		request := new(createTokenRequest)
		err := apiContext.Read(request)
		if err != nil {
			return types2.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("invalid create create token request body: %v", err))
		}

		if request.ExpiresIn != "" {
			customExpiration, err = ktime.ParseDuration(request.ExpiresIn)
			if err != nil {
				return types2.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("invalid expiresIn duration: %v", err))
			}
		}
	}

	randBytes := make([]byte, randomTokenLength+tokenIDLength)
	if _, err := rand.Read(randBytes); err != nil {
		return types2.NewErrHttp(http.StatusInternalServerError, fmt.Sprintf("error refreshing token: %v", err))
	}

	id := randBytes[:tokenIDLength]
	token := randBytes[tokenIDLength:]

	tkn := &types.AuthToken{
		ID:          fmt.Sprintf("%x", id),
		UserID:      userID,
		HashedToken: hashToken(fmt.Sprintf("%x", token)),
		ExpiresAt:   time.Now().Add(customExpiration),
	}

	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		provider := new(types.AuthProvider)
		if err := tx.Where("id = ?", authProviderID).First(provider).Error; err != nil {
			return fmt.Errorf("error refreshing token: %v", err)
		}

		if customExpiration == 0 {
			tkn.ExpiresAt = time.Now().Add(provider.ExpirationDur)
		}
		tkn.AuthProviderID = provider.ID
		return tx.Create(tkn).Error
	}); err != nil {
		return types2.NewErrHttp(http.StatusInternalServerError, fmt.Sprintf("error refreshing token: %v", err))
	}

	return apiContext.Write(refreshTokenResponse{
		Token:     publicToken(id, token),
		ExpiresAt: tkn.ExpiresAt,
	})
}

func (s *Server) tokenRequest(apiContext api.Context) error {
	reqObj := new(tokenRequestRequest)
	if err := json.NewDecoder(apiContext.Request.Body).Decode(reqObj); err != nil {
		return types2.NewErrHttp(http.StatusBadRequest, fmt.Sprintf("invalid token request body: %v", err))
	}

	tokenReq := &types.TokenRequest{
		ID:                    reqObj.ID,
		CompletionRedirectURL: reqObj.CompletionRedirectURL,
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if reqObj.ServiceName != "" {
			// Ensure the OAuth provider exists, if one was provided.
			if err := tx.Where("service_name = ?", reqObj.ServiceName).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
				return fmt.Errorf("failed to find oauth provider %q: %v", reqObj.ServiceName, err)
			}
		}

		return tx.Create(tokenReq).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return types2.NewErrHttp(http.StatusConflict, "token request already exists")
		}
		return types2.NewErrHttp(http.StatusInternalServerError, err.Error())
	}

	if reqObj.ServiceName != "" {
		return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/api/oauth/start/%s/%s", s.baseURL, reqObj.ID, oauthProvider.Slug)})
	}
	return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/login?id=%s", s.uiURL, reqObj.ID)})
}

func (s *Server) redirectForTokenRequest(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	service := apiContext.PathValue("service")

	oauthProvider := new(types.AuthProvider)
	tokenReq := new(types.TokenRequest)
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		// Ensure the OAuth provider exists, if one was provided.
		if err := tx.Where("slug = ?", service).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
			return fmt.Errorf("failed to find oauth provider %q: %v", service, err)
		}

		return tx.Where("id = ?", id).First(tokenReq).Error
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return types2.NewErrNotFound("token or service not found")
		}
		return types2.NewErrHttp(http.StatusInternalServerError, err.Error())
	}

	return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/api/oauth/start/%s/%s", s.baseURL, tokenReq.ID, oauthProvider.Slug)})
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
		return apiContext.Write(map[string]any{"error": tr.Error})
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

	return state, nil
}

func (s *Server) verifyState(ctx context.Context, state string) (*types.TokenRequest, error) {
	tr := new(types.TokenRequest)
	return tr, s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("state = ?", state).First(tr).Error; err != nil {
			if tr.ID == "" {
				return err
			}
			tr.Error = err.Error()
		}

		return tx.Model(tr).Clauses(clause.Returning{}).Updates(map[string]any{"state": "", "error": tr.Error}).Error
	})
}

func (s *Server) errorToken(ctx context.Context, tr *types.TokenRequest, code int, err error) error {
	if tr != nil {
		tr.Error = err.Error()
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Updates(tr).Error
		}); err != nil {
			kcontext.GetLogger(ctx).ErrorContext(ctx, "failed to update token", "id", tr.ID, "error", err)
		}
	}

	return types2.NewErrHttp(code, err.Error())
}

// autoCleanupTokens will delete token requests that have been retrieved and are older than the cleanupTick.
// It will also delete tokens that are older than 2 minutes that have not been retrieved.
// Finally, tokens that are older than the expiration duration and deleted.
func (s *Server) autoCleanupTokens(ctx context.Context) {
	db := s.db.WithContext(ctx)
	cleanupTick := 5 * time.Second
	timer := time.NewTimer(cleanupTick)
	for {
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}

		var (
			errs []error
			now  = time.Now()
		)
		if err := db.Transaction(func(tx *gorm.DB) error {
			errs = append(errs, tx.Where("created_at < ?", now.Add(-2*time.Minute)).Where("token_retrieved = ?", false).Delete(new(types.TokenRequest)).Error)
			errs = append(errs, tx.Where("token_retrieved = ?", true).Where("updated_at < ?", time.Now().Add(-cleanupTick)).Delete(new(types.TokenRequest)).Error)
			errs = append(errs, tx.Where("expires_at < ?", now).Delete(new(types.AuthToken)).Error)
			return errors.Join(errs...)
		}); err != nil {
			slog.ErrorContext(ctx, "error cleaning up state", "error", err)
		}

		timer.Reset(cleanupTick)
	}
}

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

	"github.com/google/uuid"
	"github.com/gptscript-ai/otto/pkg/api"
	kcontext "github.com/gptscript-ai/otto/pkg/gateway/context"
	ktime "github.com/gptscript-ai/otto/pkg/gateway/time"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
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
	logger := kcontext.GetLogger(apiContext.Context())
	user := kcontext.GetUser(apiContext.Context())

	var tokens []types.AuthToken
	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ?", user.ID).Find(&tokens).Error; err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusInternalServerError, fmt.Errorf("error getting tokens: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, tokens)
	return nil
}

func (s *Server) deleteToken(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	user := kcontext.GetUser(apiContext.Context())
	id := apiContext.PathValue("id")
	if id == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("id path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ? AND id = ?", user.ID, id).Delete(new(types.AuthToken)).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
			err = fmt.Errorf("not found")
		}
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("error deleting token: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"deleted": true})
	return nil
}

type createTokenRequest struct {
	ExpiresIn string `json:"expiresIn"`
}

func (s *Server) newToken(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	user := kcontext.GetUser(apiContext.Context())
	identity := kcontext.GetIdentity(apiContext.Context())

	var customExpiration time.Duration
	if apiContext.ContentLength != 0 {
		request := new(createTokenRequest)
		err := apiContext.Read(request)
		if err != nil {
			writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid create create token request body: %v", err))
			return nil
		}

		if request.ExpiresIn != "" {
			customExpiration, err = ktime.ParseDuration(request.ExpiresIn)
			if err != nil {
				writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid expiresIn duration: %v", err))
				return nil
			}
		}
	}

	randBytes := make([]byte, randomTokenLength+tokenIDLength)
	if _, err := rand.Read(randBytes); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusInternalServerError, fmt.Errorf("error refreshing token: %v", err))
		return nil
	}

	id := randBytes[:tokenIDLength]
	token := randBytes[tokenIDLength:]

	tkn := &types.AuthToken{
		ID:          fmt.Sprintf("%x", id),
		UserID:      user.ID,
		HashedToken: hashToken(fmt.Sprintf("%x", token)),
		ExpiresAt:   time.Now().Add(customExpiration),
	}

	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		provider := new(types.AuthProvider)
		if err := tx.Where("id = ?", identity.AuthProviderID).First(provider).Error; err != nil {
			return fmt.Errorf("error refreshing token: %v", err)
		}

		if customExpiration == 0 {
			tkn.ExpiresAt = time.Now().Add(provider.ExpirationDur)
		}
		tkn.AuthProviderID = provider.ID
		return tx.Create(tkn).Error
	}); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusInternalServerError, fmt.Errorf("error refreshing token: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, refreshTokenResponse{
		Token:     publicToken(id, token),
		ExpiresAt: tkn.ExpiresAt,
	})

	return nil
}

func (s *Server) tokenRequest(w http.ResponseWriter, r *http.Request) {
	logger := kcontext.GetLogger(r.Context())
	reqObj := new(tokenRequestRequest)
	if err := json.NewDecoder(r.Body).Decode(reqObj); err != nil {
		writeError(r.Context(), logger, w, http.StatusBadRequest, fmt.Errorf("invalid token request body: %v", err))
		return
	}

	tokenReq := &types.TokenRequest{
		ID:                    reqObj.ID,
		CompletionRedirectURL: reqObj.CompletionRedirectURL,
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if reqObj.ServiceName != "" {
			// Ensure the OAuth provider exists, if one was provided.
			if err := tx.Where("service_name = ?", reqObj.ServiceName).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
				return fmt.Errorf("failed to find oauth provider %q: %v", reqObj.ServiceName, err)
			}
		}

		return tx.Create(tokenReq).Error
	}); err != nil {
		logger.DebugContext(r.Context(), "failed to create token", "error", err)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			writeError(r.Context(), logger, w, http.StatusConflict, fmt.Errorf("token request already exists"))
		} else {
			writeError(r.Context(), logger, w, http.StatusInternalServerError, err)
		}
		return
	}

	if reqObj.ServiceName != "" {
		writeResponse(r.Context(), logger, w, map[string]any{"token-path": fmt.Sprintf("%s/oauth/start/%s/%s", s.baseURL, reqObj.ID, oauthProvider.Slug)})
		return
	}

	writeResponse(r.Context(), logger, w, map[string]any{"token-path": fmt.Sprintf("%s/login?id=%s", s.uiURL, reqObj.ID)})
}

func (s *Server) redirectForTokenRequest(w http.ResponseWriter, r *http.Request) {
	logger := kcontext.GetLogger(r.Context())
	id := r.PathValue("id")
	service := r.PathValue("service")

	oauthProvider := new(types.AuthProvider)
	tokenReq := new(types.TokenRequest)
	if err := s.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		// Ensure the OAuth provider exists, if one was provided.
		if err := tx.Where("slug = ?", service).Where("disabled IS NULL OR disabled != ?", true).First(oauthProvider).Error; err != nil {
			return fmt.Errorf("failed to find oauth provider %q: %v", service, err)
		}

		return tx.Where("id = ?", id).First(tokenReq).Error
	}); err != nil {
		logger.DebugContext(r.Context(), "failed to create token", "error", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeError(r.Context(), logger, w, http.StatusNotFound, fmt.Errorf("token or service not found"))
		} else {
			writeError(r.Context(), logger, w, http.StatusInternalServerError, err)
		}
		return
	}

	writeResponse(r.Context(), logger, w, map[string]any{"token-path": fmt.Sprintf("%s/oauth/start/%s/%s", s.baseURL, tokenReq.ID, oauthProvider.Slug)})
}

func (s *Server) checkForToken(w http.ResponseWriter, r *http.Request) {
	logger := kcontext.GetLogger(r.Context())
	tr := new(types.TokenRequest)
	if err := s.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", r.PathValue("id")).First(tr).Error; err != nil {
			return err
		}

		if tr.Token != "" && !tr.TokenRetrieved {
			return tx.Model(tr).Where("id = ?", tr.ID).Update("token_retrieved", true).Error
		}
		return nil
	}); err != nil || tr.ID == "" {
		logger.DebugContext(r.Context(), "failed to check token retrieved", "error", err)
		writeError(r.Context(), logger, w, http.StatusNotFound, fmt.Errorf("not found"))
		return
	}

	if tr.Error != "" {
		writeResponse(r.Context(), logger, w, map[string]any{"error": tr.Error})
	}

	writeResponse(r.Context(), logger, w, refreshTokenResponse{Token: tr.Token, ExpiresAt: tr.ExpiresAt})
}

func (s *Server) createState(ctx context.Context, id string) (string, string, error) {
	state := strings.ReplaceAll(uuid.NewString(), "-", "")
	nonce := strings.ReplaceAll(uuid.NewString(), "-", "")

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		tr := new(types.TokenRequest)
		if err := tx.Where("id = ?", id).First(tr).Error; err != nil {
			return err
		}

		return tx.Model(tr).Updates(map[string]any{"state": state, "nonce": nonce, "error": ""}).Error
	}); err != nil {
		return "", "", fmt.Errorf("failed to create state: %w", err)
	}

	return state, nonce, nil
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

func (s *Server) errorToken(ctx context.Context, tr *types.TokenRequest, logger *slog.Logger, w http.ResponseWriter, code int, err error) {
	if tr != nil {
		tr.Error = err.Error()
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Updates(tr).Error
		}); err != nil {
			logger.ErrorContext(ctx, "failed to set error field on token", "id", tr.ID, "error", err)
		}
	}

	writeError(ctx, logger, w, code, err)
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

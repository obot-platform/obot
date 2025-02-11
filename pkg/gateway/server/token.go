package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	kcontext "github.com/obot-platform/obot/pkg/gateway/context"
	ktime "github.com/obot-platform/obot/pkg/gateway/time"
	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	randomTokenLength = 32
	tokenIDLength     = 8
)

type tokenRequestRequest struct {
	ID                    string `json:"id"`
	ProviderName          string `json:"providerName"`
	ProviderNamespace     string `json:"providerNamespace"`
	CompletionRedirectURL string `json:"completionRedirectURL"`
}

type refreshTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
}

func (s *Server) getTokens(apiContext api.Context) error {
	var tokens []types.AuthToken
	if err := s.db.WithContext(apiContext.Context()).Where("user_id = ?", apiContext.UserID()).Find(&tokens).Error; err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("error getting tokens: %v", err))
	}

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

	return apiContext.Write(map[string]any{"deleted": true})
}

type createTokenRequest struct {
	ExpiresIn string `json:"expiresIn"`
}

func (s *Server) newToken(apiContext api.Context) error {
	namespace, name := apiContext.AuthProviderNameAndNamespace()
	userID := apiContext.UserID()
	if namespace == "" || name == "" || userID <= 0 {
		return types2.NewErrHTTP(http.StatusForbidden, "forbidden")
	}

	var customExpiration time.Duration
	if apiContext.ContentLength != 0 {
		request := new(createTokenRequest)
		err := apiContext.Read(request)
		if err != nil {
			return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid create create token request body: %v", err))
		}

		if request.ExpiresIn != "" {
			customExpiration, err = ktime.ParseDuration(request.ExpiresIn)
			if err != nil {
				return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid expiresIn duration: %v", err))
			}
		}
	}

	randBytes := make([]byte, randomTokenLength+tokenIDLength)
	if _, err := rand.Read(randBytes); err != nil {
		return types2.NewErrHTTP(http.StatusInternalServerError, fmt.Sprintf("error refreshing token: %v", err))
	}

	id := randBytes[:tokenIDLength]
	token := randBytes[tokenIDLength:]

	// Make sure the auth provider exists.
	if providerList := s.dispatcher.ListConfiguredAuthProviders(namespace); !slices.Contains(providerList, name) {
		return types2.NewErrHTTP(http.StatusNotFound, "auth provider not found")
	}

	return apiContext.Write(refreshTokenResponse{
		Token:     publicToken(id, token),
		ExpiresAt: time.Now().Add(customExpiration),
	})
}

func (s *Server) tokenRequest(apiContext api.Context) error {
	reqObj := new(tokenRequestRequest)
	if err := json.NewDecoder(apiContext.Request.Body).Decode(reqObj); err != nil {
		return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("invalid token request body: %v", err))
	}

	if reqObj.ProviderName != "" {
		if providerList := s.dispatcher.ListConfiguredAuthProviders(reqObj.ProviderNamespace); !slices.Contains(providerList, reqObj.ProviderName) {
			return types2.NewErrHTTP(http.StatusBadRequest, fmt.Sprintf("auth provider %q not found", reqObj.ProviderName))
		}
	}

	tokenReq := &types.TokenRequest{
		ID:                    reqObj.ID,
		CompletionRedirectURL: reqObj.CompletionRedirectURL,
	}

	if err := s.db.WithContext(apiContext.Context()).Create(tokenReq).Error; err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return types2.NewErrHTTP(http.StatusConflict, "token request already exists")
		}
		return types2.NewErrHTTP(http.StatusInternalServerError, err.Error())
	}

	if reqObj.ProviderName != "" {
		return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/api/oauth/start/%s/%s/%s", s.baseURL, reqObj.ID, reqObj.ProviderNamespace, reqObj.ProviderName)})
	}
	return apiContext.Write(map[string]any{"token-path": fmt.Sprintf("%s/login?id=%s", s.uiURL, reqObj.ID)})
}

func (s *Server) redirectForTokenRequest(apiContext api.Context) error {
	id := apiContext.PathValue("id")
	namespace := apiContext.PathValue("namespace")
	name := apiContext.PathValue("name")

	if namespace != "" && name != "" {
		if providerList := s.dispatcher.ListConfiguredAuthProviders(namespace); !slices.Contains(providerList, name) {
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

	return types2.NewErrHTTP(code, err.Error())
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

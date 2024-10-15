package server

import (
	"errors"
	"fmt"
	"net/http"

	types2 "github.com/otto8-ai/otto8/apiclient/types"
	"github.com/otto8-ai/otto8/pkg/api"
	kcontext "github.com/otto8-ai/otto8/pkg/gateway/context"
	ktime "github.com/otto8-ai/otto8/pkg/gateway/time"
	"github.com/otto8-ai/otto8/pkg/gateway/types"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type authProviderResponse struct {
	types.AuthProvider `json:",inline"`
	RedirectURL        string `json:"redirectURL"`
}

func (s *Server) createAuthProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	oauthProvider := new(types.AuthProvider)

	if err := apiContext.Read(oauthProvider); err != nil {
		logger.DebugContext(apiContext.Context(), "failed to decode oauth provider", "error", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid auth provider request body: %v", err))
		return nil
	}

	if err := oauthProvider.ValidateAndSetDefaults(); err != nil {
		logger.DebugContext(apiContext.Context(), "failed to validate oauth provider", "error", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid auth provider: %v", err))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Clauses(clause.Returning{}).Create(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
			status = http.StatusBadRequest
		}

		logger.DebugContext(apiContext.Context(), "failed to create auth provider", "error", err, "status", status)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to create auth provider: %v", err))
		return nil
	}

	oauthProvider.ClientSecret = ""
	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, authProviderResponse{AuthProvider: *oauthProvider, RedirectURL: oauthProvider.RedirectURL(s.baseURL)})
	return nil
}

func (s *Server) updateAuthProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	oauthProvider := new(types.AuthProvider)

	if err := apiContext.Read(oauthProvider); err != nil {
		logger.DebugContext(apiContext.Context(), "failed to decode oauth provider", "error", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid auth provider request body: %v", err))
		return nil
	}

	// If the expiration field is being changed, ensure the expiration dur field is also updated.
	if oauthProvider.Expiration != "" {
		var err error
		oauthProvider.ExpirationDur, err = ktime.ParseDuration(oauthProvider.Expiration)
		if err != nil {
			writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid expiration duration: %v", err))
			return nil
		}
	}

	if err := s.db.WithContext(apiContext.Context()).Where("slug = ?", apiContext.PathValue("slug")).Updates(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
			status = http.StatusBadRequest
		}

		logger.DebugContext(apiContext.Context(), "failed to update auth provider", "error", err, "status", status)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to create auth provider: %v", err))
		return nil
	}

	oauthProvider.ClientSecret = ""
	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, authProviderResponse{AuthProvider: *oauthProvider, RedirectURL: oauthProvider.RedirectURL(s.baseURL)})
	return nil
}

func (s *Server) getAuthProviders(apiContext api.Context) error {
	var authProviders []types.AuthProvider
	if err := s.db.WithContext(apiContext.Context()).Find(&authProviders).Error; err != nil {
		return types2.NewErrHttp(http.StatusInternalServerError, err.Error())
	}

	resp := make([]authProviderResponse, len(authProviders))
	for i, authProvider := range authProviders {
		authProvider.ClientSecret = ""
		resp[i] = authProviderResponse{
			AuthProvider: authProvider,
			RedirectURL:  authProvider.RedirectURL(s.baseURL),
		}
	}

	return apiContext.Write(resp)
}

func (s *Server) getAuthProvider(apiContext api.Context) error {
	slug := apiContext.PathValue("slug")
	if slug == "" {
		return types2.NewErrHttp(http.StatusBadRequest, "id path parameter is required")
	}

	oauthProvider := new(types.AuthProvider)
	if err := s.db.WithContext(apiContext.Context()).Where("slug = ?", slug).Find(oauthProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		return types2.NewErrHttp(status, fmt.Sprintf("failed to query auth provider: %v", err))
	}

	oauthProvider.ClientSecret = ""
	return apiContext.Write(authProviderResponse{
		AuthProvider: *oauthProvider,
		RedirectURL:  oauthProvider.RedirectURL(s.baseURL),
	})
}

func (s *Server) deleteAuthProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	var count int64
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(new(types.AuthProvider)).Count(&count).Error; err != nil {
			return err
		}
		if count == 1 {
			return fmt.Errorf("cannot delete last auth provider")
		}

		authProvider := new(types.AuthProvider)
		if err := tx.Where("slug = ?", slug).First(authProvider).Error; err != nil {
			return err
		}

		if err := tx.Where("auth_provider_id = ?", authProvider.ID).Delete(new(types.Identity)).Error; err != nil {
			return err
		}

		if err := tx.Where("auth_provider_id = ?", authProvider.ID).Delete(new(types.AuthToken)).Error; err != nil {
			return err
		}

		return tx.Unscoped().Where("slug = ?", slug).Delete(new(types.AuthProvider)).Error
	}); err != nil {
		if count == 1 {
			writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("cannot delete last auth provider"))
			return nil
		}

		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to delete auth provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to delete auth providers: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"deleted": true})
	return nil
}

func (s *Server) disableAuthProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	var count int64
	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(new(types.AuthProvider)).Where("disabled IS NULL OR disabled = false").Count(&count).Error; err != nil {
			return err
		}
		if count == 1 {
			return fmt.Errorf("cannot disable last auth provider")
		}

		return tx.Model(new(types.AuthProvider)).Where("slug = ?", slug).Update("disabled", true).Error
	}); err != nil {
		if count == 1 {
			writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("cannot disable last auth provider"))
			return nil
		}

		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to disable auth provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to disable auth providers: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"disabled": true})
	return nil
}

func (s *Server) enableAuthProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Model(new(types.AuthProvider)).Where("slug = ?", slug).Update("disabled", false).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to enable auth provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to enable auth providers: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"enabled": true})
	return nil
}

package server

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gptscript-ai/otto/pkg/api"
	kcontext "github.com/gptscript-ai/otto/pkg/gateway/context"
	"github.com/gptscript-ai/otto/pkg/gateway/types"
	"gorm.io/gorm"
)

type llmProviderResponse struct {
	types.LLMProvider `json:",inline"`
	RequestBaseURL    string `json:"requestBaseURL"`
}

func (s *Server) createLLMProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	llmProvider := new(types.LLMProvider)
	if err := apiContext.Read(llmProvider); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid llm provider body: %v", err))
		return nil
	}

	if err := llmProvider.Validate(); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, err)
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Create(llmProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
			status = http.StatusBadRequest
		}
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to create llm provider: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, llmProviderResponse{LLMProvider: *llmProvider, RequestBaseURL: llmProvider.RequestBaseURL(s.baseURL)})
	return nil
}

func (s *Server) updateLLMProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	llmProvider := new(types.LLMProvider)
	if err := apiContext.Read(llmProvider); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid llm provider body: %v", err))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Where("slug = ?", apiContext.PathValue("slug")).Updates(llmProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
			status = http.StatusBadRequest
		}
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to update llm provider: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, llmProviderResponse{LLMProvider: *llmProvider, RequestBaseURL: llmProvider.RequestBaseURL(s.baseURL)})
	return nil
}

func (s *Server) getLLMProviders(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	var llmProviders []types.LLMProvider
	if err := s.db.WithContext(apiContext.Context()).Find(&llmProviders).Error; err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusInternalServerError, fmt.Errorf("failed to get llm providers: %v", err))
		return nil
	}

	resp := make([]llmProviderResponse, len(llmProviders))
	for i, llmProvider := range llmProviders {
		llmProvider.Token = ""
		resp[i] = llmProviderResponse{
			LLMProvider:    llmProvider,
			RequestBaseURL: llmProvider.RequestBaseURL(s.baseURL),
		}
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, resp)
	return nil
}

func (s *Server) getLLMProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	llmProvider := new(types.LLMProvider)
	if err := s.db.WithContext(apiContext.Context()).Where("slug = ?", slug).First(llmProvider).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to get llm provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to get llm provider: %v", err))
		return nil
	}

	llmProvider.Token = ""
	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, llmProviderResponse{LLMProvider: *llmProvider, RequestBaseURL: llmProvider.RequestBaseURL(s.baseURL)})
	return nil
}

func (s *Server) deleteLLMProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		llmProvider := new(types.LLMProvider)
		if err := tx.Where("slug = ?", slug).First(llmProvider).Error; err != nil {
			return err
		}

		// Delete all models associated with this LLM provider.
		if err := tx.Where("llm_provider_id = ?", llmProvider.ID).Delete(new(types.Model)).Error; err != nil {
			return err
		}

		return tx.Unscoped().Model(llmProvider).Delete("id = ?", llmProvider.ID).Error
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to delete llm provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to delete llm provider: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"deleted": true})
	return nil
}

func (s *Server) disableLLMProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Model(new(types.LLMProvider)).Where("slug = ?", slug).Update("disabled", true).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to disable llm provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to disable llm provider: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"disabled": true})
	return nil
}

func (s *Server) enableLLMProvider(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	slug := apiContext.PathValue("slug")
	if slug == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("slug path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Model(new(types.LLMProvider)).Where("slug = ?", slug).Update("disabled", false).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to enable llm provider by slug", "slug", slug, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to enable llm provider: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"enabled": true})
	return nil
}

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

type modelLisResponse struct {
	Data []types.Model `json:"data"`
}

func (s *Server) createModel(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	model := new(types.Model)
	if err := apiContext.Read(model); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid Model body: %v", err))
		return nil
	}

	if err := model.Validate(); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, err)
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", model.LLMProviderID).First(new(types.LLMProvider)).Error; err != nil {
			return fmt.Errorf("invalid LLM provider: %v", err)
		}
		return tx.Create(model).Error
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
			status = http.StatusBadRequest
		}
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to create model: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, model)
	return nil
}

func (s *Server) updateModel(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	model := new(types.Model)
	if err := apiContext.Read(model); err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, fmt.Errorf("invalid model body: %v", err))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Transaction(func(tx *gorm.DB) error {
		if model.LLMProviderID != 0 {
			if err := tx.Where("id = ?", model.LLMProviderID).First(new(types.LLMProvider)).Error; err != nil {
				return fmt.Errorf("invalid LLM provider: %v", err)
			}
		}

		return tx.Where("id = ?", apiContext.PathValue("id")).Updates(model).Error
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrDuplicatedKey) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
			status = http.StatusBadRequest
		}
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to update model: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, model)
	return nil
}

func (s *Server) getModels(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	var models []types.Model
	if err := s.db.WithContext(apiContext.Context()).Find(&models).Error; err != nil {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusInternalServerError, fmt.Errorf("failed to get models: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, modelLisResponse{Data: models})
	return nil
}

func (s *Server) getModel(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	id := apiContext.PathValue("id")
	if id == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("id path parameter is required"))
		return nil
	}

	model := new(types.Model)
	if err := s.db.WithContext(apiContext.Context()).Where("id = ?", id).First(model).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to get model by id", "id", id, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to get model: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, model)
	return nil
}

func (s *Server) deleteModel(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	id := apiContext.PathValue("id")
	if id == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("id path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Unscoped().Where("id = ?", id).Delete(new(types.Model)).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to delete model by id", "id", id, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to delete model: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"deleted": true})
	return nil
}

func (s *Server) disableModel(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	id := apiContext.PathValue("id")
	if id == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("id path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Model(new(types.Model)).Where("id = ?", id).Update("disabled", true).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to disable model by id", "id", id, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to disable model: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"disabled": true})
	return nil
}

func (s *Server) enableModel(apiContext api.Context) error {
	logger := kcontext.GetLogger(apiContext.Context())
	id := apiContext.PathValue("id")
	if id == "" {
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, http.StatusBadRequest, errors.New("id path parameter is required"))
		return nil
	}

	if err := s.db.WithContext(apiContext.Context()).Model(new(types.Model)).Where("id = ?", id).Update("disabled", false).Error; err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gorm.ErrRecordNotFound) {
			status = http.StatusNotFound
		}

		logger.DebugContext(apiContext.Context(), "failed to enable model by id", "id", id, "err", err)
		writeError(apiContext.Context(), logger, apiContext.ResponseWriter, status, fmt.Errorf("failed to enable model: %v", err))
		return nil
	}

	writeResponse(apiContext.Context(), logger, apiContext.ResponseWriter, map[string]any{"enabled": true})
	return nil
}

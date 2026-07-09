package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	nanobottypes "github.com/obot-platform/nanobot/pkg/types"
	types2 "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/api"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
)

func (l *llmProviderProxy) serveModelsList(req api.Context, providerName string) (bool, error) {
	if req.Method != http.MethodGet || strings.TrimPrefix(req.PathValue("path"), "v1/") != "models" {
		return false, nil
	}

	models, err := l.mapHelper.GetUserAccessibleProviderModels(req.User, providerName)
	if err != nil {
		return true, fmt.Errorf("failed to determine accessible models: %w", err)
	}

	response, err := modelListResponse(models, l.routeDialect)
	if err != nil {
		return true, err
	}
	return true, req.Write(response)
}

type openAIModelListResponse struct {
	Object string            `json:"object"`
	Data   []openAIModelInfo `json:"data"`
}

type openAIModelInfo struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type anthropicModelListResponse struct {
	Data    []anthropicModelInfo `json:"data"`
	FirstID string               `json:"first_id"`
	HasMore bool                 `json:"has_more"`
	LastID  string               `json:"last_id"`
}

type anthropicModelInfo struct {
	ID             string                     `json:"id"`
	Capabilities   anthropicModelCapabilities `json:"capabilities"`
	CreatedAt      string                     `json:"created_at"`
	DisplayName    string                     `json:"display_name"`
	MaxInputTokens int                        `json:"max_input_tokens"`
	MaxTokens      int                        `json:"max_tokens"`
	Type           string                     `json:"type"`
}

type anthropicModelCapabilities struct {
	Batch             anthropicCapabilitySupport           `json:"batch"`
	Citations         anthropicCapabilitySupport           `json:"citations"`
	CodeExecution     anthropicCapabilitySupport           `json:"code_execution"`
	ContextManagement anthropicContextManagementCapability `json:"context_management"`
	Effort            anthropicEffortCapability            `json:"effort"`
	ImageInput        anthropicCapabilitySupport           `json:"image_input"`
	PDFInput          anthropicCapabilitySupport           `json:"pdf_input"`
	StructuredOutputs anthropicCapabilitySupport           `json:"structured_outputs"`
	Thinking          anthropicThinkingCapability          `json:"thinking"`
}

type anthropicCapabilitySupport struct {
	Supported bool `json:"supported"`
}

type anthropicContextManagementCapability struct {
	ClearThinking anthropicCapabilitySupport `json:"clear_thinking_20251015"`
	ClearToolUses anthropicCapabilitySupport `json:"clear_tool_uses_20250919"`
	Compact       anthropicCapabilitySupport `json:"compact_20260112"`
	Supported     bool                       `json:"supported"`
}

type anthropicEffortCapability struct {
	High      anthropicCapabilitySupport `json:"high"`
	Low       anthropicCapabilitySupport `json:"low"`
	Max       anthropicCapabilitySupport `json:"max"`
	Medium    anthropicCapabilitySupport `json:"medium"`
	Supported bool                       `json:"supported"`
	XHigh     anthropicCapabilitySupport `json:"xhigh"`
}

type anthropicThinkingCapability struct {
	Supported bool                   `json:"supported"`
	Types     anthropicThinkingTypes `json:"types"`
}

type anthropicThinkingTypes struct {
	Adaptive anthropicCapabilitySupport `json:"adaptive"`
	Enabled  anthropicCapabilitySupport `json:"enabled"`
}

func modelListResponse(models []v1.Model, routeDialect nanobottypes.Dialect) (any, error) {
	models = filterModelsByDialect(models, routeDialect)
	switch routeDialect {
	case nanobottypes.DialectOpenAIResponses:
		data := make([]openAIModelInfo, 0, len(models))
		for _, model := range models {
			created := int64(0)
			if !model.CreationTimestamp.IsZero() {
				created = model.CreationTimestamp.Unix()
			}
			data = append(data, openAIModelInfo{
				ID:      model.Spec.Manifest.TargetModel,
				Object:  "model",
				Created: created,
				OwnedBy: model.Spec.Manifest.ModelProvider,
			})
		}
		return openAIModelListResponse{Object: "list", Data: data}, nil
	case nanobottypes.DialectAnthropicMessages:
		data := make([]anthropicModelInfo, 0, len(models))
		for _, model := range models {
			displayName := model.Spec.Manifest.DisplayName
			if displayName == "" {
				displayName = model.Spec.Manifest.TargetModel
			}
			createdAt := time.Unix(0, 0).UTC()
			if !model.CreationTimestamp.IsZero() {
				createdAt = model.CreationTimestamp.Time.UTC()
			}
			data = append(data, anthropicModelInfo{
				ID:          model.Spec.Manifest.TargetModel,
				CreatedAt:   createdAt.Format(time.RFC3339),
				DisplayName: displayName,
				Type:        "model",
			})
		}
		response := anthropicModelListResponse{Data: data}
		if len(data) > 0 {
			response.FirstID = data[0].ID
			response.LastID = data[len(data)-1].ID
		}
		return response, nil
	default:
		return nil, types2.NewErrBadRequest("unsupported model route dialect %q", routeDialect)
	}
}

func filterModelsByDialect(models []v1.Model, routeDialect nanobottypes.Dialect) []v1.Model {
	filtered := make([]v1.Model, 0, len(models))
	for _, model := range models {
		if !model.Spec.Manifest.Active || nanobottypes.Dialect(model.Spec.Manifest.Dialect) != routeDialect {
			continue
		}
		filtered = append(filtered, model)
	}
	return filtered
}

package handlers

import (
	"fmt"

	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gemini"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type GeneratedImageHandler struct {
	gatewayClient *gateway.Client
	geminiClient  *gemini.Client
}

func NewGeneratedImageHandler(gatewayClient *gateway.Client, geminiClient *gemini.Client) *GeneratedImageHandler {
	return &GeneratedImageHandler{
		gatewayClient: gatewayClient,
		geminiClient:  geminiClient,
	}
}

type generateImageRequest struct {
	Prompt string `json:"prompt"`
}

type generateImageResponse struct {
	ImageURL string `json:"imageUrl"`
}

func (h *GeneratedImageHandler) GenerateImage(req api.Context) error {
	if h.geminiClient == nil {
		return apierrors.NewServiceUnavailable("Image generation API disabled")
	}
	var request generateImageRequest
	if err := req.Read(&request); err != nil {
		return err
	}

	if request.Prompt == "" {
		return apierrors.NewBadRequest("prompt is required")
	}

	generated, err := h.geminiClient.GenerateImage(req.Context(), request.Prompt)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to generate image: %w", err))
	}

	stored, err := h.gatewayClient.CreateGeneratedImage(req.Context(), generated.ImageData, generated.MIMEType)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to store generated image: %w", err))
	}

	return req.Write(&generateImageResponse{
		ImageURL: fmt.Sprintf("/api/generated/images/%s", stored.ID),
	})
}

func (h *GeneratedImageHandler) GetGeneratedImage(req api.Context) error {
	id := req.PathValue("id")
	if id == "" {
		return apierrors.NewBadRequest("id is required")
	}

	image, err := h.gatewayClient.GetGeneratedImage(req.Context(), id)
	if err != nil {
		return apierrors.NewNotFound(schema.GroupResource{}, id)
	}

	if image.Data == nil {
		return apierrors.NewInternalError(fmt.Errorf("generated image data is empty"))
	}

	if image.MIMEType == "" {
		return apierrors.NewInternalError(fmt.Errorf("generated image mime type is empty"))
	}

	req.ResponseWriter.Header().Set("Content-Type", image.MIMEType)
	req.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", len(image.Data)))
	req.ResponseWriter.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year

	if _, err := req.ResponseWriter.Write(image.Data); err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to write image data: %w", err))
	}

	return nil
}

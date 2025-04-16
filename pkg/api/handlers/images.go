package handlers

import (
	"bytes"
	"fmt"
	"image"
	"io"

	"github.com/gen2brain/webp"
	"github.com/obot-platform/obot/pkg/api"
	gateway "github.com/obot-platform/obot/pkg/gateway/client"
	"github.com/obot-platform/obot/pkg/gemini"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var allowedUploadedImageMIMETypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

const maxUploadSize = 2 * 1024 * 1024 // 2MB

type ImageHandler struct {
	gatewayClient *gateway.Client
	geminiClient  *gemini.Client
}

func NewImageHandler(gatewayClient *gateway.Client, geminiClient *gemini.Client) *ImageHandler {
	return &ImageHandler{
		gatewayClient: gatewayClient,
		geminiClient:  geminiClient,
	}
}

type generateImageRequest struct {
	Prompt string `json:"prompt"`
}

type imageResponse struct {
	ImageURL string `json:"imageUrl"`
}

func (h *ImageHandler) GenerateImage(req api.Context) error {
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

	data, err := convertToWebP(generated.ImageData)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to convert image to WebP: %w", err))
	}

	stored, err := h.gatewayClient.CreateImage(req.Context(), data, "image/webp")
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to store generated image: %w", err))
	}

	return req.Write(&imageResponse{
		ImageURL: fmt.Sprintf("/api/image/%s", stored.ID),
	})
}

func (h *ImageHandler) UploadImage(req api.Context) error {
	file, header, err := req.FormFile("image")
	if err != nil {
		return apierrors.NewBadRequest("failed to retrieve image file")
	}
	defer file.Close()

	// Validate file size
	if header.Size > maxUploadSize {
		return apierrors.NewRequestEntityTooLargeError(fmt.Sprintf("file size exceeds 2MB limit: %d bytes", header.Size))
	}

	// Validate MIME type
	mimeType := header.Header.Get("Content-Type")
	if !allowedUploadedImageMIMETypes[mimeType] {
		return apierrors.NewBadRequest(fmt.Sprintf("unsupported file type: %s", mimeType))
	}

	// Read file data
	data, err := io.ReadAll(io.LimitReader(file, maxUploadSize))
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to read image data: %w", err))
	}

	// Convert PNG/JPEG to WebP
	data, err = convertToWebP(data)
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to convert image to WebP: %w", err))
	}

	// Store image in gateway
	stored, err := h.gatewayClient.CreateImage(req.Context(), data, "image/webp")
	if err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to store uploaded image: %w", err))
	}

	return req.Write(&imageResponse{
		ImageURL: fmt.Sprintf("/api/image/%s", stored.ID),
	})
}

func (h *ImageHandler) GetImage(req api.Context) error {
	id := req.PathValue("id")
	if id == "" {
		return apierrors.NewBadRequest("id is required")
	}

	image, err := h.gatewayClient.GetImage(req.Context(), id)
	if err != nil {
		return apierrors.NewNotFound(schema.GroupResource{}, id)
	}

	if image.Data == nil {
		return apierrors.NewInternalError(fmt.Errorf("image data is empty"))
	}

	if image.MIMEType == "" {
		return apierrors.NewInternalError(fmt.Errorf("image mime type is empty"))
	}

	req.ResponseWriter.Header().Set("Content-Type", image.MIMEType)
	req.ResponseWriter.Header().Set("Content-Length", fmt.Sprintf("%d", len(image.Data)))
	req.ResponseWriter.Header().Set("Cache-Control", "public, max-age=31536000") // 1 year

	if _, err := req.ResponseWriter.Write(image.Data); err != nil {
		return apierrors.NewInternalError(fmt.Errorf("failed to write image data: %w", err))
	}

	return nil
}

func convertToWebP(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	var buf bytes.Buffer
	if err := webp.Encode(&buf, img, webp.Options{Lossless: true}); err != nil {
		return nil, fmt.Errorf("failed to encode WebP: %w", err)
	}

	return buf.Bytes(), nil
}

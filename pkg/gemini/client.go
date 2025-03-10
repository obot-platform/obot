package gemini

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image/png"

	"github.com/gen2brain/webp"
	"google.golang.org/genai"
)

type Config struct {
	GeminiAPIKey               string `usage:"The Google Gemini API Key used to generate images" env:"GEMINI_API_KEY"`
	GeminiImageGenerationModel string `usage:"The Google Gemini model to use to generate images" env:"GEMINI_IMAGE_GENERATION_MODEL" default:"imagen-3.0-generate-002"`
}

type Client struct {
	client               *genai.Client
	imageGenerationModel string
}

func NewClient(ctx context.Context, config Config) (*Client, error) {
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: config.GeminiAPIKey,
	})
	if err != nil {
		return nil, err
	}

	return &Client{client: geminiClient, imageGenerationModel: config.GeminiImageGenerationModel}, nil
}

type GeneratedImage struct {
	ImageData []byte
	MIMEType  string
}

func (c *Client) GenerateImage(ctx context.Context, prompt string) (*GeneratedImage, error) {
	response, err := c.client.Models.GenerateImages(ctx, c.imageGenerationModel, prompt, &genai.GenerateImagesConfig{
		NumberOfImages:   ptr(int32(1)),
		OutputMIMEType:   "image/png",
		AspectRatio:      "1:1",
		IncludeRAIReason: true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate image: %w", err)
	}

	var generated *genai.GeneratedImage
	for _, image := range response.GeneratedImages {
		if image != nil {
			generated = image
			break
		}
	}

	if generated == nil {
		return nil, errors.New("no image generated")
	}

	if generated.RAIFilteredReason != "" {
		return nil, fmt.Errorf("generated image was filtered: %s", generated.RAIFilteredReason)
	}

	if generated.Image == nil || generated.Image.ImageBytes == nil {
		return nil, errors.New("image generated but no image data was returned")
	}

	pngImage, err := png.Decode(bytes.NewReader(generated.Image.ImageBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to decode generated PNG image: %w", err)
	}

	var webpBuffer bytes.Buffer
	if err := webp.Encode(&webpBuffer, pngImage, webp.Options{Lossless: true}); err != nil {
		return nil, fmt.Errorf("failed to encode generated PNG image as WEBP: %w", err)
	}

	return &GeneratedImage{
		ImageData: webpBuffer.Bytes(),
		MIMEType:  "image/webp",
	}, nil
}

func ptr[T any](v T) *T {
	return &v
}

package gemini

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/genai"
)

type Config struct {
	GeminiAPIKey               string `usage:"The Google Gemini API Key used to generate images" env:"GEMINI_API_KEY"`
	GeminiImageGenerationModel string `usage:"The Google Gemini model to use to generate images" env:"GEMINI_IMAGE_GENERATION_MODEL" default:"imagen-3.0-generate-002"`
	GeminiOnFilteredRetry      int    `usage:"The number of times to retry image generation when a generated image is filtered by Gemini's people generation policies" env:"GEMINI_ON_FILTERED_RETRY" default:"3"`
}

type Client struct {
	client               *genai.Client
	imageGenerationModel string
	onFilteredRetry      int
}

func NewClient(ctx context.Context, config Config) (*Client, error) {
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: config.GeminiAPIKey,
	})
	if err != nil {
		return nil, err
	}

	return &Client{client: geminiClient, imageGenerationModel: config.GeminiImageGenerationModel, onFilteredRetry: config.GeminiOnFilteredRetry}, nil
}

type GeneratedImage struct {
	ImageData []byte
	MIMEType  string
}

func (c *Client) GenerateImage(ctx context.Context, prompt string) (*GeneratedImage, error) {
	var generated *genai.GeneratedImage
	for i := c.onFilteredRetry; i > 0; i-- {
		response, err := c.client.Models.GenerateImages(ctx, c.imageGenerationModel, prompt, &genai.GenerateImagesConfig{
			NumberOfImages:   int32(1),
			OutputMIMEType:   "image/png",
			AspectRatio:      "1:1",
			IncludeRAIReason: true,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to generate image: %w", err)
		}

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
			if i > 1 {
				// Retry
				continue
			}

			return nil, fmt.Errorf("generated image was filtered on all %d generation attempts: %s", c.onFilteredRetry, generated.RAIFilteredReason)
		}

		if generated.Image == nil || generated.Image.ImageBytes == nil {
			return nil, errors.New("image generated but no image data was returned")
		}

		return &GeneratedImage{
			ImageData: generated.Image.ImageBytes,
			MIMEType:  generated.Image.MIMEType,
		}, nil
	}

	return nil, errors.New("no image generated")
}

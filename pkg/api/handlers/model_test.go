package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	v1 "github.com/obot-platform/obot/pkg/storage/apis/obot.obot.ai/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateModelManifestAndSetDefaults(t *testing.T) {
	t.Run("accepts valid manifest", func(t *testing.T) {
		model := &v1.Model{Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			Name:          "openai-gpt-4.1",
			TargetModel:   "openai/gpt-4.1",
			ModelProvider: "openai-model-provider",
		}}}

		err := validateModelManifestAndSetDefaults(model)
		require.NoError(t, err)
	})

	t.Run("rejects slash-containing name", func(t *testing.T) {
		model := &v1.Model{Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			Name:          "openai/gpt-4.1",
			TargetModel:   "openai/gpt-4.1",
			ModelProvider: "openai-model-provider",
		}}}

		err := validateModelManifestAndSetDefaults(model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "field name must be a single path segment")
	})

	t.Run("defaults empty name from target model", func(t *testing.T) {
		model := &v1.Model{Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			Name:          "   ",
			TargetModel:   "openai/gpt-4.1",
			ModelProvider: "openai-model-provider",
		}}}

		err := validateModelManifestAndSetDefaults(model)
		require.NoError(t, err)
		assert.Equal(t, "openai-gpt-4.1", model.Spec.Manifest.Name)
	})

	t.Run("rejects empty name when target model is empty", func(t *testing.T) {
		model := &v1.Model{Spec: v1.ModelSpec{Manifest: types.ModelManifest{
			Name:          "   ",
			TargetModel:   "   ",
			ModelProvider: "openai-model-provider",
		}}}

		err := validateModelManifestAndSetDefaults(model)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "field targetModel is required")
	})
}

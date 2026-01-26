package handlers

import (
	"testing"

	"github.com/obot-platform/obot/apiclient/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToWorkflowManifest_ModelFields(t *testing.T) {
	tests := []struct {
		name             string
		input            types.TaskManifest
		expectedModel    string
		expectedProvider string
	}{
		{
			name: "preserves model fields when both are set",
			input: types.TaskManifest{
				Name:          "test-task",
				Description:   "A test task",
				Model:         "claude-3-opus",
				ModelProvider: "anthropic-model-provider",
				Steps:         []types.TaskStep{{Step: "Do something"}},
			},
			expectedModel:    "claude-3-opus",
			expectedProvider: "anthropic-model-provider",
		},
		{
			name: "handles empty model fields",
			input: types.TaskManifest{
				Name:        "task-no-model",
				Description: "Task without model selection",
				Steps:       []types.TaskStep{{Step: "Step 1"}},
			},
			expectedModel:    "",
			expectedProvider: "",
		},
		{
			name: "preserves model when provider is empty",
			input: types.TaskManifest{
				Name:  "task-model-only",
				Model: "gpt-4",
			},
			expectedModel:    "gpt-4",
			expectedProvider: "",
		},
		{
			name: "preserves provider when model is empty",
			input: types.TaskManifest{
				Name:          "task-provider-only",
				ModelProvider: "openai-model-provider",
			},
			expectedModel:    "",
			expectedProvider: "openai-model-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ToWorkflowManifest(tt.input)

			assert.Equal(t, tt.expectedModel, result.Model, "Model field mismatch")
			assert.Equal(t, tt.expectedProvider, result.ModelProvider, "ModelProvider field mismatch")
		})
	}
}

func TestConvertTaskManifest_ModelFields(t *testing.T) {
	tests := []struct {
		name             string
		input            *types.WorkflowManifest
		expectedModel    string
		expectedProvider string
	}{
		{
			name: "preserves model fields when both are set",
			input: &types.WorkflowManifest{
				Name:          "test-workflow",
				Description:   "A test workflow",
				Model:         "claude-3-sonnet",
				ModelProvider: "anthropic-model-provider",
				Steps:         []types.Step{{Step: "Do something"}},
			},
			expectedModel:    "claude-3-sonnet",
			expectedProvider: "anthropic-model-provider",
		},
		{
			name: "handles empty model fields",
			input: &types.WorkflowManifest{
				Name:        "workflow-no-model",
				Description: "Workflow without model selection",
			},
			expectedModel:    "",
			expectedProvider: "",
		},
		{
			name:             "handles nil input gracefully",
			input:            nil,
			expectedModel:    "",
			expectedProvider: "",
		},
		{
			name: "preserves model fields with steps",
			input: &types.WorkflowManifest{
				Name:          "workflow-with-steps",
				Model:         "llama3.2",
				ModelProvider: "ollama-model-provider",
				Steps: []types.Step{
					{Step: "First step"},
					{Step: "Second step"},
				},
			},
			expectedModel:    "llama3.2",
			expectedProvider: "ollama-model-provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertTaskManifest(tt.input)

			assert.Equal(t, tt.expectedModel, result.Model, "Model field mismatch")
			assert.Equal(t, tt.expectedProvider, result.ModelProvider, "ModelProvider field mismatch")
		})
	}
}

func TestTaskWorkflowConversion_RoundTrip(t *testing.T) {
	// Test that converting Task -> Workflow -> Task preserves model fields
	original := types.TaskManifest{
		Name:          "roundtrip-task",
		Description:   "Testing round trip conversion",
		Model:         "claude-3-opus",
		ModelProvider: "anthropic-model-provider",
		Steps:         []types.TaskStep{{Step: "Do something"}},
	}

	// Convert to workflow
	workflow := ToWorkflowManifest(original)

	// Verify workflow has the model fields
	require.Equal(t, original.Model, workflow.Model, "Workflow should have model from task")
	require.Equal(t, original.ModelProvider, workflow.ModelProvider, "Workflow should have modelProvider from task")

	// Convert back to task
	result := ConvertTaskManifest(&workflow)

	// Verify round-trip preserves model fields
	assert.Equal(t, original.Model, result.Model, "Round-trip should preserve model")
	assert.Equal(t, original.ModelProvider, result.ModelProvider, "Round-trip should preserve modelProvider")
	assert.Equal(t, original.Name, result.Name, "Round-trip should preserve name")
	assert.Equal(t, original.Description, result.Description, "Round-trip should preserve description")
}

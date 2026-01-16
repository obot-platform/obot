package types

import (
	"testing"
)

func TestOneLine(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single line shorter than 80 chars",
			input:    "This is a short line",
			expected: "This is a short line",
		},
		{
			name:     "single line exactly 80 chars",
			input:    "This is a line that is exactly eighty characters long and should not be truncat",
			expected: "This is a line that is exactly eighty characters long and should not be truncat",
		},
		{
			name:     "single line longer than 80 chars",
			input:    "This is a very long line that exceeds the maximum length of eighty characters and should be truncated with ellipsis",
			expected: "This is a very long line that exceeds the maximum length of eighty characters an...",
		},
		{
			name:     "multiline string - takes only first line",
			input:    "First line\nSecond line\nThird line",
			expected: "First line",
		},
		{
			name:     "multiline string with first line > 80 chars",
			input:    "This is a very long first line that exceeds eighty characters and should be truncated\nSecond line",
			expected: "This is a very long first line that exceeds eighty characters and should be trun...",
		},
		{
			name:     "only newlines",
			input:    "\n\n\n",
			expected: "",
		},
		{
			name:     "newline at start",
			input:    "\nThis is the second line",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := oneLine(tt.input)
			if result != tt.expected {
				t.Errorf("oneLine(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStepDisplay(t *testing.T) {
	tests := []struct {
		name     string
		step     Step
		expected string
	}{
		{
			name: "step with ID and short step content",
			step: Step{
				ID:   "step1",
				Step: "Process data",
			},
			expected: "> Step(step1):  Process data",
		},
		{
			name: "step with ID and long step content",
			step: Step{
				ID:   "step2",
				Step: "This is a very long step description that exceeds eighty characters and should be truncated with ellipsis at the end",
			},
			expected: "> Step(step2):  This is a very long step description that exceeds eighty characters and should b...",
		},
		{
			name: "step with ID but no step content",
			step: Step{
				ID: "step3",
			},
			expected: "> Step(step3): ",
		},
		{
			name: "step with empty ID and step content",
			step: Step{
				ID:   "",
				Step: "Some step",
			},
			expected: "> Step():  Some step",
		},
		{
			name: "step with multiline content",
			step: Step{
				ID:   "step4",
				Step: "First line\nSecond line\nThird line",
			},
			expected: "> Step(step4):  First line",
		},
		{
			name: "empty step",
			step: Step{},
			expected: "> Step(): ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.step.Display()
			if result != tt.expected {
				t.Errorf("Step.Display() = %q; want %q", result, tt.expected)
			}
		})
	}
}

func TestFindStep(t *testing.T) {
	tests := []struct {
		name           string
		manifest       *WorkflowManifest
		id             string
		expectedStep   *Step
		expectedParent string
	}{
		{
			name:           "nil manifest",
			manifest:       nil,
			id:             "step1",
			expectedStep:   nil,
			expectedParent: "",
		},
		{
			name: "empty id",
			manifest: &WorkflowManifest{
				Steps: []Step{
					{ID: "step1", Step: "First step"},
				},
			},
			id:             "",
			expectedStep:   nil,
			expectedParent: "",
		},
		{
			name: "find existing step",
			manifest: &WorkflowManifest{
				Steps: []Step{
					{ID: "step1", Step: "First step"},
					{ID: "step2", Step: "Second step"},
					{ID: "step3", Step: "Third step"},
				},
			},
			id:             "step2",
			expectedStep:   &Step{ID: "step2", Step: "Second step"},
			expectedParent: "",
		},
		{
			name: "step not found",
			manifest: &WorkflowManifest{
				Steps: []Step{
					{ID: "step1", Step: "First step"},
				},
			},
			id:             "step999",
			expectedStep:   nil,
			expectedParent: "",
		},
		{
			name: "empty steps array",
			manifest: &WorkflowManifest{
				Steps: []Step{},
			},
			id:             "step1",
			expectedStep:   nil,
			expectedParent: "",
		},
		{
			name: "id with curly brace (parameter placeholder) - looks up base ID",
			manifest: &WorkflowManifest{
				Steps: []Step{
					{ID: "step1", Step: "First step"},
					{ID: "step2", Step: "Second step"},
				},
			},
			id:             "step2{param=value}",
			expectedStep:   &Step{ID: "step2{param=value}", Step: "Second step"}, // ID should be updated to match searched ID
			expectedParent: "",
		},
		{
			name: "multiple curly braces in id",
			manifest: &WorkflowManifest{
				Steps: []Step{
					{ID: "loop", Step: "Loop step"},
				},
			},
			id:             "loop{i=1}{j=2}",
			expectedStep:   &Step{ID: "loop{i=1}{j=2}", Step: "Loop step"},
			expectedParent: "",
		},
		{
			name: "step with loop field",
			manifest: &WorkflowManifest{
				Steps: []Step{
					{ID: "step1", Step: "First", Loop: []string{"a", "b", "c"}},
				},
			},
			id:             "step1",
			expectedStep:   &Step{ID: "step1", Step: "First", Loop: []string{"a", "b", "c"}},
			expectedParent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resultStep, resultParent := FindStep(tt.manifest, tt.id)

			// Check parent ID
			if resultParent != tt.expectedParent {
				t.Errorf("FindStep() parentID = %q; want %q", resultParent, tt.expectedParent)
			}

			// Check step
			if tt.expectedStep == nil {
				if resultStep != nil {
					t.Errorf("FindStep() step = %+v; want nil", resultStep)
				}
				return
			}

			if resultStep == nil {
				t.Fatalf("FindStep() step = nil; want %+v", tt.expectedStep)
			}

			if resultStep.ID != tt.expectedStep.ID {
				t.Errorf("FindStep() step.ID = %q; want %q", resultStep.ID, tt.expectedStep.ID)
			}
			if resultStep.Step != tt.expectedStep.Step {
				t.Errorf("FindStep() step.Step = %q; want %q", resultStep.Step, tt.expectedStep.Step)
			}
			if len(resultStep.Loop) != len(tt.expectedStep.Loop) {
				t.Errorf("FindStep() step.Loop length = %d; want %d", len(resultStep.Loop), len(tt.expectedStep.Loop))
			}
		})
	}
}

package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelAccessPolicyManifestValidate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		manifest    ModelAccessPolicyManifest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid with single user subject and single model",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with multiple user subjects and multiple models",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
					{Type: SubjectTypeUser, ID: "user2"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
					{ID: "m7654321"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with group subject and model",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeGroup, ID: "group1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with mixed user and group subjects",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
					{Type: SubjectTypeGroup, ID: "group1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
					{ID: "m7654321"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with wildcard selector subject only",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeSelector, ID: "*"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with wildcard model only",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "*"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with wildcard selector and wildcard model",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeSelector, ID: "*"},
				},
				Models: []ModelResource{
					{ID: "*"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with wildcard suffix pattern only",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "claude-haiku-4-5*"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with wildcard suffix pattern, model ID, and alias",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "claude-haiku-4-5*"},
					{ID: "m1234567"},
					{ID: "obot://llm"},
				},
			},
			expectError: false,
		},
		{
			name: "valid with overlapping wildcard suffix patterns",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "gpt*"},
					{ID: "gpt-4*"},
				},
			},
			expectError: false,
		},

		// Validation failures - subjects
		{
			name: "empty subjects list",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "at least one subject is required",
		},
		{
			name: "nil subjects list",
			manifest: ModelAccessPolicyManifest{
				Subjects: nil,
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "at least one subject is required",
		},
		{
			name: "invalid subject - empty user ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: ""},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "invalid subject",
		},
		{
			name: "invalid subject - empty group ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeGroup, ID: ""},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "invalid subject",
		},
		{
			name: "invalid subject - selector with non-wildcard ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeSelector, ID: "invalid"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "invalid subject",
		},
		{
			name: "wildcard selector with other subjects",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeSelector, ID: "*"},
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard subject (*) must be the only subject",
		},
		{
			name: "duplicate user subjects",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "duplicate subject: user/user1",
		},
		{
			name: "duplicate group subjects",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeGroup, ID: "group1"},
					{Type: SubjectTypeGroup, ID: "group1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "duplicate subject: group/group1",
		},

		// Models
		{
			name: "empty models list, for a profile with only budgets or guardrails",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{},
			},
			expectError: false,
		},
		{
			name: "nil models list",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: nil,
			},
			expectError: false,
		},
		{
			name: "invalid model - empty ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: ""},
				},
			},
			expectError: true,
			errorMsg:    "invalid model",
		},
		{
			name: "wildcard model with other models",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "*"},
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard model (*) must be the only model",
		},
		{
			name: "duplicate models",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "m1234567"},
					{ID: "m1234567"},
				},
			},
			expectError: true,
			errorMsg:    "duplicate model m1234567",
		},
		{
			name: "wildcard in the middle of model ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "claude*haiku"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard (*) is only allowed",
		},
		{
			name: "leading wildcard in model ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "*haiku"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard (*) is only allowed",
		},
		{
			name: "double trailing wildcard in model ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "claude**"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard (*) is only allowed",
		},
		{
			name: "bare double wildcard model ID",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "**"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard (*) is only allowed",
		},
		{
			name: "whitespace-only wildcard suffix prefix",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: " *"},
				},
			},
			expectError: true,
			errorMsg:    "must not begin or end with whitespace",
		},
		{
			name: "leading whitespace in wildcard suffix prefix",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: " gpt*"},
				},
			},
			expectError: true,
			errorMsg:    "must not begin or end with whitespace",
		},
		{
			name: "trailing whitespace in wildcard suffix prefix",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "gpt *"},
				},
			},
			expectError: true,
			errorMsg:    "must not begin or end with whitespace",
		},
		{
			name: "interior whitespace in wildcard suffix prefix is allowed",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "gpt 4*"},
				},
			},
			expectError: false,
		},
		{
			name: "wildcard suffix on model alias reference",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "obot://llm*"},
				},
			},
			expectError: true,
			errorMsg:    "unknown model alias type reference: llm*",
		},
		{
			name: "wildcard model with wildcard suffix pattern",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "*"},
					{ID: "claude-haiku-4-5*"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard model (*) must be the only model",
		},
		{
			name: "duplicate wildcard suffix patterns",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{
					{ID: "claude-haiku-4-5*"},
					{ID: "claude-haiku-4-5*"},
				},
			},
			expectError: true,
			errorMsg:    "duplicate model claude-haiku-4-5*",
		},

		// Combined validation failures
		{
			name: "both subjects and models empty",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{},
				Models:   []ModelResource{},
			},
			expectError: true,
			errorMsg:    "at least one subject is required",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()

			if tt.expectError {
				require.Error(t, err, "expected validation to fail")
				assert.Contains(t, err.Error(), tt.errorMsg, "error message should contain expected text")
			} else {
				assert.NoError(t, err, "expected validation to pass")
			}
		})
	}
}

func TestMatchesTargetModel(t *testing.T) {
	for _, tt := range []struct {
		pattern, target string
		want            bool
	}{
		{pattern: "anthropic/claude-haiku-4-5*", target: "anthropic/claude-haiku-4-5-20251001", want: true},
		// A pattern written for provider-native names still matches gateway models.
		{pattern: "claude-haiku-4-5*", target: "anthropic/claude-haiku-4-5-20251001", want: true},
		{pattern: "claude-haiku-4-5*", target: "claude-haiku-4-5", want: true},
		{pattern: "anthropic/claude*", target: "bedrock/claude-x"},
		{pattern: "gpt-5*", target: "openai/gpt-4.1"},
		{pattern: "*", target: "openai/gpt-5"},
		{pattern: "gpt-5", target: "openai/gpt-5"},
	} {
		if got := (ModelResource{ID: tt.pattern}).MatchesTargetModel(tt.target); got != tt.want {
			t.Errorf("%q matches %q = %v, want %v", tt.pattern, tt.target, got, tt.want)
		}
	}
}

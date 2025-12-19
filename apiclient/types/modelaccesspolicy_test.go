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

		// Validation failures - models
		{
			name: "empty models list",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: []ModelResource{},
			},
			expectError: true,
			errorMsg:    "at least one model resource is required",
		},
		{
			name: "nil models list",
			manifest: ModelAccessPolicyManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Models: nil,
			},
			expectError: true,
			errorMsg:    "at least one model resource is required",
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

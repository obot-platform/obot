package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkillAccessRuleManifestValidate(t *testing.T) {
	for _, tt := range []struct {
		name        string
		manifest    SkillAccessRuleManifest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid skill rule",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{{Type: SubjectTypeUser, ID: "user1"}},
				Resources: []SkillResource{{
					Type: SkillResourceTypeSkill,
					ID:   "sk1abcd",
				}},
			},
		},
		{
			name: "valid repository rule",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{{Type: SubjectTypeGroup, ID: "eng"}},
				Resources: []SkillResource{{
					Type: SkillResourceTypeSkillRepository,
					ID:   "skr1abcd",
				}},
			},
		},
		{
			name: "valid wildcard rule",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{{Type: SubjectTypeSelector, ID: "*"}},
				Resources: []SkillResource{{
					Type: SkillResourceTypeSelector,
					ID:   "*",
				}},
			},
		},
		{
			name: "duplicate subject",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "user1"},
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Resources: []SkillResource{{
					Type: SkillResourceTypeSkill,
					ID:   "sk1abcd",
				}},
			},
			expectError: true,
			errorMsg:    "duplicate subject: user/user1",
		},
		{
			name: "duplicate resource",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{{Type: SubjectTypeUser, ID: "user1"}},
				Resources: []SkillResource{
					{Type: SkillResourceTypeSkill, ID: "sk1abcd"},
					{Type: SkillResourceTypeSkill, ID: "sk1abcd"},
				},
			},
			expectError: true,
			errorMsg:    "duplicate resource: skill/sk1abcd",
		},
		{
			name: "empty subjects",
			manifest: SkillAccessRuleManifest{
				Resources: []SkillResource{{Type: SkillResourceTypeSkill, ID: "sk1abcd"}},
			},
			expectError: true,
			errorMsg:    "at least one subject is required",
		},
		{
			name: "empty resources",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{{Type: SubjectTypeUser, ID: "user1"}},
			},
			expectError: true,
			errorMsg:    "at least one resource is required",
		},
		{
			name: "invalid subject selector ID",
			manifest: SkillAccessRuleManifest{
				Subjects:  []Subject{{Type: SubjectTypeSelector, ID: "all"}},
				Resources: []SkillResource{{Type: SkillResourceTypeSkill, ID: "sk1abcd"}},
			},
			expectError: true,
			errorMsg:    "invalid subject",
		},
		{
			name: "invalid resource selector ID",
			manifest: SkillAccessRuleManifest{
				Subjects:  []Subject{{Type: SubjectTypeUser, ID: "user1"}},
				Resources: []SkillResource{{Type: SkillResourceTypeSelector, ID: "all"}},
			},
			expectError: true,
			errorMsg:    "invalid resource",
		},
		{
			name: "wildcard subject with other subjects",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{
					{Type: SubjectTypeSelector, ID: "*"},
					{Type: SubjectTypeUser, ID: "user1"},
				},
				Resources: []SkillResource{{Type: SkillResourceTypeSkill, ID: "sk1abcd"}},
			},
			expectError: true,
			errorMsg:    "wildcard subject (*) must be the only subject",
		},
		{
			name: "wildcard resource with other resources",
			manifest: SkillAccessRuleManifest{
				Subjects: []Subject{{Type: SubjectTypeUser, ID: "user1"}},
				Resources: []SkillResource{
					{Type: SkillResourceTypeSelector, ID: "*"},
					{Type: SkillResourceTypeSkill, ID: "sk1abcd"},
				},
			},
			expectError: true,
			errorMsg:    "wildcard resource (*) must be the only resource",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifest.Validate()

			if tt.expectError {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.errorMsg)
				return
			}

			require.NoError(t, err)
		})
	}
}

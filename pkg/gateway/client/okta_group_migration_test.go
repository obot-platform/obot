package client

import (
	"testing"

	types2 "github.com/obot-platform/obot/apiclient/types"
)

func TestUpdateSubjects(t *testing.T) {
	tests := []struct {
		name            string
		subjects        []types2.Subject
		idMap           map[string]string
		expectChanged   bool
		expectSubjects  []types2.Subject
	}{
		{
			name: "no group subjects",
			subjects: []types2.Subject{
				{Type: types2.SubjectTypeUser, ID: "user1"},
			},
			idMap:         map[string]string{"okta/eng": "okta/00g123"},
			expectChanged: false,
			expectSubjects: []types2.Subject{
				{Type: types2.SubjectTypeUser, ID: "user1"},
			},
		},
		{
			name: "group subject with matching mapping",
			subjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "okta/eng"},
				{Type: types2.SubjectTypeUser, ID: "user1"},
			},
			idMap:         map[string]string{"okta/eng": "okta/00g123"},
			expectChanged: true,
			expectSubjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "okta/00g123"},
				{Type: types2.SubjectTypeUser, ID: "user1"},
			},
		},
		{
			name: "group subject without matching mapping",
			subjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "entra/some-group"},
			},
			idMap:         map[string]string{"okta/eng": "okta/00g123"},
			expectChanged: false,
			expectSubjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "entra/some-group"},
			},
		},
		{
			name: "multiple group subjects mixed",
			subjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "okta/eng"},
				{Type: types2.SubjectTypeGroup, ID: "okta/sales"},
				{Type: types2.SubjectTypeGroup, ID: "entra/other"},
			},
			idMap: map[string]string{
				"okta/eng":   "okta/00g123",
				"okta/sales": "okta/00g456",
			},
			expectChanged: true,
			expectSubjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "okta/00g123"},
				{Type: types2.SubjectTypeGroup, ID: "okta/00g456"},
				{Type: types2.SubjectTypeGroup, ID: "entra/other"},
			},
		},
		{
			name:          "empty subjects",
			subjects:      nil,
			idMap:         map[string]string{"okta/eng": "okta/00g123"},
			expectChanged: false,
		},
		{
			name: "empty idMap",
			subjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "okta/eng"},
			},
			idMap:         map[string]string{},
			expectChanged: false,
			expectSubjects: []types2.Subject{
				{Type: types2.SubjectTypeGroup, ID: "okta/eng"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := updateSubjects(tt.subjects, tt.idMap)
			if changed != tt.expectChanged {
				t.Errorf("updateSubjects() changed = %v, want %v", changed, tt.expectChanged)
			}
			if tt.expectSubjects != nil {
				if len(tt.subjects) != len(tt.expectSubjects) {
					t.Fatalf("subjects length = %d, want %d", len(tt.subjects), len(tt.expectSubjects))
				}
				for i, s := range tt.subjects {
					if s != tt.expectSubjects[i] {
						t.Errorf("subjects[%d] = %+v, want %+v", i, s, tt.expectSubjects[i])
					}
				}
			}
		})
	}
}

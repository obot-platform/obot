package state

import (
	"testing"
)

func TestGroupInfoList_FilterByAllowed(t *testing.T) {
	tests := []struct {
		name          string
		groups        GroupInfoList
		allowedGroups []string
		want          GroupInfoList
	}{
		{
			name: "empty allowed groups returns all groups",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
				{ID: "group-3", Name: "Sales"},
			},
			allowedGroups: []string{},
			want: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
				{ID: "group-3", Name: "Sales"},
			},
		},
		{
			name: "nil allowed groups returns all groups",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
			},
			allowedGroups: nil,
			want: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
			},
		},
		{
			name: "filters to only allowed groups",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
				{ID: "group-3", Name: "Sales"},
				{ID: "group-4", Name: "HR"},
			},
			allowedGroups: []string{"group-1", "group-3"},
			want: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-3", Name: "Sales"},
			},
		},
		{
			name: "no matching groups returns empty list",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
			},
			allowedGroups: []string{"group-99", "group-100"},
			want:          GroupInfoList{},
		},
		{
			name:          "empty groups list returns empty list",
			groups:        GroupInfoList{},
			allowedGroups: []string{"group-1"},
			want:          GroupInfoList{},
		},
		{
			name: "preserves group descriptions",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering", Description: stringPtr("Software engineering team")},
				{ID: "group-2", Name: "Marketing", Description: stringPtr("Marketing team")},
				{ID: "group-3", Name: "Sales", Description: nil},
			},
			allowedGroups: []string{"group-1", "group-3"},
			want: GroupInfoList{
				{ID: "group-1", Name: "Engineering", Description: stringPtr("Software engineering team")},
				{ID: "group-3", Name: "Sales", Description: nil},
			},
		},
		{
			name: "preserves icon URLs",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering", IconURL: stringPtr("https://example.com/icon1.png")},
				{ID: "group-2", Name: "Marketing", IconURL: stringPtr("https://example.com/icon2.png")},
			},
			allowedGroups: []string{"group-2"},
			want: GroupInfoList{
				{ID: "group-2", Name: "Marketing", IconURL: stringPtr("https://example.com/icon2.png")},
			},
		},
		{
			name: "single group single allowed",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
			},
			allowedGroups: []string{"group-1"},
			want: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.groups.FilterByAllowed(tt.allowedGroups)

			if len(got) != len(tt.want) {
				t.Errorf("FilterByAllowed() returned %d groups, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i].ID != tt.want[i].ID {
					t.Errorf("FilterByAllowed() group[%d].ID = %v, want %v", i, got[i].ID, tt.want[i].ID)
				}
				if got[i].Name != tt.want[i].Name {
					t.Errorf("FilterByAllowed() group[%d].Name = %v, want %v", i, got[i].Name, tt.want[i].Name)
				}
				if !equalStringPtr(got[i].Description, tt.want[i].Description) {
					t.Errorf("FilterByAllowed() group[%d].Description = %v, want %v", i, ptrToString(got[i].Description), ptrToString(tt.want[i].Description))
				}
				if !equalStringPtr(got[i].IconURL, tt.want[i].IconURL) {
					t.Errorf("FilterByAllowed() group[%d].IconURL = %v, want %v", i, ptrToString(got[i].IconURL), ptrToString(tt.want[i].IconURL))
				}
			}
		})
	}
}

func TestGroupInfoList_IDs(t *testing.T) {
	tests := []struct {
		name   string
		groups GroupInfoList
		want   []string
	}{
		{
			name: "returns all group IDs",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
				{ID: "group-2", Name: "Marketing"},
				{ID: "group-3", Name: "Sales"},
			},
			want: []string{"group-1", "group-2", "group-3"},
		},
		{
			name:   "empty list returns empty slice",
			groups: GroupInfoList{},
			want:   []string{},
		},
		{
			name: "single group",
			groups: GroupInfoList{
				{ID: "group-1", Name: "Engineering"},
			},
			want: []string{"group-1"},
		},
		{
			name: "preserves order",
			groups: GroupInfoList{
				{ID: "z-group", Name: "Z"},
				{ID: "a-group", Name: "A"},
				{ID: "m-group", Name: "M"},
			},
			want: []string{"z-group", "a-group", "m-group"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.groups.IDs()

			if len(got) != len(tt.want) {
				t.Errorf("IDs() returned %d IDs, want %d", len(got), len(tt.want))
				return
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("IDs()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGroupInfo_WithDescription(t *testing.T) {
	desc := "Engineering team"
	group := GroupInfo{
		ID:          "group-1",
		Name:        "Engineering",
		Description: &desc,
	}

	if group.ID != "group-1" {
		t.Errorf("GroupInfo.ID = %v, want %v", group.ID, "group-1")
	}
	if group.Name != "Engineering" {
		t.Errorf("GroupInfo.Name = %v, want %v", group.Name, "Engineering")
	}
	if group.Description == nil {
		t.Error("GroupInfo.Description is nil, want non-nil")
	} else if *group.Description != "Engineering team" {
		t.Errorf("GroupInfo.Description = %v, want %v", *group.Description, "Engineering team")
	}
}

func TestGroupInfo_WithoutDescription(t *testing.T) {
	group := GroupInfo{
		ID:   "group-1",
		Name: "Engineering",
	}

	if group.Description != nil {
		t.Errorf("GroupInfo.Description = %v, want nil", group.Description)
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func equalStringPtr(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func ptrToString(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

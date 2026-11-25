package types

import "testing"

func TestAgentProfileValidate(t *testing.T) {
	everyone := []Subject{{Type: SubjectTypeSelector, ID: SubjectWildcardID}}
	for _, tt := range []struct {
		name   string
		access AgentProfile
		// want is a part of the error, or empty for none.
		want string
	}{
		{
			name: "All Obot Users",
			access: AgentProfile{
				Name:     "everyone",
				Subjects: everyone,
			},
		},
		{
			name: "users, groups and Obot groups",
			access: AgentProfile{
				Name: "some",
				Subjects: []Subject{
					{Type: SubjectTypeUser, ID: "5"},
					{Type: SubjectTypeGroup, ID: "eng"},
					{Type: SubjectTypeObotGroup, ID: GroupAdmin},
				},
			},
		},
		{
			name: "no name",
			access: AgentProfile{
				Name:     " ",
				Subjects: everyone,
			},
			want: "name is required",
		},
		{
			name: "no subjects",
			access: AgentProfile{
				Name: "nobody",
			},
			want: "at least one subject",
		},
		{
			name: "agent subject",
			access: AgentProfile{
				Name:     "agent",
				Subjects: []Subject{{Type: SubjectTypeAgent, ID: "7"}},
			},
			want: "agent subjects are not supported in agent profiles",
		},
		{
			name: "duplicate subject",
			access: AgentProfile{
				Name:     "twice",
				Subjects: []Subject{{Type: SubjectTypeUser, ID: "5"}, {Type: SubjectTypeUser, ID: "5"}},
			},
			want: "duplicate subject",
		},
		{
			name: "All Obot Users with another subject",
			access: AgentProfile{
				Name:     "everyone and more",
				Subjects: []Subject{{Type: SubjectTypeSelector, ID: SubjectWildcardID}, {Type: SubjectTypeUser, ID: "5"}},
			},
			want: "must be the only subject",
		},
		{
			name: "user wildcard",
			access: AgentProfile{
				Name:     "users",
				Subjects: []Subject{{Type: SubjectTypeUser, ID: SubjectWildcardID}},
			},
			want: "names one user by ID",
		},
		{
			name: "invalid subject",
			access: AgentProfile{
				Name:     "selector",
				Subjects: []Subject{{Type: SubjectTypeSelector, ID: "all"}},
			},
			want: "selector subject ID must be '*'",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkError(t, "Validate", tt.access.Validate(), tt.want)
		})
	}
}

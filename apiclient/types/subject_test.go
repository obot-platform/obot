package types

import (
	"strings"
	"testing"
)

var allPolicySystems = []PolicySystem{
	PolicySystemVMCPProfile,
	PolicySystemModelAccess,
	PolicySystemSkillAccess,
	PolicySystemAccessControl,
	PolicySystemAgentProfile,
}

func TestSubjectValidateFor(t *testing.T) {
	// agentSystems are the policy systems whose subjects can match agents.
	agentSystems := map[PolicySystem]bool{
		PolicySystemVMCPProfile: true,
		PolicySystemModelAccess: true,
		PolicySystemSkillAccess: true,
	}
	for _, tt := range []struct {
		name    string
		subject Subject
		// valid is whether the subject is valid in every system, or only (agentOnly) in those that can match agents.
		valid     bool
		agentOnly bool
	}{
		{
			name:    "selector wildcard",
			subject: Subject{Type: SubjectTypeSelector, ID: "*"},
			valid:   true,
		},
		{
			name:    "selector other than the wildcard",
			subject: Subject{Type: SubjectTypeSelector, ID: "all"},
		},
		{
			name:    "user",
			subject: Subject{Type: SubjectTypeUser, ID: "5"},
			valid:   true,
		},
		{
			// It has always been valid, and names no one.
			name:    "user wildcard",
			subject: Subject{Type: SubjectTypeUser, ID: "*"},
			valid:   true,
		},
		{
			name:    "user without an ID",
			subject: Subject{Type: SubjectTypeUser},
		},
		{
			name:    "group",
			subject: Subject{Type: SubjectTypeGroup, ID: "eng"},
			valid:   true,
		},
		{
			name:    "Obot group",
			subject: Subject{Type: SubjectTypeObotGroup, ID: GroupAdmin},
			valid:   true,
		},
		{
			name:      "agent",
			subject:   Subject{Type: SubjectTypeAgent, ID: "7"},
			valid:     true,
			agentOnly: true,
		},
		{
			name:    "agent wildcard",
			subject: Subject{Type: SubjectTypeAgent, ID: "*"},
		},
		{
			name:    "agent storage encoding",
			subject: Subject{Type: SubjectTypeAgent, ID: "agent:7"},
		},
		{
			name:    "agent ID with a leading zero",
			subject: Subject{Type: SubjectTypeAgent, ID: "07"},
		},
		{
			name:    "agent ID zero",
			subject: Subject{Type: SubjectTypeAgent, ID: "0"},
		},
		{
			name:    "agent ID that is not a number",
			subject: Subject{Type: SubjectTypeAgent, ID: "builder"},
		},
		{
			name:    "agent without an ID",
			subject: Subject{Type: SubjectTypeAgent},
		},
		{
			name:    "unknown type",
			subject: Subject{Type: "service", ID: "*"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// Validate is permissive about the policy system.
			if err := tt.subject.Validate(); (err == nil) != tt.valid {
				t.Errorf("Validate() = %v, want valid %v", err, tt.valid)
			}
			for _, system := range allPolicySystems {
				want := tt.valid && (!tt.agentOnly || agentSystems[system])
				if err := tt.subject.ValidateFor(system); (err == nil) != want {
					t.Errorf("ValidateFor(%s) = %v, want valid %v", system, err, want)
				}
			}
		})
	}
}

// TestPolicySubjectWildcards checks the wildcard rules of each manifest that can name agents. Model access policies
// and skill access rules require a subject with the ID "*" to be their only subject, as they always have; vMCP
// profiles never have. Agents are only named by ID, so the agent wildcard is refused everywhere.
func TestPolicySubjectWildcards(t *testing.T) {
	var (
		everyone = Subject{Type: SubjectTypeSelector, ID: "*"}
		users    = Subject{Type: SubjectTypeUser, ID: "*"}
		agents   = Subject{Type: SubjectTypeAgent, ID: "*"}
		user     = Subject{Type: SubjectTypeUser, ID: "5"}
		agent    = Subject{Type: SubjectTypeAgent, ID: "5"}
		group    = Subject{Type: SubjectTypeGroup, ID: "eng"}
	)
	validateModel := func(subjects []Subject) error {
		return ModelAccessPolicyManifest{Subjects: subjects}.Validate()
	}
	validateSkill := func(subjects []Subject) error {
		return SkillAccessRuleManifest{Subjects: subjects, Resources: []SkillResource{{Type: SkillResourceTypeSelector, ID: "*"}}}.Validate()
	}
	validateProfile := func(subjects []Subject) error {
		m := validVMCPManifest()
		m.Profiles = []VMCPProfile{{Name: "profile", Subjects: subjects}}
		return m.Validate()
	}

	for _, tt := range []struct {
		name     string
		subjects []Subject
		// wantPolicy and wantProfile are the error a model access policy or skill access rule, and a vMCP profile,
		// report: empty for none.
		wantPolicy  string
		wantProfile string
	}{
		{
			name:     "every user",
			subjects: []Subject{everyone},
		},
		{
			name:     "an agent and a user that share an ID",
			subjects: []Subject{agent, user},
		},
		{
			name:        "the agent wildcard",
			subjects:    []Subject{agents},
			wantPolicy:  "agents must be named by ID",
			wantProfile: "agents must be named by ID",
		},
		{
			name:        "every user and the agent wildcard",
			subjects:    []Subject{everyone, agents},
			wantPolicy:  "must be the only subject",
			wantProfile: "agents must be named by ID",
		},
		{
			// {user, "*"} names no one, but has always been valid.
			name:     "user wildcard",
			subjects: []Subject{users},
		},
		{
			name:       "user wildcard and a group",
			subjects:   []Subject{users, group},
			wantPolicy: "must be the only subject",
		},
		{
			// The wildcard is every user and no agent, so a policy for every user names its agents next to it.
			// Profiles may combine the selector wildcard with other subjects too.
			name:     "every user and an agent",
			subjects: []Subject{everyone, agent},
		},
		{
			// A user or group next to the wildcard is already included in it.
			name:       "every user and a user",
			subjects:   []Subject{everyone, user},
			wantPolicy: "must be the only subject, apart from agents",
		},
		{
			name:       "every user, an agent and a group",
			subjects:   []Subject{everyone, agent, group},
			wantPolicy: "must be the only subject, apart from agents",
		},
		{
			name:        "the same agent twice",
			subjects:    []Subject{agent, agent},
			wantPolicy:  "duplicate subject",
			wantProfile: "",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for name, validate := range map[string]func([]Subject) error{
				"model access policy": validateModel,
				"skill access rule":   validateSkill,
			} {
				checkError(t, name, validate(tt.subjects), tt.wantPolicy)
			}
			checkError(t, "vMCP profile", validateProfile(tt.subjects), tt.wantProfile)
		})
	}
}

// TestUserOnlyPolicySystemsRefuseAgentSubjects checks that access control rules, which agents never match, refuse
// agent subjects, and still accept what they accepted.
func TestUserOnlyPolicySystemsRefuseAgentSubjects(t *testing.T) {
	validateRule := func(subjects []Subject) error {
		return AccessControlRuleManifest{Subjects: subjects}.Validate()
	}
	for _, tt := range []struct {
		name    string
		subject Subject
		want    string
	}{
		{
			name:    "selector wildcard",
			subject: Subject{Type: SubjectTypeSelector, ID: "*"},
		},
		{
			name:    "user",
			subject: Subject{Type: SubjectTypeUser, ID: "5"},
		},
		{
			name:    "group",
			subject: Subject{Type: SubjectTypeGroup, ID: "eng"},
		},
		{
			name:    "user wildcard",
			subject: Subject{Type: SubjectTypeUser, ID: "*"},
		},
		{
			name:    "agent",
			subject: Subject{Type: SubjectTypeAgent, ID: "5"},
			want:    "agent subjects are not supported",
		},
		{
			name:    "agent wildcard",
			subject: Subject{Type: SubjectTypeAgent, ID: "*"},
			want:    "agents must be named by ID",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkError(t, "access control rule", validateRule([]Subject{tt.subject}), tt.want)
		})
	}
}

func checkError(t *testing.T, what string, err error, want string) {
	t.Helper()
	switch {
	case want == "" && err != nil:
		t.Errorf("%s: unexpected error %v", what, err)
	case want != "" && err == nil:
		t.Errorf("%s: no error, want one containing %q", what, want)
	case want != "" && !strings.Contains(err.Error(), want):
		t.Errorf("%s: error %q, want one containing %q", what, err, want)
	}
}

func TestValidateMCPGuardrailFilterSubjects(t *testing.T) {
	subject := func(typ, id string) MCPGuardrailFilterSubject {
		return MCPGuardrailFilterSubject{Type: typ, ID: id}
	}
	for _, tt := range []struct {
		name     string
		subjects []MCPGuardrailFilterSubject
		want     string
	}{
		{
			name: "no subjects is everyone",
		},
		{
			name:     "everyone",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectEveryone, "")},
		},
		{
			name:     "a user and a group, as before",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectUser, "5"), subject(MCPGuardrailSubjectGroup, "okta/eng")},
		},
		{
			// {user, "*"} names no one, but has always been valid.
			name:     "user wildcard",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectUser, "*")},
		},
		{
			name:     "an agent and the user with its ID",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectAgent, "5"), subject(MCPGuardrailSubjectUser, "5")},
		},
		{
			name:     "everyone and an agent",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectEveryone, ""), subject(MCPGuardrailSubjectAgent, "5")},
		},
		{
			name:     "agent wildcard",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectAgent, "*")},
			want:     "agents must be named by ID",
		},
		{
			name:     "everyone and the agent wildcard",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectEveryone, ""), subject(MCPGuardrailSubjectAgent, "*")},
			want:     "agents must be named by ID",
		},
		{
			name:     "agent storage encoding",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectAgent, "agent:5")},
			want:     "agent subject ID must be",
		},
		{
			name:     "agent ID with a leading zero",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectAgent, "05")},
			want:     "agent subject ID must be",
		},
		{
			name:     "agent ID zero",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectAgent, "0")},
			want:     "agent subject ID must be",
		},
		{
			name:     "agent without an ID",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectAgent, "")},
			want:     "need an id",
		},
		{
			name:     "user without an ID",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectUser, "")},
			want:     "need an id",
		},
		{
			name:     "group without an ID",
			subjects: []MCPGuardrailFilterSubject{subject(MCPGuardrailSubjectGroup, "")},
			want:     "need an id",
		},
		{
			// Only stored filters may still have the legacy selector; new ones use everyone.
			name:     "legacy selector",
			subjects: []MCPGuardrailFilterSubject{subject("selector", "*")},
			want:     "must be user, agent, group or everyone",
		},
		{
			name:     "unknown type",
			subjects: []MCPGuardrailFilterSubject{subject("service", "x")},
			want:     "must be user, agent, group or everyone",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkError(t, "guardrail filter", ValidateMCPGuardrailFilterSubjects(tt.subjects), tt.want)
		})
	}
}

package types

import (
	"strings"
	"testing"
)

var (
	everyone   = Subject{Type: SubjectTypeSelector, ID: SubjectWildcardID}
	everyUser  = Subject{Type: SubjectTypeUser, ID: SubjectWildcardID}
	everyAgent = Subject{Type: SubjectTypeAgent, ID: SubjectWildcardID}
	agentSeven = Subject{Type: SubjectTypeAgent, ID: "7"}
	someGroup  = Subject{Type: SubjectTypeGroup, ID: "engineering"}
)

func profileWith(name string, subjects ...Subject) VMCPProfile {
	return VMCPProfile{Name: name, Subjects: subjects, Permissions: VMCPProfilePermissions{AllowAllComponents: true}}
}

func TestVMCPManifestValidateWildcardProfiles(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profiles []VMCPProfile
		wantErr  string
	}{
		{
			name: "no wildcards",
			profiles: []VMCPProfile{
				profileWith("a", someGroup),
				profileWith("b", agentSeven),
			},
		},
		{
			name: "one selector wildcard",
			profiles: []VMCPProfile{
				profileWith("a", everyone),
				profileWith("b", someGroup),
			},
		},
		{
			name: "one user and one agent wildcard",
			profiles: []VMCPProfile{
				profileWith("users", everyUser),
				profileWith("agents", everyAgent),
			},
		},
		{
			name: "two selector wildcards",
			profiles: []VMCPProfile{
				profileWith("a", everyone),
				profileWith("b", everyone, someGroup),
			},
			wantErr: `profiles "a" and "b" both use the selector wildcard`,
		},
		{
			name: "two user wildcards",
			profiles: []VMCPProfile{
				profileWith("a", everyUser),
				profileWith("b", everyUser),
			},
			wantErr: `profiles "a" and "b" both use the user wildcard`,
		},
		{
			name: "two agent wildcards",
			profiles: []VMCPProfile{
				profileWith("a", everyAgent),
				profileWith("b", everyAgent, someGroup),
			},
			wantErr: `profiles "a" and "b" both use the agent wildcard`,
		},
		{
			name: "selector and user wildcards",
			profiles: []VMCPProfile{
				profileWith("everyone", everyone),
				profileWith("users", everyUser),
			},
			wantErr: `profile "everyone" uses the selector wildcard (*) and profile "users" the user wildcard`,
		},
		{
			name: "selector and agent wildcards",
			profiles: []VMCPProfile{
				profileWith("agents", everyAgent),
				profileWith("everyone", everyone),
			},
			wantErr: `profile "everyone" uses the selector wildcard (*) and profile "agents" the agent wildcard`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := VMCPManifest{Profiles: tc.profiles}.ValidateWildcardProfiles()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateWildcardProfiles() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateWildcardProfiles() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestVMCPManifestValidateProfileIDsAndAgents(t *testing.T) {
	for _, tc := range []struct {
		name     string
		profiles []VMCPProfile
		wantErr  string
	}{
		{
			name: "profiles without IDs",
			profiles: []VMCPProfile{
				profileWith("a", someGroup),
			},
		},
		{
			name: "assigned and catalog IDs",
			profiles: []VMCPProfile{
				{ID: "vp10123456789abcdef0123456789abcdef", Name: "a", Subjects: []Subject{someGroup}},
				{ID: "engineering-agents", Name: "b", Subjects: []Subject{agentSeven}},
			},
		},
		{
			name: "uppercase ID",
			profiles: []VMCPProfile{
				{ID: "Profile", Name: "a", Subjects: []Subject{someGroup}},
			},
			wantErr: `invalid profile ID "Profile"`,
		},
		{
			name: "ID with a slash",
			profiles: []VMCPProfile{
				{ID: "a/b", Name: "a", Subjects: []Subject{someGroup}},
			},
			wantErr: `invalid profile ID "a/b"`,
		},
		{
			name: "ID longer than a DNS label",
			profiles: []VMCPProfile{
				{ID: strings.Repeat("a", 64), Name: "a", Subjects: []Subject{someGroup}},
			},
			wantErr: "invalid profile ID",
		},
		{
			name: "duplicate IDs",
			profiles: []VMCPProfile{
				{ID: "same", Name: "a", Subjects: []Subject{someGroup}},
				{ID: "same", Name: "b", Subjects: []Subject{someGroup}},
			},
			wantErr: `duplicate profile ID "same"`,
		},
		{
			name: "one agent named by two profiles",
			profiles: []VMCPProfile{
				profileWith("a", agentSeven),
				profileWith("b", someGroup, agentSeven),
			},
			wantErr: `agent "7" is named by profiles "a" and "b"`,
		},
		{
			name: "agent wildcard in several profiles is the wildcard rule, not this one",
			profiles: []VMCPProfile{
				profileWith("a", everyAgent),
				profileWith("b", everyAgent),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := VMCPManifest{DisplayName: "vMCP", Profiles: tc.profiles}.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("Validate() = %v, want error containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestVMCPManifestAgentWildcardProfiles(t *testing.T) {
	manifest := VMCPManifest{Profiles: []VMCPProfile{
		profileWith("everyone", everyone),
		profileWith("users", everyUser),
		profileWith("agents", everyAgent),
		profileWith("seven", agentSeven),
	}}
	var names []string
	for _, profile := range manifest.AgentWildcardProfiles() {
		names = append(names, profile.Name)
	}
	if strings.Join(names, ",") != "everyone,agents" {
		t.Fatalf("AgentWildcardProfiles() = %v, want [everyone agents]", names)
	}
}

func TestVMCPProfileIsAgentCapable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile VMCPProfile
		want    bool
	}{
		{
			name:    "selector wildcard",
			profile: profileWith("a", everyone),
			want:    true,
		},
		{
			name:    "agent wildcard",
			profile: profileWith("a", everyAgent),
			want:    true,
		},
		{
			name:    "explicit agent",
			profile: profileWith("a", someGroup, agentSeven),
			want:    true,
		},
		{
			name:    "user wildcard",
			profile: profileWith("a", everyUser),
		},
		{
			name:    "group",
			profile: profileWith("a", someGroup),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.profile.IsAgentCapable(); got != tc.want {
				t.Fatalf("IsAgentCapable() = %t, want %t", got, tc.want)
			}
		})
	}
}

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

func TestVMCPProfileIsAgentCapable(t *testing.T) {
	for _, tc := range []struct {
		name    string
		profile VMCPProfile
		want    bool
	}{
		{
			name:    "explicit agent",
			profile: profileWith("a", someGroup, agentSeven),
			want:    true,
		},
		{
			name:    "explicit agent and every user",
			profile: profileWith("a", everyone, agentSeven),
			want:    true,
		},
		{
			name:    "every user",
			profile: profileWith("a", everyone),
		},
		{
			name:    "stored agent wildcard",
			profile: profileWith("a", everyAgent),
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

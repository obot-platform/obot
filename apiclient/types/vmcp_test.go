package types

import (
	"testing"
)

func TestVMCPToolSetValidation(t *testing.T) {
	manifest := VMCPManifest{Components: []VMCPComponent{{
		ID: "everything",
		ToolOverrides: []ToolOverride{
			{Name: "echo", OverrideName: "renamed", Enabled: true},
			{Name: "disabled"},
		},
	}}}
	for _, ref := range []VMCPToolReference{
		{Name: "echo"},
		{ComponentID: "other", Name: "echo"},
		{ComponentID: "everything", Name: "renamed"},
		{ComponentID: "everything", Name: "disabled"},
	} {
		if err := manifest.ValidateToolReference(ref); err == nil {
			t.Fatalf("accepted invalid reference %#v", ref)
		}
	}
	if err := manifest.ValidateToolReference(VMCPToolReference{ComponentID: "everything", Name: "echo"}); err != nil {
		t.Fatal(err)
	}
	if err := manifest.ValidateToolSet(VMCPToolSet{"other": {}}); err == nil {
		t.Fatal("ValidateToolSet() accepted an empty unknown component")
	}
	if err := manifest.ValidateToolSet(VMCPToolSet{"everything": {"*"}}); err != nil {
		t.Fatalf("component wildcard rejected: %v", err)
	}
	if err := manifest.ValidateToolSet(VMCPToolSet{"other": {"*"}}); err == nil {
		t.Fatal("wildcard accepted for an unknown component")
	}
}

func TestVMCPManifestDefault(t *testing.T) {
	manifest := VMCPManifest{
		Components: []VMCPComponent{{
			Configuration: []VMCPConfigurationPolicy{{Key: "TOKEN"}},
		}},
	}

	manifest.Default()

	if got := manifest.Components[0].Configuration[0].Policy; got != VMCPConfigurationPolicyProhibited {
		t.Fatalf("default configuration policy = %q, want %q", got, VMCPConfigurationPolicyProhibited)
	}
	if len(manifest.Profiles) != 1 {
		t.Fatalf("default profile count = %d, want 1", len(manifest.Profiles))
	}
	profile := manifest.Profiles[0]
	if !profile.AllowAllTools {
		t.Fatal("default profile must allow all tools")
	}
	if len(profile.Subjects) != 1 || profile.Subjects[0].Type != SubjectTypeSelector || profile.Subjects[0].ID != "*" {
		t.Fatalf("unexpected default profile subjects: %#v", profile.Subjects)
	}
}

func TestVMCPManifestDefaultPreservesExplicitEmptyProfiles(t *testing.T) {
	manifest := VMCPManifest{Profiles: []VMCPProfile{}}

	manifest.Default()

	if manifest.Profiles == nil || len(manifest.Profiles) != 0 {
		t.Fatalf("explicit empty profiles changed to %#v", manifest.Profiles)
	}
}

func TestVMCPProfileWithoutAllowAllToolsMayGrantNoTools(t *testing.T) {
	manifest := validVMCPManifest()
	manifest.Profiles = []VMCPProfile{{
		Name:     "access-without-tools",
		Subjects: []Subject{{Type: SubjectTypeUser, ID: "user-1"}},
	}}

	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if manifest.Profiles[0].AllowAllTools {
		t.Fatal("AllowAllTools must remain false")
	}
	if len(manifest.Profiles[0].AllowedTools) != 0 {
		t.Fatalf("AllowedTools = %#v, want empty", manifest.Profiles[0].AllowedTools)
	}
}

func TestVMCPManifestRejectsInvalidConfigurationPolicy(t *testing.T) {
	manifest := validVMCPManifest()
	manifest.Components[0].Configuration = []VMCPConfigurationPolicy{{
		Key:    "TOKEN",
		Policy: "anything",
	}}

	if err := manifest.Validate(); err == nil {
		t.Fatal("Validate() unexpectedly accepted an invalid configuration policy")
	}
}

func validVMCPManifest() VMCPManifest {
	return VMCPManifest{
		DisplayName: "Example",
		Components: []VMCPComponent{{
			Name:                    "component",
			MCPCatalogID:            "catalog",
			MCPServerCatalogEntryID: "entry",
		}},
	}
}

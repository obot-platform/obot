package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestVMCPSnapshotOmitsToolPreviews(t *testing.T) {
	snapshot := MCPServerCatalogEntrySnapshot{
		Manifest: MCPServerCatalogEntryManifest{
			Name:        "server",
			ToolPreview: []MCPServerTool{{Name: "echo"}},
		},
		UnsupportedTools: []string{"disabled"},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var decoded MCPServerCatalogEntrySnapshot
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Manifest.ToolPreview) != 1 {
		t.Fatal("serialization modified the source manifest")
	}
	if decoded.Manifest.ToolPreview != nil || decoded.Manifest.Name != "server" || !reflect.DeepEqual(decoded.UnsupportedTools, snapshot.UnsupportedTools) {
		t.Fatalf("unexpected snapshot: %#v", decoded)
	}
	snapshot.Manifest.ToolPreview = nil
	withoutPreview, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(withoutPreview) {
		t.Fatal("tool previews changed the serialized snapshot")
	}
}

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

	manifest.Default(false, "user-1")

	if got := manifest.Components[0].Configuration[0].Policy; got != VMCPConfigurationPolicyProhibited {
		t.Fatalf("default configuration policy = %q, want %q", got, VMCPConfigurationPolicyProhibited)
	}
	if len(manifest.Profiles) != 1 {
		t.Fatalf("default profile count = %d, want 1", len(manifest.Profiles))
	}
	profile := manifest.Profiles[0]
	if profile.Name != "me" {
		t.Fatalf("default profile name = %q, want me", profile.Name)
	}
	if !profile.AllowAllTools {
		t.Fatal("default profile must allow all tools")
	}
	if len(profile.Subjects) != 1 || profile.Subjects[0].Type != SubjectTypeUser || profile.Subjects[0].ID != "user-1" {
		t.Fatalf("unexpected default profile subjects: %#v", profile.Subjects)
	}
}

func TestVMCPManifestDefaultPreservesExplicitEmptyProfiles(t *testing.T) {
	manifest := VMCPManifest{Profiles: []VMCPProfile{}}

	manifest.Default(false, "user-1")

	if manifest.Profiles == nil || len(manifest.Profiles) != 0 {
		t.Fatalf("explicit empty profiles changed to %#v", manifest.Profiles)
	}
}

func TestVMCPManifestDefaultPersonalServer(t *testing.T) {
	for _, profiles := range [][]VMCPProfile{nil, {}, {{Name: "existing", AllowAllTools: true}}} {
		manifest := VMCPManifest{
			Profiles: profiles,
			Components: []VMCPComponent{{
				Configuration: []VMCPConfigurationPolicy{{Key: "TOKEN"}},
			}},
		}

		manifest.Default(true, "user-1")

		if manifest.Profiles != nil {
			t.Fatalf("personal server profiles = %#v, want nil", manifest.Profiles)
		}
		if got := manifest.Components[0].Configuration[0].Policy; got != VMCPConfigurationPolicyProhibited {
			t.Fatalf("default configuration policy = %q, want %q", got, VMCPConfigurationPolicyProhibited)
		}
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

func TestVMCPManifestRequiredConfigurationPolicy(t *testing.T) {
	for _, tc := range []struct {
		name          string
		policy        VMCPConfigurationPolicyType
		omitPolicy    bool
		optional      bool
		value         string
		secretBinding *MCPSecretBinding
		wantError     bool
	}{
		{
			name:      "prohibited",
			policy:    VMCPConfigurationPolicyProhibited,
			wantError: true,
		},
		{
			name:      "empty policy",
			wantError: true,
		},
		{
			name:       "omitted policy",
			omitPolicy: true,
			wantError:  true,
		},
		{
			name:   "fixed",
			policy: VMCPConfigurationPolicyFixed,
		},
		{
			name:   "user allowed",
			policy: VMCPConfigurationPolicyUserAllowed,
		},
		{
			name:     "optional prohibited",
			policy:   VMCPConfigurationPolicyProhibited,
			optional: true,
		},
		{
			name:   "catalog value",
			policy: VMCPConfigurationPolicyProhibited,
			value:  "supplied",
		},
		{
			name:          "secret binding",
			policy:        VMCPConfigurationPolicyProhibited,
			secretBinding: &MCPSecretBinding{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manifest := validVMCPManifest()
			manifest.Components[0].CatalogEntry.Manifest.Config = []MCPConfig{{
				Key:           "TOKEN",
				Required:      !tc.optional,
				Value:         tc.value,
				SecretBinding: tc.secretBinding,
			}}
			if !tc.omitPolicy {
				manifest.Components[0].Configuration = []VMCPConfigurationPolicy{{
					Key:    "TOKEN",
					Policy: tc.policy,
				}}
			}
			if err := manifest.Validate(); (err != nil) != tc.wantError {
				t.Fatalf("Validate() = %v, want error %v", err, tc.wantError)
			}
		})
	}
}

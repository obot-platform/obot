package client

import (
	"testing"

	apitypes "github.com/obot-platform/obot/apiclient/types"
	"github.com/obot-platform/obot/pkg/gateway/types"
)

func renderedArtifact(platform, osName, content string) types.MDMConfigurationArtifact {
	return types.MDMConfigurationArtifact{
		Slug:         platform + "-" + osName,
		Platform:     platform,
		OS:           osName,
		Instructions: "Install " + platform + "/" + osName,
		Content:      []byte(content),
	}
}

func TestCreateMDMConfigurationAssignsDefaultStatus(t *testing.T) {
	client := newTestClient(t)
	first, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	if !first.IsDefault || first.CreatedBy != 42 {
		t.Fatalf("first configuration = %#v", first)
	}
	second, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	if second.IsDefault {
		t.Fatalf("second configuration unexpectedly became default: %#v", second)
	}
}

func TestCreateMDMConfigurationStoresArtifactRows(t *testing.T) {
	client := newTestClient(t)
	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{
		AssetDigest:       "source-digest",
		ObotSentryVersion: "1.2.3",
		Values:            `{"interval":60}`,
		Artifacts: []types.MDMConfigurationArtifact{
			renderedArtifact("intune", "windows", "windows-zip"),
			renderedArtifact("jamf", "macos", "macos-zip"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(configuration.Artifacts) != 2 {
		t.Fatalf("artifact count = %d, want 2", len(configuration.Artifacts))
	}
	for _, artifact := range configuration.Artifacts {
		if artifact.ID == 0 || artifact.MDMConfigurationID != configuration.ID || artifact.Digest == "" || len(artifact.Content) == 0 {
			t.Fatalf("incomplete stored artifact: %#v", artifact)
		}
	}
	loaded, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Artifacts) != 2 || string(loaded.Artifacts[0].Content) != "windows-zip" {
		t.Fatalf("loaded configuration = %#v", loaded)
	}
	if loaded.ObotSentryVersion != "1.2.3" {
		t.Fatalf("stored version = %q, want 1.2.3", loaded.ObotSentryVersion)
	}
}

func TestUpdateMDMConfigurationAtomicallyReplacesArtifacts(t *testing.T) {
	client := newTestClient(t)
	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	configuration.AssetDigest = "source-digest"
	configuration.ObotSentryVersion = "1.0.0"
	configuration.Values = `{"interval":60}`
	configuration.Artifacts = []types.MDMConfigurationArtifact{
		renderedArtifact("intune", "windows", "windows-zip"),
		renderedArtifact("jamf", "macos", "macos-zip"),
	}
	if err := client.UpdateMDMConfiguration(t.Context(), configuration); err != nil {
		t.Fatal(err)
	}

	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.ObotSentryVersion = "2.0.0"
	stored.Values = `{"interval":120}`
	stored.Artifacts = []types.MDMConfigurationArtifact{renderedArtifact("intune", "windows", "replacement")}
	if err := client.UpdateMDMConfiguration(t.Context(), stored); err != nil {
		t.Fatal(err)
	}
	replaced, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(replaced.Artifacts) != 1 || string(replaced.Artifacts[0].Content) != "replacement" {
		t.Fatalf("replacement configuration = %#v", replaced)
	}
	if replaced.ObotSentryVersion != "2.0.0" {
		t.Fatalf("replacement version = %q, want 2.0.0", replaced.ObotSentryVersion)
	}
}

func TestInvalidateMDMConfigurationArtifactsPreservesValues(t *testing.T) {
	client := newTestClient(t)
	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{
		AssetDigest: "old-source",
		Values:      `{"interval":60}`,
		Artifacts:   []types.MDMConfigurationArtifact{renderedArtifact("intune", "windows", "zip")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.InvalidateMDMConfigurationArtifacts(t.Context(), "new-source"); err != nil {
		t.Fatal(err)
	}
	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AssetDigest != "old-source" || stored.Values != `{"interval":60}` || len(stored.Artifacts) != 0 {
		t.Fatalf("invalidated configuration = %#v", stored)
	}
}

func TestCreateDeviceEnrollmentKeyIsSeparate(t *testing.T) {
	client := newTestClient(t)
	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{})
	if err != nil {
		t.Fatal(err)
	}
	key, err := client.CreateDeviceEnrollmentKey(t.Context(), configuration.ID, 42, "bootstrap", nil)
	if err != nil {
		t.Fatal(err)
	}
	if key.EnrollmentCredential == "" || key.MDMConfigurationID != configuration.ID {
		t.Fatalf("created enrollment key = %#v", key)
	}
	validated, err := client.ValidateDeviceEnrollmentCredential(t.Context(), key.EnrollmentCredential)
	if err != nil {
		t.Fatal(err)
	}
	if validated.ID != key.ID {
		t.Fatalf("validated key = %#v, want %d", validated, key.ID)
	}
}

func TestUpdateMDMConfigurationRollsBackOnBadArtifacts(t *testing.T) {
	original := func(t *testing.T) (*Client, *types.MDMConfiguration) {
		t.Helper()
		client := newTestClient(t)
		configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{
			AssetDigest: "source-digest",
			Values:      `{"interval":60}`,
			Artifacts: []types.MDMConfigurationArtifact{
				renderedArtifact("intune", "windows", "windows-zip"),
				renderedArtifact("jamf", "macos", "macos-zip"),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return client, configuration
	}

	// Each bad update also changes Values, so a successful rollback must leave the
	// original values and both original artifact rows untouched.
	cases := map[string][]types.MDMConfigurationArtifact{
		"duplicate slug": {
			renderedArtifact("intune", "windows", "one"),
			renderedArtifact("intune", "windows", "two"),
		},
		"blank slug": {
			{Slug: "", Platform: "intune", OS: "windows", Content: []byte("one")},
		},
		"empty content": {
			{Slug: "intune-windows", Platform: "intune", OS: "windows"},
		},
	}
	for name, artifacts := range cases {
		t.Run(name, func(t *testing.T) {
			client, configuration := original(t)
			bad := *configuration
			bad.Values = `{"interval":999}`
			bad.Artifacts = artifacts
			if err := client.UpdateMDMConfiguration(t.Context(), &bad); err == nil {
				t.Fatal("bad update was accepted")
			}
			stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Values != `{"interval":60}` {
				t.Fatalf("values changed after failed update: %q", stored.Values)
			}
			if len(stored.Artifacts) != 2 {
				t.Fatalf("artifact rows changed after failed update: %#v", stored.Artifacts)
			}
			if string(stored.Artifacts[0].Content) != "windows-zip" || string(stored.Artifacts[1].Content) != "macos-zip" {
				t.Fatalf("artifact content changed after failed update: %#v", stored.Artifacts)
			}
		})
	}
}

func TestMDMConfigurationEnforcementColumnsPersistOnCreate(t *testing.T) {
	client := newTestClient(t)

	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{
		AssetDigest:        "source-digest",
		Values:             `{"interval":60}`,
		EnforcementEnabled: true,
		EnforcementAllowlist: apitypes.EnforcementAllowlist{
			AllowAllObotHostedMCP: true,
			Servers: []apitypes.AllowlistServer{
				{Hostname: "gitmcp.io"},
			},
		},
		Artifacts: []types.MDMConfigurationArtifact{
			renderedArtifact("intune", "windows", "windows-zip"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !stored.EnforcementEnabled ||
		!stored.EnforcementAllowlist.AllowAllObotHostedMCP ||
		len(stored.EnforcementAllowlist.Servers) != 1 ||
		stored.EnforcementAllowlist.Servers[0].Hostname != "gitmcp.io" {
		t.Fatalf("created configuration did not persist enforcement columns: %#v", stored)
	}
}

// TestMDMConfigurationUpdatePreservesEnforcement verifies the general update
// path leaves the enforcement policy untouched, while
// UpdateMDMConfigurationEnforcement is the only path that changes it.
func TestMDMConfigurationUpdatePreservesEnforcement(t *testing.T) {
	client := newTestClient(t)

	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{
		AssetDigest:        "source-digest",
		Values:             `{"interval":60}`,
		EnforcementEnabled: true,
		EnforcementAllowlist: apitypes.EnforcementAllowlist{
			AllowAllObotHostedMCP: true,
			Servers:               []apitypes.AllowlistServer{{Hostname: "gitmcp.io"}},
		},
		Artifacts: []types.MDMConfigurationArtifact{
			renderedArtifact("intune", "windows", "windows-zip"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// A general update that even asks to clear enforcement must not change it.
	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	stored.Values = `{"interval":120}`
	stored.EnforcementEnabled = false
	stored.EnforcementAllowlist = apitypes.EnforcementAllowlist{}
	if err := client.UpdateMDMConfiguration(t.Context(), stored); err != nil {
		t.Fatal(err)
	}

	afterUpdate, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterUpdate.Values != `{"interval":120}` {
		t.Fatalf("general update did not persist values: %q", afterUpdate.Values)
	}
	if !afterUpdate.EnforcementEnabled ||
		!afterUpdate.EnforcementAllowlist.AllowAllObotHostedMCP ||
		len(afterUpdate.EnforcementAllowlist.Servers) != 1 {
		t.Fatalf("general update unexpectedly modified enforcement: %#v", afterUpdate)
	}

	// The dedicated enforcement update is the path that changes the policy.
	if err := client.UpdateMDMConfigurationEnforcement(t.Context(), configuration.ID, false, apitypes.EnforcementAllowlist{AllowEverything: true}); err != nil {
		t.Fatal(err)
	}
	afterEnforcement, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterEnforcement.EnforcementEnabled ||
		!afterEnforcement.EnforcementAllowlist.AllowEverything ||
		afterEnforcement.EnforcementAllowlist.AllowAllObotHostedMCP ||
		len(afterEnforcement.EnforcementAllowlist.Servers) != 0 {
		t.Fatalf("enforcement update did not persist enforcement columns: %#v", afterEnforcement)
	}
	// The enforcement update must not disturb the asset-side columns.
	if afterEnforcement.Values != `{"interval":120}` {
		t.Fatalf("enforcement update unexpectedly changed values: %q", afterEnforcement.Values)
	}
}

func TestDeleteMDMConfigurationRemovesArtifactsAndKeysKeepsDevices(t *testing.T) {
	client := newTestClient(t)
	configuration, err := client.CreateMDMConfiguration(t.Context(), 42, &types.MDMConfiguration{
		AssetDigest: "source-digest",
		Values:      `{"interval":60}`,
		Artifacts:   []types.MDMConfigurationArtifact{renderedArtifact("intune", "windows", "zip")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateDeviceEnrollmentKey(t.Context(), configuration.ID, 42, "bootstrap", nil); err != nil {
		t.Fatal(err)
	}
	device, err := client.EnrollDevice(t.Context(), DeviceEnrollment{
		DeviceID:           "device-1",
		MDMConfigurationID: configuration.ID,
		PublicKey:          []byte("public-key"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := client.DeleteMDMConfiguration(t.Context(), configuration.ID); err != nil {
		t.Fatal(err)
	}

	if _, err := client.GetMDMConfiguration(t.Context(), configuration.ID); err == nil {
		t.Fatal("configuration still readable after delete")
	}
	var remainingArtifacts int64
	if err := client.db.WithContext(t.Context()).Model(&types.MDMConfigurationArtifact{}).
		Where("mdm_configuration_id = ?", configuration.ID).Count(&remainingArtifacts).Error; err != nil {
		t.Fatal(err)
	}
	if remainingArtifacts != 0 {
		t.Fatalf("artifact rows survived configuration delete: %d", remainingArtifacts)
	}
	keys, err := client.ListDeviceEnrollmentKeys(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 0 {
		t.Fatalf("enrollment keys survived configuration delete: %d", len(keys))
	}
	preserved, err := client.GetDeviceByDeviceID(t.Context(), device.DeviceID)
	if err != nil {
		t.Fatalf("enrolled device was not preserved: %v", err)
	}
	if preserved.MDMConfigurationID != configuration.ID {
		t.Fatalf("preserved device = %#v", preserved)
	}
}

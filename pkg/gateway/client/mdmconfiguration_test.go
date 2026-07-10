package client

import (
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
)

func createPinnedConfiguration(t *testing.T, client *Client, name string, asset *types.MDMAssetBundle) *types.MDMConfiguration {
	t.Helper()
	configuration, _, err := client.CreateMDMConfiguration(t.Context(), 42, types.MDMConfiguration{
		Name:        name,
		Platform:    "intune",
		OS:          "windows",
		AssetDigest: asset.Digest,
		Values:      `{"interval":30}`,
	})
	if err != nil {
		t.Fatalf("failed to create pinned configuration: %v", err)
	}
	return configuration
}

func TestCreateMDMConfigurationPersistsTargetAndFirstKeyAtomically(t *testing.T) {
	client := newTestClient(t)
	asset := storeTestBundle(t, client, "windows-assets")
	configuration, key, err := client.CreateMDMConfiguration(t.Context(), 42, types.MDMConfiguration{
		Name:        "Windows fleet",
		Description: "managed",
		Platform:    "intune",
		OS:          "windows",
		AssetDigest: asset.Digest,
		Values:      `{"interval":60}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ID == 0 || configuration.CreatedBy != 42 || configuration.AssetDigest != asset.Digest {
		t.Fatalf("unexpected configuration: %#v", configuration)
	}
	if key.MDMConfigurationID != configuration.ID || key.EnrollmentCredential == "" {
		t.Fatalf("unexpected first enrollment key: %#v", key)
	}
	validated, err := client.ValidateDeviceEnrollmentCredential(t.Context(), key.EnrollmentCredential)
	if err != nil {
		t.Fatal(err)
	}
	if validated.MDMConfigurationID != configuration.ID {
		t.Fatalf("credential configuration = %d, want %d", validated.MDMConfigurationID, configuration.ID)
	}
}

func TestCreateMDMConfigurationTargetInvariant(t *testing.T) {
	client := newTestClient(t)
	if _, _, err := client.CreateMDMConfiguration(t.Context(), 1, types.MDMConfiguration{
		Name:     "half pair",
		Platform: "intune",
	}); err == nil {
		t.Fatal("half target pair was accepted")
	}
	if _, _, err := client.CreateMDMConfiguration(t.Context(), 1, types.MDMConfiguration{
		Name:     "missing pin",
		Platform: "intune",
		OS:       "windows",
	}); err == nil {
		t.Fatal("configured target without asset pin was accepted")
	}

	if _, _, err := client.CreateMDMConfiguration(t.Context(), 1, types.MDMConfiguration{
		Name:        "digest only",
		AssetDigest: "digest",
	}); err == nil {
		t.Fatal("asset digest without target was accepted")
	}

	configuration, _, err := client.CreateMDMConfiguration(t.Context(), 1, types.MDMConfiguration{
		Name:   "blank",
		Values: `{"ignored":true}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if configuration.AssetDigest != "" || configuration.Values != "" {
		t.Fatalf("blank target retained deployment data: %#v", configuration)
	}
}

func TestUpdateMDMConfigurationUpdatesFieldsAndPinsExplicitAssetDigest(t *testing.T) {
	client := newTestClient(t)
	oldAsset := storeTestBundle(t, client, "old")
	configuration := createPinnedConfiguration(t, client, "fleet", oldAsset)
	newAsset := storeTestBundle(t, client, "new")

	configuration.Name = "renamed fleet"
	configuration.Description = "updated description"
	configuration.Platform = "jamf"
	configuration.OS = "macos"
	configuration.AssetDigest = newAsset.Digest
	configuration.Values = `{"interval":60}`
	if err := client.UpdateMDMConfiguration(t.Context(), *configuration); err != nil {
		t.Fatal(err)
	}
	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "renamed fleet" || stored.Description != "updated description" ||
		stored.Platform != "jamf" || stored.OS != "macos" ||
		stored.AssetDigest != newAsset.Digest || stored.Values != `{"interval":60}` {
		t.Fatalf("updated configuration = %#v, want updated fields and explicitly selected target", stored)
	}
	requireBundle(t, client, oldAsset.Digest)
	requireBundle(t, client, newAsset.Digest)
}

func TestUpdateMDMConfigurationDoesNotDependOnLatestAsset(t *testing.T) {
	client := newTestClient(t)
	first := storeTestBundle(t, client, "first")
	configuration := createPinnedConfiguration(t, client, "fleet", first)
	storeTestBundle(t, client, "later")

	configuration.Values = `{"interval":90}`
	if err := client.UpdateMDMConfiguration(t.Context(), *configuration); err != nil {
		t.Fatal(err)
	}
	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AssetDigest != first.Digest || stored.Values != `{"interval":90}` {
		t.Fatalf("saved deployment = %#v, want the caller-selected asset", stored)
	}
}

func TestUpdateMDMConfigurationClearsDeploymentAndLeavesAssetForStartupPrune(t *testing.T) {
	client := newTestClient(t)
	asset := storeTestBundle(t, client, "assets")
	configuration := createPinnedConfiguration(t, client, "fleet", asset)

	configuration.Platform = ""
	configuration.OS = ""
	configuration.AssetDigest = ""
	configuration.Values = `{"ignored":true}`
	if err := client.UpdateMDMConfiguration(t.Context(), *configuration); err != nil {
		t.Fatal(err)
	}
	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Platform != "" || stored.OS != "" || stored.AssetDigest != "" || stored.Values != "" {
		t.Fatalf("cleared deployment retained target data: %#v", stored)
	}
	requireBundle(t, client, asset.Digest)
}

func TestUpdateMDMConfigurationRequiresCompleteDeploymentIdentity(t *testing.T) {
	client := newTestClient(t)
	configuration, _, err := client.CreateMDMConfiguration(t.Context(), 42, types.MDMConfiguration{Name: "fleet"})
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*types.MDMConfiguration){
		"digest only": func(configuration *types.MDMConfiguration) {
			configuration.AssetDigest = "digest"
		},
		"platform only": func(configuration *types.MDMConfiguration) {
			configuration.Platform = "intune"
		},
		"os only": func(configuration *types.MDMConfiguration) {
			configuration.OS = "windows"
		},
		"target without digest": func(configuration *types.MDMConfiguration) {
			configuration.Platform = "intune"
			configuration.OS = "windows"
		},
	} {
		t.Run(name, func(t *testing.T) {
			updated := *configuration
			mutate(&updated)
			if err := client.UpdateMDMConfiguration(t.Context(), updated); err == nil {
				t.Fatal("incomplete deployment identity was accepted")
			}
		})
	}
}

func TestUpdateMDMConfigurationRequiresExistingID(t *testing.T) {
	client := newTestClient(t)
	if err := client.UpdateMDMConfiguration(t.Context(), types.MDMConfiguration{Name: "missing id"}); err == nil {
		t.Fatal("zero id was accepted")
	}
	if err := client.UpdateMDMConfiguration(t.Context(), types.MDMConfiguration{ID: 999, Name: "missing"}); err == nil {
		t.Fatal("missing configuration was accepted")
	}
}

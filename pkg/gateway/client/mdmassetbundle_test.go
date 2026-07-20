package client

import (
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func storeTestBundle(t *testing.T, client *Client, content string) *types.MDMAssetBundle {
	t.Helper()
	digest, err := client.StoreMDMAssetBundle(t.Context(), []byte(content))
	if err != nil {
		t.Fatalf("failed to store test bundle: %v", err)
	}
	return requireBundle(t, client, digest)
}

func requireBundle(t *testing.T, client *Client, digest string) *types.MDMAssetBundle {
	t.Helper()
	bundle, err := client.GetMDMAssetBundle(t.Context(), digest)
	if err != nil {
		t.Fatalf("failed to get bundle %q: %v", digest, err)
	}
	return bundle
}

func requireNoBundle(t *testing.T, client *Client, digest string) {
	t.Helper()
	if _, err := client.GetMDMAssetBundle(t.Context(), digest); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("get pruned bundle %q error = %v, want record not found", digest, err)
	}
}

func TestStoreMDMAssetBundleIsIdempotent(t *testing.T) {
	client := newTestClient(t)

	firstDigest, err := client.StoreMDMAssetBundle(t.Context(), []byte("canonical-content"))
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := client.StoreMDMAssetBundle(t.Context(), []byte("canonical-content"))
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("same content produced digests %q and %q", firstDigest, secondDigest)
	}

	stored := requireBundle(t, client, firstDigest)
	if string(stored.Content) != "canonical-content" {
		t.Fatalf("stored content = %q, want canonical-content", stored.Content)
	}
	var count int64
	if err := client.db.WithContext(t.Context()).Model(&types.MDMAssetBundle{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored bundle count = %d, want 1", count)
	}
}

func TestStoreMDMAssetBundleRejectsEmptyContent(t *testing.T) {
	client := newTestClient(t)

	if _, err := client.StoreMDMAssetBundle(t.Context(), nil); err == nil {
		t.Fatal("empty bundle content was accepted")
	}

	var count int64
	if err := client.db.WithContext(t.Context()).Model(&types.MDMAssetBundle{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rejected store persisted %d bundles", count)
	}
}

func TestPruneUnusedMDMAssetBundlesRetainsLatestAndPinned(t *testing.T) {
	client := newTestClient(t)
	unused := storeTestBundle(t, client, "unused")
	pinned := storeTestBundle(t, client, "pinned")
	latest := storeTestBundle(t, client, "latest")

	configuration, _, err := client.CreateMDMConfiguration(t.Context(), 1, types.MDMConfiguration{
		Name:        "pinned fleet",
		Platform:    "intune",
		OS:          "windows",
		AssetDigest: pinned.Digest,
		Values:      `{}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.PruneUnusedMDMAssetBundles(t.Context(), latest.Digest); err != nil {
		t.Fatal(err)
	}

	requireNoBundle(t, client, unused.Digest)
	requireBundle(t, client, pinned.Digest)
	requireBundle(t, client, latest.Digest)
	stored, err := client.GetMDMConfiguration(t.Context(), configuration.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.AssetDigest != pinned.Digest {
		t.Fatalf("configuration pin = %q, want %q", stored.AssetDigest, pinned.Digest)
	}
}

func TestPruneUnusedMDMAssetBundlesWithNoLatestRetainsOnlyPinned(t *testing.T) {
	client := newTestClient(t)
	pinned := storeTestBundle(t, client, "pinned")
	unused := storeTestBundle(t, client, "unused")
	if _, _, err := client.CreateMDMConfiguration(t.Context(), 1, types.MDMConfiguration{
		Name:        "pinned fleet",
		Platform:    "jamf",
		OS:          "macos",
		AssetDigest: pinned.Digest,
		Values:      `{}`,
	}); err != nil {
		t.Fatal(err)
	}

	if err := client.PruneUnusedMDMAssetBundles(t.Context()); err != nil {
		t.Fatal(err)
	}

	requireBundle(t, client, pinned.Digest)
	requireNoBundle(t, client, unused.Digest)
}

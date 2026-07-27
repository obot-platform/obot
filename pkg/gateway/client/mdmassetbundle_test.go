package client

import (
	"errors"
	"testing"

	"github.com/obot-platform/obot/pkg/gateway/types"
	"gorm.io/gorm"
)

func storeTestMDMAssetBundle(t *testing.T, client *Client, content string) *types.MDMAssetBundle {
	t.Helper()
	digest, err := client.StoreMDMAssetBundle(t.Context(), []byte(content))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := client.GetMDMAssetBundle(t.Context(), digest)
	if err != nil {
		t.Fatal(err)
	}
	return bundle
}

func TestStoreMDMAssetBundleIsContentAddressedAndIdempotent(t *testing.T) {
	client := newTestClient(t)
	first := storeTestMDMAssetBundle(t, client, "canonical-content")
	second := storeTestMDMAssetBundle(t, client, "canonical-content")
	if first.Digest != second.Digest || string(second.Content) != "canonical-content" {
		t.Fatalf("stored bundles = %#v, %#v", first, second)
	}
	var count int64
	if err := client.db.WithContext(t.Context()).Model(&types.MDMAssetBundle{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("stored bundle count = %d, want 1", count)
	}
}

func TestGetMDMAssetBundleRejectsCorruptedContent(t *testing.T) {
	client := newTestClient(t)
	bundle := storeTestMDMAssetBundle(t, client, "canonical-content")
	if err := client.db.WithContext(t.Context()).Model(&types.MDMAssetBundle{}).
		Where("digest = ?", bundle.Digest).
		Update("content", []byte("corrupted")).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMDMAssetBundle(t.Context(), bundle.Digest); err == nil {
		t.Fatal("corrupted bundle was returned")
	}
}

func TestPruneUnusedMDMAssetBundles(t *testing.T) {
	client := newTestClient(t)
	unused := storeTestMDMAssetBundle(t, client, "unused")
	retained := storeTestMDMAssetBundle(t, client, "retained")
	if err := client.PruneUnusedMDMAssetBundles(t.Context(), retained.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetMDMAssetBundle(t.Context(), unused.Digest); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unused bundle error = %v, want record not found", err)
	}
	if _, err := client.GetMDMAssetBundle(t.Context(), retained.Digest); err != nil {
		t.Fatal(err)
	}
}
